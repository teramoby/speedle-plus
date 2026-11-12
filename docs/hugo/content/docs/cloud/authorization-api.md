+++
title = "Speedle Cloud authorization API"
linkTitle = "Authorization API"
description = "Speedle Cloud ADS IsAllowed request and response, Node.js 22 integration, policy snapshots and transport constraints."
date = 2026-11-08T00:00:00+08:00
lastmod = 2026-11-12T00:00:00+08:00
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Server-side authorization with an account API key."
+++

**Speedle Cloud is coming soon.** Production sign-in is not open on cloud.speedle.io yet, and the Cloud header shows **Coming soon**. When sign-in is closed, the sign-in page says that it opens soon.

Use an account API key for server-side authorization calls. Account sign-in must be available before you can create keys. A key with a management scope cannot call ADS; use `ads` or the response is `403 insufficient_scope`.

## Call IsAllowed from your server

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

The [getting-started policy](/docs/cloud/getting-started/#create-a-service-and-a-policy) allows the example request:

```json
{"allowed":true,"reason":0}
```

## Interpret decisions and caller identity

A policy denial also returns HTTP 200, with `allowed` set to `false`. Your application must check `allowed` before performing the protected action and must fail closed on exceptions or invalid responses.

The application establishes the caller's principals through its own user authentication. The API key identifies the account and its permitted API scope, not the end user. Do not trust a principal name supplied by an unauthenticated browser.

Speedle Cloud does not validate subject tokens. Like self-hosted Speedle without a token asserter, it accepts `subject.token` and `tokenType` and discards them without validation, so a non-empty token field is never evidence that authentication happened; decisions use the principals only. Do not forward an end-user token and treat the call as asserted.

The session Playground evaluates the currently saved policies. Authenticated ADS uses published snapshots. A successful Playground test does not prove that every ADS location has the new snapshot. ADS has no public Diagnose endpoint.

## Node.js 22 example

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

## Transport and caching constraints

Use POST for ADS. A non-POST request may be stopped at the public edge with Cloudflare's 403 response; do not parse that response as an application authorization decision. There is no browser CORS workflow for direct ADS calls with a secret API key.

Matched application ADS responses use `Cache-Control: no-store`. Do not put decisions or credentials in shared caches.

For client libraries in Java, Python, Go, .NET, Rust and Node.js/TypeScript, see the [SDKs section](/docs/sdks/). For failures, quotas and response headers, see [Limits and errors](/docs/cloud/limits-and-errors/). API schemas and Markdown downloads are linked from the [Speedle Cloud overview](/docs/cloud/#downloads).
