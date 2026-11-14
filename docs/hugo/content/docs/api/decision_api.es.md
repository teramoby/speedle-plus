+++
title = "API de decisiones de autorización"
description = "Documentación de Swagger para el endpoint en tiempo de ejecución. Obtenga decisiones y roles y permisos concedidos; descubra políticas dinámicamente; depure la evaluación de sus políticas."
tags = ["v0.1.0", "api", "guide", "core", "ads"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
lastmod = 2026-11-12T10:00:00+08:00
+++

Referencia para su ADS configurado. Los atributos de solicitud son matrices tipadas; los atributos de diagnóstico son objetos.

[Configuración de seguridad y endpoints](/es/docs/security/). Esta es una referencia de contrato, no un backend hospedado por speedle.io. Try it out está desactivado: la especificación no indica un host de servidor, así que una petición se enviaría a este sitio de documentación.

{{< swagger-spec url="/swagger/policy-check.yaml" tryitout="false" >}}

## Endpoints de comprobación de estado

`GET /healthz` informa sobre la vitalidad del proceso y siempre responde `200 {"status":"ok"}` mientras el proceso se encuentra en ejecución.

`GET /readyz` no requiere autenticación y devuelve `200 {"ready":true,"servicesTotal":0,"servicesUsable":0}` para el servicio de decisiones local. Los contadores permanecen en cero; la sonda no enumera los servicios almacenados.
