+++
title = "Performance testing guide"
description = "Run a fair, reproducible authorization-engine comparison on your own machine with the help of an AI coding tool."
weight = 15
draft = false
toc = true
singlelinetitle = true
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "performance", "benchmark", "policy"]
categories = ["docs"]
bref = "Measure Speedle and other authorization engines on your own hardware, with your own policies"
lastmod = 2026-11-11T12:00:00+08:00
+++

This guide helps you compare Speedle with other authorization engines on your own machine. It does not publish a performance report. Numbers depend on your hardware, your policy set and the engine versions you pin, so the result worth trusting is the one you measure yourself.

The guide provides a shared scenario, equivalent policies for four engines, a methodology checklist and a prompt you can paste into an AI coding tool such as Claude Code, ChatGPT Codex, DeepSeek Harness or a similar tool. The prompt asks the tool to write the scripts, run them and keep the raw output on your machine. Treat anything the tool writes as code to review, not as a published result.

## What you get

- Equivalent RBAC and ABAC rules for Speedle, Open Policy Agent (OPA), Casbin and Cedar, with the expected decision for every request.
- Runnable benchmark scripts, exact commands and pinned engine versions, generated and run in a scratch directory on your machine.
- Latency percentiles, throughput, memory use and scaling, with policy loading measured separately from steady-state evaluation.
- Raw output, environment details and a results template that stay local. Nothing is uploaded by this guide.

The candidate set is a starting point, not a ranking. Confirm that each engine version you pick supports the scenario, record any case you exclude and why, and do not present an excluded case as a result for that engine.

## Before you start

- Use a machine you control and can leave idle for the runs. Close other heavy processes; note the CPU model, core count, memory and operating system.
- Install the tools the comparison needs: Git, Go 1.26 or newer for Speedle and the Go-based engines, plus the runtime each engine's official quick start requires. Pin every engine version, including the Speedle release tag.
- Use an AI coding tool that can create files and run shell commands in a scratch directory. Review its scripts before running them, and keep the whole directory, including the raw output, for your notes.
- Budget about 30–60 minutes for a first comparison with small policy sets, and more if you add policy counts and concurrency.

Work in an empty directory. Do not run the comparison inside the Speedle checkout; keep the generated scripts, policies, logs and results separate from any repository.

## The shared scenario

The comparison uses one set of business rules:

- Employees can read documents in their own department.
- Editors can read and edit documents in their own department.
- Bob, an editor, is explicitly denied editing the Q4 finance report.
- A request to another department is denied even when the user holds the matching role.

The request carries the user's `department` and the resource's `owner_department` as attributes supplied by the application. All engines receive the same requests and must produce the same decision.

| User | Role | Action | Resource | department | owner_department | Expected |
| --- | --- | --- | --- | --- | --- | --- |
| alice | employee | read | `/docs/finance/report` | finance | finance | Allow |
| alice | employee | read | `/docs/hr/salaries` | finance | hr | Deny |
| alice | employee | read | `/payroll/finance/report` | finance | finance | Deny (resource outside `/docs`) |
| alice | employee | read | `/payroll/docs/finance/report` | finance | finance | Deny (mid-path `/docs` does not match) |
| alice | employee | readwrite | `/docs/finance/report` | finance | finance | Deny (action not granted) |
| bob | editor | edit | `/docs/finance/report` | finance | finance | Allow |
| bob | editor | edit | `/docs/finance/q4` | finance | finance | Deny (explicit deny) |
| carol | editor | edit | `/docs/finance/q4` | hr | finance | Deny (department condition) |
| carol | editor | read | `/docs/hr/handbook` | hr | hr | Allow |

The examples in this guide were validated with Speedle v1.6.0, OPA v1.22.3, Casbin v2.123.0 and Cedar CLI v4.14.1. Pin the versions you install; these are examples, not a claim that they are the only supported versions.

### Speedle (SPDL)

```spdl
[service.documents]

[policy]
grant role employee read expr:^/docs/.* if department == owner_department
grant role editor read,edit expr:^/docs/.* if department == owner_department
deny user bob edit /docs/finance/q4

[rolepolicy]
grant user alice role employee
grant user bob role editor
grant user carol role editor
```

Speedle evaluates an applicable deny over a grant. A condition that references an attribute missing from the request does not match, so the application must send `department` and `owner_department` on every request. The policy file is validated with `spctl check`; see [SPDL](../spdl/) for the grammar and [Policy Management](../pms/policy-mgmt/) for services and role policies.

### OPA (Rego)

Policy:

```rego
package documents

import rego.v1

default allow := false

allow if {
	not denied
	input.action == "read"
	"employee" in roles
	startswith(input.resource, "/docs/")
	input.attributes.department == input.attributes.owner_department
}

allow if {
	not denied
	input.action in {"read", "edit"}
	"editor" in roles
	startswith(input.resource, "/docs/")
	input.attributes.department == input.attributes.owner_department
}

roles contains role if {
	some role in data.roles[input.user]
}

denied if {
	input.user == "bob"
	input.action == "edit"
	input.resource == "/docs/finance/q4"
}
```

