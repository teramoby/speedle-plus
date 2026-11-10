+++
title = "Diagnóstico de autorización"
description = "Diagnóstico de políticas de autorización"
date = 2019-01-18T15:46:59+08:00
weight = 3
draft = false
bref = "Esta función se utiliza para diagnosticar el proceso de evaluación de autorización. Cuando a un usuario no se le permite operar sobre un recurso, esta función resulta útil para averiguar el motivo, es decir, cuál es la política que deniega la operación sobre el recurso."
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
+++

## ¿Qué muestra el diagnóstico?

`Diagnose` explica una evaluación de autorización: decisión, motivo, roles efectivos, atributos y las políticas examinadas. Acepta el mismo contexto de solicitud que `IsAllowed`; REST utiliza `/authz-check/v1/diagnose`, y tanto Go como gRPC utilizan `Diagnose`.

El diagnóstico expone definiciones de políticas y el contexto de la solicitud. Restrínjalo a operadores de confianza utilizando los [controles de seguridad de la API](/es/docs/security/); no es un endpoint de depuración público.

## Antes de ejecutar los ejemplos

Compile `./bin/spctl`, `./bin/speedle-pms` y `./bin/speedle-ads` mediante el [inicio rápido](/es/quick-start/). Utilice Go 1.26.10 o Go 1.27.3 (o un parche más reciente de la misma rama de lanzamiento), Bash, curl y Python 3. Ejecute cada terminal desde la raíz del repositorio. Utilice un archivo de políticas nuevo para esta página; reinícielo únicamente tras detener ambos servicios.

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

## Crear las políticas de ejemplo

El [archivo de prueba](https://github.com/teramoby/speedle-plus/blob/master/website/tests/examples/diagnosis.json) concede `get` bajo `^/api/v1/example/.*$` pero deniega explícitamente `/api/v1/example/res1` para `user1`.

<!-- docs-check: diagnosis-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/diagnosis.json
```

## Comparar decisión y diagnóstico

<!-- docs-check: diagnosis-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"srv1","resource":"/api/v1/example/res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

Esperado: `allowed=false`, `reason=1` (denegación explícita).

<!-- docs-check: diagnose -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"srv1","resource":"/api/v1/example/res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/diagnose
```

La respuesta de diagnóstico utiliza `allowed` como un booleano, `reason` como una cadena de texto como `DENY_POLICY_FOUND` y `attributes` como un objeto. `request_month` es un mes numérico. `requestContext.subject.principals` es una matriz; las políticas de autorización contienen una matriz bidimensional `principals` que representa grupos de entidades principales OR-de-AND.

| Campo | Significado |
| --- | --- |
| `requestContext` | La solicitud utilizada en la evaluación; los atributos REST permanecen como una matriz tipada aquí |
| `attributes` | Atributos disponibles de la solicitud e integrados, organizados por nombre |
| `grantedRoles` | Roles efectivos, cuando están presentes |
| `rolePolicies`, `policies` | Detalles de políticas de autorización y de roles evaluadas, cuando están presentes |
| `condition` | Expresión y resultado; puede estar vacío en políticas incondicionales |

Los valores de estado de política son `takeEffect`, `conditionFailed` e `ignored`. `ignored` significa que el resultado final ya estaba determinado, por lo que esa política no necesitó ser evaluada. El orden puede variar; inspeccione IDs y estados en lugar de comparar una respuesta histórica textualmente. Consulte el [esquema de decisiones](/es/docs/api/decision_api/) para el contrato de respuesta completo.
