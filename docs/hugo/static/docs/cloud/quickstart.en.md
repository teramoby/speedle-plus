# Speedle Cloud documentation

Speedle Cloud is a hosted access-control service for developers, built on the open-source Speedle policy engine. It is designed to let you manage SPDL authorization and role policies, test decisions in a Playground, and call an authorization API from your application server without operating your own authorization service.

**Speedle Cloud is coming soon.** The Cloud header displays a **Coming soon** badge, and production sign-in is not open on [cloud.speedle.io](https://cloud.speedle.io/) yet. The console workflow and authenticated Authorization Decision Service (ADS) described here are previews of the published service contract. Issuing a token will not enable server-side authorization calls while authenticated ADS remains pending. Verify deployment and identity-provider availability in your chosen stage.

The free public beta is intended for trials, learning and evaluation. It has no SLA, is not recommended for production use, and its features and limits may change.

## Documentation

- [Getting started](https://engine.speedle.io/docs/cloud/getting-started/): Sign in with GitHub or Google (opens soon), account management, services, SPDL policies, session management requests and all four session Playground operations.
- [Application tokens](https://engine.speedle.io/docs/cloud/tokens/): recent-authentication requirements, one-time token disclosure, metadata pagination, rotation and revocation.
- [Authorization API preview](https://engine.speedle.io/docs/cloud/authorization-api/): the IsAllowed request and response contract, a Node.js 22 example, policy snapshots and integration constraints.
- [Limits and errors](https://engine.speedle.io/docs/cloud/limits-and-errors/): account quotas, request limits, error responses, rate and quota headers, and retry guidance.

## Downloads

- [English Markdown](https://engine.speedle.io/docs/cloud/quickstart.en.md): all four Cloud chapters in one offline document.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): the canonical checked-in Cloud API specification.
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): generated from the same YAML source bytes.
- [Serving build version](https://cloud.speedle.io/build.json): identifies the serving build; it does not certify identity-provider or endpoint availability.

The schemas include the four session Playground operations and the authenticated ADS contract. Publishing a schema or downloading a guide does not enable a pending endpoint or open sign-in. The YAML and generated JSON are versioned, promoted and restored with the corresponding sealed Cloud build. The English and Chinese Markdown downloads are maintained with these Engine documentation pages.

Read the Cloud [Terms](https://cloud.speedle.io/terms) and [Privacy Policy](https://cloud.speedle.io/privacy).

## Getting started with Speedle Cloud

**Speedle Cloud is coming soon.** Production sign-in is not open on cloud.speedle.io yet, and the Cloud header shows **Coming soon**. When sign-in is closed, the sign-in page says that it opens soon. This guide describes the published workflow rather than a currently open production console; screenshots will be added after production sign-in opens.

### Production origin

Use `https://cloud.speedle.io` for account sessions, management requests and application tokens. Accounts, resources and tokens belong to that origin. The examples in this section are illustrative and were not executed against a live environment. Confirm that your chosen stage is deployed and that the identity provider you intend to use is available there.

### Sign in with GitHub or Google (opens soon)

The sign-in page shows **Sign in with GitHub** and **Sign in with Google** even while sign-in is disabled. When production sign-in opens, choose an available provider. A new identity whose verified email address is unused requires an explicit **Create account** confirmation. If the email address is already in use, sign in with the linked provider identified by the console, then connect the second identity in **Account settings**. Accounts are not merged automatically.

Account settings let you inspect connected identities and connect or disconnect an available provider; keep at least one connected identity. **Sign out** is in the header beside the language switch, and the Account menu offers **Sign out everywhere**. Successful sign-out returns to the sign-in page and clears the workspace and any one-time token display. The audit log shows account activity.

Deleting an account requires recent provider authentication and typing `DELETE`. It permanently removes the account's services, policies, tokens and identities and ends its sessions. Export any policies you need from the Service workspace before deletion.

### Create a service and a policy

For a new account, open the [Service workspace](https://cloud.speedle.io/service), enter `books` as the service name, and choose **Create service**. Each account has one service. If the account already has a service or policies, inspect them and use an appropriate existing configuration; do not overwrite or delete existing data to make this example work.

Choose **New policy**, enter `read-books` as the policy name, and put this authorization policy in the SPDL editor:

```text
grant user alice get /books
```

Because this grant has no `from` clause, it matches a user named `alice` in every identity domain. If your application accepts more than one identity source, write `grant user alice from <identity-domain> get /books` so the grant stays inside one domain; see [identity domain](https://engine.speedle.io/docs/idd/) for the rule and the matching request field.

Choose **Validate policy** and check that the console reports **Detected: authorization policy**, then choose **Save policy**. Validation detects the policy type and checks syntax; it does not save the draft. Saving is a separate, manual action. You can also import existing policies in the workspace instead of entering them one by one.

Speedle Cloud supports authorization policies and role policies; custom external functions are not supported. The policy list groups the two types and shows their counts. Role assignments use these forms:

- Global role: `grant user alice editor`
- Resource-scoped role: `grant user alice editor on /books`

The ambiguous statement `grant user alice role editor` is rejected. Omit the optional `role` prefix for a role assignment, or write `grant (user alice) role editor` for an authorization policy. Editing retains the stored policy type. To change the type, delete the policy and create a new one.

If the console reports a conflict or an uncertain response, keep your draft and refresh the saved data before choosing another mutation. Changing language preserves drafts. Monaco's built-in menus keep the language selected when the editor first loaded and change after a page reload.

Service deletion requires you to type the service name, complete recent provider authentication, and confirm again after verification.

### Send management session requests

The Policy Management Service (PMS) uses the account session. Let a supported same-origin client manage its secure session cookie; do not extract cookies or use a test-provider bootstrap. Mutating requests require the exact stage `Origin`, `X-SC-CSRF: 1`, and a JSON content type. An ADS Bearer token cannot authorize PMS. Clients must not send account or service ownership selectors, or authentication timestamps.

The equivalent requests for the service and policy example are:

| Action | Session request | Success |
| --- | --- | --- |
| Create the account's service | `POST /api/service` with `{"name":"books"}` | 201 |
| Add an authorization policy | `POST /api/service/policies` with the body below | 201 |

```json
{"name":"read-books","spdl":"grant user alice get /books"}
```

Paste only the `spdl` value into the editor when using the console. The JSON body is for the management API.

### Validate and export policies

Validate SPDL with `POST /api/service/validate` and a `{spdl}` body to detect its type. The legacy `{kind: "authz", spdl}` form still selects the authorization parser. **Download SPDL** and **Download JSON** call `GET /api/service/export?format=spdl` and `GET /api/service/export?format=json`, respectively.

The session Playground evaluates current account-owned D1 data. The future authenticated ADS release evaluates published policy snapshots, whose propagation takes approximately 60 seconds. A successful Playground result therefore does not prove that every ADS location has received the new snapshot.

### Test saved policies in the Playground

In the Service workspace, scroll to **Playground** and select **Is allowed**. Enter `[{"type":"user","name":"alice"}]` in **Principals (JSON)**, `/books` in **Resource**, `get` in **Action**, and `[]` in **Attributes (JSON)**, then choose **Run test**.

The other operations are **Diagnose**, **All granted roles**, and **All granted permissions**. Playground runs use saved policies, not an unsaved editor draft. Changing the input, operation or language does not automatically send a request. Run the test again after changing inputs or saved policies. The console marks the earlier result as stale and keeps it visible if a later request fails.

### Use the session Playground API

`POST /api/service/test` implements all four operations with the same Origin/CSRF rules and daily account quota as other management requests. It reads a consistent snapshot of the current account's D1 policies. It does not publish or save policies and does not accept an account or service ID. The displayed revision is the workspace revision known when the request was submitted, not proof of the server-side snapshot revision that was evaluated.

```http
POST /api/service/test
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"op":"isAllowed","request":{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3}]}}
```

| `request.op` | Success body | Published console and session API contract |
| --- | --- | --- |
| `isAllowed` | Speedle IsAllowedResponse: `allowed`, `reason`, optional `errorMessage` | Included |
| `diagnose` | Speedle EvaluationDebugResponse: reason name, `requestContext`, `attributes`, `policies` | Included |
| `allGrantedRoles` | `string[]`; `[]` when none | Included |
| `allGrantedPermissions` | `{resource, actions}[]`; `[]` when none | Included |

Success bodies are the plain upstream values, without an `op` or `result` envelope. Both all-granted results may be `null` or an empty array; neither shape grants authorization. ADS has no public Diagnose endpoint.

Next, read about [application tokens](https://engine.speedle.io/docs/cloud/tokens/), the [authorization API preview](https://engine.speedle.io/docs/cloud/authorization-api/), and [limits and errors](https://engine.speedle.io/docs/cloud/limits-and-errors/).

## Speedle Cloud application tokens

Application tokens authenticate an application to Speedle Cloud ADS for the token's own service. They do not identify an end user and cannot authorize management API requests. The token console becomes available when account sign-in opens; while authenticated ADS is pending, issuing a token does not enable server-side authorization calls.

### Issue a token

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

### Protect the one-time secret

A successful creation returns 201 with token metadata and a one-time `token` field in this format:

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

This is a format description, not a usable credential. The ID embedded in the token equals the returned token ID. Store the issued value immediately in the application's server-side secret store.

Do not embed the token in frontend code or persist it in browser storage. Keep it out of URLs, source control, screenshots, issues, telemetry and logs. The normal management client may hold and display a newly issued value transiently for one-time, user-directed disclosure and copying. It must not automatically copy, download, store or log the secret.

Token listing returns metadata only; plaintext cannot be retrieved later. Tokens have no intrinsic expiry in v1. At most two tokens may be active for a service, and revoked history does not consume either active slot.

### List token metadata

`GET /api/tokens` returns active and revoked metadata in bounded pages, ordered by ID descending. The default page size is 25; `limit` may be 1–100. The cursor is an exclusive token ID. Follow `nextCursor` until it is `null`. The response never returns the secret.

Use **Refresh newest tokens** and **Next page** to review metadata. `lastUsedAt` is best-effort and updates at most hourly, so it cannot prove that a caller has stopped using a token.

### Rotate and revoke a token

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

For request limits and management error handling, see [Limits and errors](https://engine.speedle.io/docs/cloud/limits-and-errors/). For using a token in an authorization request, see the [Authorization API preview](https://engine.speedle.io/docs/cloud/authorization-api/).

## Speedle Cloud authorization API preview

**Authenticated ADS is pending.** The requests and examples on this page describe the published contract for a future release; they were not executed against a live Cloud environment. Issuing an application token does not enable these calls. Use the approved stage URL explicitly, and verify that authenticated ADS is available in that stage before integrating.

### Call IsAllowed from your server

Send the issued token in one `Authorization: Bearer` header and make a POST request:

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <issued token>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

The body is the Speedle REST request, with the same fields and types as a self-hosted Speedle ADS. `attributes` is an array of `{name, type, value}` items. The supported attribute types are `string`, `numeric`, `bool`, and `datetime` in RFC 3339 format.

`idd` is the principal's identity domain: the namespace your application authenticated the caller in, such as an identity provider or a tenant. Set it on the server from trusted authentication data, never from a value the browser supplies.

A policy such as `grant user alice from corp get /books` matches that domain only. A policy without `from` matches the name in every domain, so the getting-started policy accepts the request above. A principal without `idd` matches every domain as well, which makes two different users who share a name indistinguishable when the application accepts more than one identity source. Multi-tenant integrations must send `idd` in every request.

`serviceName` is required in every request and is never defaulted. Send the name of the token's own service. An omitted or empty name receives Speedle's `SERVICE_NOT_FOUND` response; a different name returns `403 {"error":"service_mismatch"}`.

The [getting-started policy](https://engine.speedle.io/docs/cloud/getting-started/#create-a-service-and-a-policy) allows the example request:

```json
{"allowed":true,"reason":0}
```

### Interpret decisions and caller identity

A policy denial also returns HTTP 200, with `allowed` set to `false`. Your application must check `allowed` before performing the protected action and must fail closed on exceptions or invalid responses.

The application establishes the caller's principals through its own user authentication. The API token identifies the application and service, not the end user. Do not trust a principal name supplied by an unauthenticated browser.

Speedle Cloud does not validate subject tokens. Like self-hosted Speedle without a token asserter, it accepts `subject.token` and `tokenType` and discards them without validation, so a non-empty token field is never evidence that authentication happened; decisions use the principals only. Do not forward an end-user token and treat the call as asserted.

The session Playground evaluates current D1 policy data. Authenticated ADS uses published snapshots, with propagation of approximately 60 seconds. A successful Playground test does not prove that every ADS location has the new snapshot. ADS has no public Diagnose endpoint.

### Node.js 22 example

Node.js 22 can call ADS without an SDK:

```js
export async function canReadBooks(authenticatedUserName, authenticatedUserIdd) {
  if (!authenticatedUserName || !authenticatedUserIdd) {
    throw new Error("principal name and identity domain are required");
  }
  const apiToken = process.env.SPEEDLE_ADS_TOKEN;
  if (!apiToken) throw new Error("Speedle credential is not configured");
  const response = await fetch("https://cloud.speedle.io/authz-check/v1/is-allowed", {
    method: "POST",
    redirect: "error",
    signal: AbortSignal.timeout(10_000),
    headers: {
      Authorization: `Bearer ${apiToken}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      subject: {
        principals: [
          { type: "user", name: authenticatedUserName, idd: authenticatedUserIdd },
        ],
      },
      serviceName: "books",
      resource: "/books",
      action: "get",
    }),
  });
  if (!response.ok) throw new Error(`Speedle HTTP ${response.status}`);
  const result = await response.json();
  if (
    typeof result !== "object" || result === null || Array.isArray(result) ||
    typeof result.allowed !== "boolean" || !Number.isInteger(result.reason) ||
    result.reason < 0 || result.reason > 6
  ) {
    throw new Error("Invalid Speedle decision");
  }
  return result.allowed;
}
```

Inject `SPEEDLE_ADS_TOKEN` from a server-side secret store; never hard-code it. Pass the principal name and identity domain that your server derived from the authenticated user. The example rejects an empty name or domain before building the body, because `JSON.stringify` would otherwise drop `idd` and send a domain-less principal. Leave `idd` out only for a service with a single identity source, where a name cannot collide across domains. The caller must fail closed on exceptions and must not turn failures into permission grants.

This minimal example has no retry loop. Production handling should distinguish the documented errors, respect `Retry-After`, and use bounded retries. Setting `redirect: "error"` prevents fetch from following a redirect with the credential; retain that behavior when adapting the example.

### Transport and caching constraints

Use POST for ADS. A non-POST request may be stopped at the public edge with Cloudflare's 403 response; do not parse that response as an application authorization decision. There is no browser CORS workflow for direct ADS calls with a secret token.

Matched application ADS responses use `Cache-Control: no-store`. Do not put decisions or credentials in shared caches.

For client libraries in Java, Python, Go, .NET, Rust and Node.js/TypeScript, see the [SDKs section](https://engine.speedle.io/docs/sdks/). For failures, quotas and response headers, see [Limits and errors](https://engine.speedle.io/docs/cloud/limits-and-errors/). API schemas and Markdown downloads are linked from the [Speedle Cloud overview](https://engine.speedle.io/docs/cloud/#downloads).

## Speedle Cloud limits and errors

The ADS errors and headers below describe the authenticated ADS contract, whose runtime is not yet available. The management contract already defines account limits, session and CSRF checks, and token-management errors for use when account access is available. Downloading a schema does not enable a pending endpoint.

### Limits

| Limit | Meaning |
| --- | --- |
| 10,000 calls per day | Account-wide matched PMS and ADS handler calls per UTC day, across sessions, tokens and services; resets at 00:00 UTC. |
| 200 policies | Authorization and role policies combined per account. |
| 4 KiB of SPDL | 4,096 UTF-8 bytes per complete policy, not 4,096 characters. |
| 2 active tokens | Per service; revoked history does not consume this cap. |
| 600 requests per minute per token | Approximate per-location burst limit, separate from the strict daily account cap. |
| 64 KiB request body | Global bound before parsing. Optional incoming attributes also retain their 50-attribute and 16,384-byte canonical UTF-8 JSON bound. |

Each account has one service. Token names are 1–128 characters. See [Application tokens](https://engine.speedle.io/docs/cloud/tokens/) for credential lifecycle rules.

### Daily quota accounting

Entering a matched handler spends one daily unit even for a policy denial, invalid business input or internal error. Authentication and session endpoints do not spend these units. Requests rejected before handler entry by authentication, routing, CSRF, body-size, burst-limit or quota-reservation checks do not spend them either.

Reserved leases that expire or are abandoned are not reissued, so the number of usable calls can be fewer than 10,000. `GET /api/me` reports reserved daily usage and the reset time without charging a daily unit.

Switching tokens does not add account quota. The daily account cap and the approximate per-location token burst cap are enforced separately.

### HTTP errors

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

### Rate and quota headers

After verification, `X-RateLimit-Limit` is 600. ADS does not expose an exact `X-RateLimit-Remaining` value.

When present after a charged handler, `X-Quota-Daily-Remaining` estimates the account-wide calls remaining today: the unreserved calls at lease grant plus this isolate's remaining lease. The estimate is conservative at grant time. It is cached for at most the valid 30-second lease and discarded when the lease expires or the UTC day rolls over.

Concurrent isolates can make the value overstate availability by up to the unreserved balance at grant time, not merely 32 calls. Unused or abandoned reservations can make it understate availability. During rollout or rollback, legacy allocators fall back to the local lease remaining. This header is informational and does not change quota enforcement. Early rejections omit it.

Matched application ADS responses also use `Cache-Control: no-store`. Do not place decisions or credentials in shared caches.

Return to the [Authorization API preview](https://engine.speedle.io/docs/cloud/authorization-api/) for request examples and integration constraints.
