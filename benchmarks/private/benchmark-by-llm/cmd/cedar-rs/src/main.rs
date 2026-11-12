//! cedar-rs measures Cedar's reference implementation (the `cedar-policy`
//! crate) with exactly the policies, entities and requests the Go harness
//! uses for cedar-go. The Go test binary writes them as JSON case files
//! (`TestExportCedarSpecs`); this program times them the way `go test -bench`
//! does and prints lines in Go's benchmark format, so the same summary
//! scripts read both.
//!
//!   cedar-rs bench    --cpu 2 [--benchtime 1s] [--count 5] CASE.json...
//!   cedar-rs parallel --cpu 1,2,4 [--benchtime 1s] [--count 5] CASE.json...
//!   cedar-rs check    CASE.json...           (functional scenarios)
//!   cedar-rs loadcost --label L --n N --users U POLICIES ENTITIES
//!   cedar-rs serve    --addr A POLICIES ENTITIES   (thin HTTP API of authzd)
//!
//! Requests are built before timing, as in the Go harness. Each timed call
//! checks the decision and the diagnostics, as the Go adapters do.

use std::alloc::{GlobalAlloc, Layout, System};
use std::collections::HashMap;
use std::str::FromStr;
use std::sync::atomic::{AtomicBool, AtomicI64, AtomicU64, Ordering::Relaxed};
use std::sync::Arc;
use std::time::{Duration, Instant};

use cedar_policy::{
    Authorizer, Context, Decision, Entities, EntityId, EntityTypeName, EntityUid, PolicySet,
    Request, RestrictedExpression,
};
use serde::Deserialize;

// ------------------------------------------------------------- allocations

/// Counts allocations only while enabled, so timed runs pay one relaxed load.
struct Counting;
static COUNT: AtomicBool = AtomicBool::new(false);
static LIVE: AtomicBool = AtomicBool::new(false);
static ALLOCS: AtomicU64 = AtomicU64::new(0);
static BYTES: AtomicU64 = AtomicU64::new(0);
static LIVE_BYTES: AtomicI64 = AtomicI64::new(0);

unsafe impl GlobalAlloc for Counting {
    unsafe fn alloc(&self, l: Layout) -> *mut u8 {
        note_alloc(l.size());
        System.alloc(l)
    }
    unsafe fn alloc_zeroed(&self, l: Layout) -> *mut u8 {
        note_alloc(l.size());
        System.alloc_zeroed(l)
    }
    unsafe fn dealloc(&self, p: *mut u8, l: Layout) {
        if LIVE.load(Relaxed) {
            LIVE_BYTES.fetch_sub(l.size() as i64, Relaxed);
        }
        System.dealloc(p, l)
    }
    unsafe fn realloc(&self, p: *mut u8, l: Layout, new: usize) -> *mut u8 {
        // Like Go's growth of a slice, a reallocation is a new allocation.
        if COUNT.load(Relaxed) {
            ALLOCS.fetch_add(1, Relaxed);
            BYTES.fetch_add(new as u64, Relaxed);
        }
        if LIVE.load(Relaxed) {
            LIVE_BYTES.fetch_add(new as i64 - l.size() as i64, Relaxed);
        }
        System.realloc(p, l, new)
    }
}

fn note_alloc(size: usize) {
    if COUNT.load(Relaxed) {
        ALLOCS.fetch_add(1, Relaxed);
        BYTES.fetch_add(size as u64, Relaxed);
    }
    if LIVE.load(Relaxed) {
        LIVE_BYTES.fetch_add(size as i64, Relaxed);
    }
}

#[global_allocator]
static GLOBAL: Counting = Counting;

// ------------------------------------------------------------- case files

#[derive(Deserialize, Clone)]
struct Ref {
    #[serde(rename = "type")]
    ty: String,
    id: String,
}

#[derive(Deserialize)]
struct Req {
    principal: Ref,
    action: Ref,
    resource: Ref,
    context: serde_json::Value,
    #[serde(default)]
    principal_parents: Vec<Ref>,
}

/// One benchmark (or scenario) for one Cedar variant.
#[derive(Deserialize)]
struct Case {
    /// Go benchmark path without the variant, e.g. BenchmarkRBACMixed/n=1000.
    name: String,
    /// Variant: cedar-rs or cedar-rs-data.
    label: String,
    policies: String,
    entities: String,
    requests: Vec<Req>,
    want: Vec<bool>,
    /// Requests checked before timing; 0 means all of them.
    #[serde(default)]
    check_first: usize,
}

