+++
title = "Primeros pasos con Speedle Cloud"
linkTitle = "Primeros pasos"
description = "Prepárese para Speedle Cloud: inicie sesión, guarde una política SPDL, compruebe la autorización desde su aplicación con una clave API y pruebe en el Playground."
weight = 10
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Inicie sesión, guarde una política, cree una clave API y compruebe la autorización desde su aplicación."
lastmod = 2026-11-12T00:00:00+08:00
+++

**Speedle Cloud estará disponible próximamente.** Esta guía describe el flujo para cuando se habiliten el inicio de sesión en producción y las comprobaciones de autorización autenticadas; los ejemplos no se han ejecutado en un entorno Cloud real.

## Iniciar sesión con GitHub o Google (próximamente)

Abra la [consola Cloud](https://cloud.speedle.io/es/) en el navegador e inicie sesión con un proveedor disponible. Para una identidad nueva con un correo verificado sin usar, confirme **Crear cuenta**. Si el correo ya está en uso, inicie sesión con el proveedor vinculado que indica la consola.

## Crear un servicio y una política

Abra el [espacio de trabajo del servicio](https://cloud.speedle.io/es/service), introduzca `books` y seleccione **Crear servicio**. Use un servicio nuevo para que otras políticas no afecten a los resultados; no sobrescriba datos existentes.

Seleccione **Nueva política**, asígnele el nombre `read-books` e introduzca este SPDL:

```text
grant user alice from corp get /books
```

Esta política permite que `alice`, del dominio de identidad `corp`, ejecute `get` sobre `/books`. Una concesión sin `from` coincide con el nombre en cualquier dominio. Obtenga el dominio de datos de autenticación fiables y envíelo como `idd` en cada solicitud. Consulte [Dominios de identidad](/es/docs/idd/).

Seleccione **Validar política** y después **Guardar política**. La validación comprueba la sintaxis, pero no guarda el borrador. Espere aproximadamente 60 segundos para que la política guardada llegue a la API de autorización.

## Crear una clave API

Abra [Claves API](https://cloud.speedle.io/es/tokens), introduzca un nombre como `books-app`, seleccione el alcance `ads` (comprobaciones de autorización), elija una caducidad (la consola usa 3 meses de forma predeterminada y muestra la hora UTC) y pulse **Crear clave API**. Si se solicita verificar su identidad, complete la verificación y vuelva a enviar el formulario. Copie el secreto: solo se muestra una vez. Todas las claves caducan, incluidas las creadas antes de que la caducidad fuera obligatoria. [Gestión de claves API](/es/docs/cloud/tokens/) indica cuándo. Lea la caducidad de la lista y cree un reemplazo antes. Revóquela antes si ya no la necesita.

Guarde la clave en el almacén de secretos del servidor y proporciónela mediante la variable de entorno `SPEEDLE_API_KEY`. No la incluya en código del navegador ni en el repositorio.

## Comprobar la autorización desde su aplicación

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

Consulte la [API de autorización](/es/docs/cloud/authorization-api/) y los [SDK](/es/docs/sdks/), incluidos [Node.js/TypeScript](/es/docs/sdks/node/), [Python](/es/docs/sdks/python/), [Java](/es/docs/sdks/java/) y [Go](/es/docs/sdks/go/), para integrar su aplicación.

## Probar políticas guardadas en el Playground

Opcionalmente, abra **Playground** en el servicio y seleccione **Is allowed**. Introduzca `[{"type":"user","name":"alice","idd":"corp"}]` en **Principales (JSON)**, `/books` en **Recurso**, `get` en **Acción** y `[]` en **Atributos (JSON)**; después, seleccione **Ejecutar prueba**.

**Is allowed** muestra la decisión; **Diagnosticar** explica cómo contribuyeron las políticas; **Todos los roles concedidos** enumera los roles del sujeto; **Todos los permisos concedidos** enumera los recursos y acciones permitidos. Las pruebas usan políticas guardadas, no borradores. Repita la prueba después de cambiar entradas o políticas. Las aplicaciones usan la [API ADS](/es/docs/cloud/authorization-api/) con una clave API.

Siguientes pasos: para la gestión mediante código, cree una clave aparte con el ámbito `pms:write` (o `pms:read` para lectura) y use la [API PMS o spctl (Speedle Cloud)](/es/docs/pms/policy-mgmt/#speedle-cloud). La clave `ads` del paso anterior no puede gestionar políticas.
