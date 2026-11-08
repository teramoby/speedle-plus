+++
title = "Speedle Cloud authorization API preview"
linkTitle = "Authorization API"
description = "Preview the Speedle Cloud ADS IsAllowed request and response, Node.js 22 integration, policy snapshots and transport constraints."
date = 2026-11-08T00:00:00+08:00
lastmod = 2026-11-08T00:00:00+08:00
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "The published server-side ADS contract for a future authenticated release."
+++

**Authenticated ADS is pending.** The requests and examples on this page describe the published contract for a future release; they were not executed against a live Cloud environment. Issuing an application token does not enable these calls. Use the approved stage URL explicitly, and verify that authenticated ADS is available in that stage before integrating.

## Call IsAllowed from your server

Send the issued token in one `Authorization: Bearer` header and make a POST request:

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <issued token>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

The body is the Speedle REST request, with the same fields and types as a self-hosted Speedle ADS. `attributes` is an array of `{name, type, value}` items. The supported attribute types are `string`, `numeric`, `bool`, and `datetime` in RFC 3339 format.

`serviceName` is required in every request and is never defaulted. Send the name of the token's own service. An omitted or empty name receives Speedle's `SERVICE_NOT_FOUND` response; a different name returns `403 {"error":"service_mismatch"}`.

The [getting-started policy](/docs/cloud/getting-started/#create-a-service-and-a-policy) allows the example request:

```json
{"allowed":true,"reason":0}
```

## Interpret decisions and caller identity

A policy denial also returns HTTP 200, with `allowed` set to `false`. Your application must check `allowed` before performing the protected action and must fail closed on exceptions or invalid responses.

The application establishes the caller's principals through its own user authentication. The API token identifies the application and service, not the end user. Do not trust a principal name supplied by an unauthenticated browser.

Speedle Cloud does not validate subject tokens. Like self-hosted Speedle without a token asserter, it accepts and ignores `subject.token` and `tokenType`; decisions use the principals only.

The session Playground evaluates current D1 policy data. Authenticated ADS uses published snapshots, with propagation of approximately 60 seconds. A successful Playground test does not prove that every ADS location has the new snapshot. ADS has no public Diagnose endpoint.

## Node.js 22 example

Node.js 22 can call ADS without an SDK:

```js
export async function canReadBooks(authenticatedUserName) {
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
      subject: { principals: [{ type: "user", name: authenticatedUserName }] },
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

Inject `SPEEDLE_ADS_TOKEN` from a server-side secret store; never hard-code it. Pass a principal derived from the authenticated user. The caller must fail closed on exceptions and must not turn failures into permission grants.

This minimal example has no retry loop. Production handling should distinguish the documented errors, respect `Retry-After`, and use bounded retries. Setting `redirect: "error"` prevents fetch from following a redirect with the credential; retain that behavior when adapting the example.

## Transport and caching constraints

Use POST for ADS. A non-POST request may be stopped at the public edge with Cloudflare's 403 response; do not parse that response as an application authorization decision. There is no browser CORS workflow for direct ADS calls with a secret token.

Matched application ADS responses use `Cache-Control: no-store`. Do not put decisions or credentials in shared caches.

For client libraries in Java, Python, Go, .NET, Rust and Node.js/TypeScript, see the [SDKs section](/docs/sdks/). For failures, quotas and response headers, see [Limits and errors](/docs/cloud/limits-and-errors/). API schemas and Markdown downloads are linked from the [Speedle Cloud overview](/docs/cloud/#downloads).
