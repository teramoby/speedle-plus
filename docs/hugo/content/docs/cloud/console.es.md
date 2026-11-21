+++
title = "Manual de usuario de Speedle Cloud Console"
linkTitle = "Manual de consola"
description = "Guía práctica de Speedle Cloud Console (SCC): espacios de trabajo, miembros, servicios, políticas SPDL, Playground, claves de API, funciones, flota de ADS, registros de auditoría y configuración de la cuenta."
date = 2026-11-21T00:00:00+08:00
lastmod = 2026-11-21T00:00:00+08:00
weight = 15
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Gestione espacios de trabajo, servicios, políticas, pruebas en Playground, claves de API, funciones, instancias de ADS y registros de auditoría en Speedle Cloud Console."
+++

**Speedle Cloud está en beta pública.** La beta pública gratuita es para evaluación y aprendizaje, no ofrece SLA y no se recomienda para producción. Las funciones y límites pueden cambiar.

Este manual explica las tareas diarias en **Speedle Cloud Console (SCC)** en [cloud.speedle.io](https://cloud.speedle.io/es/). Para realizar rápidamente una llamada a la API desde su servidor, consulte [Primeros pasos](/es/docs/cloud/getting-started/). Para conocer los cuerpos de solicitud, códigos de error y cuotas, consulte la [API de autorización](/es/docs/cloud/authorization-api/), la [Referencia de la API](/es/docs/cloud/api-reference/) y [Límites y errores](/es/docs/cloud/limits-and-errors/).

> **Sobre las capturas de pantalla:** Las figuras de este manual muestran la Speedle Cloud Console real renderizada en inglés con datos de demostración (cuentas `example.test`, el servicio de ejemplo `books`, endpoints de ejemplo y valores de uso de ejemplo).

## Iniciar sesión y diseño de la consola

### Requisitos previos

- Un navegador con JavaScript y cookies habilitados.
- Una cuenta de GitHub o Google con una dirección de correo verificada.

### Iniciar sesión o crear una cuenta

1. Abra [cloud.speedle.io](https://cloud.speedle.io/es/) y elija **Empezar** (o abra `/login` directamente).
2. Elija **Iniciar sesión con GitHub** o **Iniciar sesión con Google** y complete la autenticación en el proveedor.
3. Si inicia sesión por primera vez con un correo verificado que no está en uso, revise los enlaces de los Términos de servicio y la Política de privacidad y confirme **Crear cuenta**.

**Resultado esperado:** La consola abre su espacio de trabajo activo en **Servicios y políticas** (`/w/<workspaceId>/service`). Las cuentas nuevas comienzan con un espacio de trabajo **Personal** donde su rol es **Propietario**.

**Recuperación:**

- **Correo electrónico ya en uso:** Inicie sesión con el proveedor ya vinculado a ese correo y luego conecte el segundo proveedor en **Configuración de la cuenta**.
- **Se requiere correo verificado:** Verifique su correo en la configuración de GitHub o Google e inicie sesión de nuevo.
- **Registros pausados:** La beta alcanzó su límite de cuentas nuevas; las cuentas existentes pueden seguir iniciando sesión.
- **Sesión caducada:** Las sesiones caducan tras 7 días de inactividad o 30 días desde su creación, lo que ocurra primero. Inicie sesión de nuevo para continuar.

### Navegar por la consola

La consola autenticada tiene dos áreas de navegación:

- **Barra superior:**
  - Logotipo **Speedle | Cloud**: vuelve a **Servicios y políticas** en el espacio de trabajo activo.
  - Selector **Espacio de trabajo**: muestra el nombre del espacio de trabajo activo y su rol (**Propietario** o **Administrador**), permite cambiar de espacio de trabajo y abrir **Crear espacio de trabajo**.
  - Enlace de ayuda **¿Tienes problemas? ¡Escríbenos!**, selector de **Idioma** y menú **Cuenta** (correo electrónico, **Resumen de la cuenta**, **Configuración de la cuenta**, **Cerrar sesión** y **Cerrar sesión en todos los dispositivos**).
- **Barra lateral izquierda:**
  - **Consola:** **Servicios y políticas**, **Claves de API**, **Flota de ADS** y **Registro de auditoría**, todos limitados al espacio de trabajo activo.
  - **Ajustes del espacio de trabajo:** **Miembros**, para consultar o gestionar la membresía.
  - **Documentación de Cloud:** abre esta documentación.

[![Barra superior de Speedle Cloud Console con el selector Espacio de trabajo desplegado y la barra lateral izquierda](/img/cloud/console/01-console-shell-workspace-switcher.png)](https://engine.speedle.io/img/cloud/console/01-console-shell-workspace-switcher.png)

Diseño de la consola con el menú **Espacio de trabajo** abierto y la navegación en la barra lateral izquierda (datos de demostración, interfaz en inglés).

## Identidad personal y espacios de trabajo

Speedle Cloud separa su **cuenta personal** de sus **espacios de trabajo**:

- Su **cuenta personal** guarda su correo verificado, los proveedores de inicio de sesión conectados (GitHub y/o Google), las sesiones del navegador y el registro **Actividad personal** (`/account/audit`).
- Un **espacio de trabajo** es un contenedor aislado para servicios, políticas de autorización y de roles (incluidos **Roles globales**), funciones personalizadas, claves de API, instancias de la flota de ADS, el **Registro de auditoría** del espacio de trabajo y la cuota diaria de API.

> Algunas etiquetas de la interfaz actual todavía muestran «cuenta». Los límites de recursos, políticas, servicios, instancias ADS y claves de API se aplican al espacio de trabajo seleccionado, mientras que la identidad personal y las sesiones pertenecen a la cuenta.

Cada cuenta comienza con un espacio de trabajo **Personal**. Puede pertenecer a hasta 10 espacios de trabajo y cada espacio de trabajo admite hasta 10 miembros (contando invitaciones pendientes).

### Cambiar o crear un espacio de trabajo

1. En la barra superior, abra el menú **Espacio de trabajo**.
2. Seleccione otro espacio de trabajo o elija **Crear espacio de trabajo**.
3. Al crear un espacio de trabajo, introduzca un nombre (1–64 caracteres) y elija **Crear espacio de trabajo**.

**Resultado esperado:** El nuevo espacio de trabajo comienza vacío con sus propios servicios, políticas, claves de API y cuota diaria, y usted se convierte en su **Propietario**. La URL cambia a `/w/<workspaceId>/service`. Al cambiar de espacio de trabajo se borra cualquier secreto de un solo uso visible en pantalla.

### Gestionar miembros y roles del espacio de trabajo

Abra **Ajustes del espacio de trabajo → Miembros** en la barra lateral izquierda.

[![Página Miembros del espacio de trabajo con roles de Propietario y Administrador, formulario de invitación, invitaciones pendientes y Salir del espacio de trabajo](/img/cloud/console/02-workspace-members-page.png)](https://engine.speedle.io/img/cloud/console/02-workspace-members-page.png)

Página **Miembros** con roles, formulario de invitación, invitaciones pendientes y **Salir del espacio de trabajo** (datos de demostración, interfaz en inglés).

| Rol | Qué puede hacer el miembro |
| --- | --- |
| **Propietario** | Todo lo que puede hacer un Administrador, además de invitar miembros, revocar invitaciones pendientes, cambiar roles y quitar miembros. |
| **Administrador** | Gestionar servicios, políticas, importaciones/exportaciones, funciones, Playground, claves de API y flota de ADS; consultar la lista de miembros y el registro de auditoría del espacio de trabajo; salir del espacio de trabajo. |

**Invitar a un miembro (solo Propietario):**

1. En **Invitar a alguien**, introduzca la **Dirección de correo** y seleccione un rol (**Administrador** o **Propietario**).
2. Elija **Enviar invitación**.

**Resultado esperado:** La consola envía un correo con un enlace de invitación de un solo uso válido durante 7 días y lo añade a **Invitaciones pendientes** (cuenta para el límite de 10 miembros; cada invitador puede enviar hasta 20 invitaciones por día UTC). Los Propietarios ven los correos completos y los Administradores los ven enmascarados. Para cancelar una invitación pendiente, elija **Revocar** junto a ella.

Para aceptar una invitación, la persona invitada abre el enlace del correo, inicia sesión con una cuenta de GitHub o Google que tenga ese correo verificado y elige unirse al espacio de trabajo.

**Cambiar roles, quitar un miembro o salir:**

- **Cambiar rol (solo Propietario):** Use el selector de rol junto al miembro para ascender a un Administrador a Propietario o degradar a un Propietario a Administrador.
- **Quitar un miembro (solo Propietario):** Elija **Quitar**, revise la advertencia y confirme. Su acceso a la consola termina en su siguiente solicitud.
- **Salir del espacio de trabajo:** Elija **Salir del espacio de trabajo** y confirme.

> **Las claves de API siguen activas cuando un miembro sale o es eliminado.** Las claves de API pertenecen al espacio de trabajo, no a quien las creó. Tras quitar a un miembro, la consola muestra las claves activas que creó (nombre, ID y caducidad, sin secretos). Revíselas en **Claves de API** y revoque o rote las que ya no deban ser de confianza.

**Recuperación:**

- **Verificación reciente (`reauth_required`):** Cambiar roles, quitar miembros y salir del espacio de trabajo requieren haber iniciado sesión en los últimos 10 minutos. Verifique con un proveedor conectado y repita la acción.
- **Último propietario (`last_owner`):** Un espacio de trabajo debe mantener al menos un Propietario. Ascienda primero a otro miembro a **Propietario** antes de degradarse o salir.
- **Correo distinto o enlace no válido:** Pida a la persona invitada que inicie sesión con una cuenta donde ese correo esté verificado (cualquier correo verificado en GitHub sirve), o envíe una nueva invitación si el enlace de 7 días caducó o fue revocado.

## Gestionar servicios y políticas

El siguiente recorrido usa un servicio `books` con una asignación de rol y una política de autorización.

### Crear un servicio

1. Abra **Servicios y políticas** en la barra lateral izquierda.
2. Si el espacio de trabajo aún no tiene servicios, escriba `books` y elija **Crear servicio**. Si ya existen servicios, abra el selector **Servicio actual**, elija **Añadir otro servicio**, escriba `books` y elija **Crear servicio**.

Los nombres de servicio tienen entre 1 y 64 caracteres (letras, números, puntos, guiones bajos o guiones, empezando por letra o número). El nombre `global` está reservado para **Roles globales**. Las aplicaciones envían este nombre como `serviceName` en las solicitudes de autorización.

### Crear políticas de autorización y de roles

1. Con `books` seleccionado, abra la pestaña **Políticas** y elija **Crear política**.
2. Opcionalmente introduzca un **Nombre de la política** (hasta 128 caracteres Unicode), como `reader-role`. Si lo deja vacío, se genera un nombre automáticamente.
3. En **Código SPDL**, escriba una sentencia SPDL (hasta 1.024 bytes UTF-8):

```text
grant user alice from corp Reader
```

4. Espere brevemente a la validación en vivo. El editor valida automáticamente tras pausar la escritura y muestra el tipo detectado (política de roles o de autorización), así como errores de sintaxis o advertencias de revisión.
5. Elija **Guardar política**.
6. Repita el proceso para crear una política de autorización (por ejemplo `read-books`) que conceda el permiso al rol:

```text
grant role Reader get /books
```

**Resultado esperado:** Ambas políticas aparecen en la tabla de `books`. Las políticas guardadas se aplican de inmediato en **Playground** y se propagan a la API de autorización alojada en unos 60 segundos.

[![Editor Crear política con read-books, código SPDL, validación automática válida y botón Guardar política](/img/cloud/console/04-policy-editor-live-validation.png)](https://engine.speedle.io/img/cloud/console/04-policy-editor-live-validation.png)

Editor de políticas mostrando la validación automática SPDL antes de guardar `read-books` (datos de demostración, interfaz en inglés).

[![Vista Servicios y políticas para books con reader-role, read-books y pestañas del servicio](/img/cloud/console/03-service-policies-list.png)](https://engine.speedle.io/img/cloud/console/03-service-policies-list.png)

Tabla **Servicios y políticas** de `books` con `reader-role` y `read-books` junto a las pestañas del servicio (datos de demostración, interfaz en inglés).

Puede alternar entre el editor de código y el editor de texto plano, buscar o filtrar por efecto, entidad principal, acción, recurso, rol o condición, y editar, inspeccionar detalles, probar o eliminar desde cada fila.

### Usar Roles globales entre servicios

Cuando varios servicios del mismo espacio de trabajo comparten una asignación de rol, defínala una vez en **Roles globales**:

1. Abra el selector **Servicio actual** y elija **Roles globales** en la parte superior.
2. Cree una política de roles, como `grant user alice from corp BookAdmin`.
3. Cambie a cada servicio normal que deba respetar ese rol (como `books`) y cree una política de autorización que conceda permisos a `BookAdmin`.

**Roles globales** solo acepta políticas de roles y no se puede renombrar ni eliminar. Un rol global no concede permisos hasta que un servicio normal defina una política de autorización para ese rol. Consulte [Roles globales](/es/docs/cloud/global-roles/).

### Inspeccionar relaciones en el Mapa de acceso

Abra la pestaña **Mapa de acceso** dentro de **Servicios y políticas** para visualizar cómo se conectan entidades principales, roles y recursos en las políticas guardadas del servicio. Puede filtrar el grafo, cambiar a la vista de tabla, enfocar un nodo y enviar una ruta directamente a **Playground** para ejecutar un diagnóstico.

[![Grafo Mapa de acceso de books conectando a alice mediante el rol Reader con /books](/img/cloud/console/05-access-map.png)](https://engine.speedle.io/img/cloud/console/05-access-map.png)

**Mapa de acceso** de `books` mostrando `alice` → `Reader` → `/books` desde políticas guardadas (datos de demostración, interfaz en inglés).

### Importar y exportar políticas

Abra la pestaña **Importar y exportar** dentro de **Servicios y políticas**:

- **Descargar SPDL** o **Descargar JSON:** Exporta las políticas guardadas del servicio seleccionado. Mantenga los archivos exportados en privado y conserve sus propias copias de seguridad durante la beta.
- **Importar políticas:** Seleccione un archivo JSON exportado desde Speedle Cloud (hasta 32 MiB), revise el resumen previo y elija **Confirmar importación de políticas**. Las importaciones son atómicas: nunca sobrescriben políticas existentes y permiten nombres visibles duplicados (tanto dentro del lote como respecto a políticas existentes), ya que cada política importada recibe un nuevo ID generado por el servidor. Solo una política no válida o superar el límite de 200 políticas del espacio de trabajo rechaza todo el lote.

Si una importación se interrumpe o su resultado es incierto, mantenga la página abierta y use **Reintentar la misma importación**: reutiliza la misma clave de idempotencia para no insertar dos veces políticas ya confirmadas. Iniciar una importación nueva del mismo archivo (con una nueva clave de idempotencia) tras una importación completada crea políticas duplicadas y consume cupo de políticas.

### Cambiar el nombre o eliminar un servicio

Use el menú **Acciones del servicio** junto al encabezado del servicio:

- **Cambiar nombre…:** Cambia el nombre del servicio. Los ID de políticas, claves de API e identidades de instancias ADS se mantienen, pero las aplicaciones deben actualizar `serviceName` y la propagación no es inmediata.
- **Eliminar servicio…:** Elimina permanentemente el servicio y todas sus políticas, y lo retira de las instancias ADS asociadas. Escriba el nombre del servicio para confirmar. Requiere autenticación en los últimos 10 minutos; si se solicita, verifique con un proveedor conectado y vuelva a confirmar. Las claves de API y las instancias ADS permanecen.

**Recuperación:**

- **Editar no cambia el tipo de política:** Una política de autorización guardada no puede convertirse en política de roles (ni viceversa). Elimínela y cree una nueva.
- **Edición obsoleta:** Si otra sesión modificó o eliminó la política mientras editaba, su borrador se conserva. Actualice el espacio de trabajo y abra la última versión antes de guardar.
- **Límite de revisiones alcanzado (`revision_exhausted`):** Exporte las políticas, cree un nuevo servicio, importe el archivo, cambie los clientes al nuevo `serviceName` y opcionalmente elimine el servicio anterior. Consulte [Límites y errores](/es/docs/cloud/limits-and-errors/#revision-exhausted).

## Probar decisiones en Playground

1. Seleccione `books` en **Servicios y políticas** y abra la pestaña **Playground**.
2. Elija una **Operación**:
   - **Está permitido:** Devuelve permitido o denegado y el código de motivo.
   - **Diagnosticar:** Muestra la decisión y la traza de cómo contribuyeron las políticas, roles y condiciones.
   - **Todos los roles concedidos:** Lista todos los roles concedidos al sujeto en este servicio (incluidos Roles globales).
   - **Todos los permisos concedidos:** Lista todos los recursos y acciones concedidos al sujeto.
3. Complete los campos de prueba:
   - **Entidades principales (JSON):** `[{"type":"user","name":"alice","idd":"corp"}]`
   - **Recurso:** `/books`
   - **Acción:** `get`
   - **Atributos (JSON):** `[]`
4. Elija **Ejecutar prueba**.

**Resultado esperado:** **Está permitido** muestra permitido con el resultado JSON completo. Al cambiar **Acción** a `delete` y volver a ejecutar, muestra denegado porque ninguna política coincide.

[![Prueba Está permitido en Playground para alice en corp sobre /books devolviendo Allowed y resultado JSON completo](/img/cloud/console/06-playground-is-allowed-and-diagnose.png)](https://engine.speedle.io/img/cloud/console/06-playground-is-allowed-and-diagnose.png)

Comprobación **Está permitido** en **Playground** para `alice` (`corp`) en `/books` (`get`) con resultado permitido y JSON completo (datos de demostración, interfaz en inglés).

También puede probar una fila de política o precargar un diagnóstico desde **Mapa de acceso** sin consumir una llamada hasta pulsar **Ejecutar prueba**.

**Tenga en cuenta:**

- Playground evalúa las políticas **guardadas** al instante; ignora borradores sin guardar.
- Cada ejecución consume una llamada de la cuota diaria del espacio de trabajo y nunca modifica políticas.
- Probar directamente contra **Roles globales** siempre deniega; pruebe los roles globales en el Playground de un servicio normal.
- Una prueba exitosa en Playground no garantiza que todas las instantáneas publicadas de ADS hayan terminado de propagarse (calcule ~60 segundos para la [API de autorización](/es/docs/cloud/authorization-api/) alojada).

## Gestionar claves de API del espacio de trabajo

Las claves de API autentican aplicaciones de servidor e instancias autoalojadas de Speedle Cloud ADS frente al espacio de trabajo activo. Gestionar claves siempre requiere una sesión iniciada en la consola.

### Crear una clave de API

1. Abra **Claves de API** en la barra lateral izquierda.
2. Elija **Crear clave de API**.
3. Introduzca un **Nombre de la clave de API** descriptivo (1–128 caracteres, por ejemplo `books-app`). No escriba un secreto en el nombre.
4. Seleccione un **Ámbito** (fijo al crear la clave):
   - **Solo comprobaciones de autorización (ADS)** (`ads`, predeterminado): solo llama a `/authz-check/v1/*`.
   - **Gestión de políticas: lectura** (`pms:read`): lee servicios y políticas del espacio de trabajo.
   - **Gestión de políticas: escritura** (`pms:write`): lee, crea y elimina servicios y políticas del espacio de trabajo.
   - **Sincronización de ADS** (`pms:read+ads`): sincroniza instantáneas de políticas para Speedle Cloud ADS (`speedle-cloud`), lee políticas y ejecuta comprobaciones de autorización; sin escritura.
5. Seleccione **Caduca en** (1 semana, 1 mes, 3 meses [predeterminado], 6 meses o 1 año) y revise **Vista previa de caducidad (UTC)**. No existen claves sin caducidad.
6. Elija **Crear clave de API**. Crear una clave requiere autenticación con un proveedor en los últimos 10 minutos; si se solicita, verifique su identidad y vuelva a enviar el formulario.
7. Copie el secreto de un solo uso de inmediato, guárdelo en el almacén de secretos de su servidor (por ejemplo `SPEEDLE_API_KEY`) y cierre la ventana tras confirmar que lo guardó.

**Resultado esperado:** La clave aparece en **Claves de API activas** con su ID, ámbito, creación, último uso aproximado y fecha de caducidad (UTC). El secreto nunca vuelve a mostrarse ni puede recuperarse desde los metadatos.

[![Página Claves de API con el formulario Crear clave de API y la lista de metadatos activos sin secretos](/img/cloud/console/07-api-keys-list-and-create-form.png)](https://engine.speedle.io/img/cloud/console/07-api-keys-list-and-create-form.png)

Página **Claves de API** con el formulario **Crear clave de API** y la lista de metadatos activos (datos de demostración, interfaz en inglés; no se muestra ningún secreto).

### Rotar y revocar claves de API

Cada espacio de trabajo admite hasta **10 claves de API activas** entre todos los ámbitos. Las claves que caducan en 7 días se marcan como próximas a caducar; las caducadas pasan a **Claves de API caducadas**.

Para rotar una clave sin interrupción:

1. Cree una clave de reemplazo con el ámbito y caducidad necesarios.
2. Actualice su aplicación o configuración de ADS para usar la nueva clave.
3. En **Claves de API**, elija **Revocar** junto a la clave anterior, compruebe el nombre y el ID y confirme **Revocar clave de API**.

**Recuperación:**

- **Retraso de propagación:** La verificación en caché puede seguir aceptando una clave revocada hasta 60 segundos; la revocación no es instantánea en todo el mundo.
- **Límite alcanzado (`quota_exceeded`):** Revoque una clave sin usar o deje caducar una existente. Las claves revocadas y caducadas siguen en el historial y no cuentan para el límite de 10.
- **Resultado incierto:** Si un error de red interrumpe la creación o revocación, actualice los metadatos y revise las listas antes de actuar de nuevo. Nunca asuma que un nombre igual identifica un secreto perdido.

Consulte [Claves API](/es/docs/cloud/tokens/) para ver el contrato completo.

## Consultar uso y cuotas

En la beta pública, los límites de recursos y la cuota diaria se aplican por espacio de trabajo:

- **Servicios:** 100 por espacio de trabajo (incluido **Roles globales**).
- **Políticas:** 200 políticas de autorización y roles combinadas en todo el espacio de trabajo (cada sentencia SPDL hasta 1.024 bytes UTF-8).
- **Claves de API activas:** 10 por espacio de trabajo entre todos los ámbitos.
- **Llamadas API diarias:** 10.000 por espacio de trabajo y día UTC (se restablece a las `00:00 UTC`). Las lecturas/escrituras de gestión, validación en vivo, pruebas en Playground, importaciones/exportaciones y comprobaciones de autorización comparten esta cuota.

Dónde consultar el uso en la consola:

- **Cuenta → Resumen de la cuenta (`/account`):** Muestra la tarjeta **Cuota diaria** con barra de progreso y la próxima hora de restablecimiento UTC de su espacio de trabajo Personal.
- **Servicios y políticas:** Muestra el recuento de servicios y políticas y las llamadas diarias restantes aproximadas del espacio de trabajo activo tras las operaciones.

[![Resumen de la cuenta mostrando el perfil y la tarjeta Cuota diaria con barra de uso y hora de restablecimiento UTC](/img/cloud/console/10b-account-overview-quota.png)](https://engine.speedle.io/img/cloud/console/10b-account-overview-quota.png)

**Resumen de la cuenta** con el perfil y el medidor de **Cuota diaria** del espacio de trabajo Personal (datos de demostración, interfaz en inglés).

Si agota la cuota diaria (`429 daily_quota_exceeded`), espere hasta las `00:00 UTC`; cambiar de clave de API en el mismo espacio de trabajo no añade cuota. Consulte [Límites y errores](/es/docs/cloud/limits-and-errors/).

## Funciones personalizadas (por servicio)

> Cuando las funciones personalizadas están habilitadas en el entorno, los servicios normales muestran la pestaña **Funciones** entre **Mapa de acceso** y **Playground**.

Las funciones personalizadas permiten que las condiciones SPDL llamen a endpoints HTTPS externos durante la evaluación. Cada servicio normal puede registrar hasta **20 funciones**.

### Registrar y gestionar una función

1. Seleccione un servicio normal y abra la pestaña **Funciones**.
2. Elija **Registrar función**.
3. Configure los campos:
   - **Nombre:** Identificador SPDL de 1–64 caracteres (no puede ser una palabra reservada).
   - **URL del endpoint:** Endpoint HTTPS (hasta 2.048 bytes UTF-8; HTTP sin cifrar no está permitido) que recibe los argumentos de la condición. La consola nunca llama al endpoint. Las reglas dependen de **Invocación**:
     - **Alojado y ADS:** Requiere `https://` en el puerto predeterminado `443` con un nombre DNS público y no reservado (al menos dos etiquetas; rechaza direcciones IP literales, puertos distintos de 443 y nombres internos o reservados como `localhost`, `*.local`, `*.internal`, `*.home.arpa`, `.test`, `.example`, `.invalid`, `.alt` y `.onion`).
     - **Solo ADS:** Requiere `https://` y permite cualquier puerto, así como IP literales y nombres privados o internos (por ejemplo `https://10.20.30.40:8443/func/check`).
     - Ambos modos rechazan credenciales en la URL (`user@host`), fragmentos (`#...`), espacios, caracteres de control y barras invertidas.
   - **Invocación:** **Alojado y ADS** (invocable desde la evaluación alojada en Cloud, Playground y ADS autoalojado) o **Solo ADS** (invocable solo desde ADS autoalojado; la evaluación alojada y Playground fallan si una condición alcanza una función Solo ADS).
   - **Tipos declarados (opcional):** Añada hasta 16 parámetros y un tipo de resultado. Sin declaraciones no se comprueban los tipos de argumentos; una discrepancia de tipo declarado hace fallar la evaluación.
   - **Almacenar resultados en caché (opcional):** TTL local del evaluador (1–86.400 segundos; los fallos no se almacenan en caché).
   - **Tiempo de espera alojado (ms):** 100–2.000 ms (predeterminado 1.000 ms), aplicado solo a llamadas alojadas.
   - **Firmar solicitudes alojadas (opcional):** Habilita firma HMAC solo para solicitudes alojadas desde Cloud. El ADS autoalojado siempre envía solicitudes sin firmar, incluso cuando está seleccionado **Alojado y ADS**.
4. Elija **Guardar función**. Si habilitó la firma, copie y guarde el secreto de firma de un solo uso antes de cerrar el diálogo.

Desde la tabla de funciones puede editar una función, **Rotar secreto de firma** (el secreto anterior deja de funcionar de inmediato; puede requerir autenticación reciente) o **Eliminar función**.

[![Pestaña Funciones de books listando funciones personalizadas Alojado y ADS y Solo ADS](/img/cloud/console/08-functions-tab.png)](https://engine.speedle.io/img/cloud/console/08-functions-tab.png)

Pestaña **Funciones** de `books` mostrando funciones de ejemplo **Alojado y ADS** y **Solo ADS** (datos de demostración, interfaz en inglés; sin llamadas a endpoints reales).

**Reglas importantes y recuperación:**

- **`function_url_rejected`:** Compruebe que la URL usa `https://` sin información de usuario, fragmentos, espacios ni barras invertidas. Para IP literales, nombres internos o puertos distintos de 443, elija **Solo ADS**; para **Alojado y ADS**, use un nombre DNS público en el puerto `443`.
- Registre la función en el servicio de destino **antes** de guardar o importar una política que la llame. Las exportaciones de políticas no incluyen definiciones de funciones.
- No se puede eliminar una función mientras alguna política la referencie; edite o elimine primero esas políticas.

## Supervisar y controlar la flota de ADS

Abra **Flota de ADS** en la barra lateral izquierda para supervisar las instancias autoalojadas de Speedle Cloud ADS (`speedle-cloud`) que sincronizan políticas desde el espacio de trabajo activo. Cloud nunca inicia conexiones hacia su red; los comandos viajan por el canal que abre cada instancia.

### Conectar su primera instancia de ADS

No hay registro manual. Cuando la flota está vacía, **Flota de ADS** muestra una guía de tres pasos y un fragmento de configuración copiable:

1. Cree una clave de API con el ámbito **Sincronización de ADS** (`pms:read+ads`) en **Claves de API**.
2. Coloque la clave en la configuración de ADS (permisos `0600`) con un `CacheDir` persistente para conservar la identidad de la instancia entre reinicios.
3. Inicie `speedle-cloud` con su configuración y el TLS del listener de aplicaciones. Aparecerá automáticamente en **Flota de ADS** al conectarse por primera vez.

Cada espacio de trabajo admite hasta **10 instancias ADS en línea** y conserva hasta **100 registros de instancias** (las instancias sin conexión durante 7 días se eliminan automáticamente).

### Inspeccionar instancias y ejecutar acciones de distribución

Use **Filtro de servicio** para ver instancias que sincronizan un servicio concreto o **Actualizar flota** para refrescar de inmediato (la página se actualiza automáticamente cada 5 minutos mientras es visible).

[![Página Flota de ADS mostrando una instancia ADS en línea sincronizando books](/img/cloud/console/09-ads-fleet-view.png)](https://engine.speedle.io/img/cloud/console/09-ads-fleet-view.png)

Vista **Flota de ADS** con una instancia en línea de demostración sincronizando `books` (datos de demostración, interfaz en inglés).

Elija **Detalles** en una instancia para revisar el estado en tiempo real, el último latido (UTC), el canal y transporte (WebSocket o sondeo largo), el conjunto de servicios con retraso de revisión y **Roles globales** (siempre sincronizados), la cadena de distribución, la cronología de eventos, el historial de respaldo y los datos del host.

Acciones disponibles (cada una pide confirmación):

- **Asignar servicios:** Sincronice todos los servicios (incluidos los futuros) o solo los seleccionados. El cambio se aplica en el siguiente latido sin reiniciar ADS.
- **Resincronizar desde una instantánea:** Descarta la copia local de la instancia y vuelve a cargar una instantánea. **Impacto en la disponibilidad:** el servicio afectado deja de responder decisiones de autorización locales de inmediato hasta descargar y verificar la nueva instantánea.
- **Pausar distribución / Reanudar distribución:** Detiene o reanuda el envío de políticas a instancias ADS descendentes mientras continúan la sincronización y evaluación locales.
- **Detener este ADS:** Indica a la instancia que deniegue todas las solicitudes de autorización y deje de reconectarse hasta reiniciarla.
- **Eliminar esta instancia:** Elimina el registro y el historial (una instancia aún en ejecución volverá a registrarse en su siguiente conexión salvo que la detenga o revoque su clave de API).

## Revisar registros de auditoría y gestionar la configuración de la cuenta

### Registros de auditoría

Speedle Cloud mantiene dos registros de auditoría independientes, ordenados del más reciente al más antiguo, con **180 días de retención** y paginación:

- **Registro de auditoría del espacio de trabajo** (barra lateral izquierda → **Registro de auditoría**): Registra la actividad administrativa del espacio de trabajo activo: cambios en servicios y políticas, creación/revocación de claves de API, acciones de ADS, cambios de espacio de trabajo y cambios de miembros/invitaciones. Legible por Propietarios y Administradores.
- **Actividad personal** (barra superior → **Cuenta** → **Resumen de la cuenta** → **Actividad personal**, en `/account/audit`): Registra los eventos personales de su cuenta: inicio/cierre de sesión, cierre en todos los dispositivos, conexión/desconexión de proveedores y creación/eliminación de la cuenta.

[![Tabla del Registro de auditoría del espacio de trabajo con hora UTC, acción, objetivo y resultado](/img/cloud/console/10a-workspace-audit-log.png)](https://engine.speedle.io/img/cloud/console/10a-workspace-audit-log.png)

**Registro de auditoría** del espacio de trabajo con eventos administrativos recientes en UTC (datos de demostración, interfaz en inglés).

### Configuración de la cuenta

Abra el menú **Cuenta** en la barra superior y elija **Configuración de la cuenta** (`/account/settings`):

- **Identidades conectadas:** Conecte tanto **GitHub** como **Google** para disponer de un método de acceso de respaldo. Debe conservar al menos una identidad conectada; desconectar un proveedor requiere autenticación en los últimos 10 minutos.
- **Cerrar sesión / Cerrar sesión en todos los dispositivos:** Disponibles en el menú **Cuenta** de la barra superior.
- **Eliminar cuenta:** Elimina permanentemente su cuenta personal, identidades conectadas, espacios de trabajo en los que sea el único miembro y sesiones. Elija **Eliminar cuenta**, verifique su identidad si se solicita, escriba `DELETE` y confirme. Exporte antes las políticas que desee conservar.

[![Configuración de la cuenta mostrando Identidades conectadas (GitHub y Google) y la tarjeta Eliminar cuenta](/img/cloud/console/10c-account-settings.png)](https://engine.speedle.io/img/cloud/console/10c-account-settings.png)

**Configuración de la cuenta** con **Identidades conectadas** y **Eliminar cuenta** (datos de demostración, interfaz en inglés).

**Recuperación al eliminar la cuenta:** Si es el único **Propietario** de un espacio de trabajo que aún tiene otros miembros, la eliminación queda bloqueada (`last_owner`). Abra **Miembros** en ese espacio de trabajo y ascienda a otro miembro a **Propietario** antes de eliminar su cuenta.
