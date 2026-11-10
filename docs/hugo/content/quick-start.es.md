+++
title = "Inicio rápido"
description = "Instale Speedle con go install o desde el código fuente, luego verifique la gestión de políticas, concesiones directas, concesiones de roles, decisiones condicionales (ABAC) y denegaciones."
weight = 10
draft = false
toc = false
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tocheading = "h2"
bref = "Instale Speedle con go install o compílelo desde el código fuente, cree políticas con SPDL y spctl, y pruebe decisiones de autorización mediante la API REST. Esta guía utiliza un almacenamiento de archivos local."
icon = "9. Getting Started.svg"
lastmod = 2026-11-10T17:04:32+08:00
+++

## Pruebe primero Speedle en el Playground

**Puede probar las funciones básicas de Speedle antes de instalar nada.** Abra el [Playground](../playground/) para editar políticas SPDL, fijar un usuario, un recurso y una acción, y ver decisiones reales de permitir o denegar en el navegador. No hace falta instalar Go ni tener un servidor en ejecución.

Continúe más abajo cuando esté listo para instalar y ejecutar Speedle localmente.

## Requisitos previos

Use Linux o macOS con Bash, Git, curl y **Go 1.26.10 o Go 1.27.3** (o un parche más reciente de la misma rama). Speedle admite las versiones con parches de Go 1.26 y Go 1.27; esta guía está verificada con **Go 1.26.10 y Go 1.27.3**. Go 1.27.3 es la versión estable recomendada para este recorrido.

