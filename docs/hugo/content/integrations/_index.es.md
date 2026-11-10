+++
title = "Integraciones"
noindex = false
categories = ["docs"]
description = "Conecte Speedle con sus aplicaciones e infraestructura."
aliases = ["/es/integrations/istio/", "/es/integrations/kubernetes/", "/es/integrations/docker/"]
+++

Speedle toma decisiones de autorización. Un punto de aplicación de políticas (PEP) en su aplicación, puerta de enlace o plataforma llama a Speedle para cada decisión y aplica el resultado. Las siguientes interfaces son compatibles actualmente.

## APIs REST y gRPC

Gestione servicios, políticas y roles a través del Servicio de Gestión de Políticas (PMS) y solicite decisiones de tiempo de ejecución del Servicio de Decisiones de Autorización (ADS). Ambos exponen APIs REST; inicie cualquiera de los servidores con `--grpc-endpoint` para habilitar su API gRPC.

- [API de gestión de políticas](/es/docs/api/management_api/)
- [API de decisiones de autorización](/es/docs/api/decision_api/)
- [Webhook de aserción de tokens](/es/docs/api/asserter_api/)
- [Descripción general de la API (incluidos protos de PMS y ADS)](/es/docs/api/)

## Go integrado

Ejecute el evaluador dentro de una aplicación Go, sin un proceso PMS o ADS independiente. La [guía de modo integrado](/es/docs/deployment/embedded/) crea un ejemplo completo con módulos Go.

## SDKs

Los SDKs de cliente están disponibles para Java, Go, Python, Node.js/TypeScript, Rust y C#/.NET. Consulte [SDKs](/es/docs/sdks/).

## Almacenes de políticas

Utilice el almacenamiento integrado en archivos, etcd, MySQL o MongoDB, o implemente las interfaces de almacenamiento para otro backend. Consulte [Almacenamiento conectable](/es/docs/store/).

## Guías anteriores

Las guías de ejemplo anteriores de Istio, Kubernetes y Docker ya no se publican. Utilice las interfaces anteriores para nuevas integraciones.
