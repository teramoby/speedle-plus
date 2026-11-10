+++
title = "API de decisiones de autorización"
description = "Documentación de Swagger para el endpoint en tiempo de ejecución. Obtenga decisiones y roles y permisos concedidos; descubra políticas dinámicamente; depure la evaluación de sus políticas."
tags = ["v0.1.0", "api", "guide", "core", "ads"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
lastmod = 2026-11-09T21:18:26+08:00
+++

Referencia para su ADS configurado. Los atributos de solicitud son matrices tipadas; los atributos de diagnóstico son objetos.

[Configuración de seguridad y endpoints](/es/docs/security/). Esta es una referencia de contrato, no un backend hospedado por speedle.io.

{{< swagger-spec url="/swagger/policy-check.yaml" >}}

## Endpoints de comprobación de estado

`GET /healthz` informa sobre la vitalidad del proceso y siempre responde `200 {"status":"ok"}` mientras el proceso se encuentra en ejecución. No reinicia una instancia detenida.

`GET /readyz` informa si la instancia puede responder actualmente a una solicitud de autorización: `200` cuando está lista, `503` en caso contrario. El endpoint no requiere autenticación y solo devuelve recuentos agregados, por lo que no expone identidades de servicio, revisiones, motivos de parada o estado de revocación. Un almacenamiento sin soporte de distribución siempre está listo.
