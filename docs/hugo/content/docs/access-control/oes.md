+++
title = "Oracle Entitlements Server (OES) and Speedle"
description = "Compare Oracle Entitlements Server (OES) and Speedle+ authorization architecture, policy management and enforcement, with a practical migration assessment checklist."
weight = 50
keywords = ["OES", "Oracle Entitlements Server", "authorization", "access control", "Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "centralized authorization", "authorization policy management"]
bref = "Compare authorization responsibilities before evaluating a migration"
+++

## Why compare OES and Speedle?

Oracle Entitlements Server (OES) and Speedle+ both address externalized, fine-grained authorization. Oracle's [OES architecture documentation](https://docs.oracle.com/cd/E27559_01/admin.1112/e27153/intro.htm) describes centralized policy management and security modules that evaluate and enforce authorization policies. Speedle provides policy management and authorization decisions through its own services and APIs.

The comparison is useful when reviewing an existing OES integration or evaluating an open-source authorization engine. Similar responsibilities do not make the products interchangeable. Check the Oracle documentation for the exact OES version in use; this page does not assert compatibility with OES policy exports, APIs or security modules.

## Compare responsibilities

| Concern | OES concept | Speedle+ approach |
| --- | --- | --- |
| Policy administration | Central policy administration | Policy Management Service (PMS) |
| Authorization evaluation | Security modules and their decision integration | Authorization Decision Service (ADS) or embedded Go evaluator |
| Application enforcement | Integration with an OES security module | Application or gateway enforces the Speedle result |
| Policy representation | OES policy model and version-specific formats | SPDL and the Speedle management API data model |
| Identity context | Configured identity and attribute integrations | Explicit principals, identity domains, attributes and configured token asserter |

See the [Speedle architecture](/docs/architecture/) for the actual component boundaries. These rows map responsibilities, not binary or wire-level compatibility.

## Assess a migration with real requests

1. Inventory the OES policies, roles, resource matching, identity sources, custom functions and enforcement integrations used by the application.
2. Record representative requests and expected decisions, including explicit deny, no matching policy, attribute failures and role inheritance.
3. Model those rules in [SPDL](/docs/spdl/) and identify semantics that require application changes or cannot be represented directly.
4. Test proposed policies in the [Playground](/playground/) where supported. Token validation and external functions require a server-side integration test.
5. Compare both engines on the same trusted request context before switching enforcement. Define a rollback plan and verify policy-update behavior.

An automatic OES importer or drop-in replacement is not provided by this guide. Whether Speedle is suitable depends on the application's required semantics and integrations, rather than a matching feature name.

Continue with [RBAC](/docs/access-control/rbac/), [ABAC](/docs/access-control/abac/), [identity and IAM](/docs/access-control/identity-iam/) and [XACML concepts](/docs/access-control/xacml/).
