+++
title = "Policy Management API"
description = "Swagger documentation for the management endpoint. CRUD on services, functions, policies and role-policies. Handle your simple and complex usecases using a simple declarative way!"
tags = ["v0.1.0", "api", "guide", "core", "pms"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
lastmod = 2026-11-12T10:00:00+08:00
+++

Reference for your configured PMS. Requests and responses are JSON; policy IDs are strings and responses are flat objects.

[Security and endpoint configuration](/docs/security/). This is a contract reference, not a backend hosted by speedle.io. Try it out is disabled because this specification has no server host, so a request would otherwise be sent to this documentation site.

{{< swagger-spec url="/swagger/policy-manage.yaml" tryitout="false" >}}
