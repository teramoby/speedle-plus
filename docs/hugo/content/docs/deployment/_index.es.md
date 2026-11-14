+++
title = "Despliegue"
description = "Ejecute Speedle integrado en una aplicación Go, desde la imagen de contenedor publicada, con Homebrew, o como servicios independientes de PMS y ADS."
weight = 304
draft = false
toc = true
linksection = true
tocheading = "h2"
categories = ["docs"]
bref = "Modo integrado, imagen de Docker, Homebrew, compilación desde el código fuente y servicios independientes de PMS y ADS."
lastmod = 2026-11-14T10:30:00+08:00
+++

Speedle se ejecuta dentro de una aplicación Go, desde una imagen de contenedor publicada o como servicios independientes de PMS y ADS.

- [Modo integrado](/es/docs/deployment/embedded/) ejecuta el evaluador en su proceso Go.
- [Imagen de Docker](/es/docs/deployment/docker/) ejecuta `speedle-pms`, `speedle-ads` y `spctl` desde `docker.io/speedle/speedle` o `ghcr.io/teramoby/speedle`.
- [Homebrew](/es/docs/deployment/homebrew/) instala la herramienta de línea de comandos y ambos servicios, y ejecuta una demostración local.

## Elija un modo de despliegue

Utilice el [modo integrado](/es/docs/deployment/embedded/) para ejecutar el evaluador dentro de su proceso Go. Utilice la [imagen de Docker](/es/docs/deployment/docker/) o los servicios independientes de PMS y ADS cuando las aplicaciones llamen a Speedle a través de REST o gRPC. [Homebrew](/es/docs/deployment/homebrew/) instala esos programas y ejecuta una demostración local. El [Inicio rápido](../../quick-start/) proporciona una guía completa paso a paso de gestión de políticas y autorización local.

## Compilar con la versión actual de Go

Utilice Go 1.26.10 o Go 1.27.3 (o un parche más reciente en la misma rama de lanzamiento) (se recomienda Go 1.27.3), Git y un shell en Linux o macOS. Utilice módulos de Go; el repositorio clonado puede ubicarse en cualquier directorio con permisos de escritura. No se requiere una configuración personalizada de `GOROOT`, `GOPATH`, `dep` ni una estructura de fuentes especial de GOPATH.

<!-- deployment-check: source-build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

Los ejecutables generados son `bin/spctl`, `bin/speedle-pms` y `bin/speedle-ads`. Desde la raíz del repositorio, utilice un único archivo compartido para el ejemplo local:

<!-- deployment-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

Mantenga PMS en ejecución; inicie ADS en otra terminal dentro del mismo directorio:

<!-- deployment-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

Utilice un archivo de políticas nuevo; PMS lo crea si no existe. Siga la guía de [Inicio rápido](../../quick-start/) para la creación y las solicitudes de permitir o denegar, y después detenga ambos servicios con Ctrl+C. gRPC está deshabilitado salvo que se habilite de forma explícita con `--grpc-endpoint`. HTTP sin cifrar es para este ejemplo de loopback; consulte [Seguridad](../security/) para TLS y la autenticación de clientes.

## Binarios de la versión e imágenes de contenedor

