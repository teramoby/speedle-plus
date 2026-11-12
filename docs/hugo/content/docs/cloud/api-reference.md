+++
title = "API reference"
description = "Read-only Swagger UI for the public Speedle Cloud API. Try it out is disabled."
date = 2026-11-12T10:00:00+08:00
lastmod = 2026-11-12T10:00:00+08:00
weight = 35
draft = false
toc = false
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
bref = "The public Cloud API, rendered with Swagger UI. Try it out is disabled."
+++

This page renders the public Speedle Cloud API. The specification text is English on every language of this site. Try it out is turned off, so an application token is not entered in the browser.

The rendered file is [the vendored public subset](/openapi/cloud-public.yaml). This copy is a placeholder for the authorization-check path until Speedle Cloud publishes the public OpenAPI document. It is not the full checked-in specification linked from the [Cloud documentation index](/docs/cloud/).

[Authorization API preview](/docs/cloud/authorization-api/) explains the IsAllowed call. [Limits and errors](/docs/cloud/limits-and-errors/) lists quotas and error responses.

{{< swagger-spec url="/openapi/cloud-public.yaml" tryitout="false" >}}
