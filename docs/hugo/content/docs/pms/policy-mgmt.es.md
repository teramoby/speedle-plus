+++
title = "Gestión de políticas"
description = "Gestión del ciclo de vida de las políticas"
weight = 1
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["pms", "policy", "core"]
categories = ["docs"]
bref = "Conceptos básicos de la gestión de políticas"
lastmod = 2026-11-09T07:57:54+08:00
+++

## ¿Qué es una política de Speedle?

El Servicio de Gestión de Políticas (PMS) administra servicios, políticas de autorización, políticas de roles y funciones personalizadas. Un almacén de políticas persiste estos objetos utilizando un [backend de almacenamiento](/es/docs/store/) registrado.

<img src="/img/speedle/policystore.png" width="1942" height="540" loading="lazy"/>

## Servicios y políticas de autorización

Un servicio delimita el alcance de sus políticas. Una política de autorización concede o deniega a una entidad principal el permiso para realizar una acción sobre un recurso, opcionalmente sujeto a una condición. Salvo por las [políticas de roles globales](/es/docs/pms/global-policy/), las políticas no se aplican automáticamente en otro servicio.

<img src="/img/speedle/authzpolicy.png" width="1770" height="444" loading="lazy"/>

```text
grant group Administrators list,watch,get expr:^c1/default/core/pods/.*$
```

Esta expresión regular coincide con los recursos bajo `c1/default/core/pods/`. No coincide con `c1/default/core/podssecret` ni con un prefijo adicional antes de `c1`. `expr:` utiliza coincidencias de expresiones regulares, no patrones glob de shell: use `.*` para caracteres arbitrarios y `^`/`$` cuando deba coincidir el identificador completo.

## Políticas de roles

<img src="/img/speedle/rolepolicy.png" width="1598" height="436" loading="lazy"/>

```text
grant user alan manager on res1
grant role manager update res1
```

La primera instrucción asigna a Alan un rol con alcance de recurso; la segunda concede a ese rol una operación. La asignación de un rol por sí sola no genera permisos. Una denegación explícita aplicable anula una concesión.

## Entidades principales y condiciones

Una entidad principal puede ser un usuario, grupo, entidad o rol. Una entidad representa una identidad no humana, como un servicio. Las integraciones deben derivar estas identidades a partir de fuentes de confianza.

<img src="/img/speedle/principal.png" width="1769" height="505" loading="lazy"/>

Un grupo AND requiere cada entidad principal enumerada; los grupos separados son alternativas. Por ejemplo, ambos roles a continuación deben cumplirse:

<img src="/img/speedle/andprincipal.png" width="1584" height="349" loading="lazy"/>

```text
grant (role designer, role dba) update db_design_doc
```

Un recurso designa el objeto protegido; una acción designa una operación de la aplicación. Una condición es una expresión booleana evaluada a partir de atributos de confianza, constantes, operadores y funciones compatibles.

<img src="/img/speedle/resource.png" width="1732" height="349" loading="lazy"/>

Consulte [SPDL](/es/docs/spdl/) para la gramática y [RBAC](/es/docs/overview/)/[ABAC](/es/docs/overview/) para ejemplos completos de decisiones.

## Antes de ejecutar los ejemplos

Compile `./bin/spctl`, `./bin/speedle-pms` y `./bin/speedle-ads` mediante el [inicio rápido](/es/quick-start/). Utilice Go 1.26.10 o Go 1.27.3 (o un parche más reciente de la misma rama de lanzamiento), Bash, curl y Python 3. Ejecute cada terminal desde la raíz del repositorio. Utilice un archivo de políticas nuevo para esta página; reinícielo únicamente tras detener ambos servicios.

Inicialice un archivo de políticas vacío:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

> **macOS:** `/usr/sbin/spctl` es la herramienta Gatekeeper de Apple y puede seleccionarse al ejecutar el comando básico `spctl`. Invoque la CLI de Speedle mediante su ruta completa o use `./spctl` desde el directorio que lo contiene; los ejemplos a continuación utilizan `./bin/spctl`. Ejecute `command -v spctl` para comprobar qué ejecutable selecciona su shell. Si la versión descargada incluye un binario `speedle-spctl_<os>_<arch>`, se trata de la misma CLI con un nombre que no colisiona con la herramienta de Apple.

En la primera terminal, inicie PMS y déjelo en ejecución:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

En la segunda terminal, inicie ADS y déjelo en ejecución:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

Ejecute los comandos restantes en una tercera terminal. PMS y ADS comparten el archivo; watch propaga los cambios de forma asíncrona. Confirme que la actualización deseada ha llegado a ADS antes de depender de ella. `--insecure=true` es solo para este ejemplo en loopback; consulte [seguridad](/es/docs/security/) para conexiones de producción. Indicar direcciones explícitas de PMS evita modificar `~/.spctlconfig`. Detenga ambas terminales de servidor con Ctrl-C al terminar.

## Gestionar servicios y políticas con spctl

Ejecute esta secuencia en un almacén limpio. Los IDs proceden de las respuestas de consultas en tiempo real; no son valores fijos copiados de una ejecución anterior. La eliminación se produce después de todas las operaciones que necesitan el servicio.

