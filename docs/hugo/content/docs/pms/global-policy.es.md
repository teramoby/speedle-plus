+++
title = "Políticas globales"
description = "Políticas que surten efecto a nivel global"
date = 2019-01-18T21:19:44+08:00
weight = 2
draft = false
bref = "Cuando existen políticas comunes a todos los servicios, crearlas en cada servicio resulta tedioso y difícil de gestionar. En este escenario, puede utilizar políticas globales para simplificar las tareas de gestión de políticas."
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
lastmod = 2026-10-29T11:53:01+08:00
+++

## ¿Cuándo debe ser global un rol?

Los roles ordinarios están restringidos al alcance de un servicio. Cuando varios servicios comparten una asignación de rol, defina la asignación en el servicio con el nombre especial `global` y utilice ese rol en las políticas de autorización de cada servicio.

Solo se admiten **políticas de roles** globales. Una asignación de rol global no concede por sí misma todas las operaciones; cada servicio ordinario sigue definiendo sus propios permisos. No se admiten políticas de autorización globales.

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

## Crear un rol global y un permiso local

<!-- docs-check: global-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service global
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy -c "grant user Emma AdminRole" --service-name global
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service library
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant role AdminRole borrow books" --service-name library
```

`global` debe utilizar exactamente ese nombre de servicio. `library` se crea antes de su política. Ambos se administran a través de las APIs normales de servicio.

## Comprobar el resultado

Emma está autorizada a través de `AdminRole`; Bob es denegado.

<!-- docs-check: global-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Emma"}]},"serviceName":"library","resource":"books","action":"borrow"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: global-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"library","resource":"books","action":"borrow"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

Una política de rol global con alcance de recurso se aplica únicamente al alcance de dicho recurso. Mantenga bajo supervisión las asignaciones de roles entre servicios, especialmente cuando las identidades abarcan [diferentes dominios](/es/docs/idd/).