fn uid(r: &Ref) -> EntityUid {
    EntityUid::from_type_name_and_id(
        EntityTypeName::from_str(&r.ty).expect("entity type"),
        EntityId::new(&r.id),
    )
}

fn load(policies: &str, entities: &str) -> (PolicySet, Entities, String) {
    let src =
        std::fs::read_to_string(policies).unwrap_or_else(|e| fail(&format!("{policies}: {e}")));
    let ps = PolicySet::from_str(&src).unwrap_or_else(|e| fail(&format!("{policies}: {e}")));
    let json = if entities.is_empty() {
        "[]".to_string()
    } else {
        std::fs::read_to_string(entities).unwrap_or_else(|e| fail(&format!("{entities}: {e}")))
    };
    let es =
        Entities::from_json_str(&json, None).unwrap_or_else(|e| fail(&format!("{entities}: {e}")));
    (ps, es, json)
}

fn fail(msg: &str) -> ! {
    eprintln!("cedar-rs: {msg}");
    std::process::exit(1)
}

/// The request, plus the entities to use: the shared ones, or for a request
/// that carries principal parents (groups), a copy with that principal.
fn prepare(c: &Req, base_json: &str) -> (Request, Option<Entities>) {
    let context = Context::from_json_value(c.context.clone(), None).expect("context");
    let req = Request::new(
        uid(&c.principal),
        uid(&c.action),
        uid(&c.resource),
        context,
        None,
    )
    .expect("request");
    if c.principal_parents.is_empty() {
        return (req, None);
    }
    let principal = uid(&c.principal);
    // The request adds parents to the principal. Rewrite only its "parents"
    // array in the entity JSON, so its attributes and tags stay as loaded.
    let mut all: Vec<serde_json::Value> = serde_json::from_str(base_json).expect("entities");
    let existing = all.iter().position(|e| {
        e.get("uid")
            .and_then(|u| EntityUid::from_json(u.clone()).ok())
            .as_ref()
            == Some(&principal)
    });
    let mut parents = existing
        .and_then(|i| all[i].get("parents").and_then(|p| p.as_array()).cloned())
        .unwrap_or_default();
    for p in &c.principal_parents {
        let value = serde_json::json!({"type": p.ty, "id": p.id});
        if !parents.contains(&value) {
            parents.push(value);
        }
    }
    match existing {
        Some(i) => all[i]["parents"] = serde_json::Value::Array(parents),
        None => all.push(serde_json::json!({
            "uid": {"type": c.principal.ty, "id": c.principal.id},
            "attrs": {},
            "parents": parents,
        })),
    }
    (
        req,
        Some(Entities::from_json_value(serde_json::Value::Array(all), None).expect("entities")),
    )
}

fn decide(auth: &Authorizer, req: &Request, ps: &PolicySet, es: &Entities) -> Result<bool, String> {
    let resp = auth.is_authorized(req, ps, es);
    if let Some(err) = resp.diagnostics().errors().next() {
        return Err(err.to_string());
    }
    Ok(resp.decision() == Decision::Allow)
}

fn read_case(path: &str) -> Case {
    let text = std::fs::read_to_string(path).unwrap_or_else(|e| fail(&format!("{path}: {e}")));
    serde_json::from_str(&text).unwrap_or_else(|e| fail(&format!("{path}: {e}")))
}

// ------------------------------------------------------------- timing

/// Runs body(n) for growing n until one run lasts benchtime, like testing.B.
fn measure(benchtime: Duration, mut body: impl FnMut(u64) -> Duration) -> (u64, Duration) {
    let mut n: u64 = 1;
    loop {
        let elapsed = body(n);
        if elapsed >= benchtime || n >= 1_000_000_000 {
            return (n, elapsed);
        }
        let per = (elapsed.as_nanos() as f64 / n as f64).max(1.0);
        let predicted = (benchtime.as_nanos() as f64 * 1.2 / per) as u64;
        n = predicted.min(n * 100).max(n + 1);
    }
}

fn go_name(name: &str, label: &str, cpu: usize) -> String {
    if cpu == 1 {
        format!("{name}/{label}")
    } else {
        format!("{name}/{label}-{cpu}")
    }
}

