+++
title = "Speedle Cloud Limits and errors"
linkTitle = "Limits and errors"
description = "Workspace limits, error codes, reset times and retry guidance."
weight = 40
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-21T00:00:00+08:00
+++

**Speedle Cloud is in public beta.** This guide describes the workflow for the live service.

Limits apply to the whole workspace unless a row says otherwise. API keys are workspace credentials; changing keys in the same workspace does not increase the daily quota.

## Limits

| What is limited | Limit | When reached / how to recover |
| --- | --- | --- |
| Services per workspace | 100 | `409 quota_exceeded`; delete an unused service. No timed reset. |
| Authorization and role policies combined, across all services | 200 per workspace | `409 quota_exceeded`; delete policies. No timed reset. |
| Complete SPDL source for one policy | 1,024 UTF-8 bytes | `400 invalid_request` on console policy writes; shorten the source. This counts bytes, not characters. No timed reset. |
| Active API keys, all scopes combined | 10 per workspace | `409 quota_exceeded` with `limit: 10`; revoke an unused key or let it expire. The cap does not reset on a schedule. Revoked and expired keys do not count. |
| Policy name / API key name | 128 Unicode code points / 1–128 characters after trimming | `400 invalid_request`; shorten or correct the name. No timed reset. |
| Billable policy-management and authorization calls | 10,000 per workspace per UTC day | `429 daily_quota_exceeded`; resets at 00:00 UTC. Follow `Retry-After`. |
| Short bursts of API-key requests | Approximately 600 per minute per key at each Cloud location | `429 rate_limited` with `Retry-After: 60`; wait before retrying. This is a short-term limit, not a guaranteed throughput or a separate daily allowance. |
| Ordinary request body | 65,536 bytes (64 KiB) | `413 request_too_large`; send a smaller body. No timed reset. |
| Atomic JSON import body (`POST /api/workspaces/<workspaceId>/services/<serviceId>/import` only) | 5,100,000 bytes | `413 request_too_large`; reduce the import. The policy and workspace limits still apply. No timed reset. |
| Incoming attributes | 50 attributes; 16,384 UTF-8 bytes of canonical JSON | `400 SPDL-0004`; send fewer or smaller attributes. No timed reset. |

For scopes, expiry, creation and revocation, see [API keys](/docs/cloud/tokens/).

## Daily quota

Reads, writes, validation, Playground tests, imports, exports and authorization checks share the daily allowance. A policy denial or invalid business input still counts. Authentication, session operations, `GET /api/me` and `GET /api/workspaces/<workspaceId>/usage` do not count. Requests rejected by authentication, scope checks, routing, CSRF, body-size checks, burst limits or the daily quota do not count.

Cloud-side failures may receive a quota refund, up to 1,000 per workspace per UTC day; if a refund cannot be confirmed, the call stays charged. Errors caused by the request still count. During a temporary quota-counter failure, authorization calls may continue at a reduced rate within an additional allowance of up to 10,000 calls per workspace per UTC day. That allowance is separate from the normal quota, is not guaranteed, and resets at 00:00 UTC along with the refund allowance. If it cannot be granted, the response is `503 quota_unavailable`.

Some daily capacity can be reserved but unused, so the usable total can be below 10,000. `GET /api/workspaces/<workspaceId>/usage` reports the workspace's usage and reset time (`GET /api/me` reports the personal workspace). Do not retry continuously or switch keys to bypass workspace limits.

## Errors and response headers

Errors are flat JSON, for example `{"error":"rate_limited"}`. An error is not an authorization grant; fail closed if a valid decision cannot be obtained.

| Status and error | What to do |
| --- | --- |
| `400 invalid_request` or a Speedle error message | Correct the request; do not retry it unchanged. |
| `401 unauthorized` | Check the API key or signed-in session. A session cookie alone cannot authorize ADS. |
| `401 api_key_expired` | The API key is at or past its expiry. Create a replacement; retrying this key will not succeed. Keys created before an expiry was required have one too. The API keys page states when; read `expiresAt` and replace the key before then. |
| `403 reauth_required` | Authenticate with a linked provider again before creating a key. |
| `403 insufficient_scope` | Use a key with the required scope. |
| `403 csrf_failed` | Use the console session with the required origin and CSRF header. |
| `404 service_not_found` (ADS) / `404 not_found` (management) | Select an existing service in the matching workspace. Unknown, other-workspace or already-revoked key IDs also return `404 not_found`. |
| `409 quota_exceeded` | Free a service or policy slot by deleting an unused item; time does not reset those caps. Free an API-key slot by revoking the key or letting it expire. |
| `409 revision_exhausted` | The service's revision reached its maximum; it stays readable and exportable. [Follow the recovery steps](#revision-exhausted). |
| `413 request_too_large` | Reduce the body size. |
| `429 rate_limited` / `429 daily_quota_exceeded` | Respect `Retry-After` and use bounded backoff; the daily limit resets at UTC midnight. |
| `503 quota_unavailable` / `503 snapshot_unavailable` / `500 internal_error` | Fail closed and retry with bounded backoff, respecting `Retry-After` when present. |

For verified API-key requests, `X-RateLimit-Limit` is 600. There is no exact `X-RateLimit-Remaining`. `X-Quota-Daily-Remaining`, when present, is an approximate balance for the API key's workspace; concurrent calls and unused reservations can make it differ from what you can actually use. It does not guarantee that the next request will succeed, and early rejections omit it.

ADS responses use `Cache-Control: no-store`. Keep decisions and credentials out of shared caches. Use POST; other methods can receive `405 method_not_allowed` or an earlier non-JSON block response. See the [Authorization API](/docs/cloud/authorization-api/).

<a id="revision-exhausted"></a>
## `409 revision_exhausted`

The service's revision has reached its maximum. The service stays readable and exportable.

To recover:

1. Export its policies.
2. Create a new service.
3. Import the export into the new service.
4. Switch callers to the new service name.
5. Optionally, delete the old service afterwards.

Workspace service and policy limits still apply. Before steps 2 and 3, make sure the workspace has a free service slot and enough policy slots for the import; delete unused services or policies if necessary. If no other capacity can be freed, confirm the export, delete the old service, and then perform steps 2–4. Deleting the old service before the replacement is ready interrupts access for its callers.
