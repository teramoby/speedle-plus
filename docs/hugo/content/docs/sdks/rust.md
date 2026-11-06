+++
title = "Rust SDK"
description = "Add the Speedle Rust SDK and check authorization against a local Speedle or Speedle Cloud."
weight = 50
aliases = ["/docs/api/sdk-rust/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Add the speedle 0.1.0 crate with cargo add, then call is_allowed and check against a local Speedle or Speedle Cloud."
+++

The Rust SDK needs Rust 1.75 or later. It is async on `tokio` and `reqwest` (rustls); the `blocking` feature adds the blocking client used below. Crate: [`speedle`](https://crates.io/crates/speedle) 0.1.0 on crates.io. Reference: [speedle-sdk/rust](https://github.com/teramoby/speedle-sdk/tree/main/rust).

The examples check the policy described in the [SDKs overview](/docs/sdks/#example-policy).

## Add the dependency

In a new package:

```bash
cargo new speedle-example
cd speedle-example
cargo add speedle@0.1.0 --features blocking
```

## Call a local Speedle

Start Speedle and create the example policy as described in the [SDKs overview](/docs/sdks/#example-policy). Replace `src/main.rs` with:

```rust
use speedle::blocking::Client;
use speedle::{Auth, Backend};

fn main() {
    if let Err(e) = run() {
        eprintln!("Speedle error: {e}");
        std::process::exit(1);
    }
}

fn run() -> Result<(), speedle::Error> {
    let speedle = Client::builder(Backend::SpeedlePlus, "http://localhost:6734", Auth::none(), "docs").build()?;

    let allowed = speedle.is_allowed(&"alice".into(), "get", "/docs/public", &[])?;
    println!("alice get /docs/public: allowed={allowed}");

    let d = speedle.check(&"alice".into(), "delete", "/docs/public", &[])?;
    println!("alice delete /docs/public: allowed={} reason={:?}", d.allowed, d.reason);
    Ok(())
}
```

Run it:

```bash
cargo run
```

Output:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

## Call Speedle Cloud

> **Speedle Cloud is coming soon.**

The program reads the service name from `SPEEDLE_SERVICE` and the service's token from `SPEEDLE_TOKEN`. `SPEEDLE_ENDPOINT` overrides the default endpoint `https://cloud.speedle.io`. Replace `src/main.rs` with:

```rust
use std::env;

use speedle::blocking::Client;
use speedle::{Auth, Backend};

fn main() {
    if let Err(e) = run() {
        eprintln!("Speedle error: {e}");
        std::process::exit(1);
    }
}

fn run() -> Result<(), speedle::Error> {
    let endpoint = env::var("SPEEDLE_ENDPOINT").unwrap_or_else(|_| "https://cloud.speedle.io".to_string());
    let token = env::var("SPEEDLE_TOKEN").unwrap_or_default();
    let service = env::var("SPEEDLE_SERVICE").unwrap_or_default();
    let speedle = Client::builder(Backend::SpeedleCloud, &endpoint, Auth::bearer(token), service).build()?;

    let allowed = speedle.is_allowed(&"alice".into(), "get", "/docs/public", &[])?;
    println!("alice get /docs/public: allowed={allowed}");

    let d = speedle.check(&"alice".into(), "delete", "/docs/public", &[])?;
    println!("alice delete /docs/public: allowed={} reason={:?}", d.allowed, d.reason);
    Ok(())
}
```

Set the environment and run it. `read -rs` keeps the token out of your shell history and screen:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
cargo run
```

Output, when the service has the example policy:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

The token is a server-side secret. Never ship it to browsers, mobile apps or desktop clients.

## Error handling

A denied request is not an error: `is_allowed` returns `false`, and `check` returns a decision with `allowed` false and the reason, as in the `delete` call above. With the default settings, when a call cannot return a decision, it returns `Err(speedle::Error)`. The examples pass the error to `main` with `?`, print `Speedle error: <message>` to standard error and exit with status 1.