struct Loaded {
    case: Case,
    ps: PolicySet,
    es: Entities,
    reqs: Vec<(Request, Option<Entities>)>,
}

fn load_case(path: &str) -> Loaded {
    let case = read_case(path);
    let (ps, es, json) = load(&case.policies, &case.entities);
    let reqs = case.requests.iter().map(|r| prepare(r, &json)).collect();
    Loaded { case, ps, es, reqs }
}

fn verify(l: &Loaded, auth: &Authorizer) {
    let limit = if l.case.check_first == 0 {
        l.reqs.len()
    } else {
        l.case.check_first.min(l.reqs.len())
    };
    for (j, (req, own)) in l.reqs.iter().take(limit).enumerate() {
        let es = own.as_ref().unwrap_or(&l.es);
        match decide(auth, req, &l.ps, es) {
            Ok(got) if got == l.case.want[j] => {}
            other => fail(&format!(
                "{}/{}: request {j}: got {other:?}, want {}",
                l.case.name, l.case.label, l.case.want[j]
            )),
        }
    }
}

fn bench(paths: &[String], cpu: usize, benchtime: Duration, count: usize) {
    let auth = Authorizer::new();
    for path in paths {
        let l = load_case(path);
        verify(&l, &auth);
        let k = l.reqs.len();
        let run = |n: u64| {
            let start = Instant::now();
            for i in 0..n as usize {
                let (req, own) = &l.reqs[i % k];
                let es = own.as_ref().unwrap_or(&l.es);
                match decide(&auth, req, &l.ps, es) {
                    Ok(got) if got == l.case.want[i % k] => {}
                    other => fail(&format!("{}: decision {other:?}", l.case.name)),
                }
            }
            start.elapsed()
        };
        for _ in 0..count {
            let (n, elapsed) = measure(benchtime, run);
            // Allocations come from a separate, shorter pass, so that the
            // timed pass never pays for counting.
            let sample = n.clamp(1, 4096);
            ALLOCS.store(0, Relaxed);
            BYTES.store(0, Relaxed);
            COUNT.store(true, Relaxed);
            run(sample);
            COUNT.store(false, Relaxed);
            println!(
                "{}\t{n}\t{:.1} ns/op\t{} B/op\t{} allocs/op",
                go_name(&l.case.name, &l.case.label, cpu),
                elapsed.as_nanos() as f64 / n as f64,
                BYTES.load(Relaxed) / sample,
                ALLOCS.load(Relaxed) / sample
            );
        }
    }
}

/// All threads decide at once, like b.RunParallel: ns/op is wall time
/// divided by the decisions made by every thread together.
fn parallel(paths: &[String], cpus: &[usize], benchtime: Duration, count: usize) {
    let auth = Authorizer::new();
    for path in paths {
        let l = Arc::new(load_case(path));
        verify(&l, &auth);
        for &cpu in cpus {
            for _ in 0..count {
                let run = |n: u64| {
                    let per_thread = (n / cpu as u64).max(1);
                    let start = Instant::now();
                    std::thread::scope(|s| {
                        for _ in 0..cpu {
                            let l = &l;
                            let auth = &auth;
                            s.spawn(move || {
                                let (req, _) = &l.reqs[0];
                                for _ in 0..per_thread {
                                    if decide(auth, req, &l.ps, &l.es) != Ok(l.case.want[0]) {
                                        fail(&format!("{}: wrong decision", l.case.name));
                                    }
                                }
                            });
                        }
                    });
                    start.elapsed()
                };
                let (n, elapsed) = measure(benchtime, run);
                let done = (n / cpu as u64).max(1) * cpu as u64;
                // Allocations from a separate, shorter pass, as in bench().
                // Thread-spawn allocations are part of the measured run, as
                // the timed ns/op includes them too.
                let sample = n.clamp(1, 4096);
                ALLOCS.store(0, Relaxed);
                BYTES.store(0, Relaxed);
                COUNT.store(true, Relaxed);
                run(sample);
                COUNT.store(false, Relaxed);
                let sampled = (sample / cpu as u64).max(1) * cpu as u64;
                println!(
                    "{}\t{done}\t{:.1} ns/op\t{} B/op\t{} allocs/op",
                    go_name(&l.case.name, &l.case.label, cpu),
                    elapsed.as_nanos() as f64 / done as f64,
                    BYTES.load(Relaxed) / sampled,
                    ALLOCS.load(Relaxed) / sampled
                );
            }
        }
    }
}

