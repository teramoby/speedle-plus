# Documentación de Speedle Cloud

Speedle Cloud es un servicio de control de acceso hospedado para desarrolladores, creado sobre el motor de políticas de código abierto Speedle. Está diseñado para permitirle gestionar políticas de autorización y roles en SPDL, probar decisiones en un Playground e invocar una API de autorización desde el servidor de su aplicación sin necesidad de operar su propio servicio de autorización.

**Speedle Cloud estará disponible próximamente.** El encabezado de Cloud muestra la etiqueta **Próximamente** y el inicio de sesión para producción aún no está habilitado en [cloud.speedle.io](https://cloud.speedle.io/es/). El flujo de la consola y el Servicio de Decisiones de Autorización (ADS) autenticado descritos aquí son vistas previas del contrato de servicio publicado. La emisión de un token no habilitará llamadas de autorización del lado del servidor mientras el ADS autenticado continúe pendiente. Verifique la disponibilidad del despliegue y del proveedor de identidad en su entorno seleccionado.

La beta pública gratuita está pensada para pruebas, aprendizaje y evaluación. No incluye SLA, no está recomendada para producción y sus funcionalidades y límites están sujetos a cambios.

## Documentación

- [Primeros pasos](https://engine.speedle.io/es/docs/cloud/getting-started/): inicio de sesión con GitHub o Google (próximamente), gestión de cuentas, servicios, políticas SPDL, solicitudes de gestión de sesiones y las cuatro operaciones del Playground de sesión.
- [Tokens de aplicación](https://engine.speedle.io/es/docs/cloud/tokens/): requisitos de reautenticación reciente, divulgación única del token, paginación de metadatos, rotación y revocación.
- [Vista previa de la API de autorización](https://engine.speedle.io/es/docs/cloud/authorization-api/): contrato de solicitud y respuesta de IsAllowed, ejemplo con Node.js 22, instantáneas de políticas y restricciones de integración.
- [Límites y errores](https://engine.speedle.io/es/docs/cloud/limits-and-errors/): cuotas de cuenta, límites de solicitudes, respuestas de error, encabezados de tasa y cuota, y directrices de reintento.

## Descargas

- [Markdown en español](https://engine.speedle.io/docs/cloud/quickstart.es.md): los cuatro capítulos de Cloud en un único documento sin conexión.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): especificación canónica de la API de Cloud registrada.
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): generado a partir de la misma fuente YAML.
- [Versión de compilación en servicio](https://cloud.speedle.io/build.json): identifica la compilación en servicio; no certifica la disponibilidad del proveedor de identidad o del endpoint.

Los esquemas incluyen las cuatro operaciones del Playground de sesión y el contrato autenticado de ADS. La publicación de un esquema o la descarga de una guía no habilita un endpoint pendiente ni abre el inicio de sesión. El archivo YAML y el JSON generado se versionan, promueven y restauran junto con la compilación sellada de Cloud correspondiente. Las descargas de Markdown en español, coreano, japonés, inglés y chino se mantienen con estas páginas de documentación de Engine.

Consulte los [Términos](https://cloud.speedle.io/es/terms) y la [Política de privacidad](https://cloud.speedle.io/es/privacy) de Cloud.

## Primeros pasos con Speedle Cloud

**Speedle Cloud estará disponible próximamente.** El inicio de sesión para producción aún no está habilitado en cloud.speedle.io y el encabezado de Cloud muestra **Próximamente**. Cuando el inicio de sesión esté cerrado, la página de inicio de sesión indicará que abrirá pronto. Esta guía describe el flujo de trabajo publicado y no una consola de producción abierta actualmente.

### Origen de producción

Utilice `https://cloud.speedle.io` para sesiones de cuenta, solicitudes de gestión y tokens de aplicación. Las cuentas, recursos y tokens pertenecen a ese origen.

### Crear la primera política de autorización

Introduzca la siguiente declaración SPDL en el editor:

```text
grant user alice from corp get /books
```

`from corp` limita esta autorización al dominio de identidad `corp`. Reemplace `corp` por el dominio que su aplicación derive de datos de autenticación de confianza, y envíe el mismo valor como el `idd` de la entidad principal en cada solicitud, tal como hacen los ejemplos del Playground a continuación. Una concesión sin "from" coincide con un usuario llamado `alice` en todos los dominios. Omita la cláusula únicamente para un servicio con una sola fuente de identidades, donde los nombres no puedan colisionar entre dominios. Consulte [dominio de identidad](/es/docs/idd/) para conocer la regla y el campo correspondiente en la solicitud.

### Probar decisiones de autorización en el Playground

Abra el Playground para probar la política.

Introduzca la entidad principal en el sujeto de la solicitud:

```json
{"type":"user","name":"alice","idd":"corp"}
```

Introduzca `get` en Acción y `/books` en Recurso. Al hacer clic en **Evaluar**, el resultado será `allowed: true` (código de motivo 0).

Si cambia el `idd` de la entidad principal a `other` o cambia el recurso a `/secret`, la solicitud será denegada (`allowed: false`).

## Tokens de aplicación de Speedle Cloud

Los tokens de aplicación permiten que su aplicación backend se autentique para invocar la API de autorización de Speedle Cloud.

### Requisitos de seguridad

Para crear, rotar o revocar un token, se requiere una reautenticación reciente en la consola. El secreto del token se muestra **una sola vez** en el momento de la creación. Almacénelo inmediatamente en un almacén seguro de secretos. Si se extravía, no se puede volver a mostrar y deberá regenerar el token.

### Alcance del token

Los tokens están delimitados a una cuenta y espacio de trabajo específicos. Utilice tokens independientes para cargas de trabajo de producción y de desarrollo.

### Rotación y revocación de tokens

Se recomienda rotar los tokens periódicamente. Cree un nuevo token, despliéguelo en sus aplicaciones y luego revoque el token anterior para evitar tiempos de inactividad.

## Vista previa de la API de autorización de Speedle Cloud

Speedle Cloud proporciona una API de autorización rápida y de baja latencia para decisiones de acceso.

### Contrato de solicitud

La aplicación invoca `POST /authz-check/v1/is-allowed` en el Servicio de Decisiones de Autorización (ADS).

### Ejemplo en Node.js 22

El siguiente ejemplo en Node.js muestra cómo enviar una solicitud de autorización utilizando un token Bearer:

```javascript
import { request } from 'node:https';

export async function checkAccess(token, authenticatedUserId, authenticatedUserIdd, resource, action) {
  if (!authenticatedUserId || !authenticatedUserIdd) {
    throw new Error('principal name and identity domain are required');
  }

  const payload = JSON.stringify({
    subject: {
      principals: [
        {
          type: 'user',
          name: authenticatedUserId,
          idd: authenticatedUserIdd
        }
      ]
    },
    serviceName: 'default',
    resource: resource,
    action: action
  });

  return new Promise((resolve, reject) => {
    const req = request('https://cloud.speedle.io/authz-check/v1/is-allowed', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(payload)
      }
    }, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        if (res.statusCode === 200) {
          resolve(JSON.parse(data));
        } else {
          reject(new Error(`Authorization failed: ${res.statusCode} ${data}`));
        }
      });
    });
    req.on('error', reject);
    req.write(payload);
    req.end();
  });
}
```

Al invocar esta función contra la política de ejemplo `grant user alice from corp get /books`, se devuelve la decisión de autorización según lo esperado.

## Límites y errores de Speedle Cloud

### Límites de tasa y cuotas

Durante la beta pública gratuita, se aplican cuotas por cuenta en Speedle Cloud para garantizar un uso justo.

### Respuestas de error

Las respuestas de error devuelven códigos de estado HTTP estándar y un objeto de error en JSON:

```json
{
  "error": {
    "code": "rate_limit_exceeded",
    "message": "Too many requests. Please retry later."
  }
}
```

### Directrices de reintento

En caso de errores de red transitorios o `429 Too Many Requests`, implemente reintentos con retroceso exponencial (exponential backoff) y variación aleatoria (jitter).
