+++
title = "Descubrimiento de políticas"
description = "¡Descubra políticas sobre la marcha!"
weight = 4
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["policy", "core", "discovery"]
categories = ["docs"]
bref = "Speedle puede utilizarse para decisiones de autorización en sistemas con un gran número de recursos protegidos. Antes de acceder a dichos recursos, se invoca la API de decisiones de autorización de Speedle (API is-allowed). Definir políticas de autorización para esos recursos protegidos resulta difícil si no se sabe qué acciones ocurren o cómo se expresa un recurso protegido en una solicitud de decisión de autorización. El modo de descubrimiento de políticas puede facilitar enormemente esta tarea."
lastmod = 2026-10-29T11:32:30+08:00
+++

## ¿Qué hace el descubrimiento?

El descubrimiento registra solicitudes y devuelve `allowed=true` con motivo 5 para una operación de registro exitosa. No demuestra que una política existente haya autorizado la operación. Utilícelo únicamente en un entorno de prueba aislado, revise las concesiones generadas y restablezca la aplicación normal de `is-allowed` posteriormente. Los errores de aserción o almacenamiento deben seguir gestionándose; nunca trate un error como una concesión de autorización.

## Iniciar un ejemplo local reproducible

Compile los binarios mediante el [Inicio rápido](/es/quick-start/), luego ejecute todas las terminales desde la raíz del repositorio. Utilice un archivo limpio y detenga los procesos de páginas anteriores antes de comenzar.

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

En la primera terminal:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

En la segunda terminal:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

Mantenga ambos en ejecución. En una tercera terminal, registre una solicitud:

<!-- docs-check: discover-record -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/discover
```

Inspeccione la solicitud capturada y luego genere una propuesta de definición de servicio:

<!-- docs-check: discover-list -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ discover request --service-name observed
```
<!-- docs-check: discover-export -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ discover policy --service-name observed > discovered-service.json
```

Revise `discovered-service.json`, incluidos los roles generados, dominios de identidad y recursos. El ejemplo limpio no tiene un servicio existente llamado `observed`, por lo que se puede importar directamente; para un servicio existente, concilie sus políticas en lugar de recrearlo a ciegas.

<!-- docs-check: discover-import -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file discovered-service.json
```

## Restablecer y verificar la aplicación normal

Invoque `is-allowed` nuevamente. Alice debería estar autorizada después de que ADS haya cargado la actualización de políticas; Bob aún debe ser denegado:

<!-- docs-check: discover-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: discover-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

Detenga ambos servicios con Ctrl+C. Para gRPC utilice `Discover`/`IsAllowed` en el cliente generado; Go en modo integrado utiliza `evaluator.Discover(ctx)`/`evaluator.IsAllowed(ctx)`. Para conocer las opciones de la CLI, ejecute `./bin/spctl discover --help`. No deje una aplicación real apuntando a `/discover`.