fn check(paths: &[String]) {
    let auth = Authorizer::new();
    let mut failed = false;
    for path in paths {
        let l = load_case(path);
        let mut pass = 0;
        for (j, (req, own)) in l.reqs.iter().enumerate() {
            let es = own.as_ref().unwrap_or(&l.es);
            match decide(&auth, req, &l.ps, es) {
                Ok(got) if got == l.case.want[j] => pass += 1,
                other => {
                    failed = true;
                    eprintln!(
                        "{}: request {j}: got {other:?}, want {}",
                        l.case.name, l.case.want[j]
                    );
                }
            }
        }
        println!("{}\t{}\t{pass}/{}", l.case.name, l.case.label, l.reqs.len());
    }
    if failed {
        std::process::exit(1);
    }
}

fn loadcost(label: &str, n: &str, users: &str, policies: &str, entities: &str) {
    let before = LIVE_BYTES.load(Relaxed);
    let start = Instant::now();
    let loaded = load(policies, entities);
    let elapsed = start.elapsed();
    drop(loaded.2); // the JSON text is not retained by the engine
    let retained = LIVE_BYTES.load(Relaxed) - before;
    let n: u64 = n.parse().expect("n");
    let users: u64 = users.parse().expect("users");
    println!(
        "| {n} ({}) | {label} | {}ms | {:.1} MB |",
        n * users,
        elapsed.as_millis(),
        retained as f64 / (1u64 << 20) as f64
    );
    std::hint::black_box(&loaded.0);
    std::hint::black_box(&loaded.1);
}

// ------------------------------------------------------------- HTTP

#[derive(Deserialize)]
struct Check {
    user: String,
    action: String,
    resource: String,
}

struct Server {
    auth: Authorizer,
    ps: PolicySet,
    es: Entities,
    user: EntityTypeName,
    action: EntityTypeName,
    resource: EntityTypeName,
}

async fn handle(
    axum::extract::State(s): axum::extract::State<Arc<Server>>,
    axum::Json(c): axum::Json<Check>,
) -> axum::response::Response {
    use axum::response::IntoResponse;
    // The same request mapping as the Go harness (CedarDefault).
    let context = match Context::from_pairs([(
        "path".to_string(),
        RestrictedExpression::new_string(c.resource.clone()),
    )]) {
        Ok(ctx) => ctx,
        Err(e) => {
            return (axum::http::StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response()
        }
    };
    let req = match Request::new(
        EntityUid::from_type_name_and_id(s.user.clone(), EntityId::new(&c.user)),
        EntityUid::from_type_name_and_id(s.action.clone(), EntityId::new(&c.action)),
        EntityUid::from_type_name_and_id(s.resource.clone(), EntityId::new(&c.resource)),
        context,
        None,
    ) {
        Ok(r) => r,
        Err(e) => return (axum::http::StatusCode::BAD_REQUEST, e.to_string()).into_response(),
    };
    match decide(&s.auth, &req, &s.ps, &s.es) {
        Ok(allowed) => (
            [(axum::http::header::CONTENT_TYPE, "application/json")],
            format!("{{\"allowed\":{allowed}}}"),
        )
            .into_response(),
        Err(e) => (axum::http::StatusCode::INTERNAL_SERVER_ERROR, e).into_response(),
    }
}

fn serve(addr: &str, policies: &str, entities: &str) {
    let start = Instant::now();
    let (ps, es, _) = load(policies, entities);
    let state = Arc::new(Server {
        auth: Authorizer::new(),
        ps,
        es,
        user: EntityTypeName::from_str("User").unwrap(),
        action: EntityTypeName::from_str("Action").unwrap(),
        resource: EntityTypeName::from_str("Resource").unwrap(),
    });
    eprintln!(
        "cedar-rs loaded {policies} in {:?}; listening on {addr}",
        start.elapsed()
    );
    // Worker threads follow GOMAXPROCS, which stress.sh sets for every server.
    let workers = std::env::var("GOMAXPROCS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or_else(|| {
            std::thread::available_parallelism()
                .map(|n| n.get())
                .unwrap_or(1)
        });
    let rt = tokio::runtime::Builder::new_multi_thread()
        .worker_threads(workers)
        .enable_all()
        .build()
        .expect("runtime");
    rt.block_on(async {
        let app = axum::Router::new()
            .route("/check", axum::routing::post(handle))
            .route("/health", axum::routing::get(|| async { "ok" }))
            .with_state(state);
        let listener = tokio::net::TcpListener::bind(addr).await.expect("listen");
        axum::serve(listener, app).await.expect("serve");
    });
}

