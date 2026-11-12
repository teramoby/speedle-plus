# Speedle Cloud documentation

Speedle Cloud is a hosted authorization service built on the open-source Speedle engine. Manage services, SPDL authorization and role policies, test decisions in the Playground and call the API from your server.

**Speedle Cloud is coming soon.** Production sign-in is not open on cloud.speedle.io yet, and the Cloud header shows **Coming soon**. When sign-in is closed, the sign-in page says that it opens soon.

Account sign-in must be available before using the console or creating API keys. The free public beta is for trials, learning and evaluation, has no SLA and is not recommended for production use. Features and limits may change.

## Documentation

- [Getting started](https://engine.speedle.io/docs/cloud/getting-started/): Sign-in, accounts, services, policies and the session Playground.
- [API keys](https://engine.speedle.io/docs/cloud/tokens/): Account-level credentials, fixed scopes, Create and Revoke, one-time secret display and pagination.
- [Authorization API](https://engine.speedle.io/docs/cloud/authorization-api/): IsAllowed requests, caller identity, Node.js 22 and transport constraints.
- [Limits and errors](https://engine.speedle.io/docs/cloud/limits-and-errors/): Account caps, size limits, burst limits, daily quota, error codes and reset times.

## Downloads

- [English Markdown](https://engine.speedle.io/docs/cloud/quickstart.en.md): The four Cloud chapters in one offline guide.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)

The API schemas describe the request and response contracts; downloading them does not open account sign-in.

Read the Cloud [Terms](https://cloud.speedle.io/terms) / [Privacy Policy](https://cloud.speedle.io/privacy).

## Getting started with Speedle Cloud

**Speedle Cloud is coming soon.** Production sign-in is not open on cloud.speedle.io yet, and the Cloud header shows **Coming soon**. When sign-in is closed, the sign-in page says that it opens soon. This guide describes the published workflow rather than a currently open production console; screenshots will be added after production sign-in opens.

### Production origin

Use `https://cloud.speedle.io` for account sessions, management requests and API keys. Accounts, resources and API keys belong to that origin. The examples in this section are illustrative and were not executed against a live environment. Confirm that your chosen stage is deployed and that the identity provider you intend to use is available there.

### Sign in with GitHub or Google (opens soon)

The sign-in page shows **Sign in with GitHub** and **Sign in with Google** even while sign-in is disabled. When production sign-in opens, choose an available provider. A new identity whose verified email address is unused requires an explicit **Create account** confirmation. If the email address is already in use, sign in with the linked provider identified by the console, then connect the second identity in **Account settings**. Accounts are not merged automatically.

Account settings let you inspect connected identities and connect or disconnect an available provider; keep at least one connected identity. **Sign out** is in the header beside the language switch, and the Account menu offers **Sign out everywhere**. Successful sign-out returns to the sign-in page and clears the workspace and any one-time API key display. The audit log shows account activity.

Deleting an account requires recent provider authentication and typing `DELETE`. It permanently removes the account's services, policies, API keys and identities and ends its sessions. Export any policies you need from the Service workspace before deletion.

### Create a service and a policy

For a new account, open the [Service workspace](https://cloud.speedle.io/service), enter `books` as the service name, and choose **Create service**. Each account can create up to 100 services. If the account already has a service or policies, inspect them and use an appropriate existing configuration; do not overwrite or delete existing data to make this example work.

Choose **New policy**, enter `read-books` as the policy name, and put this authorization policy in the SPDL editor:

```text
grant user alice from corp get /books
```

`from corp` keeps this grant inside the `corp` identity domain. Replace `corp` with the domain your application derives from trusted authentication data, and send the same value as the principal's `idd` in each request, as the Playground examples below do. A grant without `from` matches a user named `alice` in every domain; omit the clause only for a service with a single identity source, where names cannot collide across domains. See [identity domain](https://engine.speedle.io/docs/idd/) for the rule and the matching request field.

Choose **Validate policy** and check that the console reports **Detected: authorization policy**, then choose **Save policy**. Validation detects the policy type and checks syntax; it does not save the draft. Saving is a separate, manual action. You can also import existing policies in the workspace instead of entering them one by one.

Speedle Cloud supports authorization policies and role policies; custom external functions are not supported. The policy list groups the two types and shows their counts. Role assignments use these forms:

- Global role: `grant user alice editor`
- Resource-scoped role: `grant user alice editor on /books`

The ambiguous statement `grant user alice role editor` is rejected. Omit the optional `role` prefix for a role assignment, or write `grant (user alice) role editor` for an authorization policy. Editing retains the stored policy type. To change the type, delete the policy and create a new one.

If the console reports a conflict or an uncertain response, keep your draft and refresh the saved data before choosing another mutation. Changing language preserves drafts. Monaco's built-in menus keep the language selected when the editor first loaded and change after a page reload.

Service deletion requires you to type the service name, complete recent provider authentication, and confirm again after verification.

### Send management session requests

The Policy Management Service (PMS) uses the account session. Let a supported same-origin client manage its secure session cookie; do not extract cookies or use a test-provider bootstrap. Mutating requests require the exact stage `Origin`, `X-SC-CSRF: 1`, and a JSON content type. An ADS Bearer API key cannot authorize PMS. Clients must not send account or service ownership selectors, or authentication timestamps.

The equivalent requests for the service and policy example are:

| Action | Session request | Success |
| --- | --- | --- |
| Create the account's service | `POST /api/service` with `{"name":"books"}` | 201 |
| Add an authorization policy | `POST /api/service/policies` with the body below | 201 |

```json
{"name":"read-books","spdl":"grant user alice from corp get /books"}
```

Paste only the `spdl` value into the editor when using the console. The JSON body is for the management API.

### Validate and export policies

Validate SPDL with `POST /api/service/validate` and a `{spdl}` body to detect its type. The legacy `{kind: "authz", spdl}` form still selects the authorization parser. **Download SPDL** and **Download JSON** call `GET /api/service/export?format=spdl` and `GET /api/service/export?format=json`, respectively.

The session Playground evaluates current account-owned D1 data. The future authenticated ADS release evaluates published policy snapshots, whose propagation takes approximately 60 seconds. A successful Playground result therefore does not prove that every ADS location has received the new snapshot.

### Test saved policies in the Playground

In the Service workspace, scroll to **Playground** and select **Is allowed**. Enter `[{"type":"user","name":"alice","idd":"corp"}]` in **Principals (JSON)**, `/books` in **Resource**, `get` in **Action**, and `[]` in **Attributes (JSON)**, then choose **Run test**.

The other operations are **Diagnose**, **All granted roles**, and **All granted permissions**. Playground runs use saved policies, not an unsaved editor draft. Changing the input, operation or language does not automatically send a request. Run the test again after changing inputs or saved policies. The console marks the earlier result as stale and keeps it visible if a later request fails.

### Use the session Playground API

`POST /api/service/test` implements all four operations with the same Origin/CSRF rules and daily account quota as other management requests. It reads a consistent snapshot of the current account's D1 policies. It does not publish or save policies and does not accept an account or service ID. The displayed revision is the workspace revision known when the request was submitted, not proof of the server-side snapshot revision that was evaluated.

```http
POST /api/service/test
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"op":"isAllowed","request":{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3}]}}
```

| `request.op` | Success body | Published console and session API contract |
| --- | --- | --- |
| `isAllowed` | Speedle IsAllowedResponse: `allowed`, `reason`, optional `errorMessage` | Included |
| `diagnose` | Speedle EvaluationDebugResponse: reason name, `requestContext`, `attributes`, `policies` | Included |
| `allGrantedRoles` | `string[]`; `[]` when none | Included |
| `allGrantedPermissions` | `{resource, actions}[]`; `[]` when none | Included |

Success bodies are the plain upstream values, without an `op` or `result` envelope. Both all-granted results may be `null` or an empty array; neither shape grants authorization. ADS has no public Diagnose endpoint.

Next, read about [API keys](https://engine.speedle.io/docs/cloud/tokens/), the [authorization API preview](https://engine.speedle.io/docs/cloud/authorization-api/), and [limits and errors](https://engine.speedle.io/docs/cloud/limits-and-errors/).

## Speedle Cloud API keys

API keys belong to your account and can access its services according to their scope. They authenticate your application, not its end users. Managing API keys, sessions and linked identities always requires a signed-in console session.

**Speedle Cloud is coming soon.** Production sign-in is not open on cloud.speedle.io yet, and the Cloud header shows **Coming soon**. When sign-in is closed, the sign-in page says that it opens soon.

### Create an API key

Open [API keys](https://cloud.speedle.io/tokens), enter a name of 1–128 characters after trimming whitespace, select a scope and choose **Create**. Creating a key requires authentication with a linked provider within the last 10 minutes. If you receive `403 reauth_required`, authenticate again, then submit **Create** again; returning from the provider does not create a key automatically.

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

{"name":"books-app","scope":"ads"}
```

The request accepts `name` and optional `scope` (default `ads`). Do not send a service selector, expiry or authentication proofs. Cookie authentication and CSRF headers are required; an API key cannot replace the session.

### Protect the secret

Creation returns HTTP 201. The response's `token` field contains the API key secret and is shown only once:

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

An API key stays valid until you revoke it; it has no automatic expiry.

This describes the format, not a usable key. Save the value immediately in a server-side secret store. Never put it in browser storage, frontend code, URLs, source control, screenshots or logs. Listing keys returns metadata only; a lost secret cannot be recovered.

Your account can have **10 active API keys across all scopes**. Creating another returns `409 {"error":"quota_exceeded","limit":10}`. Revoke an unused key to free a slot; the limit does not reset with time. Revoked keys remain listed and do not count toward the cap.

### List API keys

`GET /api/tokens` lists active and revoked keys, newest first. The default page size is 25; `limit` accepts 1–100. Pass the last response's `nextCursor` as `cursor` until `nextCursor` is `null`. `lastUsedAt` is best-effort, normally updated at most hourly; it does not prove that a key is unused.

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

The available actions are **Create** and **Revoke**. To change a secret or scope, create another key, update callers and revoke the old key. If all active keys are in use at the cap, plan for interruption before revoking one. Deleting a service does not revoke account API keys.

If a creation or revocation response is lost, refresh the list before acting again. A matching name cannot recover a lost secret. See [Limits and errors](https://engine.speedle.io/docs/cloud/limits-and-errors/) and the [Authorization API](https://engine.speedle.io/docs/cloud/authorization-api/).

## Speedle Cloud authorization API

**Speedle Cloud is coming soon.** Production sign-in is not open on cloud.speedle.io yet, and the Cloud header shows **Coming soon**. When sign-in is closed, the sign-in page says that it opens soon.

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

**Speedle Cloud is coming soon.** Production sign-in is not open on cloud.speedle.io yet, and the Cloud header shows **Coming soon**. When sign-in is closed, the sign-in page says that it opens soon.

Limits apply to the whole account unless a row says otherwise. API keys are account credentials; changing keys does not increase the daily quota.

### Limits

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

For scopes, creation and revocation, see [API keys](https://engine.speedle.io/docs/cloud/tokens/).

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

ADS responses use `Cache-Control: no-store`. Keep decisions and credentials out of shared caches. Use POST; other methods can receive `405 method_not_allowed` or an earlier non-JSON block response. See the [Authorization API](https://engine.speedle.io/docs/cloud/authorization-api/).
