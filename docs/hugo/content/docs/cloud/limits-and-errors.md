+++
title = "Speedle Cloud Limits and errors"
linkTitle = "Limits and errors"
description = "Account limits, error codes, reset times and retry guidance."
weight = 40
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

Limits apply to the whole account unless a row says otherwise. API keys are account credentials; changing keys does not increase the daily quota.

## Limits

| What is limited | Limit | When reached / how to recover |
| --- | --- | --- |
| Services per account | 100 | `409 quota_exceeded`; delete an unused service. No timed reset. |
| Authorization and role policies combined, across all services | 200 per account | `409 quota_exceeded`; delete policies. No timed reset. |
| Complete SPDL source for one policy | 1,024 UTF-8 bytes | `400 invalid_request` on console policy writes; shorten the source. This counts bytes, not characters. No timed reset. |
| Active API keys, all scopes combined | 10 per account | `409 quota_exceeded` with `limit: 10`; revoke an unused key. No timed reset; revoked history does not count. |
| Policy name / API key name | 128 Unicode code points / 1–128 characters after trimming | `400 invalid_request`; shorten or correct the name. No timed reset. |
| Billable policy-management and authorization calls | 10,000 per account per UTC day | `429 daily_quota_exceeded`; resets at 00:00 UTC. Follow `Retry-After`. |
| Short bursts of API-key requests | Approximately 600 per minute per key at each Cloud location | `429 rate_limited` with `Retry-After: 60`; wait before retrying. This is a short-term limit, not a guaranteed throughput or a separate daily allowance. |
| Ordinary request body | 65,536 bytes (64 KiB) | `413 request_too_large`; send a smaller body. No timed reset. |
| Atomic JSON import body (`POST /api/service/import` only) | 5,100,000 bytes | `413 request_too_large`; reduce the import. The policy and account limits still apply. No timed reset. |
| Incoming attributes | 50 attributes; 16,384 UTF-8 bytes of canonical JSON | `400 SPDL-0004`; send fewer or smaller attributes. No timed reset. |

For scopes, creation and revocation, see [API keys](/docs/cloud/tokens/).

## Daily quota

Reads, writes, validation, Playground tests, imports, exports and authorization checks share the daily allowance. A policy denial or invalid business input still counts. Authentication, session operations and `GET /api/me` do not count. Requests rejected by authentication, scope checks, routing, CSRF, body-size checks, burst limits or the daily quota do not count.

Cloud-side failures may receive a quota refund, up to 1,000 per account per UTC day; if a refund cannot be confirmed, the call stays charged. Errors caused by the request still count. During a temporary quota-counter failure, authorization calls may continue at a reduced rate within an additional allowance of up to 10,000 calls per account per UTC day. That allowance is separate from the normal quota, is not guaranteed, and resets at 00:00 UTC along with the refund allowance. If it cannot be granted, the response is `503 quota_unavailable`.

Some daily capacity can be reserved but unused, so the usable total can be below 10,000. `GET /api/me` reports usage and the reset time. Do not retry continuously or switch keys to bypass account limits.

## Errors and response headers

Errors are flat JSON, for example `{"error":"rate_limited"}`. An error is not an authorization grant; fail closed if a valid decision cannot be obtained.

| Status and error | What to do |
| --- | --- |
| `400 invalid_request` or a Speedle error message | Correct the request; do not retry it unchanged. |
| `401 unauthorized` | Check the API key or signed-in session. A session cookie alone cannot authorize ADS. |
| `403 reauth_required` | Authenticate with a linked provider again before creating a key. |
| `403 insufficient_scope` | Use a key with the required scope. |
| `403 management_scope_disabled` | Management-scope issuance is unavailable while sign-in is closed. |
| `403 csrf_failed` | Use the console session with the required origin and CSRF header. |
| `404 service_not_found` (ADS) / `404 not_found` (management) | Select an existing service in your account. Unknown, other-account or already-revoked key IDs also return `404 not_found`. |
| `409 quota_exceeded` | Free the relevant service, policy or API-key slot; time does not reset these caps. |
| `413 request_too_large` | Reduce the body size. |
| `429 rate_limited` / `429 daily_quota_exceeded` | Respect `Retry-After` and use bounded backoff; the daily limit resets at UTC midnight. |
| `503 quota_unavailable` / `503 snapshot_unavailable` / `500 internal_error` | Fail closed and retry with bounded backoff, respecting `Retry-After` when present. |

For verified API-key requests, `X-RateLimit-Limit` is 600. There is no exact `X-RateLimit-Remaining`. `X-Quota-Daily-Remaining`, when present, is an approximate account-wide balance; concurrent calls and unused reservations can make it differ from what you can actually use. It does not guarantee that the next request will succeed, and early rejections omit it.

ADS responses use `Cache-Control: no-store`. Keep decisions and credentials out of shared caches. Use POST; other methods can receive `405 method_not_allowed` or an earlier non-JSON block response. See the [Authorization API](/docs/cloud/authorization-api/).
