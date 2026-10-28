+++
title = "Speedle Overview: Authorization Engine for Fine-Grained Access Control"
linktitle = "Overview"
description = "How Speedle maps to common authorization terms: PAP, PDP, PEP, PIP, RBAC, ABAC, PBAC, XACML, zero trust, identity and IAM."
weight = 2
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["core", "pdp", "identity", "spdl"]
categories = ["docs"]
bref = "Where Speedle fits in the vocabulary of authorization and access control"
+++

Speedle is an open-source authorization engine. Applications send it a question such as "may this user perform this action on this resource?", and Speedle answers allow or deny according to centrally managed policies. This page connects common authorization terms to the Speedle components that implement them, and links to the guides that cover each one.

The examples below use a service named `library`. Each policy was created through the Policy Management Service REST API and each decision was returned by the Authorization Decision Service REST API. The reason codes are listed in [Authorization Decisions](../decisions/).

## Authorization engine

An authorization engine evaluates access requests against policies and returns a decision, so that the authorization logic lives outside application code. Speedle consists of a [Policy Management Service (PMS), a policy repository and an Authorization Decision Service (ADS)](../architecture/). The ADS can run as a standalone service or be [embedded in a Go application](../deployment-embeded/).

## Authorization and access control

Authentication establishes who a caller is; authorization decides what that caller may do. Access control is the enforcement of those decisions. Speedle handles authorization only: it receives an already-authenticated subject and returns a decision, which the calling application or gateway enforces. See [Authorization Decisions](../decisions/).

## Fine-grained access control (FGA)

Fine-grained access control decides access per user, per resource and per action, optionally taking request context into account, rather than granting broad access to a whole application. A Speedle policy names the principals, the actions and the resource, and may add a condition. A role policy can assign a role for one resource only. See [SPDL](../spdl/).

## PAP: Policy Administration Point

The PAP is where policies are created and managed. In Speedle this is the PMS, used through its [REST API](../api/management_api/), gRPC API or the `spctl` command-line tool. See [Policy Management](../pms/policy-mgmt/).

## PDP: Policy Decision Point

The PDP evaluates a request against policies and returns a decision. In Speedle this is the ADS, or the embedded evaluator when Speedle runs inside a Go process. A decision request looks like this:

```bash
curl --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"library","resource":"book","action":"update"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

With the RBAC policies below, the response is `{"allowed":true,"reason":0}`. See the [decision API](../api/decision_api/).

## PEP: Policy Enforcement Point

The PEP intercepts an access attempt, asks the PDP for a decision and enforces it. Speedle does not ship a general-purpose PEP; the PEP is the application or gateway that calls the ADS. The [Docker, Kubernetes and Istio integrations](../../integrations/) are sample PEPs.

## PIP: Policy Information Point

The PIP supplies extra information needed to evaluate a policy. In Speedle, this information comes from request attributes and [built-in attributes](../spdl/#2-2-1-built-in-attributes), from the [token asserter](../assertor/), which turns an identity token into a subject, and from [custom functions](../custom-function/), which call external REST endpoints during evaluation.

## XACML

XACML (eXtensible Access Control Markup Language) is an OASIS standard that defines both a policy language and the PAP/PDP/PEP/PIP architecture. Speedle follows the same division of roles, but its policy language is SPDL, not XACML, and it cannot import XACML policies.

## RBAC: role-based access control

In RBAC, permissions are granted to roles and roles are assigned to users or groups. Speedle uses a policy to grant permissions to a role, and a role policy to assign the role:

```text
grant role librarian read,update book
grant user alice librarian
```

`alice` may update `book` (reason 0, grant policy found); `bob`, who has no role, is denied (reason 3, no applicable policies). Roles can also be assigned only for a particular resource; see [Policy Management](../pms/policy-mgmt/) and [Global Policy](../pms/global-policy/).

## ABAC: attribute-based access control

In ABAC, the decision depends on attributes of the subject, resource, action or environment. In Speedle, any policy or role policy can carry an `if` condition over attributes:

```text
grant user bob read report if department == 'finance'
```

A request from `bob` with the string attribute `department` set to `finance` is allowed (reason 0); with `sales`, or without the attribute, it is denied (reason 3). See [Condition](../spdl/#condition).

## PBAC: policy-based access control

PBAC manages access through centrally defined policies instead of permissions embedded in each application. All Speedle policies are kept in the PMS and evaluated with two fixed rules: deny overrides grant, and a request with no applicable grant is denied.

```text
grant group staff read,delete journal
deny user carol delete journal
```

`carol`, a member of `staff`, is denied `delete` on `journal` (reason 1, deny policy found); `erin`, also in `staff`, is allowed (reason 0).

## Identity and IAM

Identity and access management (IAM) covers both proving who a user is and deciding what the user may do. Speedle is not an identity provider and does not authenticate users. It consumes identities: principals can be qualified by an [identity domain](../idd/), and identity tokens can be resolved by the [token asserter](../assertor/).

```text
grant user dave from github read code
```

`dave` from the `github` identity domain may read `code` (reason 0); `dave` from `corp`, or with no identity domain, is denied (reason 3).

## Zero trust

Zero trust grants no access on the basis of network location and checks every request. Speedle supports this as the decision service: the ADS evaluates each request it receives, and anything not explicitly granted is denied. Authentication, device trust and network controls are outside Speedle's scope.

## SPDL: Speedle Policy Definition Language

SPDL stands for Speedle Policy Definition Language. It is the language used to write Speedle policies and role policies, for example `grant role librarian read,update book`. See the [SPDL reference](../spdl/).

## Integrations

Speedle provides integration samples for [Docker](../../integrations/docker/) (a Docker authorization plugin), [Kubernetes](../../integrations/kubernetes/) (an authorization webhook) and [Istio](../../integrations/istio/) (a Mixer adapter). Other applications call the ADS through REST or gRPC, or embed the evaluator in Go; see [Deployment](../deployment/).