Instale Go con las [instrucciones oficiales de instalación](https://go.dev/doc/install) y compruebe la versión instalada:

<!-- quick-start: version -->
```bash
go version
```

Go configura `GOROOT` y el `GOPATH` predeterminado de forma automática. No hacen falta valores personalizados. Las dos rutas siguientes colocan los tres ejecutables en un directorio `bin`.

## Instalar Speedle

Elija una ruta: la ruta A instala la última versión y la ruta B compila el código actual. El resto de la guía usa `./bin/...` y funciona con cualquiera de las dos.

### Ruta A: instalar la última versión

<!-- quick-start: install -->
```bash
GOBIN="$PWD/bin" go install github.com/teramoby/speedle-plus/cmd/...@v1.7.1
ls bin
```

`GOBIN` deja los ejecutables en la carpeta `bin` de este directorio; sin él, Go los escribe en `$(go env GOPATH)/bin`. `v1.7.1` es la etiqueta de la última versión en el momento de escribir esta guía; `go install` la compila a partir del código etiquetado y la verifica con la base de sumas de comprobación de Go.

### Ruta B: clonar y compilar

<!-- quick-start: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
ls bin
```

El directorio de salida contiene:

```text
spctl  speedle-ads  speedle-pms
```

- `spctl`: la interfaz de línea de comandos.
- `speedle-pms`: el Servicio de Gestión de Políticas (PMS).
- `speedle-ads`: el Servicio de Decisiones de Autorización (ADS).

> **macOS:** `/usr/sbin/spctl` es la herramienta Gatekeeper de Apple y puede seleccionarse al ejecutar un `spctl` sin ruta. Invoque la CLI de Speedle por su ruta completa, o use `./spctl` desde el directorio que la contiene; esta guía usa `./bin/spctl`. Ejecute `command -v spctl` para ver qué ejecutable elige el shell. Si la versión que descarga incluye un artefacto `speedle-spctl_<os>_<arch>`, es la misma CLI con un nombre que no choca con la herramienta de Apple.

Go actual usa `go build` o `go install` para compilar ejecutables; `go get` gestiona las dependencias del módulo.

## Ejecutar Speedle

Ejecute este recorrido en una máquina de desarrollo local. `--insecure=true` habilita HTTP sin cifrar para los ejemplos REST. Los servidores gRPC están apagados salvo que los inicie con `--grpc-endpoint`, por ejemplo `--grpc-endpoint 127.0.0.1:50001` para PMS y `--grpc-endpoint 127.0.0.1:50002` para ADS; gRPC usa la misma configuración TLS que REST. Para el transporte y la autorización en producción, consulte [Seguridad](../docs/security/).

### 1. Iniciar el Servicio de Gestión de Políticas

En la primera terminal, desde el directorio que contiene `bin/` (el clon de la ruta B):

<!-- quick-start: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

Déjelo en ejecución. PMS crea `quick-start-policies.json` en este directorio. Espere a que arranque el servidor REST antes de continuar. Ambos servicios deben usar este mismo archivo.

### 2. Crear políticas con spctl

Abra una segunda terminal en el mismo directorio.

Cree un contenedor de servicio:

<!-- quick-start: service -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service mysvc
```

Conceda a `user1` permiso para obtener o eliminar `res1`, y conceda a `role2` las mismas acciones sobre `res2`:

<!-- quick-start: policies -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get,del res1" --service-name=mysvc
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant role role2 get,del res2" --service-name=mysvc
```

Asigne `role2` a `user2` sobre `res2`:

<!-- quick-start: roles -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy -c "grant user user2 role2 on res2" --service-name=mysvc
```

Liste el servicio y sus políticas:

<!-- quick-start: list -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
```

La salida incluye `mysvc`, dos políticas de autorización y una política de roles.

### 3. Iniciar el Servicio de Decisiones de Autorización

Abra una tercera terminal en el mismo directorio:

<!-- quick-start: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

Déjelo en ejecución y espere a que arranque su servidor REST.

### 4. Verificar decisiones de autorización

Ejecute estas solicitudes en la segunda terminal. La primera comprueba la concesión directa a `user1`:

<!-- quick-start: allow-direct -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

La respuesta contiene `"allowed":true`.

La siguiente solicitud comprueba la concesión de rol a `user2`:

<!-- quick-start: allow-role -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user2"}]},"serviceName":"mysvc","resource":"res2","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

La respuesta contiene `"allowed":true`.

Un usuario sin una concesión coincidente es denegado:

<!-- quick-start: deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user3"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

La respuesta contiene `"allowed":false`.

El campo `reason` explica cada decisión. `0` (`GRANT_POLICY_FOUND`) significa que coincidió una política de concesión; `3` (`NO_APPLICABLE_POLICIES`) significa que no se aplicó ninguna política, así que Speedle deniega por defecto. La página [Decisiones de autorización](../docs/decisions/) enumera todos los códigos de motivo.

### 5. Añadir una política condicional (ABAC)

Cree una concesión que solo se aplica cuando la solicitud lleva un atributo `clearance` de al menos 3:

<!-- quick-start: abac-policy -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get res3 if clearance >= 3" --service-name=mysvc
```

El punto de aplicación de políticas (PEP) aporta los atributos de la solicitud, y una condición se evalúa solo con los atributos presentes en la solicitud. Los valores de `clearance` siguientes solo muestran el formato de la solicitud: un PEP real debe obtener los valores de atributos de fuentes de confianza, como las claims de un token autenticado o un almacén de confianza, y nunca de la solicitud que se está autorizando. Una solicitud que omite `clearance` no coincide con esta concesión.

Con `clearance` igual a 4, la solicitud se permite:

<!-- quick-start: abac-allow -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res3","action":"get","attributes":[{"name":"clearance","type":"numeric","value":4}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

La respuesta contiene `"allowed":true` y `"reason":0`.

Con `clearance` igual a 2, la condición no coincide, así que no se aplica ninguna política y la solicitud se deniega:

<!-- quick-start: abac-deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res3","action":"get","attributes":[{"name":"clearance","type":"numeric","value":2}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

La respuesta contiene `"allowed":false` y `"reason":3`.

Al terminar, pulse **Ctrl+C** en las terminales de PMS y ADS. El archivo de políticas permanece en este directorio; use un archivo nuevo para otra ejecución de los comandos de creación.

Para la configuración TLS, consulte [Seguridad de los mensajes](../docs/security/#protect-both-api-transports). Para otros tipos de despliegue, consulte [Despliegue](../docs/deployment/).
