+++
title = "Speedle Cloud Límites y errores"
linkTitle = "Límites y errores"
description = "Límites de cuenta, códigos de error, restablecimiento y reintentos."
weight = 40
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

**Speedle Cloud estará disponible próximamente.** Esta guía describe el flujo para cuando se habiliten el inicio de sesión en producción y las comprobaciones de autorización autenticadas; los ejemplos no se han ejecutado en un entorno Cloud real.

Salvo que se indique lo contrario, los límites se comparten en toda la cuenta. Cambiar de clave API no aumenta la cuota diaria.

## Límites

| Qué se limita | Límite | Qué hacer al alcanzarlo |
| --- | --- | --- |
| Servicios de la cuenta | 100 | `409 quota_exceeded`; eliminar un servicio sin uso. Sin restablecimiento temporal. |
| Políticas de autorización y de roles entre todos los servicios | 200 por cuenta | `409 quota_exceeded`; eliminar políticas. Sin restablecimiento temporal. |
| Fuente SPDL completa de una política | 1,024 bytes UTF-8 | `400 invalid_request` al escribir desde la consola; acortar la fuente. Se cuentan bytes, no caracteres. Sin restablecimiento temporal. |
| Claves API activas entre todos los ámbitos | 10 por cuenta | `409 quota_exceeded` con `limit: 10`; revocar una clave sin uso. Sin restablecimiento temporal; las claves revocadas y caducadas no cuentan. |
| Nombre de política / nombre de clave API | 128 puntos de código Unicode / 1–128 caracteres tras recortar espacios | `400 invalid_request`; corregir el nombre. Sin restablecimiento temporal. |
| Llamadas facturables de gestión de políticas y autorización | 10,000 por cuenta y día UTC | `429 daily_quota_exceeded`; se restablece a las 00:00 UTC. Respetar `Retry-After`. |
| Ráfagas de solicitudes con clave API | Aproximadamente 600 por minuto por clave en cada ubicación de Cloud | `429 rate_limited` con `Retry-After: 60`; esperar antes de reintentar. No garantiza rendimiento ni añade cuota diaria. |
| Cuerpo de solicitud ordinario | 65,536 bytes (64 KiB) | `413 request_too_large`; reducir el cuerpo. Sin restablecimiento temporal. |
| Cuerpo de importación JSON atómica (solo `POST /api/service/import`) | 5,100,000 bytes | `413 request_too_large`; reducir los datos. También se aplican los límites de políticas y cuenta. Sin restablecimiento temporal. |
| Atributos de entrada | 50 atributos; 16,384 bytes UTF-8 de JSON canónico | `400 SPDL-0004`; reducir el número o tamaño de atributos. Sin restablecimiento temporal. |

Consulte [Claves API](/es/docs/cloud/tokens/) para ámbitos, caducidad, creación y revocación.

## Cuota diaria

Las lecturas, escrituras, validación, Playground, importaciones, exportaciones y comprobaciones de autorización comparten la cuota. Las denegaciones de políticas y entradas de negocio inválidas cuentan. La autenticación, las operaciones de sesión y `GET /api/me` no cuentan. Tampoco cuentan las solicitudes rechazadas por autenticación, ámbito, enrutamiento, CSRF, tamaño, límite de ráfagas o cuota diaria.

Los fallos de Cloud pueden recibir una devolución de cuota de hasta 1,000 llamadas por cuenta y día UTC; si no se confirma, siguen contando. Los errores causados por la solicitud cuentan siempre. Si falla temporalmente el contador, las llamadas de autorización pueden continuar a velocidad reducida con hasta 10,000 llamadas adicionales por cuenta y día UTC. Es una asignación separada, no garantizada, que se restablece a las 00:00 UTC junto con el límite de devoluciones. Si no se concede, se devuelve `503 quota_unavailable`.

Parte de la capacidad puede quedar reservada sin usarse, por lo que el total utilizable puede ser inferior a 10,000. `GET /api/me` informa del uso y la hora de restablecimiento. No reintente continuamente ni cambie de clave para eludir los límites.

## Errores y cabeceras

Los errores son JSON plano, por ejemplo `{"error":"rate_limited"}`. Si no obtiene una decisión válida, deniegue el acceso.

| Estado y error | Acción |
| --- | --- |
| `400 invalid_request` o error de Speedle | Corregir la solicitud; no repetirla sin cambios. |
| `401 unauthorized` | Comprobar la clave API o sesión. Una cookie de sesión sola no autoriza ADS. |
| `401 api_key_expired` | La clave API ha alcanzado o superado su caducidad. Cree un reemplazo; reintentar esta clave no tendrá éxito. |
| `403 reauth_required` | Autenticarse de nuevo con un proveedor vinculado antes de crear la clave. |
| `403 insufficient_scope` | Usar una clave con el ámbito requerido. |
| `403 management_scope_disabled` | No se emiten ámbitos de gestión con el inicio de sesión cerrado. |
| `403 csrf_failed` | Comprobar sesión, origen y cabecera CSRF. |
| `404 service_not_found` (ADS) / `404 not_found` (gestión) | Elegir un servicio de la cuenta. Los ID de clave desconocidos, ajenos o revocados también devuelven `404 not_found`. |
| `409 quota_exceeded` | Liberar una plaza de servicio, política o clave API; el tiempo no restablece estos límites. |
| `413 request_too_large` | Reducir el cuerpo. |
| `429 rate_limited` / `429 daily_quota_exceeded` | Respetar `Retry-After` y usar espera incremental acotada; la cuota diaria se restablece a medianoche UTC. |
| `503 quota_unavailable` / `503 snapshot_unavailable` / `500 internal_error` | Denegar el acceso y reintentar con espera acotada, respetando `Retry-After` si existe. |

En solicitudes con clave verificada, `X-RateLimit-Limit` es 600. No hay un `X-RateLimit-Remaining` exacto. `X-Quota-Daily-Remaining` es una estimación de saldo de la cuenta; la concurrencia y las reservas sin uso pueden hacer que difiera del saldo utilizable. No garantiza la siguiente solicitud y se omite en rechazos tempranos.

ADS usa `Cache-Control: no-store`. No guarde decisiones ni credenciales en cachés compartidas. Use POST; otros métodos pueden recibir `405 method_not_allowed` o un bloqueo previo que no sea JSON. Consulte la [API de autorización](/es/docs/cloud/authorization-api/).
