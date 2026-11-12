# Documentación de Speedle Cloud

Speedle Cloud es un servicio de autorización alojado basado en el motor Speedle de código abierto. Gestione servicios y políticas SPDL de autorización y roles, pruebe decisiones en Playground y llame a la API desde su servidor.

**Speedle Cloud estará disponible próximamente.** Esta guía describe el flujo para cuando se habiliten el inicio de sesión en producción y las comprobaciones de autorización autenticadas; los ejemplos no se han ejecutado en un entorno Cloud real.

Para usar la consola o crear claves API debe estar disponible el inicio de sesión. La beta pública gratuita es para pruebas, aprendizaje y evaluación, no ofrece SLA y no se recomienda para producción. Las funciones y límites pueden cambiar.

## Documentación

- [Primeros pasos](https://engine.speedle.io/es/docs/cloud/getting-started/): inicie sesión, cree un servicio y una política en la consola, cree una clave API y compruebe la autorización desde su aplicación.
- [Claves API](https://engine.speedle.io/es/docs/cloud/tokens/): Credenciales de cuenta, ámbitos fijos, Crear y Revocar, secreto mostrado una vez y paginación.
- [API de autorización](https://engine.speedle.io/es/docs/cloud/authorization-api/): Solicitudes IsAllowed, identidad del emisor, Node.js 22 y restricciones de transporte.
- [Límites y errores](https://engine.speedle.io/es/docs/cloud/limits-and-errors/): Límites de cuenta, tamaños, ráfagas, cuota diaria, errores y restablecimiento.

## Descargas

- [Markdown en español](https://engine.speedle.io/docs/cloud/quickstart.es.md): Los cuatro capítulos de Cloud en una guía sin conexión.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)

Los esquemas API describen los contratos de solicitud y respuesta; descargarlos no abre el inicio de sesión.

Consulte los documentos de Cloud: [Términos](https://cloud.speedle.io/es/terms) / [Política de privacidad](https://cloud.speedle.io/es/privacy).

## Primeros pasos con Speedle Cloud

**Speedle Cloud estará disponible próximamente.** Esta guía describe el flujo para cuando se habiliten el inicio de sesión en producción y las comprobaciones de autorización autenticadas; los ejemplos no se han ejecutado en un entorno Cloud real.

### Iniciar sesión con GitHub o Google (próximamente)

Abra la [consola Cloud](https://cloud.speedle.io/es/) en el navegador e inicie sesión con un proveedor disponible. Para una identidad nueva con un correo verificado sin usar, confirme **Crear cuenta**. Si el correo ya está en uso, inicie sesión con el proveedor vinculado que indica la consola.

### Crear un servicio y una política

Abra el [espacio de trabajo del servicio](https://cloud.speedle.io/es/service), introduzca `books` y seleccione **Crear servicio**. Use un servicio nuevo para que otras políticas no afecten a los resultados; no sobrescriba datos existentes.

Seleccione **Nueva política**, asígnele el nombre `read-books` e introduzca este SPDL:

```text
grant user alice from corp get /books
```

Esta política permite que `alice`, del dominio de identidad `corp`, ejecute `get` sobre `/books`. Una concesión sin `from` coincide con el nombre en cualquier dominio. Obtenga el dominio de datos de autenticación fiables y envíelo como `idd` en cada solicitud. Consulte [Dominios de identidad](https://engine.speedle.io/es/docs/idd/).

Seleccione **Validar política** y después **Guardar política**. La validación comprueba la sintaxis, pero no guarda el borrador. Espere aproximadamente 60 segundos para que la política guardada llegue a la API de autorización.

### Crear una clave API

Abra [Claves API](https://cloud.speedle.io/es/tokens), introduzca un nombre como `books-app`, seleccione el alcance `ads` (comprobaciones de autorización) y pulse **Crear clave API**. Si se solicita verificar su identidad, complete la verificación y vuelva a enviar el formulario. Copie el secreto: solo se muestra una vez. Las claves actuales no permiten elegir una caducidad; revoque la clave cuando ya no la necesite. Consulte [Gestión de claves API](https://engine.speedle.io/es/docs/cloud/tokens/).

Guarde la clave en el almacén de secretos del servidor y proporciónela mediante la variable de entorno `SPEEDLE_API_KEY`. No la incluya en código del navegador ni en el repositorio.

### Comprobar la autorización desde su aplicación

Desde el servidor de su aplicación, envíe esta solicitud `isAllowed` para la política guardada. La política no necesita atributos, por lo que `attributes` está vacío:

```bash
curl --fail-with-body --silent --show-error --max-time 10 \
  'https://cloud.speedle.io/authz-check/v1/is-allowed' \
  -H "Authorization: Bearer $SPEEDLE_API_KEY" \
  -H 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[]}'
```

Con solo la política del ejemplo guardada, la respuesta esperada es:

```json
{"allowed":true,"reason":0}
```

Cambie `action` a `delete` y vuelva a enviar la solicitud. Ninguna política concede esa acción, por lo que la respuesta esperada es:

```json
{"allowed":false,"reason":3}
```

Ambas decisiones devuelven HTTP 200. Ejecute la acción protegida solo si `allowed` es `true`; la aplicación debe aplicar el resultado y denegar el acceso ante errores, respuestas no válidas o tiempos de espera agotados.

Consulte la [API de autorización](https://engine.speedle.io/es/docs/cloud/authorization-api/) y los [SDK](https://engine.speedle.io/es/docs/sdks/), incluidos [Node.js/TypeScript](https://engine.speedle.io/es/docs/sdks/node/), [Python](https://engine.speedle.io/es/docs/sdks/python/), [Java](https://engine.speedle.io/es/docs/sdks/java/) y [Go](https://engine.speedle.io/es/docs/sdks/go/), para integrar su aplicación.

### Probar políticas guardadas en el Playground

Opcionalmente, abra **Playground** en el servicio y seleccione **Is allowed**. Introduzca `[{"type":"user","name":"alice","idd":"corp"}]` en **Principales (JSON)**, `/books` en **Recurso**, `get` en **Acción** y `[]` en **Atributos (JSON)**; después, seleccione **Ejecutar prueba**.

**Is allowed** muestra la decisión; **Diagnosticar** explica cómo contribuyeron las políticas; **Todos los roles concedidos** enumera los roles del sujeto; **Todos los permisos concedidos** enumera los recursos y acciones permitidos. Las pruebas usan políticas guardadas, no borradores. Repita la prueba después de cambiar entradas o políticas. Las aplicaciones usan la [API ADS](https://engine.speedle.io/es/docs/cloud/authorization-api/) con una clave API.

Siguientes pasos: para la gestión mediante código, use una [clave API](https://engine.speedle.io/es/docs/cloud/tokens/) con la [API PMS o spctl (Speedle Cloud)](https://engine.speedle.io/es/docs/pms/policy-mgmt/#speedle-cloud).

## Speedle Cloud Claves API

Las claves API pertenecen a la cuenta y acceden a sus servicios según su ámbito. Autentican la aplicación, no al usuario final. Administrar claves API, sesiones e identidades vinculadas requiere una sesión de consola iniciada.

**Speedle Cloud estará disponible próximamente.** Esta guía describe el flujo para cuando se habiliten el inicio de sesión en producción y las comprobaciones de autorización autenticadas; los ejemplos no se han ejecutado en un entorno Cloud real.

### Crear una clave API

Abra [Claves API](https://cloud.speedle.io/es/tokens), escriba un nombre de 1–128 caracteres después de quitar espacios al principio y al final, elija un ámbito y seleccione **Crear**. Se requiere autenticación con un proveedor vinculado en los últimos 10 minutos. Si recibe `403 reauth_required`, autentíquese de nuevo y vuelva a seleccionar **Crear**; regresar del proveedor no crea la clave automáticamente.

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

{"name":"books-app","scope":"ads"}
```

La solicitud solo acepta `name` y `scope` opcional (por defecto `ads`). No envíe un servicio, una caducidad ni pruebas de autenticación. Se requieren la cookie de sesión y las cabeceras CSRF; una clave API no sustituye a la sesión.

### Proteger el secreto

La creación devuelve HTTP 201. El campo `token` contiene el secreto de la clave API y se muestra una sola vez:

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

Una clave API sigue siendo válida hasta que la revoque; no caduca automáticamente.

Es una descripción del formato, no una clave utilizable. Guárdela inmediatamente en un almacén de secretos del servidor. No la incluya en el almacenamiento del navegador, frontend, URL, control de versiones, capturas ni registros. El listado solo devuelve metadatos; un secreto perdido no se recupera.

La cuenta admite **10 claves API activas entre todos los ámbitos**. Crear otra devuelve `409 {"error":"quota_exceeded","limit":10}`. Revoque una clave sin uso para liberar una plaza; el límite no se restablece con el tiempo. Las claves revocadas siguen listadas y no cuentan para el límite.

### Listar claves API

`GET /api/tokens` devuelve claves activas y revocadas, de más reciente a más antigua. La página predeterminada tiene 25 entradas; `limit` acepta 1–100. Envíe el `nextCursor` anterior como `cursor` hasta recibir `null`. `lastUsedAt` se actualiza por mejor esfuerzo, normalmente como máximo una vez por hora; no demuestra que una clave esté sin uso.

### Revocar una clave API

Seleccione **Revocar**, compruebe nombre e ID y confirme. La revocación no requiere reautenticación reciente. La consola envía:

```http
DELETE /api/tokens/<API key id>
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{}
```

La primera revocación devuelve HTTP 204 sin cuerpo. Los ID desconocidos, ajenos o ya revocados devuelven `404 not_found`. Una verificación previa puede seguir siendo válida durante un máximo de 60 segundos; la revocación no es instantánea en todas las ubicaciones.

Las acciones disponibles son **Crear** y **Revocar**. Para cambiar el secreto o ámbito, cree otra clave, actualice los emisores y revoque la anterior. Si todas las claves están en uso al llegar al límite, planifique una interrupción antes de revocar una. Eliminar un servicio no revoca las claves API de la cuenta.

Si se pierde una respuesta, actualice la lista antes de actuar. Un nombre coincidente no recupera el secreto perdido. Consulte [Límites y errores](https://engine.speedle.io/es/docs/cloud/limits-and-errors/) y la [API de autorización](https://engine.speedle.io/es/docs/cloud/authorization-api/).

## API de autorización de Speedle Cloud

**Speedle Cloud estará disponible próximamente.** Esta guía describe el flujo para cuando se habiliten el inicio de sesión en producción y las comprobaciones de autorización autenticadas; los ejemplos no se han ejecutado en un entorno Cloud real.

Use una clave API de cuenta para llamadas de autorización desde el servidor. Para crear claves debe estar disponible el inicio de sesión. Los ámbitos de gestión no pueden llamar a ADS; use `ads` o recibirá `403 insufficient_scope`.

### Invocar IsAllowed desde su servidor

Envíe una clave API de cuenta con ámbito `ads` en un encabezado `Authorization: Bearer` y realice una solicitud POST:

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <API key>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

El cuerpo es la solicitud REST de Speedle, con los mismos campos y tipos que un Speedle ADS autohospedado. `attributes` es una matriz de elementos `{name, type, value}`. Los tipos de atributos admitidos son `string`, `numeric`, `bool` y `datetime` en formato RFC 3339.

`idd` es el dominio de identidad de la entidad principal: el espacio de nombres en el que su aplicación autenticó al emisor, como un proveedor de identidad o un inquilino. Establézcalo en el servidor a partir de datos de autenticación de confianza, nunca a partir de un valor proporcionado por el navegador.

La política de primeros pasos, `grant user alice from corp get /books`, coincide con la solicitud anterior únicamente en el dominio `corp`. Una política sin `from` coincide con el nombre en todos los dominios. Una entidad principal sin `idd` coincide también con todos los dominios, lo que hace indistinguibles a dos usuarios distintos que compartan el mismo nombre cuando la aplicación acepta más de una fuente de identidades. Las integraciones multiinquilino deben enviar `idd` en cada solicitud.

`serviceName` es obligatorio y nunca se asigna por defecto. Elija un servicio de la cuenta de la clave API. Un nombre ausente, vacío, desconocido o de otra cuenta devuelve `404 {"error":"service_not_found"}`. La clave API no se vincula a un solo servicio.

La [política de primeros pasos](https://engine.speedle.io/es/docs/cloud/getting-started/#crear-un-servicio-y-una-política) autoriza la solicitud de ejemplo:

```json
{"allowed":true,"reason":0}
```

### Interpretar decisiones e identidad del emisor

Una denegación de política también devuelve HTTP 200, con `allowed` establecido en `false`. Su aplicación debe verificar `allowed` antes de ejecutar la acción protegida y debe fallar de forma restrictiva (fail closed) ante excepciones o respuestas no válidas.

La aplicación determina las entidades principales mediante su propia autenticación de usuarios. La clave API identifica la cuenta y el ámbito permitido, no al usuario final. No confíe en identidades proporcionadas por un navegador no autenticado.

Speedle Cloud no valida tokens de sujetos. Al igual que el Speedle autohospedado sin un asertor de tokens, acepta `subject.token` y `tokenType` y los descarta sin validación, por lo que un campo de token no vacío nunca es prueba de que la autenticación haya tenido lugar; las decisiones solo utilizan las entidades principales. No reenvíe un token de usuario final ni trate la llamada como validada por aserción.

El Playground de sesión evalúa las políticas guardadas actualmente. El ADS autenticado utiliza instantáneas publicadas. Una prueba exitosa en el Playground no demuestra que todas las ubicaciones de ADS dispongan de la nueva instantánea. ADS no tiene un endpoint de depuración público.

### Ejemplo en Node.js 22

Node.js 22 puede invocar a ADS sin necesidad de un SDK:

```js
export async function canReadBooks(authenticatedUserName, authenticatedUserIdd) {
  if (!authenticatedUserName || !authenticatedUserIdd) {
    throw new Error("principal name and identity domain are required");
  }
  const apiKey = process.env.SPEEDLE_API_KEY;
  if (!apiKey) throw new Error("Speedle credential is not configured");
  const response = await fetch("https://cloud.speedle.io/authz-check/v1/is-allowed", {
    method: "POST",
    redirect: "error",
    signal: AbortSignal.timeout(10_000),
    headers: {
      Authorization: `Bearer ${apiKey}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      subject: {
        principals: [
          { type: "user", name: authenticatedUserName, idd: authenticatedUserIdd },
        ],
      },
      serviceName: "books",
      resource: "/books",
      action: "get",
    }),
  });
  if (!response.ok) throw new Error(`Speedle HTTP ${response.status}`);
  const result = await response.json();
  if (
    typeof result !== "object" || result === null || Array.isArray(result) ||
    typeof result.allowed !== "boolean" || !Number.isInteger(result.reason) ||
    result.reason < 0 || result.reason > 6
  ) {
    throw new Error("Invalid Speedle decision");
  }
  return result.allowed;
}
```

Inyecte `SPEEDLE_API_KEY` desde un almacén de secretos del lado del servidor; nunca lo incluya de forma fija en el código. Pase el nombre de la entidad principal y el dominio de identidad que su servidor derivó del usuario autenticado. El ejemplo rechaza un nombre o dominio vacío antes de construir el cuerpo, ya que `JSON.stringify` de otro modo omitiría `idd` y enviaría una entidad principal sin dominio. Omita `idd` únicamente para un servicio con una sola fuente de identidades, donde los nombres no puedan colisionar entre dominios. El cliente debe fallar de forma restrictiva (fail closed) ante excepciones y no debe convertir fallos en concesiones de permisos.

Este ejemplo básico no incluye un bucle de reintento. El manejo en entornos de producción debería distinguir los errores documentados, respetar `Retry-After` y utilizar reintentos limitados. Configurar `redirect: "error"` impide que fetch siga una redirección con la credencial; mantenga ese comportamiento al adaptar el ejemplo.

### Restricciones de transporte y almacenamiento en caché

Utilice POST para ADS. Una solicitud que no sea POST puede ser detenida en el borde público con una respuesta 403 de Cloudflare; no interprete esa respuesta como una decisión de autorización de la aplicación. No existe un flujo CORS en el navegador para llamadas directas a ADS con un clave API secreta.

Las respuestas coincidentes de ADS para aplicaciones utilizan `Cache-Control: no-store`. No coloque decisiones ni credenciales en cachés compartidas.

Para consultar las bibliotecas cliente en Java, Python, Go, .NET, Rust y Node.js/TypeScript, consulte la [sección de SDKs](https://engine.speedle.io/es/docs/sdks/). Para fallos, cuotas y encabezados de respuesta, consulte [Límites y errores](https://engine.speedle.io/es/docs/cloud/limits-and-errors/). Los esquemas de la API y las descargas de Markdown están enlazados desde la [descripción general de Speedle Cloud](https://engine.speedle.io/es/docs/cloud/#descargas).

## Speedle Cloud Límites y errores

**Speedle Cloud estará disponible próximamente.** Esta guía describe el flujo para cuando se habiliten el inicio de sesión en producción y las comprobaciones de autorización autenticadas; los ejemplos no se han ejecutado en un entorno Cloud real.

Salvo que se indique lo contrario, los límites se comparten en toda la cuenta. Cambiar de clave API no aumenta la cuota diaria.

### Límites

| Qué se limita | Límite | Qué hacer al alcanzarlo |
| --- | --- | --- |
| Servicios de la cuenta | 100 | `409 quota_exceeded`; eliminar un servicio sin uso. Sin restablecimiento temporal. |
| Políticas de autorización y de roles entre todos los servicios | 200 por cuenta | `409 quota_exceeded`; eliminar políticas. Sin restablecimiento temporal. |
| Fuente SPDL completa de una política | 1,024 bytes UTF-8 | `400 invalid_request` al escribir desde la consola; acortar la fuente. Se cuentan bytes, no caracteres. Sin restablecimiento temporal. |
| Claves API activas entre todos los ámbitos | 10 por cuenta | `409 quota_exceeded` con `limit: 10`; revocar una clave sin uso. Sin restablecimiento temporal; el historial revocado no cuenta. |
| Nombre de política / nombre de clave API | 128 puntos de código Unicode / 1–128 caracteres tras recortar espacios | `400 invalid_request`; corregir el nombre. Sin restablecimiento temporal. |
| Llamadas facturables de gestión de políticas y autorización | 10,000 por cuenta y día UTC | `429 daily_quota_exceeded`; se restablece a las 00:00 UTC. Respetar `Retry-After`. |
| Ráfagas de solicitudes con clave API | Aproximadamente 600 por minuto por clave en cada ubicación de Cloud | `429 rate_limited` con `Retry-After: 60`; esperar antes de reintentar. No garantiza rendimiento ni añade cuota diaria. |
| Cuerpo de solicitud ordinario | 65,536 bytes (64 KiB) | `413 request_too_large`; reducir el cuerpo. Sin restablecimiento temporal. |
| Cuerpo de importación JSON atómica (solo `POST /api/service/import`) | 5,100,000 bytes | `413 request_too_large`; reducir los datos. También se aplican los límites de políticas y cuenta. Sin restablecimiento temporal. |
| Atributos de entrada | 50 atributos; 16,384 bytes UTF-8 de JSON canónico | `400 SPDL-0004`; reducir el número o tamaño de atributos. Sin restablecimiento temporal. |

Consulte [Claves API](https://engine.speedle.io/es/docs/cloud/tokens/) para ámbitos, creación y revocación.

### Cuota diaria

Las lecturas, escrituras, validación, Playground, importaciones, exportaciones y comprobaciones de autorización comparten la cuota. Las denegaciones de políticas y entradas de negocio inválidas cuentan. La autenticación, las operaciones de sesión y `GET /api/me` no cuentan. Tampoco cuentan las solicitudes rechazadas por autenticación, ámbito, enrutamiento, CSRF, tamaño, límite de ráfagas o cuota diaria.

Los fallos de Cloud pueden recibir una devolución de cuota de hasta 1,000 llamadas por cuenta y día UTC; si no se confirma, siguen contando. Los errores causados por la solicitud cuentan siempre. Si falla temporalmente el contador, las llamadas de autorización pueden continuar a velocidad reducida con hasta 10,000 llamadas adicionales por cuenta y día UTC. Es una asignación separada, no garantizada, que se restablece a las 00:00 UTC junto con el límite de devoluciones. Si no se concede, se devuelve `503 quota_unavailable`.

Parte de la capacidad puede quedar reservada sin usarse, por lo que el total utilizable puede ser inferior a 10,000. `GET /api/me` informa del uso y la hora de restablecimiento. No reintente continuamente ni cambie de clave para eludir los límites.

### Errores y cabeceras

Los errores son JSON plano, por ejemplo `{"error":"rate_limited"}`. Si no obtiene una decisión válida, deniegue el acceso.

| Estado y error | Acción |
| --- | --- |
| `400 invalid_request` o error de Speedle | Corregir la solicitud; no repetirla sin cambios. |
| `401 unauthorized` | Comprobar la clave API o sesión. Una cookie de sesión sola no autoriza ADS. |
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

ADS usa `Cache-Control: no-store`. No guarde decisiones ni credenciales en cachés compartidas. Use POST; otros métodos pueden recibir `405 method_not_allowed` o un bloqueo previo que no sea JSON. Consulte la [API de autorización](https://engine.speedle.io/es/docs/cloud/authorization-api/).
