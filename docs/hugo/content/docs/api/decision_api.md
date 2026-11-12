+++
title = "Authorization Runtime/Decision API"
description = "Swagger documentation for the runtime endpoint. Get decisions and granted roles and permissions; Discover policies dynamically; Debug your policy evaluation"
tags = ["v0.1.0", "api", "guide", "core", "ads"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
lastmod = 2026-11-12T10:00:00+08:00
+++

Reference for your configured ADS. Request attributes are typed arrays; diagnostic attributes are objects.

[Security and endpoint configuration](/docs/security/). This is a contract reference, not a backend hosted by speedle.io. Try it out is disabled because this specification has no server host, so a request would otherwise be sent to this documentation site.

{{< swagger-spec url="/swagger/policy-check.yaml" tryitout="false" >}}

## Health endpoints

`GET /healthz` reports process liveness and always answers `200 {"status":"ok"}` while the process runs. It does not restart a stopped instance.

`GET /readyz` reports whether the instance can currently answer an authorization request: `200` when ready, `503` otherwise. The endpoint is unauthenticated and returns aggregate counts only, so it does not expose service identities, revisions, stop reasons or revocation state. A store without distribution support is always ready.
