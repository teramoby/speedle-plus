+++
title = "Tokens de aplicación de Speedle Cloud"
linkTitle = "Tokens de aplicación"
description = "Emisión, protección, listado, rotación y revocación de tokens de aplicación de Speedle Cloud, incluidas las reglas de autenticación reciente y divulgación única."
date = 2026-11-08T00:00:00+08:00
lastmod = 2026-11-08T17:11:18+08:00
weight = 20
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Gestione credenciales Bearer con alcance de servicio sin exponer sus valores secretos únicos."
+++

Los tokens de aplicación autentican una aplicación frente a Speedle Cloud ADS para el servicio propio del token. No identifican a un usuario final y no pueden autorizar solicitudes a la API de gestión. La consola de tokens estará disponible cuando se habilite el inicio de sesión en las cuentas; mientras el ADS autenticado permanezca pendiente, emitir un token no habilitará llamadas de autorización del lado del servidor.

## Emitir un token

Para una cuenta que ya posee un servicio, abra [Tokens de aplicación](https://cloud.speedle.io/es/tokens). Introduzca un nombre de token de 1 a 128 caracteres y seleccione **Crear token**. Si se requiere verificación, elija un proveedor vinculado disponible. Tras regresar del proveedor, introduzca el nombre y seleccione **Crear token** nuevamente; regresar no crea el token de forma automática.

La consola envía esta solicitud de sesión:

```http
POST /api/tokens
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"name":"books-app"}
```

Este ejemplo utiliza el origen de producción. El cliente proporciona la sesión mediante su gestión habitual de cookies seguras, por lo que no es una solicitud anónima independiente.

La creación requiere una autenticación normal con el proveedor (mediante inicio de sesión o reautenticación) realizada hace no más de 10 minutos. El servidor comprueba la sesión persistida frente a su propio reloj. Se permite un margen exacto de 10 minutos. Una marca de tiempo de autenticación ausente, futura o vencida devuelve `403 {"error":"reauth_required"}`. La actividad ordinaria de la cuenta no renueva la vigencia de la autenticación.

La solicitud solo acepta `name`. El ID del servicio, los alcances, el vencimiento y las pruebas de autenticación no son entradas proporcionadas por el cliente.

## Proteger el secreto de divulgación única

Una creación exitosa devuelve 201 con los metadatos del token y un campo `token` de divulgación única con este formato:

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

Esta es una descripción del formato, no una credencial utilizable. El ID incrustado en el token coincide con el ID de token devuelto. Almacene el valor emitido de inmediato en el almacén de secretos del lado del servidor de la aplicación.

No incruste el token en código frontend ni lo almacene en el almacenamiento del navegador. Manténgalo fuera de URLs, control de versiones, capturas de pantalla, incidencias, telemetría y registros. El cliente de gestión normal puede almacenar y mostrar un valor recién emitido de forma transitoria para su divulgación y copia única dirigida por el usuario. No debe copiar, descargar, almacenar o registrar el secreto automáticamente.

El listado de tokens solo devuelve metadatos; el texto en plano no se puede recuperar con posterioridad. Los tokens no tienen un vencimiento intrínseco en v1. Se permite un máximo de dos tokens activos para un servicio, y el historial de revocaciones no consume ninguna de las dos ranuras activas.

## Listar metadatos de tokens

`GET /api/tokens` devuelve metadatos de tokens activos y revocados en páginas delimitadas, ordenadas por ID de forma descendente. El tamaño de página predeterminado es 25; `limit` puede ser de 1 a 100. El cursor es un ID de token exclusivo. Siga `nextCursor` hasta que su valor sea `null`. La respuesta nunca devuelve el secreto.

Utilice **Actualizar tokens más recientes** y **Página siguiente** para revisar los metadatos. `lastUsedAt` opera por mejor esfuerzo y se actualiza a lo sumo cada hora, por lo que no puede probar que un emisor haya dejado de usar un token.

## Rotar y revocar un token

Una vez que el ADS autenticado esté disponible, utilice una autenticación reciente normal para crear un segundo token. Modifique cada referencia de la aplicación y de secretos a la nueva credencial, y confirme que funciona para la política prevista antes de retirar la anterior.

En **Tokens de aplicación**, seleccione **Revocar** para el token antiguo, verifique su nombre e ID, y confirme **Revocar token**. La sesión de gestión envía:

```http
DELETE /api/tokens/<old token id>
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{}
```

La primera revocación de un token activo propio devuelve un estado 204 vacío. Los IDs desconocidos, no propios o ya revocados devuelven 404. La autoridad de verificación en caché tiene una vida útil máxima fija de 60 segundos desde el inicio de su búsqueda autorizada, por lo que no se garantiza que la revocación surta efecto en todas partes de inmediato.

Si ambas ranuras activas están ocupadas, inventaríe y migre los clientes legítimos antes de retirar una credencial. No revoque a ciegas un token de producción en funcionamiento solo para liberar una ranura.

Si se pierde una respuesta de creación o revocación, revise los metadatos actualizados antes de elegir explícitamente otra acción. La consola no reintenta mutaciones automáticamente. Un nombre coincidente no puede recuperar ni identificar un secreto único extraviado.

Para conocer los límites de solicitudes y la gestión de errores, consulte [Límites y errores](/es/docs/cloud/limits-and-errors/). Para usar un token en una solicitud de autorización, consulte la [Vista previa de la API de autorización](/es/docs/cloud/authorization-api/).