Cada versión publicada por el flujo de publicación adjunta binarios precompilados de `spctl`, `speedle-pms` y `speedle-ads` para macOS y Linux en amd64 y arm64, con el nombre `<command>_<os>_<arch>`, además de un archivo `checksums.txt`. Descárguelos desde la [página de versiones](https://github.com/teramoby/speedle-plus/releases) y verifique cada archivo con su suma SHA-256 antes de ejecutarlo. El archivo de sumas se adjunta a la misma versión, así que detecta descargas incompletas o dañadas y no autentica la versión. Los binarios y las imágenes de contenedor los produce el flujo de publicación, por lo que están disponibles a partir de la primera versión creada después de añadir ese flujo.

El flujo de publicación también publica una imagen de contenedor multiarquitectura para Linux amd64 y arm64. Contiene `speedle-pms`, `speedle-ads` y `spctl`; el primer argumento selecciona el programa:

- Docker Hub: `docker.io/speedle/speedle`
- GHCR: `ghcr.io/teramoby/speedle`

Las descargas anónimas funcionan. Si GHCR rechaza una descarga anónima, autentíquese antes con `docker login ghcr.io`.

Descargue la versión que despliega; `latest` sigue a la versión más reciente:

<!-- deployment-template: image-pull -->
```bash
docker pull docker.io/speedle/speedle:latest
docker pull ghcr.io/teramoby/speedle:latest
```

Para fijar una imagen por digest, lea el digest publicado con `docker buildx imagetools inspect docker.io/speedle/speedle:<tag>`.

La imagen se ejecuta como UID 65532 e incluye un directorio `/data` escribible para el almacén de archivos. Pase el nombre del programa y sus opciones después del nombre de la imagen y ejecute cada comando en su propia terminal:

<!-- deployment-template: image-run -->
```bash
docker run --rm -p 127.0.0.1:6733:6733 -v speedle-policies:/data docker.io/speedle/speedle:latest speedle-pms --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6733 --insecure=true
docker run --rm -p 127.0.0.1:6734:6734 -v speedle-policies:/data docker.io/speedle/speedle:latest speedle-ads --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6734 --insecure=true
```

PMS no autentica al llamador; el enlace a `127.0.0.1` mantiene la API de gestión sin autenticar en esta máquina. No publique estos puertos en una interfaz del host ni los exponga a una red no confiable.

La imagen incluye el paquete de certificados de CA públicos. Las versiones 1.7.0 y 1.7.1 se publicaron antes de añadir ese paquete; proporcione un archivo de CA para los endpoints TLS cuando use esas etiquetas. `--insecure=true` es solo para este ejemplo local; consulte [Seguridad](../security/). La página [Imagen de Docker](/docs/deployment/docker/) cubre `spctl`, las comprobaciones de estado, Compose y la procedencia.

## Ejemplo de desarrollo en Kubernetes

Utilice un clúster de Kubernetes actual y un `kubectl` compatible. Guarde lo siguiente como `speedle-dev.yaml`. Usa `apps/v1`, un pod en el que PMS y ADS comparten un archivo, un UID de aplicación no root y un servicio ClusterIP interno. Es un ejemplo de desarrollo aislado: las políticas de `emptyDir` se pierden cuando se sustituye el pod, y los endpoints HTTP necesitan controles de acceso del clúster. El manifiesto usa el marcador `<release-tag>`; sustitúyalo por la etiqueta de la versión que despliega, o por un digest para un anclaje inmutable, y añada un secreto de extracción de imagen si su clúster lo requiere.

<!-- deployment-template: kubernetes -->
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: speedle-dev
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: speedle-dev
  template:
    metadata:
      labels:
        app: speedle-dev
    spec:
      initContainers:
        - name: policy-file
          image: busybox:1.37.0
          command: ["sh", "-c", "printf '{}\n' > /data/policies.json; chown 65532:65532 /data/policies.json; chmod 600 /data/policies.json"]
          volumeMounts:
            - name: policies
              mountPath: /data
      containers:
        - name: pms
          image: docker.io/speedle/speedle:<release-tag>
          args: ["speedle-pms", "--store-type", "file", "--filestore-loc", "/data/policies.json", "--endpoint", "0.0.0.0:6733", "--insecure=true"]
          ports:
            - containerPort: 6733
          readinessProbe:
            tcpSocket:
              port: 6733
          volumeMounts:
            - name: policies
              mountPath: /data
        - name: ads
          image: docker.io/speedle/speedle:<release-tag>
          args: ["speedle-ads", "--store-type", "file", "--filestore-loc", "/data/policies.json", "--endpoint", "0.0.0.0:6734", "--insecure=true"]
          ports:
            - containerPort: 6734
          readinessProbe:
            tcpSocket:
              port: 6734
          volumeMounts:
            - name: policies
              mountPath: /data
      volumes:
        - name: policies
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: speedle-dev
spec:
  type: ClusterIP
  selector:
    app: speedle-dev
  ports:
    - name: pms
      port: 6733
      targetPort: 6733
    - name: ads
      port: 6734
      targetPort: 6734
```

Establezca `SPEEDLE_IMAGE` con la imagen exacta que vaya a desplegar, como una etiqueta de versión o un digest sha256. Sustitúyala localmente antes de aplicar; mantenga port-forward en ejecución mientras utiliza los clientes de Inicio rápido:

<!-- deployment-template: apply -->
```bash
kubectl set image --local -f speedle-dev.yaml pms="$SPEEDLE_IMAGE" ads="$SPEEDLE_IMAGE" -o yaml | kubectl apply -f -
kubectl rollout status deployment/speedle-dev
kubectl port-forward service/speedle-dev 6733:6733 6734:6734
```

## Despliegues de producción

Utilice almacenamiento persistente de políticas, fije los digests de las imágenes de contenedor a la versión exacta que despliegue (un reintento de publicación puede volver a publicar una etiqueta de versión), proteja los endpoints de gestión y decisión, y configure TLS/mTLS para los clientes y el backend de políticas. [Almacenamiento conectable](../store/) documenta la configuración actual de almacenamiento; [Seguridad](../security/) cubre las opciones de cifrado de transporte. Dimensione y opere el despliegue de etcd utilizando la documentación actual de etcd, no el ejemplo retirado de etcd-operator.

El repositorio contiene ejemplos históricos de Helm y Kubernetes. Las instrucciones antiguas de Helm 2 `helm install -n ...`, `stable/etcd-operator`, `apps/v1beta2` y las imágenes `r.authz.fun` no corresponden al flujo de despliegue actual. La descarga de contenedores, la admisión en clúster, el acceso a registros, el almacenamiento y el despliegue dependen de su entorno; valídelos en su clúster de destino antes de usarlos en producción.
