+++
title = "Speedle Cloud Claves API"
linkTitle = "Claves API"
description = "Crear, proteger, listar y revocar claves API de cuenta con un ámbito fijo y una caducidad obligatoria."
weight = 20
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

Las claves API pertenecen a la cuenta y acceden a sus servicios según su ámbito. Autentican la aplicación, no al usuario final. Administrar claves API, sesiones e identidades vinculadas requiere una sesión de consola iniciada.

**Speedle Cloud estará disponible próximamente.** Esta guía describe el flujo para cuando se habiliten el inicio de sesión en producción y las comprobaciones de autorización autenticadas; los ejemplos no se han ejecutado en un entorno Cloud real.

## Crear una clave API

Abra [Claves API](https://cloud.speedle.io/es/tokens), escriba un nombre de 1–128 caracteres después de quitar espacios al principio y al final, elija un ámbito y una caducidad (`7d`, `30d`, `90d`, `180d` o `365d`; la consola usa 3 meses de forma predeterminada) y seleccione **Crear**. Se requiere autenticación con un proveedor vinculado en los últimos 10 minutos. Si recibe `403 reauth_required`, autentíquese de nuevo y vuelva a seleccionar **Crear**; regresar del proveedor no crea la clave automáticamente.

Cada clave tiene un ámbito fijo desde su creación:

| Ámbito | Acceso |
| --- | --- |
| `ads` (predeterminado) | Solo comprobaciones de autorización mediante `/authz-check/v1/*`. |
| `pms:read` | Leer servicios, políticas de autorización y de roles de la cuenta mediante `/policy-mgmt/v1`. |
| `pms:write` | Leer, crear y eliminar servicios, políticas de autorización y de roles de la cuenta mediante `/policy-mgmt/v1`. |

Los ámbitos de gestión no pueden llamar a ADS; `ads` no puede gestionar políticas. Un ámbito incorrecto devuelve `403 insufficient_scope`. Solicitar un ámbito de gestión con el inicio de sesión cerrado devuelve `403 management_scope_disabled`.

La consola envía una solicitud autenticada por sesión:

```http
POST /api/tokens
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"name":"books-app","scope":"ads","expiresIn":"90d"}
```

La solicitud acepta `name`, `expiresIn` obligatorio y `scope` opcional (por defecto `ads`). `expiresIn` es uno de `7d`, `30d`, `90d`, `180d` o `365d` (1 semana, 1 mes, 3 meses, 6 meses o 1 año). Cada día dura 24 horas. En la consola, **Caduca en** usa 3 meses de forma predeterminada y **Vista previa de caducidad (UTC)** muestra la hora antes de enviar. No hay claves que no caduquen. No envíe un servicio, `expiresAt` ni pruebas de autenticación. Si falta `expiresIn`, no se reconoce o el cliente envía `expiresAt`, la respuesta es `400 invalid_request`. Un cuerpo que solo envía `name` y `scope` también devuelve ese 400. El servidor no rellena `expiresIn`. Se requieren la cookie de sesión y las cabeceras CSRF; una clave API no sustituye a la sesión.

## Proteger el secreto

La creación devuelve HTTP 201. El campo `token` contiene el secreto de la clave API y se muestra una sola vez:

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

Todas las claves caducan. La respuesta incluye `expiresAt`, la caducidad calculada por el servidor en milisegundos Unix. A partir de ese instante, las llamadas de autorización y de gestión de políticas devuelven `401 {"error":"api_key_expired"}`. Cree una clave nueva; reenviar la clave caducada no funciona. Puede revocarla antes. Las claves que ya existían cuando la caducidad pasó a ser obligatoria también tienen una. Es 365 días de 24 horas después de la creación, salvo que ese momento ya hubiera pasado; en ese caso son 30 días de 24 horas después de que el requisito entrara en vigor. Cada clave de la lista incluye `expiresAt`.

Es una descripción del formato, no una clave utilizable. Guárdela inmediatamente en un almacén de secretos del servidor. No la incluya en el almacenamiento del navegador, frontend, URL, control de versiones, capturas ni registros. El listado solo devuelve metadatos; un secreto perdido no se recupera.

La cuenta admite **10 claves API activas entre todos los ámbitos**. Crear otra devuelve `409 {"error":"quota_exceeded","limit":10}`. Revoque una clave sin uso para liberar una plaza; el límite no se restablece con el tiempo. Las claves revocadas y caducadas siguen listadas y no cuentan para el límite.

## Listar claves API

`GET /api/tokens` devuelve claves activas, caducadas y revocadas, de más reciente a más antigua. La página predeterminada tiene 25 entradas; `limit` acepta 1–100. Envíe el `nextCursor` anterior como `cursor` hasta recibir `null`. Cada clave incluye `expiresAt` en milisegundos Unix. La consola muestra **Caducidad (UTC)**. Una clave que caduca en un plazo de 7 días se marca **Caduca pronto**. Las claves caducadas aparecen en el grupo **Caducada**. `lastUsedAt` se actualiza por mejor esfuerzo, normalmente como máximo una vez por hora; no demuestra que una clave esté sin uso.

## Revocar una clave API

Seleccione **Revocar**, compruebe nombre e ID y confirme. La revocación no requiere reautenticación reciente. La consola envía:

```http
DELETE /api/tokens/<API key id>
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{}
```

La primera revocación devuelve HTTP 204 sin cuerpo. Los ID desconocidos, ajenos o ya revocados devuelven `404 not_found`. Una verificación previa puede seguir siendo válida durante un máximo de 60 segundos; la revocación no es instantánea en todas las ubicaciones.

Las acciones disponibles son **Crear** y **Revocar**. Para cambiar el secreto, el ámbito o la caducidad, cree otra clave, actualice los emisores y revoque la anterior. Si todas las claves están en uso al llegar al límite, planifique una interrupción antes de revocar una. Eliminar un servicio no revoca las claves API de la cuenta.

Si se pierde una respuesta, actualice la lista antes de actuar. Un nombre coincidente no recupera el secreto perdido. Consulte [Límites y errores](/es/docs/cloud/limits-and-errors/) y la [API de autorización](/es/docs/cloud/authorization-api/).