<!-- docs-check: policy-management -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy policy1 -c "grant user alan read book" --service-name test
policy_id=$(./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get policy --all --service-name test | python3 -c 'import json,sys; print(next(p["id"] for p in json.load(sys.stdin) if p.get("name")=="policy1"))')
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get policy "$policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy rolepolicy01 -c "grant user alan manager on res1" --service-name test
role_policy_id=$(./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get rolepolicy --all --service-name test | python3 -c 'import json,sys; print(next(p["id"] for p in json.load(sys.stdin) if p.get("name")=="rolepolicy01"))')
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get rolepolicy "$role_policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete policy "$policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete rolepolicy "$role_policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete service test
```

## Speedle Cloud

Use una clave API de cuenta para gestionar servicios y políticas de Cloud con `spctl`. El endpoint de gestión es:

```text
https://cloud.speedle.io/policy-mgmt/v1
```

Elija un único ámbito fijo para una clave API de cuenta. `pms:read` permite leer servicios y políticas; `pms:write` permite además crearlos y eliminarlos. Ambos se aplican a los servicios de la cuenta. Una clave `ads` no puede gestionar políticas y recibe `403 insufficient_scope`. Consulte [Claves API](/es/docs/cloud/tokens/) para creación y revocación.

Pase el endpoint como un indicador global y lea la clave API desde `SPCTL_TOKEN`. En una sesión interactiva, esto evita que el secreto quede expuesto en los argumentos de comando o en el historial de la terminal:

<!-- deployment-template: cloud-token-environment -->
```bash
read -rsp 'API key: ' SPCTL_TOKEN; echo
export SPCTL_TOKEN
./bin/spctl --pms-endpoint https://cloud.speedle.io/policy-mgmt/v1 get service --all
unset SPCTL_TOKEN
```

El indicador `--token` sigue estando disponible por compatibilidad, pero la variable de entorno evita exponer la clave API mediante los argumentos de comando. Cada indicador global dispone también de una variable de entorno: anteponga `SPCTL_`, ponga el nombre en mayúsculas y reemplace los guiones por guiones bajos. Por ejemplo, `SPCTL_PMS_ENDPOINT` define `pms-endpoint` y `SPCTL_SKIPVERIFY` define `skipverify`.

Para una configuración guardada, utilice `~/.speedle`. Su formato es un par `nombre=valor` por línea, con los mismos nombres de indicadores que la línea de comandos:

```ini
pms-endpoint=https://cloud.speedle.io/policy-mgmt/v1
token=<api-key>
```

Cree o actualice este archivo sin colocar la clave API en argumentos de comando:

<!-- deployment-template: cloud-token-config -->
```bash
read -rsp 'API key: ' SPCTL_TOKEN; echo
printf '%s\n' "$SPCTL_TOKEN" | ./bin/spctl config token --stdin
unset SPCTL_TOKEN
```

`spctl config token --stdin` escribe en `~/.speedle` con permisos `0600`. Si edita el archivo manualmente, créelo con `install -m 600 /dev/null ~/.speedle` o ejecute `chmod 0600 ~/.speedle` a continuación. `spctl` emite una advertencia en la salida de error estándar si lee una clave API de un archivo cuyos permisos permiten acceso a grupos u otros usuarios. `spctl config --list` enmascara la clave API configurada como `********`.

Los indicadores explícitos de la línea de comandos anulan las variables de entorno, que a su vez anulan ambos archivos de configuración. Entre los archivos, `~/.speedle` anula a `~/.spctlconfig`. Definir un campo de configuración distinto de `token` con `spctl config` la escribe en `~/.spctlconfig` y elimina cualquier copia duplicada en `~/.speedle`.

La clave API se envía como una credencial `Authorization: Bearer` únicamente a través de HTTPS con la verificación de certificados activada. HTTP en texto plano solo se permite con una clave API para hosts de bucle invertido como `127.0.0.1`, `::1` o `localhost`; `spctl` rechaza endpoints HTTP remotos y redirecciones de degradación de HTTPS a HTTP antes de enviar la credencial. Para endpoints HTTPS, también rechaza enviar una clave API si se especifica `--skipverify=true`; utilice `--cacert` para confiar en una entidad emisora de certificados privada o autofirmada. Las claves API no se incluyen en la salida estándar de los comandos ni en los mensajes de error.

Los errores de Cloud generan un código de salida distinto de cero. Compruebe la clave API tras un error `401`, y sus alcances tras un `403 insufficient_scope`. Un error `404` identifica un recurso inexistente, mientras que `409 service_exists` y `409 policy_exists` identifican duplicados. En caso de `429`, respete el valor devuelto en `Retry-After` y use un tiempo de espera exponencial limitado. Para `429 daily_quota_exceeded`, espere hasta las 00:00 UTC según `Retry-After`. Los límites de servicios, políticas y claves activas devuelven `409 quota_exceeded` y no se restablecen con el tiempo; elimine o revoque elementos sin uso. Consulte [Límites y errores](/es/docs/cloud/limits-and-errors/).

## Otras interfaces

La [API REST de PMS](/es/docs/api/management_api/) utiliza JSON y devuelve objetos planos de política con IDs en formato de cadena. Las aplicaciones en Go pueden llamar a las [interfaces de PMS](https://github.com/teramoby/speedle-plus/tree/master/api/pms) directamente; los clientes gRPC utilizan el [proto publicado de PMS](/protobuf/pms.proto). Aplique [controles de acceso al transporte y al emisor](/es/docs/security/) para las operaciones administrativas.
