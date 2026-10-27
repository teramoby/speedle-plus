+++
title = "ABAC: attribute-based access control"
description = "Implement attribute-based access control (ABAC) in Speedle+ with SPDL conditions, typed attributes, trusted request context and an executable clearance example."
weight = 20
keywords = ["ABAC", "attribute-based access control", "authorization", "access control", "Speedle+"]
bref = "Use trusted attributes to express conditional authorization"
+++

## How ABAC works in Speedle

Attribute-based access control (ABAC) uses properties of the subject, resource or request environment to determine access. In Speedle, SPDL conditions evaluate runtime attributes alongside the principal, action and resource. An application or trusted integration must collect those attributes and give them consistent names and types.

For example, a role may establish that Alice is a reader, while a clearance attribute determines whether she may read a particular class of information. See [RBAC](/docs/access-control/rbac/) for the role assignment part.

## Try a clearance condition

Paste this policy into the [Playground](/playground/):

```text
grant user alice read book if clearance >= 3
```

Use user `alice`, resource `book`, action `read`. Add an attribute named `clearance`, select the numeric type, and set its value to `4`: access is allowed. Change the value to `2`: access is denied. Keeping `4` as a string is not equivalent to supplying a number; use the type expected by the condition.

The equivalent Playground request is:

```json
{
  "subject": {"principals": [{"type": "user", "name": "alice"}]},
  "resource": "book",
  "action": "read",
  "attributes": [{"name": "clearance", "type": "numeric", "value": 4}]
}
```

The Playground uses a typed attribute array. Use the [decision API reference](/docs/api/decision_api/) when constructing requests for a deployed ADS; the browser adapter is not a replacement for that API contract.

## Make attributes trustworthy

Derive clearance and tenant membership from trusted services rather than accepting arbitrary client values. Map resource classification into the request explicitly; Speedle does not automatically fetch application database fields. Use an [identity domain](/docs/idd/) to distinguish identical principal names from different domains.

A missing attribute can cause an evaluation error. Treat errors separately from a successful deny response, and enforce a fail-closed result in the application. Test absent values, wrong types and boundary values as well as the allowed case. External custom functions require server-side integration and are not available in the browser Playground.

Continue with [SPDL conditions](/docs/spdl/), [authorization decisions](/docs/decisions/) and [identity/IAM integration](/docs/access-control/identity-iam/).
