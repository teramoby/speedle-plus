# Documentación de Speedle Cloud

Speedle Cloud es un servicio de control de acceso hospedado para desarrolladores, creado sobre el motor de políticas de código abierto Speedle. Está diseñado para permitirle gestionar políticas de autorización y roles en SPDL, probar decisiones en un Playground e invocar una API de autorización desde el servidor de su aplicación sin necesidad de operar su propio servicio de autorización.

**Speedle Cloud estará disponible próximamente.** El encabezado de Cloud muestra la etiqueta **Próximamente** y el inicio de sesión para producción aún no está habilitado en [cloud.speedle.io](https://cloud.speedle.io/es/). El flujo de la consola y el Servicio de Decisiones de Autorización (ADS) autenticado descritos aquí son vistas previas del contrato de servicio publicado. La emisión de un token no habilitará llamadas de autorización del lado del servidor mientras el ADS autenticado continúe pendiente. Verifique la disponibilidad del despliegue y del proveedor de identidad en su entorno seleccionado.

La beta pública gratuita está pensada para pruebas, aprendizaje y evaluación. No incluye SLA, no está recomendada para producción y sus funcionalidades y límites están sujetos a cambios.

## Documentación

- [Primeros pasos](https://engine.speedle.io/es/docs/cloud/getting-started/): inicie sesión, cree un servicio y una política en la consola, cree una clave API y compruebe la autorización desde su aplicación.
- [Tokens de aplicación](https://engine.speedle.io/es/docs/cloud/tokens/): requisitos de reautenticación reciente, divulgación única del token, paginación de metadatos, rotación y revocación.
- [Vista previa de la API de autorización](https://engine.speedle.io/es/docs/cloud/authorization-api/): contrato de solicitud y respuesta de IsAllowed, ejemplo con Node.js 22, instantáneas de políticas y restricciones de integración.
- [Referencia de la API](https://engine.speedle.io/es/docs/cloud/api-reference/): comprobaciones de autorización y gestión de políticas con una API key. Las solicitudes salen de su servidor; esta página es de solo lectura.
- [Límites y errores](https://engine.speedle.io/es/docs/cloud/limits-and-errors/): cuotas de cuenta, límites de solicitudes, respuestas de error, encabezados de tasa y cuota, y directrices de reintento.

## Descargas

- [Markdown en español](https://engine.speedle.io/docs/cloud/quickstart.es.md): los capítulos de Cloud en un único documento sin conexión.
- [Referencia de la API](https://engine.speedle.io/es/docs/cloud/api-reference/): léala en este sitio. Los archivos sin procesar de abajo siguen disponibles.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): especificación canónica de la API de Cloud registrada.
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): generado a partir de la misma fuente YAML.
- [Versión de compilación en servicio](https://cloud.speedle.io/build.json): identifica la compilación en servicio; no certifica la disponibilidad del proveedor de identidad o del endpoint.

Los esquemas incluyen las cuatro operaciones del Playground de sesión y el contrato autenticado de ADS. La publicación de un esquema o la descarga de una guía no habilita un endpoint pendiente ni abre el inicio de sesión. El archivo YAML y el JSON generado se versionan, promueven y restauran junto con la compilación sellada de Cloud correspondiente. Las descargas de Markdown en español, coreano, japonés, inglés y chino se mantienen con estas páginas de documentación de Engine.

Consulte los [Términos](https://cloud.speedle.io/es/terms) y la [Política de privacidad](https://cloud.speedle.io/es/privacy) de Cloud.

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

## Vista previa de la API de autorización de Speedle Cloud

**El ADS autenticado está pendiente.** Las solicitudes y ejemplos en esta página describen el contrato publicado para una futura versión; no se ejecutaron contra un entorno real de Cloud. La emisión de un token de aplicación no habilita estas llamadas. Utilice explícitamente la URL del entorno aprobado y verifique que el ADS autenticado se encuentre disponible en dicho entorno antes de la integración.

### Invocar IsAllowed desde su servidor

Envíe el token emitido en un encabezado `Authorization: Bearer` y realice una solicitud POST:

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <issued token>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

El cuerpo es la solicitud REST de Speedle, con los mismos campos y tipos que un Speedle ADS autohospedado. `attributes` es una matriz de elementos `{name, type, value}`. Los tipos de atributos admitidos son `string`, `numeric`, `bool` y `datetime` en formato RFC 3339.

`idd` es el dominio de identidad de la entidad principal: el espacio de nombres en el que su aplicación autenticó al emisor, como un proveedor de identidad o un inquilino. Establézcalo en el servidor a partir de datos de autenticación de confianza, nunca a partir de un valor proporcionado por el navegador.

La política de primeros pasos, `grant user alice from corp get /books`, coincide con la solicitud anterior únicamente en el dominio `corp`. Una política sin `from` coincide con el nombre en todos los dominios. Una entidad principal sin `idd` coincide también con todos los dominios, lo que hace indistinguibles a dos usuarios distintos que compartan el mismo nombre cuando la aplicación acepta más de una fuente de identidades. Las integraciones multiinquilino deben enviar `idd` en cada solicitud.

`serviceName` es obligatorio en cada solicitud y nunca se asigna por defecto. Envíe el nombre del servicio correspondiente al propio token. Un nombre omitido o vacío recibe la respuesta `SERVICE_NOT_FOUND` de Speedle; un nombre diferente devuelve `403 {"error":"service_mismatch"}`.

La [política de primeros pasos](https://engine.speedle.io/es/docs/cloud/getting-started/#crear-un-servicio-y-una-política) autoriza la solicitud de ejemplo:

```json
{"allowed":true,"reason":0}
```

### Interpretar decisiones e identidad del emisor

Una denegación de política también devuelve HTTP 200, con `allowed` establecido en `false`. Su aplicación debe verificar `allowed` antes de ejecutar la acción protegida y debe fallar de forma restrictiva (fail closed) ante excepciones o respuestas no válidas.

La aplicación determina las entidades principales del emisor mediante su propia autenticación de usuarios. El token de API identifica a la aplicación y al servicio, no al usuario final. No confíe en un nombre de entidad principal proporcionado por un navegador no autenticado.

Speedle Cloud no valida tokens de sujetos. Al igual que el Speedle autohospedado sin un asertor de tokens, acepta `subject.token` y `tokenType` y los descarta sin validación, por lo que un campo de token no vacío nunca es prueba de que la autenticación haya tenido lugar; las decisiones solo utilizan las entidades principales. No reenvíe un token de usuario final ni trate la llamada como validada por aserción.

El Playground de sesión evalúa los datos de políticas D1 actuales. El ADS autenticado utiliza instantáneas publicadas, con una propagación de aproximadamente 60 segundos. Una prueba exitosa en el Playground no demuestra que todas las ubicaciones de ADS dispongan de la nueva instantánea. ADS no tiene un endpoint de depuración público.

### Ejemplo en Node.js 22

Node.js 22 puede invocar a ADS sin necesidad de un SDK:

```js
export async function canReadBooks(authenticatedUserName, authenticatedUserIdd) {
  if (!authenticatedUserName || !authenticatedUserIdd) {
    throw new Error("principal name and identity domain are required");
  }
  const apiToken = process.env.SPEEDLE_ADS_TOKEN;
  if (!apiToken) throw new Error("Speedle credential is not configured");
  const response = await fetch("https://cloud.speedle.io/authz-check/v1/is-allowed", {
    method: "POST",
    redirect: "error",
    signal: AbortSignal.timeout(10_000),
    headers: {
      Authorization: `Bearer ${apiToken}`,
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

Inyecte `SPEEDLE_ADS_TOKEN` desde un almacén de secretos del lado del servidor; nunca lo incluya de forma fija en el código. Pase el nombre de la entidad principal y el dominio de identidad que su servidor derivó del usuario autenticado. El ejemplo rechaza un nombre o dominio vacío antes de construir el cuerpo, ya que `JSON.stringify` de otro modo omitiría `idd` y enviaría una entidad principal sin dominio. Omita `idd` únicamente para un servicio con una sola fuente de identidades, donde los nombres no puedan colisionar entre dominios. El cliente debe fallar de forma restrictiva (fail closed) ante excepciones y no debe convertir fallos en concesiones de permisos.

Este ejemplo básico no incluye un bucle de reintento. El manejo en entornos de producción debería distinguir los errores documentados, respetar `Retry-After` y utilizar reintentos limitados. Configurar `redirect: "error"` impide que fetch siga una redirección con la credencial; mantenga ese comportamiento al adaptar el ejemplo.

### Restricciones de transporte y almacenamiento en caché

Utilice POST para ADS. Una solicitud que no sea POST puede ser detenida en el borde público con una respuesta 403 de Cloudflare; no interprete esa respuesta como una decisión de autorización de la aplicación. No existe un flujo CORS en el navegador para llamadas directas a ADS con un token secreto.

Las respuestas coincidentes de ADS para aplicaciones utilizan `Cache-Control: no-store`. No coloque decisiones ni credenciales en cachés compartidas.

Para consultar las bibliotecas cliente en Java, Python, Go, .NET, Rust y Node.js/TypeScript, consulte la [sección de SDKs](https://engine.speedle.io/es/docs/sdks/). Para fallos, cuotas y encabezados de respuesta, consulte [Límites y errores](https://engine.speedle.io/es/docs/cloud/limits-and-errors/). Los esquemas de la API y las descargas de Markdown están enlazados desde la [descripción general de Speedle Cloud](https://engine.speedle.io/es/docs/cloud/#downloads).

## Límites y errores de Speedle Cloud

Los errores y encabezados de ADS a continuación describen el contrato de ADS autenticado, cuyo entorno de ejecución aún no se encuentra disponible. El contrato de gestión ya define límites de cuenta, comprobaciones de sesión y CSRF, y errores de gestión de tokens para su uso cuando el acceso a cuentas esté habilitado. Descargar un esquema no habilita un endpoint pendiente.

### Límites

| Límite | Significado |
| --- | --- |
| 10.000 llamadas al día | Llamadas coincidentes a controladores de PMS y ADS a nivel de cuenta por día UTC, en todas las sesiones, tokens y servicios; se reinicia a las 00:00 UTC. |
| 200 políticas | Políticas de autorización y roles combinadas por cuenta. |
| 4 KiB de SPDL | 4.096 bytes UTF-8 por política completa, no 4.096 caracteres. |
| 2 tokens activos | Por servicio; el historial de revocaciones no consume este límite. |
| 600 solicitudes por minuto por token | Límite de ráfaga aproximado por ubicación, independiente del límite diario estricto de la cuenta. |
| Cuerpo de solicitud de 64 KiB | Límite global antes del análisis sintáctico. Los atributos entrantes opcionales también conservan su límite de 50 atributos y 16.384 bytes en JSON canónico UTF-8. |

Cada cuenta dispone de un servicio. Los nombres de tokens tienen entre 1 y 128 caracteres. Consulte [Tokens de aplicación](https://engine.speedle.io/es/docs/cloud/tokens/) para conocer las reglas del ciclo de vida de credenciales.

### Contabilidad de cuotas diarias

Acceder a un controlador coincidente consume una unidad diaria, incluso en caso de denegación de política, entrada de datos de negocio no válida o error interno. Los endpoints de autenticación y de sesión no consumen estas unidades. Las solicitudes rechazadas antes de acceder al controlador por comprobaciones de autenticación, enrutamiento, CSRF, tamaño de cuerpo, límite de ráfaga o reserva de cuota tampoco las consumen.

Las reservas asignadas que caducan o se abandonan no se vuelven a emitir, por lo que el número de llamadas utilizables puede ser inferior a 10.000.

Cambiar de token no añade cuota a la cuenta. El límite diario de la cuenta y el límite aproximado de ráfaga por token y ubicación se aplican por separado.

### Errores HTTP

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

### Encabezados de tasa y cuota

Tras la verificación, `X-RateLimit-Limit` es 600. ADS no expone un valor exacto de `X-RateLimit-Remaining`.

Cuando está presente tras un controlador contabilizado, `X-Quota-Daily-Remaining` estima las llamadas restantes a nivel de cuenta para el día de hoy: las llamadas no reservadas en el momento de la asignación más la reserva restante de esta instancia aislada. La estimación es conservadora en el momento de la concesión. Se almacena en caché durante un máximo de 30 segundos (duración de la reserva) y se descarta cuando esta caduca o cambia el día UTC.

Las instancias aisladas simultáneas pueden hacer que el valor sobreestime la disponibilidad hasta en el saldo no reservado al momento de la asignación, no simplemente 32 llamadas. Las reservas no utilizadas o abandonadas pueden hacer que subestime la disponibilidad. Durante despliegues o reversiones, los asignadores heredados recurren a la reserva local restante. Este encabezado es informativo y no modifica la aplicación estricta de cuotas. Los rechazos tempranos lo omiten.

Las respuestas coincidentes de ADS para aplicaciones también utilizan `Cache-Control: no-store`. No guarde decisiones ni credenciales en cachés compartidas.

Vuelva a la [Vista previa de la API de autorización](https://engine.speedle.io/es/docs/cloud/authorization-api/) para ver ejemplos de solicitudes y restricciones de integración.
