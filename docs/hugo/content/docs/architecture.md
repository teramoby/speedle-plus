+++
title = "Architecture"
keywords = ["Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "PAP", "policy administration point", "PDP", "policy decision point", "PEP", "policy enforcement point", "PIP", "policy information point", "centralized authorization", "authorization policy management", "authorization decision service", "API authorization", "microservices authorization", "cloud-native authorization"]
description = "Speedle building blocks"
weight = 10
draft = false
toc = false
tocheading = "h2"
tocsidebar = false
bref = "Speedle architecture"
+++

## Modules

Speedle is an authorization engine with three main components:

- **Policy Management Service (PMS)** manages services, authorization policies, role policies and custom functions.
- **Policy repository** persists those objects using a registered store such as file, etcd or MongoDB.
- **Authorization Decision Service (ADS)** evaluates a subject, resource, action and trusted attributes against the applicable policies.

<img src="/img/speedle/spdlarch.jpg" />

The historical diagram labels the decision component **ARS**; this is the component called **ADS** in current documentation and APIs.

(1) Administrators manage policies through PMS.<br>
(2) PMS persists policy objects in the repository.<br>
(3) ADS loads them and observes supported store changes asynchronously.<br>
(4) The application supplies trusted request context, calls ADS and enforces the returned result, including rejecting on errors.

## Responsibilities and trust boundaries

In XACML terminology, PMS corresponds to the Policy Administration Point (PAP), ADS or the embedded evaluator to the Policy Decision Point (PDP), and the application/gateway to the Policy Enforcement Point (PEP). Attribute sources serve the information-supply responsibility (PIP); this mapping does not claim XACML protocol support.

Policy management and evaluation are separate from authenticating API callers. Both standalone transports, REST and gRPC, need appropriate [security configuration](/docs/security/). A [token asserter](/docs/assertor/) can validate configured identity tokens. A policy decision alone neither authenticates every caller nor prevents an application from executing an operation.

See [authorization/access control](/docs/access-control/) for RBAC, ABAC and PBAC, [identity/IAM](/docs/access-control/identity-iam/) for identity boundaries, [OES](/docs/access-control/oes/) and [XACML](/docs/access-control/xacml/) for architectural comparisons, and [embedded mode](/docs/deployment-embeded/) for an in-process Go integration.
