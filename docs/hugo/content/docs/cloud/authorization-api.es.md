+++
title = "Vista previa de la API de autorización de Speedle Cloud"
linkTitle = "API de autorización"
description = "Vista previa de la solicitud y respuesta de IsAllowed de Speedle Cloud ADS, integración con Node.js 22, instantáneas de políticas y restricciones de transporte."
date = 2026-11-08T00:00:00+08:00
lastmod = 2026-11-08T00:00:00+08:00
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "El contrato publicado de ADS del lado del servidor para una futura versión autenticada."
+++

**El ADS autenticado está pendiente.** Las solicitudes y ejemplos en esta página describen el contrato publicado para una futura versión; no se ejecutaron contra un entorno real de Cloud. La emisión de un token de aplicación no habilita estas llamadas. Utilice explícitamente la URL del entorno aprobado y verifique que el ADS autenticado se encuentre disponible en dicho entorno antes de la integración.

## Invocar IsAllowed desde su servidor

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

La [política de primeros pasos](/es/docs/cloud/getting-started/#crear-un-servicio-y-una-política) autoriza la solicitud de ejemplo:

```json
{"allowed":true,"reason":0}
```

## Interpretar decisiones e identidad del emisor

Una denegación de política también devuelve HTTP 200, con `allowed` establecido en `false`. Su aplicación debe verificar `allowed` antes de ejecutar la acción protegida y debe fallar de forma restrictiva (fail closed) ante excepciones o respuestas no válidas.

La aplicación determina las entidades principales del emisor mediante su propia autenticación de usuarios. El token de API identifica a la aplicación y al servicio, no al usuario final. No confíe en un nombre de entidad principal proporcionado por un navegador no autenticado.

Speedle Cloud no valida tokens de sujetos. Al igual que el Speedle autohospedado sin un asertor de tokens, acepta `subject.token` y `tokenType` y los descarta sin validación, por lo que un campo de token no vacío nunca es prueba de que la autenticación haya tenido lugar; las decisiones solo utilizan las entidades principales. No reenvíe un token de usuario final ni trate la llamada como validada por aserción.

El Playground de sesión evalúa los datos de políticas D1 actuales. El ADS autenticado utiliza instantáneas publicadas, con una propagación de aproximadamente 60 segundos. Una prueba exitosa en el Playground no demuestra que todas las ubicaciones de ADS dispongan de la nueva instantánea. ADS no tiene un endpoint de depuración público.

## Ejemplo en Node.js 22

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

## Restricciones de transporte y almacenamiento en caché

Utilice POST para ADS. Una solicitud que no sea POST puede ser detenida en el borde público con una respuesta 403 de Cloudflare; no interprete esa respuesta como una decisión de autorización de la aplicación. No existe un flujo CORS en el navegador para llamadas directas a ADS con un token secreto.

Las respuestas coincidentes de ADS para aplicaciones utilizan `Cache-Control: no-store`. No coloque decisiones ni credenciales en cachés compartidas.

Para consultar las bibliotecas cliente en Java, Python, Go, .NET, Rust y Node.js/TypeScript, consulte la [sección de SDKs](/es/docs/sdks/). Para fallos, cuotas y encabezados de respuesta, consulte [Límites y errores](/es/docs/cloud/limits-and-errors/). Los esquemas de la API y las descargas de Markdown están enlazados desde la [descripción general de Speedle Cloud](/es/docs/cloud/#downloads).
