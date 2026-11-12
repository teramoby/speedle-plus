# Speedle Cloud documentation

Speedle Cloud is a hosted access-control service for developers, built on the open-source Speedle policy engine. It is designed to let you manage SPDL authorization and role policies, test decisions in a Playground, and call an authorization API from your application server without operating your own authorization service.

**Speedle Cloud is coming soon.** The Cloud header displays a **Coming soon** badge, and production sign-in is not open on [cloud.speedle.io](https://cloud.speedle.io/) yet. The console workflow and authenticated Authorization Decision Service (ADS) described here are previews of the published service contract. Issuing a token will not enable server-side authorization calls while authenticated ADS remains pending. Verify deployment and identity-provider availability in your chosen stage.

The free public beta is intended for trials, learning and evaluation. It has no SLA, is not recommended for production use, and its features and limits may change.

## Documentation

- [Getting started](https://engine.speedle.io/docs/cloud/getting-started/): Sign in, create a service and policy in the console, create an API key, and check authorization from your application.
- [Application tokens](https://engine.speedle.io/docs/cloud/tokens/): recent-authentication requirements, one-time token disclosure, metadata pagination, rotation and revocation.
- [Authorization API preview](https://engine.speedle.io/docs/cloud/authorization-api/): the IsAllowed request and response contract, a Node.js 22 example, policy snapshots and integration constraints.
- [Limits and errors](https://engine.speedle.io/docs/cloud/limits-and-errors/): account quotas, request limits, error responses, rate and quota headers, and retry guidance.

## Downloads

- [English Markdown](https://engine.speedle.io/docs/cloud/quickstart.en.md): the Cloud documentation chapters in one offline document.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): the canonical checked-in Cloud API specification.
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): generated from the same YAML source bytes.
- [Serving build version](https://cloud.speedle.io/build.json): identifies the serving build; it does not certify identity-provider or endpoint availability.

The schemas include the four session Playground operations and the authenticated ADS contract. Publishing a schema or downloading a guide does not enable a pending endpoint or open sign-in. The YAML and generated JSON are versioned, promoted and restored with the corresponding sealed Cloud build. The English and Chinese Markdown downloads are maintained with these Engine documentation pages.

Read the Cloud [Terms](https://cloud.speedle.io/terms) and [Privacy Policy](https://cloud.speedle.io/privacy).

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

Open [API keys](https://cloud.speedle.io/tokens), enter a name such as `books-app`, select the `ads` scope (authorization checks), and choose **Create API key**. Complete provider verification if prompted, then submit the form again. Copy the secret when it appears: it is shown only once. Current keys have no selectable expiry; revoke the key when it is no longer needed. See [API key management](https://engine.speedle.io/docs/cloud/tokens/).

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

Next steps: for programmatic management, use an [API key](https://engine.speedle.io/docs/cloud/tokens/) with the [PMS API or spctl (Speedle Cloud)](https://engine.speedle.io/docs/pms/policy-mgmt/#speedle-cloud).

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

The getting-started policy, `grant user alice from corp get /books`, matches the request above only in the `corp` domain. A policy without `from` matches the name in every domain. A principal without `idd` matches every domain as well, which makes two different users who share a name indistinguishable when the application accepts more than one identity source. Multi-tenant integrations must send `idd` in every request.

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

Reserved leases that expire or are abandoned are not reissued, so the number of usable calls can be fewer than 10,000.

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
