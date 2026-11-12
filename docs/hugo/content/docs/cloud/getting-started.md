+++
title = "Getting started with Speedle Cloud"
linkTitle = "Getting started"
description = "Prepare for Speedle Cloud: sign in, save an SPDL policy, check authorization from your application with an API key, and test in the Playground."
weight = 10
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Sign in, save a policy, create an API key, and check authorization from your application."
lastmod = 2026-11-12T00:00:00+08:00
+++

**Speedle Cloud is coming soon.** This guide describes the workflow available when production sign-in and authenticated authorization checks open; the examples have not been run against a live Cloud environment.

## Sign in with GitHub or Google (opens soon)

Open the [Cloud console](https://cloud.speedle.io/) in your browser and sign in with an available provider. For a new identity with an unused verified email address, confirm **Create account**. If the email is already in use, sign in with the linked provider shown by the console.

## Create a service and a policy

Open the [Service workspace](https://cloud.speedle.io/service), enter `books`, and choose **Create service**. Use a new service for this example so existing policies do not affect the results; do not overwrite existing data.

Choose **New policy**, name it `read-books`, and enter this SPDL:

```text
grant user alice from corp get /books
```

This lets `alice` in the `corp` identity domain perform `get` on `/books`. A grant without `from` matches the name in every domain. Derive the domain from trusted authentication data and send it as `idd` in each request. See [identity domains](/docs/idd/).

Choose **Validate policy**, then **Save policy**. Validation checks syntax but does not save the draft. Allow approximately 60 seconds for saved policies to reach the authorization API.

## Create an API key

Open [API keys](https://cloud.speedle.io/tokens), enter a name such as `books-app`, select the `ads` scope (authorization checks), choose an expiry (the console defaults to 3 months and shows the UTC time), and choose **Create API key**. Complete provider verification if prompted, then submit the form again. Copy the secret when it appears: it is shown only once. Every key expires; revoke it earlier when it is no longer needed. See [API key management](/docs/cloud/tokens/).

Store the key in your server's secret store and provide it through the `SPEEDLE_API_KEY` environment variable. Keep it out of browser code and source control.

## Check authorization from your application

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

See the [authorization API](/docs/cloud/authorization-api/) and the [SDKs](/docs/sdks/), including [Node.js/TypeScript](/docs/sdks/node/), [Python](/docs/sdks/python/), [Java](/docs/sdks/java/) and [Go](/docs/sdks/go/), for integration details.

## Test saved policies in the Playground

Optionally, open the service's **Playground** and select **Is allowed**. Enter `[{"type":"user","name":"alice","idd":"corp"}]` in **Principals (JSON)**, `/books` in **Resource**, `get` in **Action**, and `[]` in **Attributes (JSON)**, then choose **Run test**.

**Is allowed** shows the decision; **Diagnose** explains how policies contributed; **All granted roles** lists the subject's roles; **All granted permissions** lists granted resources and actions. Tests use saved policies, not an unsaved draft. Run the test again after changing inputs or policies. Applications use the [ADS authorization API](/docs/cloud/authorization-api/) with an API key.

Next steps: for programmatic management, create a separate key with the `pms:write` scope (or `pms:read` for reads), then use the [PMS API or spctl (Speedle Cloud)](/docs/pms/policy-mgmt/#speedle-cloud). The `ads` key from the previous step cannot manage policies.
