+++
title = "Global roles"
linkTitle = "Global roles"
description = "Assign a role once for every service in a Speedle Cloud workspace and grant its permissions in each service."
date = 2026-11-17T00:00:00+08:00
lastmod = 2026-11-21T00:00:00+08:00
weight = 25
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "One role assignment for every service in a workspace."
+++

**Speedle Cloud is in public beta.** This guide describes the workflow for the live service.

A role assignment usually belongs to one service. When several services in the same workspace share the same assignment, define it once in the workspace's **Global roles** service and use the role in each service's own authorization policies.

Only role policies are supported there. A global role assignment does not grant any operation by itself: every service still defines the permissions it allows. Global roles belong to one workspace and never cross workspaces.

## Create the role assignment once

In the console service picker, choose **Global roles** at the top of the list. Create a role policy:

```text
grant user Emma from corp AdminRole
```

The `from corp` clause limits the assignment to the `corp` identity domain. Without it, a grant matches every user named Emma in every domain; see [Identity domains](/docs/idd/).

The role `AdminRole` is now assigned to Emma in every service of the workspace.

## Grant the permission in a service

Switch to a service that needs the permission, for example `library`, and create an authorization policy:

```text
grant role AdminRole borrow books
```

Repeat this in each service that should allow the operation. A service without its own authorization policy grants nothing, even when the role is assigned there.

## Check the result

Open the Playground for `library` and run a test with `[{"type":"user","name":"Emma","idd":"corp"}]` as the principal, `books` as the resource and `borrow` as the action. The request is allowed through `AdminRole`; the same request from `Bob` is denied.

The Global roles Playground is different: a check against the global service itself always denies. Global roles take effect in each service's Playground.

## Behaviour and limits

- Global roles accept role policies only. An authorization policy in the global service returns `400 {"error":"global_authz_unsupported"}`.
- The global service counts as one of the workspace's services, and its role policies count toward the combined workspace policy limit. See [Limits and errors](/docs/cloud/limits-and-errors/).
- The console names the special service **Global roles**; it cannot be created by hand, renamed or deleted.
- Every Speedle Cloud ADS instance of the workspace always receives the global role policies, even when it synchronizes a selected set of services.
- Changes propagate like other policies: about a minute through published snapshots, and immediately in the Playground, which reads the current state.
