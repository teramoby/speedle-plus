+++
title = "Authorization Runtime/Decision API"
keywords = ["Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "PDP", "policy decision point", "authorization decision service", "API authorization"]
description = "Swagger documentation for the runtime endpoint. Get decisions and granted roles and permissions; Discover policies dynamically; Debug your policy evaluation"
tags = ["v0.1.0", "api", "guide", "core", "ads"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
+++

Reference for your configured ADS. Request attributes are typed arrays; diagnostic attributes are objects.

[Security and endpoint configuration](/docs/security/). This is a contract reference, not a backend hosted by speedle.io.

{{< swagger-spec url="/swagger/policy-check.yaml" >}}
