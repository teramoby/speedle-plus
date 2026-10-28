+++
title = "Authorization and access control"
keywords = ["Speedle+", "authorization", "access control", "RBAC", "ABAC", "PBAC", "identity", "IAM", "OES", "XACML", "SPDL", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "PAP", "policy administration point", "PDP", "policy decision point", "PEP", "policy enforcement point", "PIP", "policy information point", "centralized authorization", "authorization policy management", "authorization decision service", "API authorization", "microservices authorization", "cloud-native authorization", "zero trust"]
description = "How Speedle+ implements authorization and fine-grained access control using RBAC, ABAC and PBAC, integrates with identity and IAM, and relates to OES and XACML."
weight = 12
bref = "Authorization concepts, working policies and integration boundaries"
+++

## What does Speedle authorize?

Speedle+ is an open-source authorization engine. It answers whether a subject may perform an action on a resource within a service. Applications enforce that answer. Moving access control into policies lets services share authorization rules without embedding each rule in application code.

Authentication establishes an identity; authorization determines what that identity can do. Speedle can work with identities supplied by a trusted application or a configured token asserter. It does not replace every component of an identity and access management (IAM) system.

## Fine-grained access control (FGA) and zero trust

Fine-grained access control (FGA) decides each request from its specific subject, action, resource and context, instead of granting broad access to a whole application or network. Speedle+ evaluates every request against SPDL policies, so a decision can depend on the user, their groups and roles, the exact resource and attributes of the request.

A zero trust architecture trusts no request because of where it comes from; every access to a protected resource is authorized explicitly. Speedle+ supplies that authorization decision. In XACML terms, the application or gateway is the policy enforcement point (PEP) and asks the Authorization Decision Service, the policy decision point (PDP), about each protected operation. Policies are administered through the Policy Management Service, the policy administration point (PAP), and trusted identity and attribute sources act as policy information points (PIP); see [XACML and SPDL](/docs/access-control/xacml/). Speedle+ does not provide authentication, device posture or network controls itself. A zero trust deployment combines it with those components.

## Choose an access control model

| Topic | How it applies to Speedle+ |
| --- | --- |
| [RBAC: role-based access control](/docs/access-control/rbac/) | Assign roles to users or groups and grant permissions to roles. |
| [ABAC: attribute-based access control](/docs/access-control/abac/) | Add conditions using attributes supplied by trusted infrastructure. |
| [PBAC: policy-based access control](/docs/access-control/pbac/) | Manage authorization rules centrally and evaluate them through a decision service or embedded evaluator. |
| [Identity and IAM](/docs/access-control/identity-iam/) | Connect identity providers, distinguish identity domains and define the trust boundary. |
| [OES and Speedle](/docs/access-control/oes/) | Compare Oracle Entitlements Server concepts and plan a migration assessment. |
| [XACML and SPDL](/docs/access-control/xacml/) | Map PAP/PDP/PEP responsibilities and understand policy-language differences. |

RBAC and ABAC can coexist in a policy-based design. These names describe different aspects of access control; choosing PBAC does not require abandoning roles or attributes.

## A first authorization policy

Paste this SPDL policy into the [Playground](/playground/):

```text
grant user alice read book
deny user alice delete book
```

Set the user to `alice`, resource to `book`, and action to `read`: the expected result is Allow. Change the action to `delete`: the expected result is Deny. With action `write`, no grant matches, so access is denied by default. Test both successful and rejected requests before deploying a policy.

In a deployed service, the [Authorization Decision Service](/docs/decisions/) evaluates the request and the application must enforce its result, including handling evaluation errors. The browser Playground runs the real Go authorization engine locally, but does not contact identity providers or external custom functions.

Continue with the [architecture](/docs/architecture/), [SPDL reference](/docs/spdl/) and [quick start](/quick-start/).
