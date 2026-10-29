+++
title = "Policy Management Console"
description = "Manage policies, analyse roles and diagnose decisions in the browser"
weight = 65
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["console", "guide", "pms", "ads"]
categories = ["docs"]
bref = "speedle-console is a web console for policies and role policies, role analysis and request diagnosis. It signs users in through the token asserter and lets Speedle policies decide what they may do."
+++

## What the console does

`speedle-console` is a separate server that serves a web page and forwards its calls to PMS and ADS. It offers:

- **Services, policies and role policies**: list, filter, create, edit and delete. Edits use `PUT` with the item's `ETag`, so a change made by someone else in the meantime is reported instead of overwritten.
- **Role analysis**: a graph of who is granted which role, directly or through other roles, including the role policies of the `global` service. It flags cycles, roles that policies use but nothing grants, and roles that are granted but never used. Select a role to see who holds it, through which roles, and whether only under a condition or resource scope.
- **Diagnosis**: send a request to ADS and see the decision, the reason, the granted roles and every role policy and policy that applied, with condition results.

The page is available in English and Chinese.

## Keep PMS and ADS private

PMS has no authentication, and it records the author of a change from the `Speedle-Principals` header that callers send. The console signs users in and checks their permissions, but anyone who can reach PMS directly can change any policy. **Expose only the console. Keep PMS and ADS reachable only from the console and your applications**: use network policies, or [mutual TLS](/docs/security/) with `forceClientCert` and give the console a client certificate.

## Try it locally

Start PMS and ADS as in the [quick start](/quick-start/), then build and run the console in development mode. Development mode has no sign-in: everyone is an administrator, so it only listens on a loopback address.

```bash
go build -o ./bin/speedle-console ./cmd/speedle-console
./bin/speedle-console --dev-no-auth --insecure --endpoint 127.0.0.1:6735 --pms-url http://127.0.0.1:6733 --ads-url http://127.0.0.1:6734
```

Open `http://127.0.0.1:6735/console/`.

## Sign-in and permissions

Users sign in with a token. The console sends it to the [token asserter](/docs/assertor/), the same webhook ADS uses, and keeps the principals it returns in a signed, `HttpOnly`, `SameSite=Strict` cookie. The token itself is not kept. Every change also needs a per-session CSRF token, which the page sends for you.

What a user may do is decided by Speedle itself. For each API call the console asks ADS whether the user's principals may perform an action on the resource `console` in the service `speedle-console` (change it with `--authz-service`):

| Action | Allows |
|---|---|
| `read` | Viewing services and policies, role analysis and diagnosis |
| `write` | Creating, changing and deleting services, policies and role policies |

If ADS cannot answer, the console refuses the request (HTTP 503). Answers are cached for `--authz-cache-ttl` (5 seconds by default).

## Write the first console policies

On a new installation nobody has `write` yet. Start the console once with `--bootstrap-admins user:alice` (a comma-separated list of `type:name` principals). Those principals get full access without asking ADS, and the console warns about it. Then write the console policies, in the console or with spctl:

```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service speedle-console
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy console-admins -c "grant user alice console-admin" --service-name speedle-console
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy console-admin -c "grant role console-admin read,write console" --service-name speedle-console
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy console-readers -c "grant group auditors read console" --service-name speedle-console
```

Restart the console without `--bootstrap-admins`. If a mistake in these policies locks everyone out, restart once more with `--bootstrap-admins` to fix them. The console warns before anyone edits the `speedle-console` service.

## Settings

Every flag can also be set with an environment variable: `--pms-url` is `SPEEDLE_CONSOLE_PMS_URL`. Flags on the command line win.

| Flag | Default | Meaning |
|---|---|---|
| `--endpoint` | `0.0.0.0:6735` | Address the console listens on |
| `--insecure` | `false` | Serve plain HTTP instead of HTTPS |
| `--cert`, `--key` | | The console's TLS certificate and key |
| `--secure-cookie` | `false` | Mark the session cookie `Secure` while serving plain HTTP behind a proxy that terminates TLS |
| `--pms-url`, `--ads-url` | | Base URLs of PMS and ADS, such as `https://pms:6733` |
| `--upstream-ca` | | CA that verifies PMS and ADS |
| `--upstream-cert`, `--upstream-key` | | Client certificate for PMS and ADS |
| `--upstream-timeout` | `30s` | Timeout of calls to PMS and ADS |
| `--asserter-endpoint` | | Token asserter webhook; required unless `--dev-no-auth` |
| `--asserter-ca`, `--asserter-cert`, `--asserter-key` | | TLS settings for the token asserter |
| `--session-key-file` | | Key of 32 bytes or more that signs session cookies. Without it a random key is used: sessions end when the console restarts, and only one replica works |
| `--session-ttl` | `8h` | How long a sign-in lasts |
| `--authz-service` | `speedle-console` | Service holding the console policies |
| `--authz-cache-ttl` | `5s` | How long permission answers are cached |
| `--bootstrap-admins` | | Principals with full access without policies |
| `--dev-no-auth` | `false` | No sign-in, everyone is an administrator; loopback addresses only |

## Deploy with Helm or Docker

`make buildConsole imageConsole` builds the binary and the `speedle-console` image (port 6735). Both Helm charts in `deployment/helm` have a `console` section, off by default:

- `speedle-dev` runs the console in the PMS and ADS pod. Set `console.enabled=true` and `console.asserterEndpoint`.
- `speedle-prod` runs it as its own deployment and service. It also takes `console.sessionKeySecret` (a secret with `session.key`, needed for more than one replica), `console.tls` for the console's own certificate, and `console.upstreamTLS` for the CA and client certificate it uses with PMS and ADS.

## Troubleshooting

| Symptom | Cause |
|---|---|
| Sign-in fails with "The token was not accepted" | The token asserter rejected the token or returned no principals |
| "You are signed in, but no policy lets you read the console" | No policy in `speedle-console` grants `read` on `console` to the user's principals |
| 503 "The authorization service is unavailable" | The console cannot reach ADS; it denies everything until ADS answers |
| 502 "The policy service is unreachable" | The console cannot reach PMS |
| "Someone changed this item after you opened it" | The item changed since it was loaded; reload it and edit again |
