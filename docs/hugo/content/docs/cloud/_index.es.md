+++
title = "Speedle Cloud"
description = "Autorización alojada, espacios de trabajo, manual de consola, claves API, límites y guías de integración."
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-21T00:00:00+08:00
+++

Speedle Cloud es un servicio de autorización alojado basado en el motor Speedle de código abierto. Gestione espacios de trabajo, servicios y políticas SPDL de autorización y roles, pruebe decisiones en Playground y llame a la API desde su servidor.

La sincronización de políticas desde Speedle Cloud usa Speedle Cloud ADS: el binario y la imagen `speedle-cloud`. El binario `speedle-ads` de Speedle Engine autoalojado ya no proporciona un almacén `policy-source`.

**Speedle Cloud está en beta pública.** Esta guía describe el flujo de trabajo del servicio en línea.

Inicie sesión para usar la consola y crear claves API. La beta pública gratuita es para pruebas, aprendizaje y evaluación, no ofrece SLA y no se recomienda para producción. Las funciones y límites pueden cambiar.

## Documentación

- [Primeros pasos](/es/docs/cloud/getting-started/): inicie sesión, cree un servicio y una política en la consola, cree una clave API y compruebe la autorización desde su aplicación.
- [Manual de consola](/es/docs/cloud/console/): Guía práctica de Speedle Cloud Console (SCC): espacios de trabajo, miembros, servicios, políticas, Playground, claves de API, funciones personalizadas, flota de ADS, registros de auditoría y configuración de la cuenta.
- [Claves API](/es/docs/cloud/tokens/): Credenciales de espacio de trabajo, ámbitos fijos, caducidad obligatoria, Crear y Revocar, secreto mostrado una vez y paginación.
- [Roles globales](/es/docs/cloud/global-roles/): Asigne un rol una vez para todos los servicios del espacio de trabajo y conceda sus permisos en cada servicio.
- [API de autorización](/es/docs/cloud/authorization-api/): Solicitudes IsAllowed, identidad del emisor, Node.js 22 y restricciones de transporte.
- [Referencia de la API](/es/docs/cloud/api-reference/): comprobaciones de autorización y gestión de políticas con una API key. Las solicitudes salen de su servidor; esta página es de solo lectura.
- [Límites y errores](/es/docs/cloud/limits-and-errors/): Límites del espacio de trabajo, tamaños, ráfagas, cuota diaria, errores y restablecimiento.

## Descargas

- [Referencia de la API (Swagger)](/es/docs/cloud/api-reference/): Consulte en línea todos los endpoints, parámetros y ejemplos de respuesta.
- [Markdown en español](https://engine.speedle.io/docs/cloud/quickstart.es.md): Los cuatro capítulos de Cloud en una guía sin conexión.
- Especificación OpenAPI: [YAML](https://cloud.speedle.io/openapi.public.yaml) · [JSON](https://cloud.speedle.io/openapi.public.json), para generadores de código y clientes de API.

Consulte los documentos de Cloud: [Términos](https://cloud.speedle.io/es/terms) / [Política de privacidad](https://cloud.speedle.io/es/privacy).