Data (`data.json`):

```json
{
  "roles": {
    "alice": ["employee"],
    "bob": ["editor"],
    "carol": ["editor"]
  }
}
```

Rego has no built-in deny-overrides rule, so the policies above subtract the explicit deny from both allow rules. The role data is part of the OPA data document. Use the Rego syntax of the OPA version you pin (the example uses the Rego v1 syntax of OPA 1.x).

### Casbin

Model (`model.conf`):

```ini
[request_definition]
r = sub, obj, act, dept, ownerDept

[policy_definition]
p = sub, obj, act, eft

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (r.sub == p.sub || g(r.sub, p.sub)) && keyMatch(r.obj, p.obj) && r.act == p.act && r.dept == r.ownerDept
```

Policy (`policy.csv`):

```csv
p, employee, /docs/*, read, allow
p, editor, /docs/*, read, allow
p, editor, /docs/*, edit, allow
p, bob, /docs/finance/q4, edit, deny
g, alice, employee
g, bob, editor
g, carol, editor
```

Casbin splits the model, the policy and the role assignments into three inputs. The ABAC department check lives in the matcher, so it applies to the deny row too; this scenario's deny request carries matching departments, which keeps the expected decisions. Record any change you make to the matcher for an unconditional deny.

### Cedar

Schema (`schema.cedar`):

```cedar
entity Role;
entity User in [Role];
entity Folder;
entity Document in [Folder];

action "read" appliesTo {
  principal: [User],
  resource: [Document],
  context: { department: String, ownerDepartment: String }
};
action "edit" appliesTo {
  principal: [User],
  resource: [Document],
  context: { department: String, ownerDepartment: String }
};
action "readwrite" appliesTo {
  principal: [User],
  resource: [Document],
  context: { department: String, ownerDepartment: String }
};
```

Policy (`policies.cedar`):

```cedar
permit (
  principal in Role::"employee",
  action == Action::"read",
  resource in Folder::"/docs"
)
when { context.department == context.ownerDepartment };

permit (
  principal in Role::"editor",
  action in [Action::"read", Action::"edit"],
  resource in Folder::"/docs"
)
when { context.department == context.ownerDepartment };

forbid (
  principal == User::"bob",
  action == Action::"edit",
  resource == Document::"/docs/finance/q4"
);
```

Entities (`entities.json`):

```json
[
  {"uid": {"type": "User", "id": "alice"}, "attrs": {}, "parents": [{"type": "Role", "id": "employee"}]},
  {"uid": {"type": "User", "id": "bob"}, "attrs": {}, "parents": [{"type": "Role", "id": "editor"}]},
  {"uid": {"type": "User", "id": "carol"}, "attrs": {}, "parents": [{"type": "Role", "id": "editor"}]},
  {"uid": {"type": "Folder", "id": "/docs"}, "attrs": {}, "parents": []},
  {"uid": {"type": "Document", "id": "/payroll/finance/report"}, "attrs": {}, "parents": []},
  {"uid": {"type": "Document", "id": "/payroll/docs/finance/report"}, "attrs": {}, "parents": []},
  {"uid": {"type": "Document", "id": "/docs/finance/report"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/finance/q4"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/hr/salaries"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/hr/handbook"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]}
]
```

Cedar enforces a forbid over a permit, and every request starts denied. Roles and the document hierarchy are data in the entity store, not text in the policy, so the schema and entities are part of the comparison. The example entity list must include every document the request matrix uses.

### Semantic differences to record

The same business rule does not always map to the same engine concepts. Before timing anything, make the four implementations agree on the expected decisions and write down:

- How each engine resolves a deny that matches an allow, and whether the deny is unconditional or shares the ABAC condition.
- What each engine does when a request omits `department` or `owner_department`.
- How resource patterns are expressed (SPDL `expr:` regular expressions, Casbin matchers, OPA helper rules, Cedar entity hierarchy).
- How an action is matched: exact action names in SPDL and Cedar, equality in the OPA rules, and the Casbin matcher (anchor the comparison so `read` does not match `readwrite`).
- Which parts are required application code or configuration outside the policy text (Cedar schema and entities, Casbin model and role data, OPA data documents, Speedle service and role policies).
- Any case an engine cannot express in the chosen version; exclude it from the ranked cases and say so.

## Measure the right thing

Use the same machine, the same request data and the same policy-set sizes for every engine. Pin versions and keep the generated dataset, so a rerun measures the same work.

