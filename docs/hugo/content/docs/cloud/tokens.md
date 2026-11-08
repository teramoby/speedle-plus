+++
title = "Speedle Cloud application tokens"
linkTitle = "Application tokens"
description = "Issue, protect, list, rotate and revoke Speedle Cloud application tokens, including recent authentication and one-time disclosure rules."
date = 2026-11-08T00:00:00+08:00
lastmod = 2026-11-08T00:00:00+08:00
weight = 20
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Manage service-scoped Bearer credentials without exposing their one-time secret values."
+++

Application tokens authenticate an application to Speedle Cloud ADS for the token's own service. They do not identify an end user and cannot authorize management API requests. The token console becomes available when account sign-in opens; while authenticated ADS is pending, issuing a token does not enable server-side authorization calls.

## Issue a token

For an account that already owns a service, open [Application tokens](https://cloud.speedle.io/tokens). Enter a token name of 1–128 characters and choose **Create token**. If verification is required, choose an available linked provider. After returning from the provider, enter the name and choose **Create token** again; returning does not create a token automatically.

The console sends this session request:

```http
POST /api/tokens
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"name":"books-app"}
```

This example uses the production origin. The client supplies the session through its existing secure cookie handling, so this is not a standalone anonymous request.

Creation requires normal provider authentication, through sign-in or reauthentication, no more than 10 minutes ago. The server checks the persisted session against its own clock. Exactly 10 minutes is allowed. A missing, future or stale authentication timestamp returns `403 {"error":"reauth_required"}`. Ordinary account activity does not refresh authentication recency.

The request accepts only `name`. Service ID, scopes, expiry and authentication proofs are not client inputs.

## Protect the one-time secret

A successful creation returns 201 with token metadata and a one-time `token` field in this format:

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

This is a format description, not a usable credential. The ID embedded in the token equals the returned token ID. Store the issued value immediately in the application's server-side secret store.

Do not embed the token in frontend code or persist it in browser storage. Keep it out of URLs, source control, screenshots, issues, telemetry and logs. The normal management client may hold and display a newly issued value transiently for one-time, user-directed disclosure and copying. It must not automatically copy, download, store or log the secret.

Token listing returns metadata only; plaintext cannot be retrieved later. Tokens have no intrinsic expiry in v1. At most two tokens may be active for a service, and revoked history does not consume either active slot.

## List token metadata

`GET /api/tokens` returns active and revoked metadata in bounded pages, ordered by ID descending. The default page size is 25; `limit` may be 1–100. The cursor is an exclusive token ID. Follow `nextCursor` until it is `null`. The response never returns the secret.

Use **Refresh newest tokens** and **Next page** to review metadata. `lastUsedAt` is best-effort and updates at most hourly, so it cannot prove that a caller has stopped using a token.

## Rotate and revoke a token

Once authenticated ADS is available, use normal recent authentication to create a second token. Switch every application and secret reference to the new credential, and confirm that it works for the intended policy before retiring the old one.

In **Application tokens**, choose **Revoke** for the old token, check its name and ID, and confirm **Revoke token**. The management session sends:

```http
DELETE /api/tokens/<old token id>
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{}
```

The first revocation of an owned active token returns an empty 204. Unknown, unowned and already-revoked IDs return 404. Cached verification authority has a fixed maximum lifetime of 60 seconds from the start of its authoritative lookup, so revocation is not guaranteed to take effect everywhere immediately.

If both active slots are occupied, inventory and switch legitimate callers before retiring a credential. Do not revoke a working production token blindly just to free a slot.

If a creation or revocation response is lost, review refreshed metadata before explicitly choosing another action. The console does not retry mutations automatically. A matching name cannot recover or identify a lost one-time secret.

For request limits and management error handling, see [Limits and errors](/docs/cloud/limits-and-errors/). For using a token in an authorization request, see the [Authorization API preview](/docs/cloud/authorization-api/).
