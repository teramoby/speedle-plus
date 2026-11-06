+++
title = "Getting started with Speedle Cloud"
linkTitle = "Getting started"
description = "Follow the planned Speedle Cloud workflow: sign in, create a service, write SPDL policies, test decisions, create an API token and call IsAllowed."
weight = 10
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "A preview of the Speedle Cloud workflow, from sign-in and SPDL policies to authorization checks."
+++

**Speedle Cloud is coming soon.** Sign-in is not open on cloud.speedle.io yet, and authenticated authorization API calls are still being rolled out. The steps below describe the planned production workflow. Screenshots will be added after production sign-in opens; no mock-up images are used.

## 1. Sign in with GitHub or Google

Open [Speedle Cloud](https://cloud.speedle.io/). When production sign-in opens, choose **Sign in with GitHub** or **Sign in with Google**. If you are creating a new account, explicitly confirm **Create account**.

> **Screenshot coming soon**
>
> The production sign-in screen will be added here.

## 2. Create a service

Open the Service workspace, enter a service name such as `books`, and choose **Create service**. The service groups your application's authorization and role policies. Each free account has one service.

> **Screenshot coming soon**
>
> The service creation screen will be added here.

## 3. Write or import SPDL policies

In the Service workspace, choose **New policy**. Write SPDL in the console, choose **Validate policy** to check the syntax, and then choose **Save policy**. Validation does not save the policy. You can also import existing policies in the workspace instead of entering them one by one.

For example, this authorization policy allows user `alice` to perform `get` on `/books`:

`grant user alice get /books`

Speedle Cloud supports authorization policies and role policies. Custom external functions are not supported.

> **Screenshot coming soon**
>
> The policy editor and import controls will be added here.

## 4. Try decisions in the Playground

In the Service workspace, scroll to **Playground** and select **Is allowed**. Enter `[{"type":"user","name":"alice"}]` for Principals, `/books` for Resource, and `get` for Action, then choose **Run test**.

The Playground evaluates saved policies; an unsaved editor draft is not included. After changing a policy or request, run the test again to refresh the result.

> **Screenshot coming soon**
>
> A Playground request and its decision will be added here.

## 5. Create an API token

Open **Application tokens**, enter a token name, and choose **Create token**. Recent GitHub or Google authentication may be required. Copy the token when it is shown once and store it in your application's server-side secret store; it cannot be retrieved again later.

The token format is `spdl_<canonical lowercase UUIDv7 id>_<43 base62 digits>`. It is a service-scoped Bearer credential for authorization checks, not an end-user identity token or a credential for the management API. Never expose it in browser code, URLs, source control, screenshots or logs.

> **Screenshot coming soon**
>
> The token creation and one-time disclosure screens will be added here.

## 6. Call the authorization API

When authenticated authorization calls are available, your application server will send a `POST` request to `https://cloud.speedle.io/authz-check/v1/is-allowed` with the issued token in a single `Authorization: Bearer` header:

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <issued token>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

Your application must authenticate its users and supply their principals directly. Token assertion is not supported. Check the `allowed` value in the response before performing the protected action, and fail closed on errors.

For client libraries in Java, Python, Go, .NET, Rust and Node.js/TypeScript, see the [SDKs section](/docs/sdks/).

> **Screenshot coming soon**
>
> The production API integration flow will be added here.

## Beta limits

| Limit | Beta allowance |
| --- | --- |
| Policies | Up to 200 policies per account, including authorization and role policies combined. |
| API calls | Up to 10,000 API calls per day per account, including management and authorization calls combined. Usage resets at 00:00 UTC. |

The free Beta is for trials, learning and evaluation. There is no SLA, and the Beta is not recommended for production. Features and limits may change.

## Beta terms and privacy

Read the [Beta terms](https://cloud.speedle.io/terms) and [Privacy policy](https://cloud.speedle.io/privacy) on Speedle Cloud.
