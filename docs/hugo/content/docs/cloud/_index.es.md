+++
title = "Speedle Cloud"
description = "Documentación de Speedle Cloud: primeros pasos, gestión de cuentas y políticas, tokens de aplicación, vista previa de la API de autorización, límites, errores y descargas."
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
+++

Speedle Cloud es un servicio de control de acceso hospedado para desarrolladores, creado sobre el motor de políticas de código abierto Speedle. Está diseñado para permitirle gestionar políticas de autorización y roles en SPDL, probar decisiones en un Playground e invocar una API de autorización desde el servidor de su aplicación sin necesidad de operar su propio servicio de autorización.

**Speedle Cloud estará disponible próximamente.** El encabezado de Cloud muestra la etiqueta **Próximamente** y el inicio de sesión para producción aún no está habilitado en [cloud.speedle.io](https://cloud.speedle.io/es/). El flujo de la consola y el Servicio de Decisiones de Autorización (ADS) autenticado descritos aquí son vistas previas del contrato de servicio publicado. La emisión de un token no habilitará llamadas de autorización del lado del servidor mientras el ADS autenticado continúe pendiente. Verifique la disponibilidad del despliegue y del proveedor de identidad en su entorno seleccionado.

La beta pública gratuita está pensada para pruebas, aprendizaje y evaluación. No incluye SLA, no está recomendada para producción y sus funcionalidades y límites están sujetos a cambios.

## Documentación

- [Primeros pasos](/es/docs/cloud/getting-started/): inicio de sesión con GitHub o Google (próximamente), gestión de cuentas, servicios, políticas SPDL, solicitudes de gestión de sesiones y las cuatro operaciones del Playground de sesión.
- [Tokens de aplicación](/es/docs/cloud/tokens/): requisitos de reautenticación reciente, divulgación única del token, paginación de metadatos, rotación y revocación.
- [Vista previa de la API de autorización](/es/docs/cloud/authorization-api/): contrato de solicitud y respuesta de IsAllowed, ejemplo con Node.js 22, instantáneas de políticas y restricciones de integración.
- [Límites y errores](/es/docs/cloud/limits-and-errors/): cuotas de cuenta, límites de solicitudes, respuestas de error, encabezados de tasa y cuota, y directrices de reintento.

<a id="downloads"></a>
## Descargas

- [Markdown en español](https://engine.speedle.io/docs/cloud/quickstart.es.md): los cuatro capítulos de Cloud en un único documento sin conexión.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): especificación canónica de la API de Cloud registrada.
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): generado a partir de la misma fuente YAML.
- [Versión de compilación en servicio](https://cloud.speedle.io/build.json): identifica la compilación en servicio; no certifica la disponibilidad del proveedor de identidad o del endpoint.

Los esquemas incluyen las cuatro operaciones del Playground de sesión y el contrato autenticado de ADS. La publicación de un esquema o la descarga de una guía no habilita un endpoint pendiente ni abre el inicio de sesión. El archivo YAML y el JSON generado se versionan, promueven y restauran junto con la compilación sellada de Cloud correspondiente. Las descargas de Markdown en español, coreano, japonés, inglés y chino se mantienen con estas páginas de documentación de Engine.

Consulte los [Términos](https://cloud.speedle.io/es/terms) y la [Política de privacidad](https://cloud.speedle.io/es/privacy) de Cloud.
