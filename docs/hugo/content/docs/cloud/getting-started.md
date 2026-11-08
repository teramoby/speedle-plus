+++
title = "Getting started with Speedle Cloud"
linkTitle = "Getting started"
description = "Prepare for Speedle Cloud: sign-in and account management, service and SPDL policy management, session API requests and Playground tests."
weight = 10
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "The published Speedle Cloud console workflow, from sign-in and account settings to saved policies and Playground decisions."
+++

**Speedle Cloud is coming soon.** Production sign-in is not open on cloud.speedle.io yet, and the Cloud header shows **Coming soon**. When sign-in is closed, the sign-in page says that it opens soon. This guide describes the published workflow rather than a currently open production console; screenshots will be added after production sign-in opens.

## Production origin

Use `https://cloud.speedle.io` for account sessions, management requests and application tokens. Accounts, resources and tokens belong to that origin. The examples in this section are illustrative and were not executed against a live environment. Confirm that your chosen stage is deployed and that the identity provider you intend to use is available there.

## Sign in with GitHub or Google (opens soon)

The sign-in page shows **Sign in with GitHub** and **Sign in with Google** even while sign-in is disabled. When production sign-in opens, choose an available provider. A new identity whose verified email address is unused requires an explicit **Create account** confirmation. If the email address is already in use, sign in with the linked provider identified by the console, then connect the second identity in **Account settings**. Accounts are not merged automatically.

Account settings let you inspect connected identities and connect or disconnect an available provider; keep at least one connected identity. **Sign out** is in the header beside the language switch, and the Account menu offers **Sign out everywhere**. Successful sign-out returns to the sign-in page and clears the workspace and any one-time token display. The audit log shows account activity.

Deleting an account requires recent provider authentication and typing `DELETE`. It permanently removes the account's services, policies, tokens and identities and ends its sessions. Export any policies you need from the Service workspace before deletion.

## Create a service and a policy

For a new account, open the [Service workspace](https://cloud.speedle.io/service), enter `books` as the service name, and choose **Create service**. Each account has one service. If the account already has a service or policies, inspect them and use an appropriate existing configuration; do not overwrite or delete existing data to make this example work.

Choose **New policy**, enter `read-books` as the policy name, and put this authorization policy in the SPDL editor:

```text
grant user alice from corp get /books
```

`from corp` keeps this grant inside the `corp` identity domain. Replace `corp` with the domain your application derives from trusted authentication data, and send the same value as the principal's `idd` in each request, as the Playground examples below do. A grant without `from` matches a user named `alice` in every domain; omit the clause only for a service with a single identity source, where names cannot collide across domains. See [identity domain](/docs/idd/) for the rule and the matching request field.

Choose **Validate policy** and check that the console reports **Detected: authorization policy**, then choose **Save policy**. Validation detects the policy type and checks syntax; it does not save the draft. Saving is a separate, manual action. You can also import existing policies in the workspace instead of entering them one by one.

Speedle Cloud supports authorization policies and role policies; custom external functions are not supported. The policy list groups the two types and shows their counts. Role assignments use these forms:

- Global role: `grant user alice editor`
- Resource-scoped role: `grant user alice editor on /books`

The ambiguous statement `grant user alice role editor` is rejected. Omit the optional `role` prefix for a role assignment, or write `grant (user alice) role editor` for an authorization policy. Editing retains the stored policy type. To change the type, delete the policy and create a new one.

If the console reports a conflict or an uncertain response, keep your draft and refresh the saved data before choosing another mutation. Changing language preserves drafts. Monaco's built-in menus keep the language selected when the editor first loaded and change after a page reload.

Service deletion requires you to type the service name, complete recent provider authentication, and confirm again after verification.

## Send management session requests

The Policy Management Service (PMS) uses the account session. Let a supported same-origin client manage its secure session cookie; do not extract cookies or use a test-provider bootstrap. Mutating requests require the exact stage `Origin`, `X-SC-CSRF: 1`, and a JSON content type. An ADS Bearer token cannot authorize PMS. Clients must not send account or service ownership selectors, or authentication timestamps.

The equivalent requests for the service and policy example are:

| Action | Session request | Success |
| --- | --- | --- |
| Create the account's service | `POST /api/service` with `{"name":"books"}` | 201 |
| Add an authorization policy | `POST /api/service/policies` with the body below | 201 |

```json
{"name":"read-books","spdl":"grant user alice from corp get /books"}
```

Paste only the `spdl` value into the editor when using the console. The JSON body is for the management API.

## Validate and export policies

Validate SPDL with `POST /api/service/validate` and a `{spdl}` body to detect its type. The legacy `{kind: "authz", spdl}` form still selects the authorization parser. **Download SPDL** and **Download JSON** call `GET /api/service/export?format=spdl` and `GET /api/service/export?format=json`, respectively.

The session Playground evaluates current account-owned D1 data. The future authenticated ADS release evaluates published policy snapshots, whose propagation takes approximately 60 seconds. A successful Playground result therefore does not prove that every ADS location has received the new snapshot.

## Test saved policies in the Playground

In the Service workspace, scroll to **Playground** and select **Is allowed**. Enter `[{"type":"user","name":"alice","idd":"corp"}]` in **Principals (JSON)**, `/books` in **Resource**, `get` in **Action**, and `[]` in **Attributes (JSON)**, then choose **Run test**.

The other operations are **Diagnose**, **All granted roles**, and **All granted permissions**. Playground runs use saved policies, not an unsaved editor draft. Changing the input, operation or language does not automatically send a request. Run the test again after changing inputs or saved policies. The console marks the earlier result as stale and keeps it visible if a later request fails.

## Use the session Playground API

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

Next, read about [application tokens](/docs/cloud/tokens/), the [authorization API preview](/docs/cloud/authorization-api/), and [limits and errors](/docs/cloud/limits-and-errors/).
