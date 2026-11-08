+++
title = "Speedle Cloud limits and errors"
linkTitle = "Limits and errors"
description = "Speedle Cloud account quotas, request limits, flat JSON errors, rate and quota headers, daily accounting and retry guidance."
date = 2026-11-08T00:00:00+08:00
lastmod = 2026-11-08T00:00:00+08:00
weight = 40
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Plan around account-wide limits and fail closed when Cloud cannot return a valid authorization decision."
+++

The ADS errors and headers below describe the authenticated ADS contract, whose runtime is not yet available. The management contract already defines account limits, session and CSRF checks, and token-management errors for use when account access is available. Downloading a schema does not enable a pending endpoint.

## Limits

| Limit | Meaning |
| --- | --- |
| 10,000 calls per day | Account-wide matched PMS and ADS handler calls per UTC day, across sessions, tokens and services; resets at 00:00 UTC. |
| 200 policies | Authorization and role policies combined per account. |
| 4 KiB of SPDL | 4,096 UTF-8 bytes per complete policy, not 4,096 characters. |
| 2 active tokens | Per service; revoked history does not consume this cap. |
| 600 requests per minute per token | Approximate per-location burst limit, separate from the strict daily account cap. |
| 64 KiB request body | Global bound before parsing. Optional incoming attributes also retain their 50-attribute and 16,384-byte canonical UTF-8 JSON bound. |

Each account has one service. Token names are 1–128 characters. See [Application tokens](/docs/cloud/tokens/) for credential lifecycle rules.

## Daily quota accounting

Entering a matched handler spends one daily unit even for a policy denial, invalid business input or internal error. Authentication and session endpoints do not spend these units. Requests rejected before handler entry by authentication, routing, CSRF, body-size, burst-limit or quota-reservation checks do not spend them either.

Reserved leases that expire or are abandoned are not reissued, so the number of usable calls can be fewer than 10,000. `GET /api/me` reports reserved daily usage and the reset time without charging a daily unit.

Switching tokens does not add account quota. The daily account cap and the approximate per-location token burst cap are enforced separately.

## HTTP errors

Errors use flat JSON responses. Correct the request or credential where indicated; otherwise fail closed and use bounded backoff rather than an unbounded retry loop.

| HTTP status and error | Application response |
| --- | --- |
| `401 {"error":"unauthorized"}` | Check the stage and credential. ADS includes a Bearer challenge. Anonymous and cookie-only requests cannot authorize ADS. |
| `403 {"error":"reauth_required"}` | Authenticate again with a linked provider before creating a token. Ordinary activity does not refresh authentication recency. |
| `403 {"error":"service_mismatch"}` | Correct the service input; never retry against another tenant's data. |
| `400 {"error":"SPDL-0004 …"}` | Speedle rejected the request, for example because an attribute value has the wrong type. The message is the Speedle message. Correct the request; do not retry it unchanged. |
| `404` for token revocation | The token ID is unknown, unowned or already revoked. Refresh metadata before choosing another action. |
| `413 {"error":"request_too_large"}` | Reduce the request body. |
| `429 {"error":"rate_limited"}` | Honor `Retry-After`, which is 60 seconds for this burst limit, and add bounded backoff. |
| `429 {"error":"daily_quota_exceeded"}` | Honor `Retry-After` until the next UTC midnight. Switching tokens does not add account quota. |
| `503 {"error":"quota_unavailable"}` or `snapshot_unavailable` | Fail closed and retry only with bounded backoff. Do not fall back to a demo or D1 policy evaluation. |
| `500 {"error":"internal_error"}` | Fail closed. Retain a safe request ID for support and avoid logging credentials. |

For `503` responses, `snapshot_unavailable` is charged after handler entry, while `quota_unavailable` is not charged. Early rejections omit the daily-remaining header described below.

A non-POST ADS request may instead be stopped at the public edge with Cloudflare's 403 response. That is not an application authorization decision.

## Rate and quota headers

After verification, `X-RateLimit-Limit` is 600. ADS does not expose an exact `X-RateLimit-Remaining` value.

When present after a charged handler, `X-Quota-Daily-Remaining` estimates the account-wide calls remaining today: the unreserved calls at lease grant plus this isolate's remaining lease. The estimate is conservative at grant time. It is cached for at most the valid 30-second lease and discarded when the lease expires or the UTC day rolls over.

Concurrent isolates can make the value overstate availability by up to the unreserved balance at grant time, not merely 32 calls. Unused or abandoned reservations can make it understate availability. During rollout or rollback, legacy allocators fall back to the local lease remaining. This header is informational and does not change quota enforcement. Early rejections omit it.

Matched application ADS responses also use `Cache-Control: no-store`. Do not place decisions or credentials in shared caches.

Return to the [Authorization API preview](/docs/cloud/authorization-api/) for request examples and integration constraints.
