+++
title = "Speedle Cloud"
description = "Autorización alojada, claves API de cuenta, límites y guías de integración."
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T21:00:00+08:00
+++

Speedle Cloud es un servicio de autorización alojado basado en el motor Speedle de código abierto. Gestione servicios y políticas SPDL de autorización y roles, pruebe decisiones en Playground y llame a la API desde su servidor.

**Speedle Cloud estará disponible próximamente.** Esta guía describe el flujo para cuando se habiliten el inicio de sesión en producción y las comprobaciones de autorización autenticadas; los ejemplos no se han ejecutado en un entorno Cloud real.

Para usar la consola o crear claves API debe estar disponible el inicio de sesión. La beta pública gratuita es para pruebas, aprendizaje y evaluación, no ofrece SLA y no se recomienda para producción. Las funciones y límites pueden cambiar.

## Documentación

- [Primeros pasos](/es/docs/cloud/getting-started/): inicie sesión, cree un servicio y una política en la consola, cree una clave API y compruebe la autorización desde su aplicación.
- [Claves API](/es/docs/cloud/tokens/): Credenciales de cuenta, ámbitos fijos, caducidad obligatoria, Crear y Revocar, secreto mostrado una vez y paginación.
- [API de autorización](/es/docs/cloud/authorization-api/): Solicitudes IsAllowed, identidad del emisor, Node.js 22 y restricciones de transporte.
- [Referencia de la API](/es/docs/cloud/api-reference/): comprobaciones de autorización y gestión de políticas con una API key. Las solicitudes salen de su servidor; esta página es de solo lectura.
- [Límites y errores](/es/docs/cloud/limits-and-errors/): Límites de cuenta, tamaños, ráfagas, cuota diaria, errores y restablecimiento.

## Descargas

- [Referencia de la API (Swagger)](/es/docs/cloud/api-reference/): Consulte en línea todos los endpoints, parámetros y ejemplos de respuesta.
- [Markdown en español](https://engine.speedle.io/docs/cloud/quickstart.es.md): Los cuatro capítulos de Cloud en una guía sin conexión.
- Especificación OpenAPI: [YAML](https://cloud.speedle.io/openapi.public.yaml) · [JSON](https://cloud.speedle.io/openapi.public.json), para generadores de código y clientes de API.

Consulte los documentos de Cloud: [Términos](https://cloud.speedle.io/es/terms) / [Política de privacidad](https://cloud.speedle.io/es/privacy).
