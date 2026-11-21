+++
title = "Speedle Cloud Console user manual"
linkTitle = "Console manual"
description = "Task-oriented guide to the Speedle Cloud Console (SCC): workspaces, members, services, SPDL policies, Playground, API keys, functions, ADS fleet, audit logs and account settings."
date = 2026-11-21T00:00:00+08:00
lastmod = 2026-11-21T00:00:00+08:00
weight = 15
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Manage workspaces, services, policies, Playground tests, API keys, functions, ADS instances and audit logs in the Speedle Cloud Console."
+++

**Speedle Cloud is in public beta.** The free public beta is for evaluation and learning, has no SLA and is not recommended for production use. Features and limits may change.

This manual walks through everyday tasks in the **Speedle Cloud Console (SCC)** at [cloud.speedle.io](https://cloud.speedle.io/). For a quick end-to-end API call from your server, see [Getting started](/docs/cloud/getting-started/). For endpoint payloads, error codes and quotas, see the [Authorization API](/docs/cloud/authorization-api/), [API reference](/docs/cloud/api-reference/) and [Limits and errors](/docs/cloud/limits-and-errors/).

## Sign in and console layout

### Prerequisites

- A browser with JavaScript and cookies enabled.
- A GitHub or Google account with a verified email address.

### Sign in or create an account

1. Open [cloud.speedle.io](https://cloud.speedle.io/) and choose **Get started** (or open `/login` directly).
2. Choose **Sign in with GitHub** or **Sign in with Google** and complete authentication at the provider.
3. If this is your first time signing in with an unused verified email address, review the Terms of Service and Privacy Policy links and confirm **Create account**.

**Expected result:** The console opens your active workspace in **Services & policies** (at `/w/<workspaceId>/service`). New accounts start with a **Personal** workspace where your role is **Owner**.

**Recovery:**

- **Email already in use:** Sign in with the provider already linked to that email address, then connect the second provider in **Account settings**.
- **Verified email required:** Verify your email address in your GitHub or Google account settings, then start sign-in again.
- **Sign-ups are paused:** The beta has reached its new-account cap; existing accounts can still sign in.
- **Session expired:** Sessions expire after 7 days of inactivity or 30 days from creation, whichever comes first. Sign in again to continue.

### Navigate the console

The signed-in console has two navigation areas:

- **Top bar:**
  - **Speedle | Cloud** logo: returns to **Services & policies** in the active workspace.
  - **Workspace** switcher: displays the active workspace name and your role (**Owner** or **Admin**), switches between workspaces and opens **Create workspace**.
  - **Need help?** link, **Language** switcher and **Account** menu (email address, **Account overview**, **Account settings**, **Sign out** and **Sign out everywhere**).
- **Left sidebar:**
  - **Console:** **Services & policies**, **API keys**, **ADS fleet** and **Audit log** — all scoped to the active workspace.
  - **Workspace settings:** **Members** — view or manage workspace membership.
  - **Cloud docs:** opens this documentation.

## Personal identity and workspaces

Speedle Cloud separates your **personal account** from your **workspaces**:

- Your **personal account** holds your verified email address, connected sign-in providers (GitHub and/or Google), browser sessions and **Personal activity** log (`/account/audit`).
- A **workspace** is an isolated container for services, authorization and role policies (including **Global roles**), custom functions, API keys, ADS fleet instances, the workspace **Audit log** and daily API quota.

Every account starts with a **Personal** workspace. You can belong to up to 10 workspaces, and each workspace can have up to 10 members (counting pending invitations).

### Switch or create a workspace

1. In the top bar, open the **Workspace** menu.
2. Select another workspace to switch to it, or choose **Create workspace**.
3. When creating a workspace, enter a **Workspace name** (1–64 characters) and choose **Create workspace**.

**Expected result:** The new workspace starts empty with its own services, policies, API keys and daily quota, and you become its **Owner**. The URL updates to `/w/<workspaceId>/service`. Switching workspaces clears any one-time secrets currently shown on screen.

### Manage workspace members and roles

Open **Workspace settings → Members** in the left sidebar.

| Role | What the member can do |
| --- | --- |
| **Owner** | Everything an Admin can do, plus invite members, revoke pending invitations, promote or demote members and remove members. |
| **Admin** | Manage services, policies, imports/exports, functions, Playground, API keys and ADS fleet; view the member list and workspace audit log; leave the workspace. |

**Invite a member (Owner only):**

1. Under **Invite someone**, enter the person's **Email address** and choose a **Role** (**Admin** or **Owner**).
2. Choose **Send invitation**.

**Expected result:** The console sends an email containing a single-use invitation link valid for 7 days and adds the entry to **Pending invitations** (which counts toward the 10-member limit; each inviter can send up to 20 invitations per UTC day). Owners see full invited email addresses; Admins see masked addresses. To cancel a pending invite, choose **Revoke** next to it.

To accept an invitation, the recipient opens the emailed link, signs in with a GitHub or Google account that has that verified email address and chooses **Join workspace**.

**Change roles, remove a member or leave a workspace:**

- **Change role (Owner only):** Use the **Role** selector next to a member to promote an Admin to Owner or demote an Owner to Admin.
- **Remove a member (Owner only):** Choose **Remove**, review the warning and confirm **Remove member**. Their console access ends on their next request.
- **Leave workspace:** Choose **Leave workspace** and confirm.

> **API keys stay active when a member leaves or is removed.** API keys belong to the workspace, not to the person who created them. After you remove a member, the console lists any active API keys that person created (name, ID and expiry, without secrets). Review them on the **API keys** page and revoke or rotate any key that should no longer be trusted.

**Recovery:**

- **Verify your identity (`reauth_required`):** Changing roles, removing members and leaving a workspace require a sign-in within the last 10 minutes. Choose a connected provider to verify, then repeat the action.
- **Last owner (`last_owner`):** A workspace must keep at least one Owner. Promote another member to **Owner** before demoting yourself or leaving.
- **Invitation mismatch or invalid link:** Ask the invited person to sign in with an account where the invited email is verified (any verified GitHub email qualifies), or send a new invitation if the 7-day link expired or was revoked.

## Manage services and policies

The walkthrough below uses a small `books` service with one role assignment and one authorization policy.

### Create a service

1. Open **Services & policies** in the left sidebar.
2. If the workspace has no services yet, enter `books` as the **Service name** and choose **Create service**. If services already exist, open the **Current service** selector, choose **Add another service**, enter `books` and choose **Create service**.

Service names must be 1–64 characters using letters, numbers, dots, underscores or hyphens, starting with a letter or number. The name `global` is reserved for **Global roles**. Applications send this name as `serviceName` in authorization requests.

### Create authorization and role policies

1. With `books` selected, open the **Policies** tab and choose **Create policy**.
2. Optionally enter a **Policy name** (up to 128 Unicode characters), such as `reader-role`. Leave it empty to generate a name automatically.
3. In **SPDL source**, enter one SPDL statement (up to 1,024 UTF-8 bytes):

```text
grant user alice from corp Reader
```

4. Wait briefly for live validation. The editor validates automatically after you pause typing and displays the detected policy type (**Valid · role policy** or **Valid · authorization policy**) along with any syntax errors or review warnings (such as an ungranted role or an unanchored regular expression).
5. Choose **Save policy**.
6. Repeat to create an authorization policy (for example `read-books`) that grants the role permission:

```text
grant role Reader get /books
```

**Expected result:** Both policies appear in the policy table for `books`. Saved policies are available immediately in the **Playground** and propagate to the hosted authorization API in roughly 60 seconds.

You can switch between the code editor and **Use plain text editor**, search or filter the policy table by effect, principal, action, resource, role or condition, and open **Edit**, **Details**, **Test** or **Delete** from each row.

### Use Global roles across services

When multiple services in the same workspace share a role assignment, define it once in **Global roles**:

1. Open the **Current service** selector and choose **Global roles** at the top of the list.
2. Create a role policy, such as `grant user alice from corp BookAdmin`.
3. Switch to each ordinary service that should honor that role (such as `books`) and create an authorization policy granting permissions to `BookAdmin`.

**Global roles** accepts role policies only; it cannot be renamed or deleted. A global role grants no permissions until an ordinary service defines an authorization policy for that role. See [Global roles](/docs/cloud/global-roles/) for full rules and ADS sync behavior.

### Inspect relationships in the Access map

Open the **Access map** tab inside **Services & policies** to visualize how principals, roles and resources connect in the selected service's saved policies. You can filter the graph, switch to a table view, focus on a node and send a path directly to the **Playground** to run a **Diagnose** check.

### Import and export policies

Open the **Import & export** tab inside **Services & policies**:

- **Download SPDL** or **Download JSON:** Exports the selected service's saved policies. Keep downloaded files private and maintain your own backups during the beta.
- **Import policies:** Choose a Speedle Cloud JSON export file (up to 32 MiB), review the preview counts and choose **Confirm policy import**. Imports are atomic: existing policies are never overwritten, and any invalid policy, name conflict or excess over the 200-policy workspace cap rejects the entire batch.

If an import's outcome is interrupted or uncertain, keep the page open and use **Retry the same import** (it uses an idempotency key so confirmed policies are not inserted twice).

### Rename or delete a service

Use the **Service actions** menu next to the service header:

- **Rename…:** Changes the service name. Policy IDs, API keys and ADS instance identities stay intact, but calling applications must update `serviceName` to the new name and propagation is not immediate.
- **Delete service…:** Permanently deletes the service and all of its policies, and removes it from associated ADS instances. Type the service name to confirm. Deleting a service requires authentication within the last 10 minutes; if prompted, verify with a connected provider and confirm deletion again. Workspace API keys and ADS instances remain.

**Recovery:**

- **Editing cannot change policy type:** A saved authorization policy cannot be turned into a role policy (or vice versa) in place. Delete the policy and create a new one.
- **Stale policy edit:** If another session modified or deleted the policy while you were editing, your draft stays on screen. Refresh the workspace and open the latest version before saving.
- **Revision limit reached (`revision_exhausted`):** Export the service's policies, create a new service, import the export, switch callers to the new `serviceName` and optionally delete the old service. See [Limits and errors](/docs/cloud/limits-and-errors/#revision-exhausted).

## Test decisions in the Playground

1. Select `books` in **Services & policies** and open the **Playground** tab.
2. Choose an **Operation**:
   - **Is allowed:** Returns Allowed or Denied and the reason code.
   - **Diagnose:** Shows the decision plus a trace of how policies, roles and conditions contributed.
   - **All granted roles:** Lists all roles granted to the subject in this service (including Global roles).
   - **All granted permissions:** Lists all granted resources and actions for the subject.
3. Fill in the test inputs:
   - **Principals (JSON):** `[{"type":"user","name":"alice","idd":"corp"}]`
   - **Resource:** `/books`
   - **Action:** `get`
   - **Attributes (JSON):** `[]`
4. Choose **Run test**.

**Expected result:** **Is allowed** displays **Allowed.** with reason **A grant policy applies.** and the full JSON result. Changing **Action** to `delete` and running again displays **Denied.** with **No applicable policies were found.**

You can also click **Test** on a policy row or prefill a diagnosis from **Access map**; prefilling populates the inputs without spending an API call until you click **Run test**.

**Keep in mind:**

- Playground evaluates **saved** policies immediately; unsaved editor drafts are ignored.
- Each Playground run uses one API call from the workspace's daily quota and never modifies policies.
- Testing against **Global roles** itself always denies; test global roles in the Playground of an ordinary service.
- A passing Playground check does not prove that published ADS snapshots have finished propagating (allow ~60 seconds for the hosted [Authorization API](/docs/cloud/authorization-api/)).

## Manage workspace API keys

API keys authenticate server-side applications and self-hosted ADS instances to the active workspace. Managing keys always requires a signed-in console session.

### Create an API key

1. Open **API keys** in the left sidebar.
2. Choose **Create API key**.
3. Enter a descriptive **API key name** (1–128 characters, such as `books-app`). Do not enter a secret as the name.
4. Select a **Scope** (fixed at creation):
   - **Authorization checks only (ADS)** (`ads`, default): calls `/authz-check/v1/*` only.
   - **Policy management: read** (`pms:read`): reads workspace services and policies.
   - **Policy management: write** (`pms:write`): reads, creates and deletes workspace services and policies.
   - **ADS sync** (`pms:read+ads`): syncs policy snapshots for Speedle Cloud ADS (`speedle-cloud`), reads policies and runs authorization checks; no writes.
5. Select **Expires after** (`1 week`, `1 month`, `3 months` [default], `6 months` or `1 year`) and verify **Preview expiry (UTC)**. Non-expiring keys are not supported.
6. Choose **Create API key**. Creating a key requires provider verification within the last 10 minutes; if prompted, choose **Verify with GitHub** or **Verify with Google**, then submit **Create API key** again.
7. Copy the **One-time API key** immediately, store it in your server's secret manager (for example `SPEEDLE_API_KEY`) and choose **I have saved it — clear this display**.

**Expected result:** The key appears under **Active API keys** with its ID, scope, creation time, approximate last-used time and **Expires (UTC)**. The secret is never shown again and cannot be recovered from metadata.

### Rotate and revoke API keys

Each workspace can have up to **10 active API keys** across all scopes. Keys expiring within 7 days are labeled **Expires soon**; expired keys move to **Expired API keys**.

To rotate a key without downtime:

1. Create a replacement key with the required scope and expiry.
2. Update your application or ADS configuration to use the new key.
3. On the **API keys** page, choose **Revoke** next to the old key, confirm the name and ID, and choose **Revoke API key**.

**Recovery:**

- **Propagation delay:** Cached verification can remain valid for up to 60 seconds after revocation; revocation is not instantaneous worldwide.
- **Limit reached (`quota_exceeded`):** Revoke an unused key or let an expiring key lapse to free a slot. Revoked and expired keys remain listed for history and do not count toward the 10-key cap.
- **Uncertain creation or revocation:** If a network error interrupts creation or revocation, refresh the metadata list and check the active and history groups before acting again. Never assume a matching name identifies a lost secret.

For full API key contract details, see [API keys](/docs/cloud/tokens/).

## Check usage and quotas

In the public beta, resource caps and the daily API call allowance apply per workspace:

- **Services:** 100 per workspace (including **Global roles**).
- **Policies:** 200 authorization and role policies combined across all services in the workspace (each SPDL statement up to 1,024 UTF-8 bytes).
- **Active API keys:** 10 per workspace across all scopes.
- **Daily API calls:** 10,000 per workspace per UTC day (resets at `00:00 UTC`). Management reads/writes, live validation, Playground tests, imports/exports and authorization checks share this allowance.

Where to check usage in the console:

- **Account → Account overview (`/account`):** Displays the **Daily quota** card with a progress bar and the next UTC reset time for your Personal workspace.
- **Services & policies:** Displays service counts, policy counts and approximate remaining daily calls for the active workspace after workspace operations.

If you reach the daily allowance (`429 daily_quota_exceeded`), wait until `00:00 UTC`; switching API keys in the same workspace does not add quota. See [Limits and errors](/docs/cloud/limits-and-errors/) for burst rate limits and error recovery.

## Custom functions (per service)

> When custom functions are enabled on the stage, ordinary services display a **Functions** tab between **Access map** and **Playground**.

Custom functions let SPDL conditions call external HTTPS endpoints during evaluation. Each ordinary service can register up to **20 functions**.

### Register and manage a function

1. Select an ordinary service and open the **Functions** tab.
2. Choose **Register function**.
3. Configure:
   - **Name:** 1–64 character SPDL identifier (cannot be a reserved word).
   - **Endpoint URL:** HTTPS endpoint that receives condition arguments. The console itself never calls the endpoint.
   - **Invocation:** **Hosted and ADS** (callable by hosted Cloud evaluation, Playground and self-hosted ADS) or **ADS only** (called only by self-hosted ADS; hosted evaluation and Playground fail if a condition reaches an ADS-only function).
   - **Declared types (optional):** Add up to 16 **Parameters** and a **Result** type. Undeclared parameters skip type checking; declared type mismatches fail evaluation.
   - **Cache results (optional):** Evaluator-local cache TTL (1–86,400 seconds; failures are not cached).
   - **Hosted timeout (ms):** 100–2,000 ms (default 1,000 ms), applied to hosted calls only.
   - **Sign hosted requests (optional):** Enables HMAC signing for hosted calls.
4. Choose **Save function**. If signing is enabled, copy and store the one-time **Signing secret** before closing the dialog.

From the functions table, you can **Edit** a function, **Rotate signing secret** (the previous secret stops working immediately; requires recent re-authentication if prompted) or **Delete function**.

**Important rules:**

- Register a function in the target service **before** saving or importing a policy that calls it. Policy exports do not include function definitions.
- A function still referenced by a policy cannot be deleted; edit or delete the referencing policies first.

## Monitor and control the ADS fleet

Open **ADS fleet** in the left sidebar to monitor self-hosted Speedle Cloud ADS (`speedle-cloud`) instances that sync policies from the active workspace. Cloud never initiates connections into your network; commands travel over the channel each instance opens.

### Connect your first ADS instance

There is no manual registration step. When the fleet is empty, the **ADS fleet** page shows a three-step onboarding guide and a copyable configuration snippet:

1. Create an API key with the **ADS sync** (`pms:read+ads`) scope on the **API keys** page.
2. Put the key into your ADS configuration (`0600` file permissions) with a persistent `CacheDir` so restarts keep the same instance identity.
3. Start `speedle-cloud` with your configuration and application listener TLS settings. It appears automatically in **ADS fleet** on first connection.

Each workspace supports up to **10 online ADS instances** and keeps up to **100 instance records** (instances offline for 7 days are removed automatically).

### Inspect instances and run distribution actions

Use **Service filter** to view instances syncing a specific service, or **Refresh fleet** for an immediate update (the page auto-refreshes every 5 minutes while visible).

Choose **Details** on an instance row to inspect:

- **Live state**, **Last heartbeat received (UTC)**, **Channel** and **Transport** (WebSocket or Long poll).
- **Service set** with head revision, reported revision, sync lag and **Global roles** (always synced).
- **Distribution chain** (upstream and direct downstream instances, if any), **Event timeline**, **Fallback history** and reported **Host** details.

Available actions (each requires confirmation):

- **Assign services:** Sync **All services, including services created later** or **Only the selected services**. Changes apply on the next heartbeat without restarting the ADS.
- **Resync from a snapshot:** Discards the instance's local copy and reloads from a snapshot.
- **Pause distribution / Resume distribution:** Stops or resumes serving policies to downstream ADS instances while local sync and evaluation continue.
- **Stop this ADS:** Instructs the instance to deny authorization requests and stop reconnecting until restarted.
- **Delete this instance:** Removes the instance record and history (a still-running instance re-registers on its next connection unless stopped or its API key is revoked).

## Review audit logs and manage account settings

### Audit logs

Speedle Cloud keeps two separate audit logs, both newest first with **180-day retention** and pagination (**Previous page**, **Next page**, **Restart audit log**):

- **Workspace Audit log** (left sidebar → **Audit log**): Records administrative activity inside the active workspace — service and policy changes, API key creation/revocation, ADS actions, workspace creation/renaming and member/invitation changes. Readable by both Owners and Admins.
- **Personal activity** (top-bar **Account** menu → **Account overview** → **Personal activity**, at `/account/audit`): Records your personal account events — sign-in, sign-out, sign-out everywhere, connecting/disconnecting sign-in providers and account creation/deletion.

### Account settings

Open the top-bar **Account** menu and choose **Account settings** (`/account/settings`):

- **Connected identities:** Connect both **GitHub** and **Google** so you have a backup sign-in method. You must keep at least one connected identity; disconnecting a provider requires authentication within the last 10 minutes.
- **Sign out / Sign out everywhere:** Available from the top-bar **Account** menu to end either the current browser session or all active sessions.
- **Delete account:** Permanently deletes your personal account, connected identities, personal-only workspaces and sessions. Choose **Delete account**, verify your identity if prompted, type `DELETE` and confirm **Permanently delete account**. Export any policies you want to keep first.

**Recovery for account deletion:** If you are the sole **Owner** of a workspace that still has other members, account deletion is blocked (`last_owner`). Open that workspace's **Members** page and promote another member to **Owner** before deleting your account.
