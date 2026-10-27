+++
title = "RBAC: role-based access control"
description = "Implement role-based access control (RBAC) with Speedle+: role assignments, resource-scoped roles, role inheritance and executable SPDL examples."
weight = 10
keywords = ["RBAC", "role-based access control", "authorization", "access control", "Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization"]
bref = "Grant permissions through roles instead of repeating grants for every user"
+++

## How RBAC works in Speedle

Role-based access control (RBAC) separates role assignments from permissions. A Speedle role policy assigns a role to a user, group or another role. An authorization policy grants a role permission to perform an action on a resource. The evaluator resolves the applicable roles when answering the request.

This is useful when many people share a job function: changing a reader's permissions updates one authorization policy rather than every user's grants.

## Try a resource-scoped role

Paste both lines into the [Playground](/playground/):

```text
grant user alice reader on book
grant role reader read book
```

Use user `alice`, resource `book`, action `read`. Access is allowed because Alice receives the `reader` role on that resource and the role has the `read` permission. User `bob` with the same resource and action is denied. Alice with action `delete` is also denied because the role grants no delete permission.

The `on book` scope matters: it limits this role assignment to `book`. Removing that scope changes where the role applies. A principal named `reader` is not automatically a role assignment; use an explicit role policy.

## Role inheritance and explicit deny

Role policies can assign one role to another. For example, replace the policies above with:

```text
grant user alice editor on book
grant role editor reader on book
grant role reader read book
```

The same `alice` / `book` / `read` request is allowed through the role chain. Keep role hierarchies understandable and test their effective permissions. An applicable explicit deny can override a grant; role membership alone is not a guarantee of access.

When the decision also depends on a department, clearance or resource classification, combine RBAC with [ABAC conditions](/docs/access-control/abac/). See [policy-based access control](/docs/access-control/pbac/) for management and enforcement responsibilities, and [authorization decisions](/docs/decisions/) for the role-query APIs.
