+++
title = "Imagen de Docker"
description = "Ejecute speedle-pms, speedle-ads y spctl desde la imagen publicada de Speedle."
weight = 20
aliases = ["/es/docs/sdks/docker/"]
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Una única imagen en Docker Hub y GHCR ejecuta el administrador de políticas, el servicio de decisiones y spctl, con un almacén de archivos compartido."
lastmod = 2026-11-10T14:45:43+08:00
+++

Speedle publica una imagen que contiene `speedle-pms`, `speedle-ads` y `spctl`. La imagen se compila `FROM scratch`, se ejecuta como UID 65532 y no contiene ningún shell. El primer argumento selecciona el programa a ejecutar.

El mismo índice de imágenes está disponible en ambos registros:

- Docker Hub: `docker.io/speedle/speedle`
- GHCR: `ghcr.io/teramoby/speedle`

La versión 1.7.0 publica las etiquetas `1.7.0`, `1.7` y `latest` en ambos registros, para las arquitecturas `linux/amd64` y `linux/arm64`.

## Descargar la imagen

```bash
docker pull docker.io/speedle/speedle:1.7.0
docker pull ghcr.io/teramoby/speedle:1.7.0
```

`docker pull` imprime el digest del índice. Para 1.7.0 ese digest es `sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0` en ambos registros.

## Elegir una etiqueta

`1.7.0` identifica esta versión. `1.7` se actualiza cuando se publica un parche 1.7 superior. `latest` se actualiza cuando se publica una versión superior desde el commit más reciente en `master`. Un reintento de publicación puede publicar la misma etiqueta de versión nuevamente, por lo que una etiqueta no es una compilación inmutable.

Para leer el digest del índice:

```bash
docker buildx imagetools inspect docker.io/speedle/speedle:1.7.0 --format '{{.Manifest.Digest}}'
```

Para la versión 1.7.0, `1.7.0`, `1.7` y `latest` en ambos registros imprimen:

```text
sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0
```

Verifique el digest nuevamente antes de desplegar `1.7` o `latest`. Para fijar una compilación específica, utilice el nombre de la imagen con `@` y ese digest, por ejemplo `docker.io/speedle/speedle@sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0`.

## Comandos en la imagen

Sin argumentos, la imagen imprime los tres programas disponibles:

```bash
docker run --rm docker.io/speedle/speedle:1.7.0
```

```text
speedle-pms
speedle-ads
spctl
```

`docker run --rm ghcr.io/teramoby/speedle:1.7.0` imprime las mismas líneas. Los comandos siguientes utilizan el nombre de Docker Hub. Sustitúyalo por el nombre de GHCR si desea utilizar esa copia. No hay shell en la imagen, por lo que `docker exec` debe llamar a `/spctl`, `/speedle-pms` o `/speedle-ads` mediante su ruta absoluta.

## Ejecutar PMS y ADS

Cree un volumen con nombre e inicie el administrador de políticas. El servidor escucha en `0.0.0.0:6733` dentro del contenedor. Publicar `127.0.0.1:6733` mantiene la API de gestión no autenticada únicamente en esta máquina.

```bash
docker volume create speedle-policies
docker run -d --name speedle-pms \
  -p 127.0.0.1:6733:6733 \
  -v speedle-policies:/data \
  docker.io/speedle/speedle:1.7.0 \
  speedle-pms --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6733 --insecure=true
```

El nuevo volumen con nombre se rellena a partir del directorio `/data` de la imagen, en el que el UID 65532 puede escribir. PMS crea `/data/policies.json` si no existe. La imagen no incluye certificados de CA. `--insecure=true` es para este ejemplo HTTP local. Consulte [Seguridad](/es/docs/security/) para configurar TLS.

Docker puede aceptar el puerto publicado y reiniciar la conexión antes de que el proceso comience a escuchar. Reintente en caso de error, incluido ese reinicio:

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6733/policy-mgmt/v1/service
```

El primer intento puede imprimir `curl: (56) Recv failure: Connection reset by peer`. Un intento posterior devolverá el cuerpo. Un almacén vacío responde:

```text
[]
```

Inicie ADS en el mismo volumen. Lee el archivo que escribe PMS, incluidas las actualizaciones posteriores.

```bash
docker run -d --name speedle-ads \
  -p 127.0.0.1:6734:6734 \
  -v speedle-policies:/data \
  docker.io/speedle/speedle:1.7.0 \
  speedle-ads --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6734 --insecure=true
```

## Comprobar estado y preparación

PMS no ofrece `/healthz`. Una respuesta exitosa de `GET /policy-mgmt/v1/service`, como se mostró anteriormente, indica que la API de gestión está aceptando solicitudes.

ADS ofrece comprobaciones de actividad (liveness) y preparación (readiness):

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/healthz
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/readyz
```

```text
{"status":"ok"}
{"ready":true,"servicesTotal":0,"servicesUsable":0}
```

`/healthz` responde `200 {"status":"ok"}` mientras el proceso se está ejecutando. `/readyz` responde `200` cuando ADS puede evaluar una solicitud. Un almacén de archivos no tiene estado de distribución, por lo que el recuento de servicios se mantiene en cero; el campo que debe comprobarse es `ready`. La imagen no tiene shell ni `curl`, por lo que estas comprobaciones se ejecutan desde el host o desde otro contenedor.

## Añadir y comprobar una política

Ejecute `spctl` en este contenedor para añadir un servicio y una política. El PMS con `--insecure=true` utiliza una dirección local `http://`:

```bash
docker exec speedle-pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
docker exec speedle-pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

```text
service created
{"name":"docs","type":"application","metadata":{"createtime":"2026-11-10T03:30:03Z"}}
policy created
{"id":"zhtbsgm54iiuwky4zgmq","name":"","effect":"grant","permissions":[{"resource":"/docs/public","actions":["get","list"]}],"principals":[["user:alice"]],"metadata":{"createtime":"2026-11-10T03:30:04Z"}}
```

PMS asigna `createtime` y el `id` de la política, por lo que esos valores cambian en cada ejecución. Es el mismo servicio y la misma política que en la [descripción general de los SDK](/es/docs/sdks/#example-policy).

El mismo binario `spctl` puede ejecutarse en un segundo contenedor que comparte el espacio de nombres de red de PMS:

```bash
docker run --rm --network container:speedle-pms \
  docker.io/speedle/speedle:1.7.0 \
  spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
```

## Pedir una decisión a ADS

Consulte a ADS a través de su puerto publicado. Solicite la acción concedida `get` para el usuario `alice`:

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"docs","resource":"/docs/public","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

```text
{"allowed":true,"reason":0}
```

`reason` `0` es `GRANT_POLICY_FOUND`. La misma política no concede `delete`:

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"docs","resource":"/docs/public","action":"delete"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

```text
{"allowed":false,"reason":3}
```

`reason` `3` es `NO_APPLICABLE_POLICIES`. Una política escrita a través de PMS mientras ADS ya está en ejecución es visible en la siguiente solicitud. Consulte [Decisiones de autorización](/es/docs/decisions/) para conocer los campos de solicitud.

## Conectar un SDK a ADS

Las páginas de documentación de los SDK llaman a `http://localhost:6734`. Con el puerto publicado anteriormente, esa dirección es este ADS. El cliente de Node.js siguiente utiliza `http://127.0.0.1:6734`, que es la dirección que publica el contenedor. Cree primero el directorio de ejemplo y luego guarde el cliente en ese directorio como `main.mjs`:

```bash
mkdir speedle-example
cd speedle-example
```

```javascript
import { Auth, SpeedleClient } from "@speedle/sdk";

const speedle = new SpeedleClient({
  backend: "speedle",
  endpoint: "http://127.0.0.1:6734",
  auth: Auth.none(),
  serviceName: "docs",
});
try {
  const allowed = await speedle.isAllowed("alice", "get", "/docs/public");
  console.log(`alice get /docs/public: allowed=${allowed}`);
} finally {
  await speedle.close();
}
```

Instale el paquete publicado en `speedle-example` y ejecute `main.mjs` allí. La instalación es la descrita en la página del [SDK de Node.js](/es/docs/sdks/node/), fijada a la misma versión:

```bash
npm init -y
npm pkg set type=module
npm install @speedle/sdk@0.2.1
node main.mjs
```

```text
alice get /docs/public: allowed=true
```

Las otras páginas de lenguajes toman el mismo endpoint y nombre de servicio. Los SDK solo aceptan `http://` en texto plano para hosts de bucle invertido (loopback).

## Ejecutar ambos servicios con Compose

Guarde este archivo como `compose.yaml`. Es otra forma de ejecutar los mismos dos procesos y el mismo volumen. Si los contenedores de la sección anterior siguen ejecutándose, deténgalos primero para que los puertos 6733 y 6734 queden libres. Al eliminar ese volumen también se elimina su archivo de políticas.

```yaml
services:
  pms:
    image: docker.io/speedle/speedle:1.7.0
    command:
      - speedle-pms
      - --store-type
      - file
      - --filestore-loc
      - /data/policies.json
      - --endpoint
      - 0.0.0.0:6733
      - --insecure=true
    ports:
      - 127.0.0.1:6733:6733
    volumes:
      - speedle-policies:/data
  ads:
    image: docker.io/speedle/speedle:1.7.0
    command:
      - speedle-ads
      - --store-type
      - file
      - --filestore-loc
      - /data/policies.json
      - --endpoint
      - 0.0.0.0:6734
      - --insecure=true
    ports:
      - 127.0.0.1:6734:6734
    volumes:
      - speedle-policies:/data
volumes:
  speedle-policies:
```

```bash
docker compose -f compose.yaml up -d
```

`up -d` regresa antes de que PMS esté escuchando. Espere a que la lista de servicios de PMS y los endpoints `/healthz` y `/readyz` de ADS respondan antes de crear nada. Como se describió anteriormente, el primer intento de PMS puede reiniciar la conexión.

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6733/policy-mgmt/v1/service
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/healthz
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/readyz
docker compose -f compose.yaml exec -T pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
docker compose -f compose.yaml exec -T pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

Envíe la misma solicitud `is-allowed`. ADS reconoce la nueva política sin necesidad de reiniciar.

```bash
docker compose -f compose.yaml down -v
```

`down -v` elimina el volumen de Compose y las políticas almacenadas en él.

## Verificar procedencia y SBOM

La compilación de publicación adjunta la procedencia (provenance) y un SBOM (`docker buildx build --provenance=mode=max --sbom=true`). Inspeccione el índice:

```bash
docker buildx imagetools inspect docker.io/speedle/speedle:1.7.0
```

El índice enumera `linux/amd64`, `linux/arm64` y dos manifiestos `unknown/unknown`. Esas plataformas desconocidas son los manifiestos de atestación. Cada uno establece `vnd.docker.reference.type` en `attestation-manifest` y `vnd.docker.reference.digest` en la imagen de plataforma que describe. `docker buildx imagetools inspect ghcr.io/teramoby/speedle:1.7.0` imprime los mismos digests de plataforma y atestación.

## Detener los contenedores

```bash
docker rm -f speedle-pms speedle-ads
docker volume rm speedle-policies
```

`docker volume rm` elimina el archivo de políticas. Utilice `docker compose -f compose.yaml down -v` si los procesos se iniciaron con Compose.