// ------------------------------------------------------------- main

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("loadcost") {
        LIVE.store(true, Relaxed);
    }
    let mut flags: HashMap<String, String> = HashMap::new();
    let mut rest: Vec<String> = Vec::new();
    let mut i = 1;
    while i < args.len() {
        if let Some(name) = args[i].strip_prefix("--") {
            flags.insert(
                name.to_string(),
                args.get(i + 1).cloned().unwrap_or_default(),
            );
            i += 2;
        } else {
            rest.push(args[i].clone());
            i += 1;
        }
    }
    let flag = |name: &str, default: &str| {
        flags
            .get(name)
            .cloned()
            .unwrap_or_else(|| default.to_string())
    };
    let benchtime = {
        let s = flag("benchtime", "1s");
        Duration::from_secs_f64(
            s.trim_end_matches('s')
                .parse()
                .unwrap_or_else(|_| fail("benchtime is seconds, like 1s")),
        )
    };
    let count: usize = flag("count", "1").parse().expect("count");
    match args.first().map(String::as_str) {
        Some("bench") => bench(
            &rest,
            flag("cpu", "1").parse().expect("cpu"),
            benchtime,
            count,
        ),
        Some("parallel") => {
            let cpus: Vec<usize> = flag("cpu", "1")
                .split(',')
                .map(|c| c.parse().expect("cpu"))
                .collect();
            parallel(&rest, &cpus, benchtime, count)
        }
        Some("check") => check(&rest),
        Some("loadcost") if rest.len() == 2 => loadcost(
            &flag("label", "cedar-rs"),
            &flag("n", "0"),
            &flag("users", "0"),
            &rest[0],
            &rest[1],
        ),
        Some("serve") if rest.len() == 2 => {
            serve(&flag("addr", "127.0.0.1:9000"), &rest[0], &rest[1])
        }
        _ => {
            fail("usage: cedar-rs bench|parallel|check|loadcost|serve ... (see the source header)")
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn req_with_group() -> Req {
        Req {
            principal: Ref {
                ty: "User".into(),
                id: "alice".into(),
            },
            action: Ref {
                ty: "Action".into(),
                id: "read".into(),
            },
            resource: Ref {
                ty: "Resource".into(),
                id: "/data1".into(),
            },
            context: serde_json::json!({"path": "/data1"}),
            principal_parents: vec![Ref {
                ty: "Group".into(),
                id: "eng".into(),
            }],
        }
    }

    // A request-supplied group must be added to the principal without
    // dropping its attributes, matching CedarEngine.Prepare in engines.go.
    #[test]
    fn request_parents_keep_principal_attributes() {
        let entities_json = r#"[
            {"uid":{"type":"User","id":"alice"},"attrs":{"dept":{"type":"String","value":"eng"},"level":{"type":"Long","value":3}},"parents":[{"type":"Role","id":"viewer"}]},
            {"uid":{"type":"Role","id":"viewer"},"attrs":{},"parents":[]},
            {"uid":{"type":"Group","id":"eng"},"attrs":{},"parents":[]}
        ]"#;
        let (_, entities) = prepare(&req_with_group(), entities_json);
        let entities = entities.expect("request entities");
        let alice = entities
            .get(&EntityUid::from_type_name_and_id(
                EntityTypeName::from_str("User").unwrap(),
                EntityId::new("alice"),
            ))
            .expect("alice");
        let json = alice.to_json_value().expect("entity json");
        assert_eq!(
            json["attrs"].as_object().map(|m| m.len()),
            Some(2),
            "principal attributes must be preserved: {json}"
        );
        let parents = json["parents"].as_array().expect("parents");
        assert!(parents.iter().any(|p| p["id"] == "viewer"));
        assert!(parents.iter().any(|p| p["id"] == "eng"));
    }
}