- **Separate embedded evaluation from service or API tests.** An in-process function call and an HTTP request measure different things. Report them in separate tables; never rank one against the other.
- **Warm up and repeat.** Run each engine enough times to reach a stable result, discard the warm-up runs, repeat the whole measurement at least three times, and report run-to-run variation.
- **Report latency and throughput.** Include p50, p95 and p99 latency and requests per second, where the engine's tooling supports them. Record failures and timeouts.
- **Measure memory.** Record peak resident memory or the engine's own memory metric, whichever you can measure consistently.
- **Vary policy count and concurrency.** Start small, then increase the policy set and the number of concurrent callers. Record every data point, not only the best one.
- **Measure policy loading and startup separately.** Time how long each engine takes to parse and load the policy set before the steady-state loop; do not hide that cost inside the first request.
- **Disclose caching and preparation.** Note indexes, compiled policies, decision caches and any precomputation, and whether they were warm or cold.
- **Keep the raw output.** Save the exact command, configuration, policy files, engine versions, environment and full output for every run.

## Run the comparison with an AI coding tool

Open an empty directory in Claude Code, ChatGPT Codex, DeepSeek Harness or a similar tool that can write files and run commands. Paste the prompt below as-is. It asks the tool to record the environment, pin versions, build the shared scenario, validate the expected decisions, run the measurements and produce a local results template.

```text
Help me run a quick performance comparison of Speedle with OPA, Casbin and Cedar on my own machine. First record my operating system, hardware and resource limits, then select and pin supported engine versions. Build equivalent RBAC and ABAC examples with a shared dataset and expected allow/deny decisions. Explain semantic differences and exclude unsupported cases from direct rankings. Validate correctness before measuring performance.

Create runnable benchmark scripts and exact setup/run commands. Keep embedded and service/API tests separate. Use identical workloads, vary policy count and concurrency, disclose caching and preparation, warm up each engine and repeat measurements. Report latency, throughput, memory use and run-to-run variation where supported, with raw output and environment details. Only report numbers you actually measured; label anything you could not run.

Show the equivalent policy and any required model/schema or application code for each engine. Compare readability, concepts to learn, authoring effort, validation, debugging and ease of changing a policy. Explain the trade-offs and which engine fits each tested workload without assuming Speedle wins.
```

The tool should work in the current directory and leave the scripts, policies and output there. Review the scripts before the first run, especially any download or install step. If the tool cannot install an engine in your environment, ask it to label that engine as not run instead of estimating a number.

## Check the AI tool's work

- Confirm the four implementations return the tabulated decision for every request before any timing starts. A fast wrong answer is not a result.
- Confirm the versions it installed match the versions it reports, and that the same machine, request data and policy-set sizes were used for every engine.
- Confirm loading and startup are reported separately from the steady-state loop, and that embedded and service/API measurements are in separate tables.
- Confirm the raw output is present for every number in the summary; if a number has no command and output behind it, treat it as unmeasured.
- Read the limitations section. An engine that could not run a case must be labelled, not ranked.

## Results template

Have the tool fill a local table like this one and keep it next to the raw output. Do not publish it as a Speedle performance report.

| Engine | Version | Mode | Policies | Concurrency | p50 | p95 | p99 | Throughput | Peak memory | Policy load time |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Speedle | | embedded | | | | | | | | |
| OPA | | embedded | | | | | | | | |
| Casbin | | embedded | | | | | | | | |
| Cedar | | embedded | | | | | | | | |

Keep a second table for the runs:

| Run | Engine | Command | Raw output | Notes |
| --- | --- | --- | --- | --- |
| 1 | | | | |

## Usability comparison

Measurements and usability are separate observations. Compare the same business rules, then record how each engine expresses them:

- **Speedle (SPDL).** Each rule is one statement, the role assignments are a separate role-policy section, and the condition is a boolean expression over request attributes. Policies can be checked with `spctl check`.
- **OPA (Rego).** Rules are written as bodies under a default-deny, role data lives in a separate document, and an explicit deny needs its own rule and guards in the allow rules.
- **Casbin.** The model, the policy rows and the role assignments are separate inputs. The ABAC check is part of the matcher expression, so changing the condition means changing the model.
- **Cedar.** A schema declares the entities and actions, policies are permit and forbid statements, and roles and resource hierarchy live in the entity store. Forbid overrides permit.

For each engine, record readability (how directly the rule reads as the business sentence), the concepts you had to learn, the boilerplate and application code, validation and error messages, testing and debugging, and what it takes to add or change one rule. Lines of policy can provide context, but do not use line count as the only measure of ease of use.

## Speedle-only benchmarks

The repository's [benchmarks](https://github.com/teramoby/speedle-plus/blob/master/benchmarks/README.md) cover a Speedle-only evaluation, including an opt-in one-million-policy case. Run them in the checkout with `go test -bench .`, and follow the README for the million-policy case. Those measurements are for Speedle alone and were taken on specific hardware; do not present them as a current cross-engine comparison.

## Report fairly

- Use the same scenario, data and policy-set sizes for every engine, and say which cases each engine could not run.
- Report the full range of your results, including cases where another engine is faster.
- Explain which engine fits which tested workload, with limitations. Do not declare a universal fastest engine from one scenario on one machine.
- Keep the raw output and the exact commands with any summary you share.
