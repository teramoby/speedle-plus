+++
title = "Policy Management API"
keywords = ["Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "PAP", "policy administration point", "authorization policy management", "API authorization"]
description = "Swagger documentation for the management endpoint. CRUD on services, functions, policies and role-policies. Handle your simple and complex usecases using a simple declarative way!"
tags = ["v0.1.0", "api", "guide", "core", "pms"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
+++

Reference for your configured PMS. Requests and responses are JSON; policy IDs are strings and responses are flat objects.

[Security and endpoint configuration](/docs/security/). This is a contract reference, not a backend hosted by speedle.io.

{{< swagger-spec url="/swagger/policy-manage.yaml" >}}
