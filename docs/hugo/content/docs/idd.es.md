+++
title = "Dominio de identidad"
description = "Speedle admite identidades de usuarios de múltiples dominios de identidad"
date = 2019-01-17T13:20:07+08:00
weight = 40
draft = false
bref = ""
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
tags = ["identity domain", "identity"]
lastmod = 2026-10-29T11:53:01+08:00
+++

## ¿Qué es un dominio de identidad?

Un dominio de identidad (`idd`) es un espacio de nombres explícito para una entidad principal. Dos usuarios llamados `user1` pueden pertenecer a dominios distintos. La cadena de texto es suministrada por código de integración de confianza o por un asertor de tokens configurado; no constituye una prueba de autenticación por sí misma. Speedle no infiere ni valida el modelo multiinquilino de un proveedor basándose únicamente en el nombre del dominio.

## Representar dominios en políticas y solicitudes

```text
grant user user1 from github read book
grant user user1 from IDCS.tenant01 read book
```

Utilice `from <identity-domain>` inmediatamente después del nombre de la entidad principal. En una política JSON de PMS, codifique la misma entidad principal como `idd=github:user:user1`. En una solicitud de decisión, represente esta entidad principal como:

```json
{
  "type": "user",
  "name": "user1",
  "idd": "github"
}
```

Una política con `from github` coincide únicamente con ese dominio. Omitir `from` coincide con ese nombre de entidad principal en todos los dominios. Revise esto explícitamente en aplicaciones multiinquilino; una cadena de dominio por sí sola no valida un token. Consulte [identidad e IAM](/es/docs/overview/) y el [asertor de tokens](/es/docs/assertor/).

## Antes de ejecutar los ejemplos

Compile `./bin/spctl`, `./bin/speedle-pms` y `./bin/speedle-ads` mediante el [inicio rápido](/es/quick-start/). Utilice Go 1.27.4 (o un parche más reciente de la misma rama de lanzamiento), Bash, curl y Python 3. Ejecute cada terminal desde la raíz del repositorio. Utilice un archivo de políticas nuevo para esta página; reinícielo únicamente tras detener ambos servicios.

Inicialice un archivo de políticas vacío:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

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

## Cargar las políticas de prueba

El [archivo de prueba](https://github.com/teramoby/speedle-plus/blob/master/website/tests/examples/idd.json) concede `read` al usuario `user1` de GitHub, `write` al usuario `user1` de Google, y `rent` al usuario `user1` de cualquier dominio. Los valores de dominio en minúsculas son espacios de nombres de demostración.

<!-- docs-check: idd-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/idd.json
```

## Comparar decisiones

En orden, espere Conceder, Denegar, Conceder, Denegar. Un usuario en `notgoogle` no puede utilizar la concesión `write` de Google.

<!-- docs-check: idd-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"github"}]},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: idd-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"gitlab"}]},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: idd-any -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"google"}]},"serviceName":"booksvc","resource":"book","action":"rent"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: idd-write-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"notgoogle"}]},"serviceName":"booksvc","resource":"book","action":"write"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

Las solicitudes JSON requieren `Content-Type: application/json`. Los ejemplos de gestión utilizan IDs de políticas en formato de cadena y `permissions` (en plural); consulte la [API de gestión de políticas](/es/docs/api/management_api/).
