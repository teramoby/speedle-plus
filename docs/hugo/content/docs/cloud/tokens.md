+++
title = "Speedle Cloud API keys"
linkTitle = "API keys"
description = "Create, protect, list and revoke account API keys and choose a fixed scope and expiry."
weight = 20
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

API keys belong to your account and can access its services according to their scope. They authenticate your application, not its end users. Managing API keys, sessions and linked identities always requires a signed-in console session.

**Speedle Cloud is coming soon.** This guide describes the workflow available when production sign-in and authenticated authorization checks open; the examples have not been run against a live Cloud environment.

## Create an API key

Open [API keys](https://cloud.speedle.io/tokens), enter a name of 1–128 characters after trimming whitespace, select a scope and an expiry (`7d`, `30d`, `90d`, `180d` or `365d`; the console defaults to 3 months), and choose **Create**. Creating a key requires authentication with a linked provider within the last 10 minutes. If you receive `403 reauth_required`, authenticate again, then submit **Create** again; returning from the provider does not create a key automatically.

Each key has one scope, fixed at creation:

| Scope | Access |
| --- | --- |
| `ads` (default) | Authorization checks through `/authz-check/v1/*` only. |
| `pms:read` | Read account services, policies and role policies through `/policy-mgmt/v1`. |
| `pms:write` | Read, create and delete account services, policies and role policies through `/policy-mgmt/v1`. |

Management scopes cannot call ADS; `ads` cannot call policy management. Using the wrong scope returns `403 insufficient_scope`. Requesting a management scope while sign-in is closed returns `403 management_scope_disabled`.

The console sends a session-authenticated request:

```http
POST /api/tokens
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"name":"books-app","scope":"ads","expiresIn":"90d"}
```

The request accepts `name`, required `expiresIn` and optional `scope` (default `ads`). `expiresIn` is one of `7d`, `30d`, `90d`, `180d` or `365d` (1 week, 1 month, 3 months, 6 months or 1 year). Each day is 24 hours. In the console, **Expires after** defaults to 3 months and **Preview expiry (UTC)** shows the resulting time before you submit. There is no non-expiring key. Do not send a service selector, `expiresAt` or authentication proofs. A missing or unknown `expiresIn`, or a client-supplied `expiresAt`, returns `400 invalid_request`. A body that contains only `name` and `scope` returns that 400. The server does not supply `expiresIn`. Cookie authentication and CSRF headers are required; an API key cannot replace the session.

## Protect the secret

Creation returns HTTP 201. The response's `token` field contains the API key secret and is shown only once:

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

Every key expires at `expiresAt`, which the server computes and returns in Unix milliseconds. At or after that instant, authorization and policy-management calls return `401 {"error":"api_key_expired"}`. Create a replacement key. Retrying the expired key does not succeed. You can revoke a key before it expires. Keys that already existed when an expiry became required have one too. It is 365 days of 24 hours after the key was created, unless that time had already passed, in which case it is 30 days of 24 hours after the requirement took effect. Every listed key includes `expiresAt`.

This describes the format, not a usable key. Save the value immediately in a server-side secret store. Never put it in browser storage, frontend code, URLs, source control, screenshots or logs. Listing keys returns metadata only; a lost secret cannot be recovered.

Your account can have **10 active API keys across all scopes**. Creating another returns `409 {"error":"quota_exceeded","limit":10}`. Revoke an unused key to free a slot; the limit does not reset with time. Revoked and expired keys remain listed and do not count toward the cap.

## List API keys

`GET /api/tokens` lists active, expired and revoked keys, newest first. The default page size is 25; `limit` accepts 1–100. Pass the last response's `nextCursor` as `cursor` until `nextCursor` is `null`. Each key includes `expiresAt` in Unix milliseconds. The console shows **Expires (UTC)**. A key that expires within 7 days is marked **Expires soon**. Expired keys appear in an **Expired** group. `lastUsedAt` is best-effort, normally updated at most hourly; it does not prove that a key is unused.

## Revoke an API key

Choose **Revoke**, check the key name and ID, and confirm. Revocation does not require recent reauthentication. The console sends:

```http
DELETE /api/tokens/<API key id>
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{}
```

The first revocation returns an empty HTTP 204. Unknown, other-account or already-revoked IDs return `404 not_found`. Previously verified requests may continue using a key for up to 60 seconds; revocation is not instantaneous everywhere.

The available actions are **Create** and **Revoke**. To change a secret, scope or expiry, create another key, update callers and revoke the old key. If all active keys are in use at the cap, plan for interruption before revoking one. Deleting a service does not revoke account API keys.

If a creation or revocation response is lost, refresh the list before acting again. A matching name cannot recover a lost secret. See [Limits and errors](/docs/cloud/limits-and-errors/) and the [Authorization API](/docs/cloud/authorization-api/).
