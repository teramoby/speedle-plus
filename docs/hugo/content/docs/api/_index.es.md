+++
title = "API"
description = "Documentación de especificación OpenAPI de Speedle"
weight = 300
draft = false
toc = false
tocheading = "h4"
tocsidebar = false
tags = ["v0.1.0", "api", "guide", "core"]
categories = ["docs"]
bref = "Documentación de la especificación OpenAPI. Utilice la API para modelar sus políticas simples y complejas de forma declarativa; obtenga decisiones, descubra políticas dinámicamente y depure la evaluación de políticas."
lastmod = 2026-11-10T14:45:43+08:00
+++

Las bibliotecas cliente para Java, Python, Go, .NET, Rust y Node.js/TypeScript se encuentran en [SDK](/es/docs/sdks/).

## APIs REST

Estos son contratos de referencia para **su punto de enlace configurado de PMS/ADS o asertor**. `speedle.io` proporciona documentación y el Playground en el navegador; no es un servicio PMS/ADS hospedado. Envíe JSON con `Content-Type: application/json` y configure [TLS/mTLS y control de acceso del cliente](/es/docs/security/).

- [API de gestión de políticas](/es/docs/api/management_api/)
- [API de decisiones de autorización](/es/docs/api/decision_api/)
- [Webhook de aserción de tokens](/es/docs/api/asserter_api/)

Los identificadores de políticas y políticas de roles son cadenas de texto. Las respuestas de PMS son objetos planos; los permisos de autorización utilizan `permissions` (en plural). Las solicitudes de decisión REST utilizan una matriz de atributos tipados, mientras que los atributos de respuesta de diagnóstico y los atributos del webhook son objetos.

## APIs gRPC

- [Proto de PMS](/protobuf/pms.proto): `pb.PolicyManager`
- [Proto de ADS](/protobuf/ads.proto): `pb.Evaluator`

Utilice el archivo proto actual junto con el cliente generado. PMS utiliza `pb.PolicyManager`, y ADS utiliza `pb.Evaluator`. Ambos contratos históricos reutilizan nombres de mensajes y deben generarse/cargarse por separado; no los combine en un único conjunto de descriptores. gRPC está deshabilitado a menos que el servidor se inicie con `--grpc-endpoint`, y utiliza la misma configuración de TLS que REST.

El mapa heredado `ContextRequest.attributes` de gRPC contiene cadenas de texto. No utiliza el formato de matriz de atributos tipados de REST y no ofrece tipos numéricos, booleanos o de fecha. Utilice REST o Go en modo integrado para condiciones que requieran dichos tipos, a menos que su adaptador gRPC implemente explícitamente la conversión requerida.

## APIs de Go en modo integrado

- [Interfaces de gestión de políticas](https://github.com/teramoby/speedle-plus/tree/master/api/pms)
- [Interfaces de decisiones de autorización](https://github.com/teramoby/speedle-plus/tree/master/api/ads)

Consulte el [modo integrado](/es/docs/deployment/embedded/) para ver un ejemplo completo del ciclo de vida de recursos y aplicaciones.
