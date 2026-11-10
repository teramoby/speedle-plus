+++
title = "Primeros pasos con Speedle Cloud"
linkTitle = "Primeros pasos"
description = "Prepárese para Speedle Cloud: inicio de sesión y gestión de cuentas, administración de servicios y políticas SPDL, solicitudes de API de sesión y pruebas en el Playground."
weight = 10
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "El flujo publicado de la consola de Speedle Cloud, desde el inicio de sesión y configuración de la cuenta hasta las políticas guardadas y decisiones en el Playground."
+++

**Speedle Cloud estará disponible próximamente.** El inicio de sesión para producción aún no está habilitado en cloud.speedle.io y el encabezado de Cloud muestra **Próximamente**. Cuando el inicio de sesión esté cerrado, la página de inicio de sesión indicará que abrirá pronto. Esta guía describe el flujo de trabajo publicado y no una consola de producción abierta actualmente; las capturas de pantalla se agregarán tras la apertura del inicio de sesión en producción.

## Origen de producción

Utilice `https://cloud.speedle.io` para sesiones de cuenta, solicitudes de gestión y tokens de aplicación. Las cuentas, recursos y tokens pertenecen a ese origen. Los ejemplos de esta sección son ilustrativos y no se ejecutaron en un entorno real. Confirme que su entorno seleccionado esté desplegado y que el proveedor de identidad que planea utilizar se encuentre disponible allí.

## Iniciar sesión con GitHub o Google (próximamente)

La página de inicio de sesión muestra **Iniciar sesión con GitHub** e **Iniciar sesión con Google** incluso mientras el inicio de sesión esté deshabilitado. Cuando el inicio de sesión en producción esté abierto, elija un proveedor disponible. Una nueva identidad cuya dirección de correo electrónico verificada no esté en uso requiere una confirmación explícita de **Crear cuenta**. Si la dirección de correo electrónico ya está en uso, inicie sesión con el proveedor vinculado identificado por la consola y luego conecte la segunda identidad en **Configuración de la cuenta**. Las cuentas no se combinan automáticamente.

La configuración de la cuenta le permite inspeccionar las identidades conectadas y conectar o desconectar un proveedor disponible; mantenga al menos una identidad conectada. **Cerrar sesión** se encuentra en el encabezado junto al selector de idiomas, y el menú de Cuenta ofrece **Cerrar sesión en todas partes**. Un cierre de sesión exitoso regresa a la página de inicio de sesión y limpia el espacio de trabajo y cualquier visualización única de token. El registro de auditoría muestra la actividad de la cuenta.

Eliminar una cuenta requiere autenticación reciente del proveedor y escribir `DELETE`. Elimina de forma permanente los servicios, políticas, tokens e identidades de la cuenta, y finaliza sus sesiones. Exporte las políticas necesarias desde el espacio de trabajo del Servicio antes de la eliminación.

## Crear un servicio y una política

