+++
title = "Decisiones de autorización"
description = "Obtenga decisiones de autorización para las interacciones de sus servicios"
weight = 30
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["pdp", "policy", "core"]
categories = ["docs"]
bref = "Obtención de decisiones de autorización"
lastmod = 2026-11-08T19:23:52+08:00
+++

## ¿Qué es una decisión de autorización?

Speedle evalúa un sujeto, una acción y un recurso dentro de un servicio. La aplicación debe hacer cumplir el resultado. Trate un error de evaluación o un tiempo de espera por separado de una denegación ordinaria y falle de forma restrictiva (fail closed). Las APIs REST, gRPC y Go en modo integrado tienen representaciones de red distintas; utilice sus respectivos [contratos](/es/docs/api/).

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

## Crear un conjunto reproducible de políticas

El [archivo de prueba](https://github.com/teramoby/speedle-plus/blob/master/website/tests/examples/bookstore.json) asigna a Alan `role1` y `role2`; `role1` puede descargar `/books/HarryPotter`.

<!-- docs-check: bookstore-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/bookstore.json
```

## Obtener una decisión

Alan está autorizado; Bob es denegado porque no existe ninguna concesión aplicable.

<!-- docs-check: decision-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: decision-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

Las decisiones exitosas devuelven `{"allowed":true,"reason":0}` o el objeto de denegación correspondiente. Los errores HTTP devuelven una respuesta de error, no una concesión exitosa. Los códigos de motivo son:

| Código | Significado |
| --- | --- |
| 0 | GRANT_POLICY_FOUND |
| 1 | DENY_POLICY_FOUND |
| 2 | SERVICE_NOT_FOUND |
| 3 | NO_APPLICABLE_POLICIES |
| 4 | ERROR_IN_EVALUATION |
| 5 | DISCOVER_MODE |
| 6 | REASON_NOT_AVAILABLE |

`DISCOVER_MODE` no es una concesión de autorización para producción. `REASON_NOT_AVAILABLE` es el valor de motivo por defecto. No asuma que estos números corresponden a códigos de estado HTTP.

## Obtener roles concedidos

<!-- docs-check: decision-roles -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/all-granted-roles
```

Los roles esperados son `role1` y `role2` (el orden no es relevante). Incluya `resource` al consultar asignaciones de roles con alcance de recurso.

## Obtener permisos concedidos

<!-- docs-check: decision-permissions -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/all-granted-permissions
```

Permiso esperado: recurso `/books/HarryPotter`, acción `download`. Esta API describe los permisos efectivos concedidos; haga cumplir una operación específica mediante `is-allowed` en lugar de tratar una respuesta de lista de permisos como un reemplazo de la evaluación de solicitudes.

## Atributos, diagnóstico e identidad de token

Los atributos de solicitud REST son objetos tipados dentro de una matriz. Por ejemplo, un elemento de atributo es:

```json
{
  "name": "clearance",
  "type": "numeric",
  "value": 4
}
```

Los atributos en Go en modo integrado son de tipo `map[string]interface{}`; los atributos de respuesta de diagnóstico también son un objeto, no esa matriz de solicitud. Consulte [SPDL](/es/docs/spdl/) para conocer los tipos y condiciones, [diagnóstico](/es/docs/pms/diagnosis/) para detalles de evaluación y [asertor de tokens](/es/docs/assertor/) para la validación de tokens configurada.

El punto de aplicación de políticas (PEP) debe suministrar cada atributo personalizado al que hagan referencia sus políticas (los atributos integrados como `request_user` y `request_time` los proporciona ADS). El PEP es la aplicación o servicio de confianza del emisor, no el usuario final, y es responsable de los valores de atributos que envía; nunca reenvíe entradas de usuario final no confiables bajo el nombre de un atributo integrado. Una condición sobre un atributo que falte en la solicitud no coincide, por lo que una denegación condicional no se aplica cuando su atributo está ausente. Asegúrese de que el PEP proporcione todos los atributos que utilicen sus condiciones de denegación y pruebe esas solicitudes.
