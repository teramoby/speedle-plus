+++
title = "Límites y errores de Speedle Cloud"
linkTitle = "Límites y errores"
description = "Cuotas de cuenta de Speedle Cloud, límites de solicitudes, errores en formato JSON plano, encabezados de tasa y cuota, contabilidad diaria y directrices de reintento."
date = 2026-11-08T00:00:00+08:00
lastmod = 2026-11-08T17:11:18+08:00
weight = 40
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Planifique en función de los límites globales de la cuenta y falle de forma restrictiva (fail closed) cuando Cloud no pueda devolver una decisión de autorización válida."
+++

Los errores y encabezados de ADS a continuación describen el contrato de ADS autenticado, cuyo entorno de ejecución aún no se encuentra disponible. El contrato de gestión ya define límites de cuenta, comprobaciones de sesión y CSRF, y errores de gestión de tokens para su uso cuando el acceso a cuentas esté habilitado. Descargar un esquema no habilita un endpoint pendiente.

## Límites

| Límite | Significado |
| --- | --- |
| 10.000 llamadas al día | Llamadas coincidentes a controladores de PMS y ADS a nivel de cuenta por día UTC, en todas las sesiones, tokens y servicios; se reinicia a las 00:00 UTC. |
| 200 políticas | Políticas de autorización y roles combinadas por cuenta. |
| 4 KiB de SPDL | 4.096 bytes UTF-8 por política completa, no 4.096 caracteres. |
| 2 tokens activos | Por servicio; el historial de revocaciones no consume este límite. |
| 600 solicitudes por minuto por token | Límite de ráfaga aproximado por ubicación, independiente del límite diario estricto de la cuenta. |
| Cuerpo de solicitud de 64 KiB | Límite global antes del análisis sintáctico. Los atributos entrantes opcionales también conservan su límite de 50 atributos y 16.384 bytes en JSON canónico UTF-8. |

Cada cuenta dispone de un servicio. Los nombres de tokens tienen entre 1 y 128 caracteres. Consulte [Tokens de aplicación](/es/docs/cloud/tokens/) para conocer las reglas del ciclo de vida de credenciales.

## Contabilidad de cuotas diarias

Acceder a un controlador coincidente consume una unidad diaria, incluso en caso de denegación de política, entrada de datos de negocio no válida o error interno. Los endpoints de autenticación y de sesión no consumen estas unidades. Las solicitudes rechazadas antes de acceder al controlador por comprobaciones de autenticación, enrutamiento, CSRF, tamaño de cuerpo, límite de ráfaga o reserva de cuota tampoco las consumen.

Las reservas asignadas que caducan o se abandonan no se vuelven a emitir, por lo que el número de llamadas utilizables puede ser inferior a 10.000. `GET /api/me` informa del uso diario reservado y de la hora de reinicio sin cobrar una unidad diaria.

Cambiar de token no añade cuota a la cuenta. El límite diario de la cuenta y el límite aproximado de ráfaga por token y ubicación se aplican por separado.

## Errores HTTP

Los errores devuelven respuestas en formato JSON plano. Corrija la solicitud o credencial según corresponda; en caso contrario, falle de forma restrictiva (fail closed) y utilice un tiempo de espera exponencial limitado en lugar de un bucle de reintento infinito.

| Estado HTTP y error | Respuesta de la aplicación |
| --- | --- |
| `401 {"error":"unauthorized"}` | Compruebe el entorno y la credencial. ADS incluye un desafío Bearer. Las solicitudes anónimas y las que solo usan cookies no pueden autorizar operaciones en ADS. |
| `403 {"error":"reauth_required"}` | Autentíquese nuevamente con un proveedor vinculado antes de crear un token. La actividad ordinaria no renueva la vigencia de la autenticación. |
| `403 {"error":"service_mismatch"}` | Corrija la entrada del servicio; nunca reintente contra los datos de otro arrendatario. |
| `400 {"error":"SPDL-0004 …"}` | Speedle rechazó la solicitud, por ejemplo debido a que un valor de atributo tiene un tipo incorrecto. El mensaje es el de Speedle. Corrija la solicitud; no la reintente sin cambios. |
| `404` en revocación de token | El ID del token es desconocido, no pertenece a la cuenta o ya ha sido revocado. Actualice los metadatos antes de elegir otra acción. |
| `413 {"error":"request_too_large"}` | Reduzca el tamaño del cuerpo de la solicitud. |
| `429 {"error":"rate_limited"}` | Respete el encabezado `Retry-After`, que es de 60 segundos para este límite de ráfaga, y aplique un retroceso limitado. |
| `429 {"error":"daily_quota_exceeded"}` | Respete el encabezado `Retry-After` hasta la medianoche UTC siguiente. Cambiar de token no añade cuota a la cuenta. |
| `503 {"error":"quota_unavailable"}` o `snapshot_unavailable` | Falle de forma restrictiva (fail closed) y reintente únicamente con un retroceso limitado. No recurra a evaluaciones de políticas de demostración o D1. |
| `500 {"error":"internal_error"}` | Falle de forma restrictiva (fail closed). Conserve un ID de solicitud seguro para soporte y evite registrar credenciales. |

En el caso de respuestas `503`, `snapshot_unavailable` se contabiliza tras acceder al controlador, mientras que `quota_unavailable` no se contabiliza. Los rechazos tempranos omiten el encabezado de cuota diaria restante descrito a continuación.

Una solicitud a ADS que no sea POST puede ser detenida en el borde público con una respuesta 403 de Cloudflare. Esa no es una decisión de autorización de la aplicación.

## Encabezados de tasa y cuota

Tras la verificación, `X-RateLimit-Limit` es 600. ADS no expone un valor exacto de `X-RateLimit-Remaining`.

Cuando está presente tras un controlador contabilizado, `X-Quota-Daily-Remaining` estima las llamadas restantes a nivel de cuenta para el día de hoy: las llamadas no reservadas en el momento de la asignación más la reserva restante de esta instancia aislada. La estimación es conservadora en el momento de la concesión. Se almacena en caché durante un máximo de 30 segundos (duración de la reserva) y se descarta cuando esta caduca o cambia el día UTC.

Las instancias aisladas simultáneas pueden hacer que el valor sobreestime la disponibilidad hasta en el saldo no reservado al momento de la asignación, no simplemente 32 llamadas. Las reservas no utilizadas o abandonadas pueden hacer que subestime la disponibilidad. Durante despliegues o reversiones, los asignadores heredados recurren a la reserva local restante. Este encabezado es informativo y no modifica la aplicación estricta de cuotas. Los rechazos tempranos lo omiten.

Las respuestas coincidentes de ADS para aplicaciones también utilizan `Cache-Control: no-store`. No guarde decisiones ni credenciales en cachés compartidas.

Vuelva a la [Vista previa de la API de autorización](/es/docs/cloud/authorization-api/) para ver ejemplos de solicitudes y restricciones de integración.