Para una cuenta nueva, abra el [espacio de trabajo del Servicio](https://cloud.speedle.io/es/service), introduzca `books` como el nombre del servicio y seleccione **Crear servicio**. Cada cuenta dispone de un servicio. Si la cuenta ya cuenta con un servicio o políticas, inspecciónelos y utilice una configuración existente adecuada; no sobrescriba ni elimine datos existentes para hacer funcionar este ejemplo.

Seleccione **Nueva política**, introduzca `read-books` como el nombre de la política y pegue esta política de autorización en el editor SPDL:

```text
grant user alice from corp get /books
```

`from corp` mantiene esta concesión dentro del dominio de identidad `corp`. Reemplace `corp` por el dominio que su aplicación derive a partir de datos de autenticación de confianza, y envíe el mismo valor como el `idd` de la entidad principal en cada solicitud, tal como hacen los ejemplos del Playground a continuación. Una concesión sin `from` coincide con un usuario llamado `alice` en todos los dominios; omita la cláusula únicamente para un servicio con una sola fuente de identidades, donde los nombres no puedan colisionar entre dominios. Consulte [dominio de identidad](/es/docs/idd/) para conocer la regla y el campo correspondiente en la solicitud.

Seleccione **Validar política** y compruebe que la consola informe **Detectado: política de autorización**, luego seleccione **Guardar política**. La validación detecta el tipo de política y comprueba la sintaxis; no guarda el borrador. Guardar es una acción manual e independiente. También puede importar políticas existentes en el espacio de trabajo en lugar de ingresarlas una por una.

Speedle Cloud admite políticas de autorización y políticas de roles; no se admiten funciones externas personalizadas. La lista de políticas agrupa ambos tipos y muestra sus recuentos. Las asignaciones de roles utilizan estos formatos:

- Rol global: `grant user alice editor`
- Rol con alcance de recurso: `grant user alice editor on /books`

La declaración ambigua `grant user alice role editor` se rechaza. Omita el prefijo opcional `role` para una asignación de rol, o escriba `grant (user alice) role editor` para una política de autorización. La edición conserva el tipo de política almacenado. Para modificar el tipo, elimine la política y cree una nueva.

Si la consola informa un conflicto o una respuesta incierta, conserve su borrador y actualice los datos guardados antes de realizar otra mutación. Cambiar de idioma conserva los borradores. Los menús integrados de Monaco mantienen el idioma seleccionado cuando el editor se cargó por primera vez y cambian después de recargar la página.

Eliminar un servicio requiere escribir el nombre del servicio, completar una autenticación reciente con el proveedor y confirmar nuevamente tras la verificación.

## Enviar solicitudes de gestión de sesión

El Servicio de Gestión de Políticas (PMS) utiliza la sesión de la cuenta. Permita que un cliente compatible del mismo origen gestione su cookie de sesión segura; no extraiga cookies ni utilice inicializaciones con proveedores de prueba. Las solicitudes de mutación requieren el `Origin` exacto del entorno, `X-SC-CSRF: 1` y un tipo de contenido JSON. Un token Bearer de ADS no puede autorizar operaciones en PMS. Los clientes no deben enviar selectores de propiedad de cuenta o servicio, ni marcas de tiempo de autenticación.

Las solicitudes equivalentes para el ejemplo de servicio y política son:

| Acción | Solicitud de sesión | Éxito |
| --- | --- | --- |
| Crear el servicio de la cuenta | `POST /api/service` con `{"name":"books"}` | 201 |
| Añadir una política de autorización | `POST /api/service/policies` con el cuerpo a continuación | 201 |

```json
{"name":"read-books","spdl":"grant user alice from corp get /books"}
```

Pegue únicamente el valor `spdl` en el editor cuando use la consola. El cuerpo JSON corresponde a la API de gestión.

## Validar y exportar políticas

Valide SPDL con `POST /api/service/validate` y un cuerpo `{spdl}` para detectar su tipo. El formato heredado `{kind: "authz", spdl}` continúa seleccionando el analizador de autorización. **Descargar SPDL** y **Descargar JSON** invocan `GET /api/service/export?format=spdl` y `GET /api/service/export?format=json`, respectivamente.

El Playground de sesión evalúa los datos D1 actuales que pertenecen a la cuenta. El futuro lanzamiento de ADS autenticado evaluará instantáneas de políticas publicadas, cuya propagación tarda aproximadamente 60 segundos. Por tanto, un resultado exitoso en el Playground no demuestra que todas las ubicaciones de ADS hayan recibido la nueva instantánea.

## Probar políticas guardadas en el Playground

En el espacio de trabajo del Servicio, desplácese hasta **Playground** y seleccione **Is allowed**. Introduzca `[{"type":"user","name":"alice","idd":"corp"}]` en **Entidades principales (JSON)**, `/books` en **Recurso**, `get` en **Acción** y `[]` en **Atributos (JSON)**, luego seleccione **Ejecutar prueba**.

Las demás operaciones son **Diagnosticar**, **Todos los roles concedidos** y **Todos los permisos concedidos**. Las ejecuciones del Playground utilizan las políticas guardadas, no un borrador no guardado del editor. Modificar los datos de entrada, la operación o el idioma no envía automáticamente una solicitud. Ejecute la prueba nuevamente después de cambiar las entradas o las políticas guardadas. La consola marca el resultado anterior como obsoleto y lo mantiene visible si una solicitud posterior falla.

## Utilizar la API del Playground de sesión

`POST /api/service/test` implementa las cuatro operaciones con las mismas reglas de Origin/CSRF y la misma cuota diaria de cuenta que otras solicitudes de gestión. Lee una instantánea consistente de las políticas D1 de la cuenta actual. No publica ni guarda políticas y no acepta un ID de cuenta o de servicio. La revisión mostrada es la revisión del espacio de trabajo conocida cuando se envió la solicitud, no una prueba de la revisión de la instantánea del lado del servidor que se evaluó.

```http
POST /api/service/test
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"op":"isAllowed","request":{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3}]}}
```

| `request.op` | Cuerpo exitoso | Contrato publicado de la consola y la API de sesión |
| --- | --- | --- |
| `isAllowed` | Speedle IsAllowedResponse: `allowed`, `reason`, `errorMessage` opcional | Incluido |
| `diagnose` | Speedle EvaluationDebugResponse: nombre de razón, `requestContext`, `attributes`, `policies` | Incluido |
| `allGrantedRoles` | `string[]`; `[]` cuando no hay ninguno | Incluido |
| `allGrantedPermissions` | `{resource, actions}[]`; `[]` cuando no hay ninguno | Incluido |

Los cuerpos de respuesta exitosos son los valores sin procesar de upstream, sin una envoltura de `op` o `result`. Ambos resultados de concesiones totales pueden ser `null` o una matriz vacía; ninguna de las dos formas concede autorización. ADS no dispone de un endpoint público de Diagnose.

A continuación, lea sobre [tokens de aplicación](/es/docs/cloud/tokens/), la [vista previa de la API de autorización](/es/docs/cloud/authorization-api/) y [límites y errores](/es/docs/cloud/limits-and-errors/).
