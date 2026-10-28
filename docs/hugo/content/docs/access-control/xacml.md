+++
title = "XACML, SPDL and authorization architecture"
description = "Relate XACML access control concepts to Speedle+: PAP, PDP, PEP and attribute sources, SPDL policy syntax, deny precedence and policy migration boundaries."
weight = 60
keywords = ["XACML", "eXtensible Access Control Markup Language", "authorization", "policy-based access control", "Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "PAP", "policy administration point", "PDP", "policy decision point", "PEP", "policy enforcement point", "PIP", "policy information point"]
bref = "Map architecture concepts without assuming policy-format compatibility"
+++

## What is the relationship to XACML?

The eXtensible Access Control Markup Language (XACML) is an OASIS access control standard defining policy and request/response concepts. Speedle+ uses its own Security Policy Definition Language (SPDL) and APIs. The [OASIS XACML 3.0 specification](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-os-en.html) is the reference for XACML behavior; this page explains architectural correspondences, not XACML conformance.

## Map the responsibilities

| XACML concept | Responsibility | Speedle+ correspondence |
| --- | --- | --- |
| PAP: Policy Administration Point | Manage authorization policy | Policy Management Service (PMS) |
| PDP: Policy Decision Point | Evaluate authorization requests | Authorization Decision Service (ADS) or embedded evaluator |
| PEP: Policy Enforcement Point | Enforce a decision on an operation | Application, gateway or integration calling Speedle |
| PIP: Policy Information Point | Supply attributes used by a decision | Trusted application data and configured identity/attribute integrations; no claim of a built-in XACML PIP interface |

Speedle's [architecture page](/docs/architecture/) already describes the PAP/PDP correspondence. The enforcement point must still prevent an operation when the decision denies access or evaluation fails.

## Policy syntax and decision semantics

An SPDL authorization statement names its effect, principal, action and resource, with an optional condition:

```text
grant user alice read book if clearance >= 3
```

For user `alice`, action `read`, resource `book` and numeric attribute `clearance = 4`, this statement grants access in the [Playground](/playground/). A value of `2` does not satisfy the condition.

XACML policies can specify combining algorithms and include other standard constructs. Speedle's applicable explicit deny takes precedence over a grant. Do not assume an arbitrary XACML combining algorithm, obligation, advice or attribute-category mapping survives translation unchanged. Likewise, a Speedle evaluation error should not be silently treated as a successful grant or as an exact XACML response-code mapping.

## Assess translation before migration

Identify the policy constructs and profiles actually used. Map principal names and domains, actions, resources, attribute names and types, and decision/error handling. Recreate representative requests and compare expected results, including denials and missing attributes. Keep unsupported constructs explicit rather than dropping them during translation.

This guide does not provide a XACML XML policy importer, a XACML protocol endpoint or a conformance certification. Read the [SPDL reference](/docs/spdl/), [decision API](/docs/api/decision_api/), [PBAC guide](/docs/access-control/pbac/) and [OES comparison](/docs/access-control/oes/) before designing an integration.
