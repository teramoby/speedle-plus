# Speedle Cloud documentation

Speedle Cloud is a hosted authorization service built on the open-source Speedle engine. Manage services, SPDL authorization and role policies, test decisions in the Playground and call the API from your server.

**Speedle Cloud is coming soon.** This guide describes the workflow available when production sign-in and authenticated authorization checks open; the examples have not been run against a live Cloud environment.

Account sign-in must be available before using the console or creating API keys. The free public beta is for trials, learning and evaluation, has no SLA and is not recommended for production use. Features and limits may change.

## Documentation

- [Getting started](https://engine.speedle.io/docs/cloud/getting-started/): Sign in, create a service and policy in the console, create an API key, and check authorization from your application.
- [API keys](https://engine.speedle.io/docs/cloud/tokens/): Account-level credentials, fixed scopes, a required expiry, Create and Revoke, one-time secret display and pagination.
- [Authorization API](https://engine.speedle.io/docs/cloud/authorization-api/): IsAllowed requests, caller identity, Node.js 22 and transport constraints.
- [API reference](https://engine.speedle.io/docs/cloud/api-reference/): authorization checks and policy management with an API key. Requests run from your server; this page is read-only.
- [Limits and errors](https://engine.speedle.io/docs/cloud/limits-and-errors/): Account caps, size limits, burst limits, daily quota, error codes and reset times.

## Downloads

- [English Markdown](https://engine.speedle.io/docs/cloud/quickstart.en.md): The four Cloud chapters in one offline guide.
- [API reference](https://engine.speedle.io/docs/cloud/api-reference/): read it on this site. The raw files below remain available.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)

The API schemas describe the request and response contracts; downloading them does not open account sign-in.

Read the Cloud [Terms](https://cloud.speedle.io/terms) / [Privacy Policy](https://cloud.speedle.io/privacy).

## Getting started with Speedle Cloud

**Speedle Cloud is coming soon.** This guide describes the workflow available when production sign-in and authenticated authorization checks open; the examples have not been run against a live Cloud environment.

### Sign in with GitHub or Google (opens soon)

Open the [Cloud console](https://cloud.speedle.io/) in your browser and sign in with an available provider. For a new identity with an unused verified email address, confirm **Create account**. If the email is already in use, sign in with the linked provider shown by the console.

### Create a service and a policy

Open the [Service workspace](https://cloud.speedle.io/service), enter `books`, and choose **Create service**. Use a new service for this example so existing policies do not affect the results; do not overwrite existing data.

Choose **New policy**, name it `read-books`, and enter this SPDL:

```text
grant user alice from corp get /books
```

This lets `alice` in the `corp` identity domain perform `get` on `/books`. A grant without `from` matches the name in every domain. Derive the domain from trusted authentication data and send it as `idd` in each request. See [identity domains](https://engine.speedle.io/docs/idd/).

Choose **Validate policy**, then **Save policy**. Validation checks syntax but does not save the draft. Allow approximately 60 seconds for saved policies to reach the authorization API.

### Create an API key

Open [API keys](https://cloud.speedle.io/tokens), enter a name such as `books-app`, select the `ads` scope (authorization checks), choose an expiry (the console defaults to 3 months and shows the UTC time), and choose **Create API key**. Complete provider verification if prompted, then submit the form again. Copy the secret when it appears: it is shown only once. Every key expires, including keys created before an expiry was required. [API key management](https://engine.speedle.io/docs/cloud/tokens/) states when. Read the listed expiry and replace the key before then. Revoke a key earlier when it is no longer needed.

Store the key in your server's secret store and provide it through the `SPEEDLE_API_KEY` environment variable. Keep it out of browser code and source control.

### Check authorization from your application

From your application's server, send this `isAllowed` request for the policy saved above. The policy needs no attributes, so `attributes` is empty:

```bash
curl --fail-with-body --silent --show-error --max-time 10 \
  'https://cloud.speedle.io/authz-check/v1/is-allowed' \
  -H "Authorization: Bearer $SPEEDLE_API_KEY" \
  -H 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[]}'
```

With only the example policy saved, the expected response is:

```json
{"allowed":true,"reason":0}
```

Change `action` to `delete` and send the request again. No policy grants that action, so the expected response is:

```json
{"allowed":false,"reason":3}
```

Both decisions return HTTP 200. Perform the protected action only when `allowed` is `true`; your application must enforce the result and deny access on errors, invalid responses or timeouts.

See the [authorization API](https://engine.speedle.io/docs/cloud/authorization-api/) and the [SDKs](https://engine.speedle.io/docs/sdks/), including [Node.js/TypeScript](https://engine.speedle.io/docs/sdks/node/), [Python](https://engine.speedle.io/docs/sdks/python/), [Java](https://engine.speedle.io/docs/sdks/java/) and [Go](https://engine.speedle.io/docs/sdks/go/), for integration details.

### Test saved policies in the Playground

Optionally, open the service's **Playground** and select **Is allowed**. Enter `[{"type":"user","name":"alice","idd":"corp"}]` in **Principals (JSON)**, `/books` in **Resource**, `get` in **Action**, and `[]` in **Attributes (JSON)**, then choose **Run test**.

**Is allowed** shows the decision; **Diagnose** explains how policies contributed; **All granted roles** lists the subject's roles; **All granted permissions** lists granted resources and actions. Tests use saved policies, not an unsaved draft. Run the test again after changing inputs or policies. Applications use the [ADS authorization API](https://engine.speedle.io/docs/cloud/authorization-api/) with an API key.

Next steps: for programmatic management, create a separate key with the `pms:write` scope (or `pms:read` for reads), then use the [PMS API or spctl (Speedle Cloud)](https://engine.speedle.io/docs/pms/policy-mgmt/#speedle-cloud). The `ads` key from the previous step cannot manage policies.

## Speedle Cloud API keys

API keys belong to your account and can access its services according to their scope. They authenticate your application, not its end users. Managing API keys, sessions and linked identities always requires a signed-in console session.

**Speedle Cloud is coming soon.** This guide describes the workflow available when production sign-in and authenticated authorization checks open; the examples have not been run against a live Cloud environment.

### Create an API key

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

### Protect the secret

Creation returns HTTP 201. The response's `token` field contains the API key secret and is shown only once:

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

Every key expires at `expiresAt`, which the server computes and returns in Unix milliseconds. At or after that instant, authorization and policy-management calls return `401 {"error":"api_key_expired"}`. Create a replacement key. Retrying the expired key does not succeed. You can revoke a key before it expires. Keys that already existed when an expiry became required have one too. If 365 days of 24 hours after creation is still in the future, the key expires then, even when that date is only a short time after the requirement took effect. If that date has already passed, the key expires 30 days of 24 hours after the requirement took effect. Every listed key includes `expiresAt`. Read it and create a replacement before that time.

This describes the format, not a usable key. Save the value immediately in a server-side secret store. Never put it in browser storage, frontend code, URLs, source control, screenshots or logs. Listing keys returns metadata only; a lost secret cannot be recovered.

Your account can have **10 active API keys across all scopes**. Creating another returns `409 {"error":"quota_exceeded","limit":10}`. Revoke an unused key, or let it expire, to free a slot. The cap does not reset on a schedule. Revoked and expired keys remain listed and do not count toward the cap.

### List API keys

`GET /api/tokens` lists active, expired and revoked keys, newest first. The default page size is 25; `limit` accepts 1–100. Pass the last response's `nextCursor` as `cursor` until `nextCursor` is `null`. Each key includes `expiresAt` in Unix milliseconds. The console shows **Expires (UTC)**. A key that expires within 7 days is marked **Expires soon**. Expired keys appear in an **Expired** group. `lastUsedAt` is best-effort, normally updated at most hourly; it does not prove that a key is unused.

### Revoke an API key

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

If a creation or revocation response is lost, refresh the list before acting again. A matching name cannot recover a lost secret. See [Limits and errors](https://engine.speedle.io/docs/cloud/limits-and-errors/) and the [Authorization API](https://engine.speedle.io/docs/cloud/authorization-api/).

## Speedle Cloud authorization API

**Speedle Cloud is coming soon.** This guide describes the workflow available when production sign-in and authenticated authorization checks open; the examples have not been run against a live Cloud environment.

Use an account API key for server-side authorization calls. Account sign-in must be available before you can create keys. A key with a management scope cannot call ADS; use `ads` or the response is `403 insufficient_scope`.

### Call IsAllowed from your server

Send an account API key with the `ads` scope in one `Authorization: Bearer` header and make a POST request:

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <API key>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

The body is the Speedle REST request, with the same fields and types as a self-hosted Speedle ADS. `attributes` is an array of `{name, type, value}` items. The supported attribute types are `string`, `numeric`, `bool`, and `datetime` in RFC 3339 format.

`idd` is the principal's identity domain: the namespace your application authenticated the caller in, such as an identity provider or a tenant. Set it on the server from trusted authentication data, never from a value the browser supplies.

The getting-started policy, `grant user alice from corp get /books`, matches the request above only in the `corp` domain. A policy without `from` matches the name in every domain. A principal without `idd` matches every domain as well, which makes two different users who share a name indistinguishable when the application accepts more than one identity source. Multi-tenant integrations must send `idd` in every request.

`serviceName` is required in every request and is never defaulted. Select a service in the API key’s account. A missing, empty, unknown or other-account service name returns `404 {"error":"service_not_found"}`. An account API key is not bound to one service.

The [getting-started policy](https://engine.speedle.io/docs/cloud/getting-started/#create-a-service-and-a-policy) allows the example request:

```json
{"allowed":true,"reason":0}
```

### Interpret decisions and caller identity

A policy denial also returns HTTP 200, with `allowed` set to `false`. Your application must check `allowed` before performing the protected action and must fail closed on exceptions or invalid responses.

The application establishes the caller's principals through its own user authentication. The API key identifies the account and its permitted API scope, not the end user. Do not trust a principal name supplied by an unauthenticated browser.

Speedle Cloud does not validate subject tokens. Like self-hosted Speedle without a token asserter, it accepts `subject.token` and `tokenType` and discards them without validation, so a non-empty token field is never evidence that authentication happened; decisions use the principals only. Do not forward an end-user token and treat the call as asserted.

The session Playground evaluates the currently saved policies. Authenticated ADS uses published snapshots. A successful Playground test does not prove that every ADS location has the new snapshot. ADS has no public Diagnose endpoint.

### Node.js 22 example

Node.js 22 can call ADS without an SDK:

```js
export async function canReadBooks(authenticatedUserName, authenticatedUserIdd) {
  if (!authenticatedUserName || !authenticatedUserIdd) {
    throw new Error("principal name and identity domain are required");
  }
  const apiKey = process.env.SPEEDLE_API_KEY;
  if (!apiKey) throw new Error("Speedle credential is not configured");
  const response = await fetch("https://cloud.speedle.io/authz-check/v1/is-allowed", {
    method: "POST",
    redirect: "error",
    signal: AbortSignal.timeout(10_000),
    headers: {
      Authorization: `Bearer ${apiKey}`,
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

Inject `SPEEDLE_API_KEY` from a server-side secret store; never hard-code it. Pass the principal name and identity domain that your server derived from the authenticated user. The example rejects an empty name or domain before building the body, because `JSON.stringify` would otherwise drop `idd` and send a domain-less principal. Leave `idd` out only for a service with a single identity source, where a name cannot collide across domains. The caller must fail closed on exceptions and must not turn failures into permission grants.

This minimal example has no retry loop. Production handling should distinguish the documented errors, respect `Retry-After`, and use bounded retries. Setting `redirect: "error"` prevents fetch from following a redirect with the credential; retain that behavior when adapting the example.

### Transport and caching constraints

Use POST for ADS. A non-POST request may be stopped at the public edge with Cloudflare's 403 response; do not parse that response as an application authorization decision. There is no browser CORS workflow for direct ADS calls with a secret API key.

Matched application ADS responses use `Cache-Control: no-store`. Do not put decisions or credentials in shared caches.

For client libraries in Java, Python, Go, .NET, Rust and Node.js/TypeScript, see the [SDKs section](https://engine.speedle.io/docs/sdks/). For failures, quotas and response headers, see [Limits and errors](https://engine.speedle.io/docs/cloud/limits-and-errors/). API schemas and Markdown downloads are linked from the [Speedle Cloud overview](https://engine.speedle.io/docs/cloud/#downloads).

## Speedle Cloud Limits and errors

**Speedle Cloud is coming soon.** This guide describes the workflow available when production sign-in and authenticated authorization checks open; the examples have not been run against a live Cloud environment.

Limits apply to the whole account unless a row says otherwise. API keys are account credentials; changing keys does not increase the daily quota.

### Limits

| What is limited | Limit | When reached / how to recover |
| --- | --- | --- |
| Services per account | 100 | `409 quota_exceeded`; delete an unused service. No timed reset. |
| Authorization and role policies combined, across all services | 200 per account | `409 quota_exceeded`; delete policies. No timed reset. |
| Complete SPDL source for one policy | 1,024 UTF-8 bytes | `400 invalid_request` on console policy writes; shorten the source. This counts bytes, not characters. No timed reset. |
| Active API keys, all scopes combined | 10 per account | `409 quota_exceeded` with `limit: 10`; revoke an unused key or let it expire. The cap does not reset on a schedule. Revoked and expired keys do not count. |
| Policy name / API key name | 128 Unicode code points / 1–128 characters after trimming | `400 invalid_request`; shorten or correct the name. No timed reset. |
| Billable policy-management and authorization calls | 10,000 per account per UTC day | `429 daily_quota_exceeded`; resets at 00:00 UTC. Follow `Retry-After`. |
| Short bursts of API-key requests | Approximately 600 per minute per key at each Cloud location | `429 rate_limited` with `Retry-After: 60`; wait before retrying. This is a short-term limit, not a guaranteed throughput or a separate daily allowance. |
| Ordinary request body | 65,536 bytes (64 KiB) | `413 request_too_large`; send a smaller body. No timed reset. |
| Atomic JSON import body (`POST /api/service/import` only) | 5,100,000 bytes | `413 request_too_large`; reduce the import. The policy and account limits still apply. No timed reset. |
| Incoming attributes | 50 attributes; 16,384 UTF-8 bytes of canonical JSON | `400 SPDL-0004`; send fewer or smaller attributes. No timed reset. |

For scopes, expiry, creation and revocation, see [API keys](https://engine.speedle.io/docs/cloud/tokens/).

### Daily quota

Reads, writes, validation, Playground tests, imports, exports and authorization checks share the daily allowance. A policy denial or invalid business input still counts. Authentication, session operations and `GET /api/me` do not count. Requests rejected by authentication, scope checks, routing, CSRF, body-size checks, burst limits or the daily quota do not count.

Cloud-side failures may receive a quota refund, up to 1,000 per account per UTC day; if a refund cannot be confirmed, the call stays charged. Errors caused by the request still count. During a temporary quota-counter failure, authorization calls may continue at a reduced rate within an additional allowance of up to 10,000 calls per account per UTC day. That allowance is separate from the normal quota, is not guaranteed, and resets at 00:00 UTC along with the refund allowance. If it cannot be granted, the response is `503 quota_unavailable`.

Some daily capacity can be reserved but unused, so the usable total can be below 10,000. `GET /api/me` reports usage and the reset time. Do not retry continuously or switch keys to bypass account limits.

### Errors and response headers

Errors are flat JSON, for example `{"error":"rate_limited"}`. An error is not an authorization grant; fail closed if a valid decision cannot be obtained.

| Status and error | What to do |
| --- | --- |
| `400 invalid_request` or a Speedle error message | Correct the request; do not retry it unchanged. |
| `401 unauthorized` | Check the API key or signed-in session. A session cookie alone cannot authorize ADS. |
| `401 api_key_expired` | The API key is at or past its expiry. Create a replacement; retrying this key will not succeed. Keys created before an expiry was required have one too. The API keys page states when; read `expiresAt` and replace the key before then. |
| `403 reauth_required` | Authenticate with a linked provider again before creating a key. |
| `403 insufficient_scope` | Use a key with the required scope. |
| `403 management_scope_disabled` | Management-scope issuance is unavailable while sign-in is closed. |
| `403 csrf_failed` | Use the console session with the required origin and CSRF header. |
| `404 service_not_found` (ADS) / `404 not_found` (management) | Select an existing service in your account. Unknown, other-account or already-revoked key IDs also return `404 not_found`. |
| `409 quota_exceeded` | Free a service or policy slot by deleting an unused item; time does not reset those caps. Free an API-key slot by revoking the key or letting it expire. |
| `413 request_too_large` | Reduce the body size. |
| `429 rate_limited` / `429 daily_quota_exceeded` | Respect `Retry-After` and use bounded backoff; the daily limit resets at UTC midnight. |
| `503 quota_unavailable` / `503 snapshot_unavailable` / `500 internal_error` | Fail closed and retry with bounded backoff, respecting `Retry-After` when present. |

For verified API-key requests, `X-RateLimit-Limit` is 600. There is no exact `X-RateLimit-Remaining`. `X-Quota-Daily-Remaining`, when present, is an approximate account-wide balance; concurrent calls and unused reservations can make it differ from what you can actually use. It does not guarantee that the next request will succeed, and early rejections omit it.

ADS responses use `Cache-Control: no-store`. Keep decisions and credentials out of shared caches. Use POST; other methods can receive `405 method_not_allowed` or an earlier non-JSON block response. See the [Authorization API](https://engine.speedle.io/docs/cloud/authorization-api/).
