+++
title = "PBAC: policy-based access control"
description = "Policy-based access control (PBAC) with Speedle+: externalize authorization in SPDL, manage policy through PMS and enforce ADS decisions with RBAC and ABAC."
weight = 30
keywords = ["PBAC", "policy-based access control", "authorization", "access control", "Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "centralized authorization", "authorization policy management", "authorization decision service"]
bref = "Separate policy management, authorization decisions and enforcement"
+++

## What PBAC means here

Policy-based access control (PBAC) expresses authorization rules as managed policies instead of scattering permission checks across application code. Speedle+ uses the Speedle Policy Definition Language (SPDL), a Policy Management Service (PMS), and an Authorization Decision Service (ADS) or embedded evaluator.

PBAC describes how rules are expressed and managed. [RBAC](/docs/access-control/rbac/) describes role relationships; [ABAC](/docs/access-control/abac/) describes attribute conditions. A Speedle policy-based system can use both.

## From policy to enforcement

1. Define the service, principal names, resource identifiers and supported actions.
2. Create authorization and role policies through [PMS](/docs/pms/policy-mgmt/).
3. Ensure the decision service has received the intended policy version.
4. Send the current subject, resource, action and trusted attributes to [ADS](/docs/decisions/).
5. Enforce the result in the application, gateway or other enforcement point. Handle timeout and evaluation errors explicitly.

Central management does not remove the need to enforce decisions. It also does not guarantee instantaneous propagation to every evaluator; validate the update behavior of the chosen [policy store](/docs/store/).

## Test deny precedence

Use these policies in the [Playground](/playground/):

```text
grant user alice read book
deny user alice read book
```

For user `alice`, resource `book`, action `read`, the result is Deny because both statements match and explicit deny takes precedence. Remove the second line and the result is Allow. Change the user to `bob` and no applicable grant remains, so access is denied by default.

## Review policy changes like code

Keep the intended rules and representative requests under version control. Check grants, explicit denials, unmatched requests, role scopes and attribute boundaries. Record which policy revision you tested and how it reaches the deployed decision service. Do not infer production latency or update consistency from the local browser demo.

See the [architecture](/docs/architecture/), [SPDL reference](/docs/spdl/) and [XACML comparison](/docs/access-control/xacml/) for more on management and decision boundaries.
