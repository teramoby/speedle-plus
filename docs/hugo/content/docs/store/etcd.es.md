+++
title = "Configuración de etcd"
description = "Configure el almacenamiento de políticas en etcd para PMS y ADS paso a paso."
weight = 20
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "etcd"]
categories = ["docs"]
bref = "Ejecute un servidor etcd independiente, conecte PMS y ADS, verifique las actualizaciones de políticas en tiempo real y guarde una copia de seguridad."
lastmod = 2026-10-31T22:36:36+08:00
+++

Esta guía paso a paso utiliza un servidor **etcd v3.6.14 externo** con un volumen persistente de Docker. PMS escribe políticas y ADS observa el mismo prefijo de clave. Este ejemplo de nodo único escucha en la interfaz de bucle invertido (loopback) y utiliza HTTP en texto plano para desarrollo local. La configuración de clúster y TLS para producción se explica más adelante.

## 1. Compilar Speedle

Prepare Bash, Git, curl, Docker y Go 1.26.10 o Go 1.27.3. Inicie Docker y luego compile los servicios nativos:

<!-- etcd-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

Ejecute los comandos posteriores desde este repositorio descargado. Mantenga disponibles los puertos locales 2379, 6733 y 6734, o actualice los comandos y la configuración en consecuencia.

## 2. Iniciar un servidor etcd externo

El volumen con nombre conserva los datos de etcd. Solo el puerto de cliente se publica en el host; el puerto entre pares (peer) permanece dentro de este contenedor de nodo único.

<!-- etcd-step: database -->
```bash
docker run --name speedle-etcd --detach \
  --publish 127.0.0.1:2379:2379 \
  --volume speedle-etcd-data:/etcd-data \
  gcr.io/etcd-development/etcd:v3.6.14 \
  /usr/local/bin/etcd \
  --name speedle-local \
  --data-dir /etcd-data \
  --listen-client-urls http://0.0.0.0:2379 \
  --advertise-client-urls http://127.0.0.1:2379 \
  --listen-peer-urls http://0.0.0.0:2380 \
  --initial-advertise-peer-urls http://127.0.0.1:2380 \
  --initial-cluster speedle-local=http://127.0.0.1:2380 \
  --initial-cluster-token speedle-local-demo \
  --initial-cluster-state new
```

Espere a que la API v3 esté en estado correcto:

<!-- etcd-step: ready -->
```bash
until docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 endpoint health
do
  sleep 1
done
```

Si ya ejecuta etcd, omita los comandos de contenedor y utilice su endpoint de cliente a continuación. Consulte la [guía oficial de contenedores](https://etcd.io/docs/v3.6/op-guide/container/) para obtener detalles sobre la red de Docker.

## 3. Configurar el almacenamiento compartido

Cree un archivo de configuración para este par local de PMS/ADS:

<!-- etcd-step: configuration -->
```bash
mkdir -p etcd-demo
cat > etcd-demo/config.json <<'JSON'
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "http://127.0.0.1:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": false
    }
  },
  "enableWatch": true
}
JSON
```

PMS y ADS deben utilizar el **mismo endpoint y prefijo de clave**. Conserve la barra diagonal final en `/speedle_ps/`; utilice un prefijo independiente para cada almacén de políticas independiente. `IsEmbeddedEtcd: false` se conecta a su servidor externo. `enableWatch: true` habilita actualizaciones de ADS mediante eventos watch de etcd; no se requiere configurar ningún intervalo de sondeo.

## 4. Iniciar PMS

En la terminal A, inicie PMS y déjelo en ejecución:

<!-- etcd-step: pms -->
```bash
./bin/speedle-pms --config-file etcd-demo/config.json --endpoint 127.0.0.1:6733 --insecure=true
```

Espere a que se inicie su escucha REST. HTTP en texto plano se vincula a loopback para esta demostración; consulte [Seguridad](/es/docs/security/) para la configuración de TLS y la autorización de API de Speedle.

## 5. Crear una política

En la terminal B, desde el mismo directorio, conceda permiso a Alice para leer `book`:

<!-- etcd-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"etcd-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

PMS devuelve el servicio y el ID de política generado. La política ya se encuentra en etcd; no se necesita esquema SQL ni inicialización manual de claves.

## 6. Iniciar ADS y verificar la autorización

En la terminal C, inicie ADS con el mismo archivo y déjelo en ejecución:

<!-- etcd-step: ads -->
```bash
./bin/speedle-ads --config-file etcd-demo/config.json --endpoint 127.0.0.1:6734 --insecure=true
```

Espere a que se complete el inicio y luego ejecute estas comprobaciones en la terminal B. Alice está autorizada:

<!-- etcd-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"etcd-demo","resource":"book","action":"read"}'
```

Esperado: `{"allowed":true,"reason":0}`. Bob no tiene ninguna concesión coincidente:

<!-- etcd-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"etcd-demo","resource":"book","action":"read"}'
```

Esperado: `{"allowed":false,"reason":3}`.

## 7. Verificar las claves almacenadas y las actualizaciones automáticas

Inspeccione únicamente las claves de este almacén de políticas:

<!-- etcd-step: keys -->
```bash
docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 get /speedle_ps/ --prefix --keys-only
```

La salida incluye claves para `etcd-demo` bajo `/speedle_ps/services/`. Trate las claves como datos administrados por el backend y realice cambios de políticas a través de PMS.

Agregue una denegación mientras ADS continúa en ejecución:

<!-- etcd-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/etcd-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

Repita la solicitud de Alice del paso 6. Tras la actualización asíncrona de watch, debería devolver `{"allowed":false,"reason":1}`. Repita la solicitud si la sincronización aún está en curso. ADS no necesita reiniciarse para esta actualización ordinaria de políticas.

## 8. Guardar una instantánea y verificar la persistencia

Tome una instantánea en caliente, inspecciónela y cópiela fuera del contenedor:

<!-- etcd-step: snapshot -->
```bash
docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 snapshot save /etcd-data/speedle-snapshot.db
docker exec speedle-etcd /usr/local/bin/etcdutl \
  snapshot status /etcd-data/speedle-snapshot.db --write-out=table
docker cp speedle-etcd:/etcd-data/speedle-snapshot.db ./etcd-demo/speedle-snapshot.db
```

Una instantánea contiene el **espacio de claves de todo el clúster etcd**, incluidos otros prefijos. Guarde las copias de seguridad en un almacenamiento protegido independiente. Consulte [Recuperación ante desastres](https://etcd.io/docs/v3.6/op-guide/recovery/) para ver los procedimientos de restauración y membresía.

Detenga PMS y ADS con Ctrl+C antes de esta comprobación de reinicio:

<!-- etcd-step: restart -->
```bash
docker restart speedle-etcd
```

Ejecute la comprobación de estado del paso 2, reinicie PMS y ADS con la misma configuración y luego repita la solicitud de Alice. Todavía debería devolver `{"allowed":false,"reason":1}`. El volumen con nombre conserva los datos tras el reinicio del contenedor.

Para una restauración ante desastres, detenga los emisores de políticas, restaure la instantánea en nuevos directorios de datos según lo indicado por etcd y reinicie ADS para que lea el almacenamiento restaurado antes de emitir decisiones. Vuelva a comprobar la autorización tras la recuperación; una caché que ya estuviera en ejecución no es prueba de que contenga el conjunto de políticas restaurado.

## Producción: TLS, autenticación y clústeres

Despliegue etcd con un certificado de servidor de confianza, un SAN de certificado que coincida con el nombre de host del endpoint y validación de certificados de cliente. Luego configure PMS/ADS para utilizar HTTPS y sus credenciales de cliente:

```json
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "https://etcd.example.com:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": false,
      "EtcdTLSTrustedCAFile": "/etc/speedle/etcd/ca.crt",
      "EtcdTLSCertFile": "/etc/speedle/etcd/client.crt",
      "EtcdTLSKeyFile": "/etc/speedle/etcd/client.key",
      "EtcdTLSServerName": "etcd.example.com",
      "EtcdTLSInsecureSkipVerify": false
    }
  },
  "enableWatch": true
}
```

Estas rutas corresponden a la máquina donde se ejecuta PMS/ADS. Restrinja el acceso a la clave privada a la cuenta de servicio. TLS hacia etcd se configura por separado de TLS en los agentes de escucha REST/gRPC de Speedle.

El adaptador expone opciones de certificado y no expone propiedades de usuario/contraseña de etcd. Para RBAC de etcd, utilice los Common Names de los certificados de cliente como identidades de usuario con `--client-cert-auth=true` y la autenticación habilitada. Conceda a PMS acceso de lectura/escritura a su prefijo y a ADS los permisos que requieran sus funciones habilitadas. Consulte las guías oficiales de [TLS](https://etcd.io/docs/v3.6/op-guide/security/) y [RBAC](https://etcd.io/docs/v3.6/op-guide/authentication/rbac/) para la configuración del servidor y roles. Un prefijo de clave agrupa datos; el control de acceso requiere RBAC.

Para alta disponibilidad, siga el [ejemplo oficial de clúster de tres miembros](https://etcd.io/docs/v3.6/op-guide/container/#running-a-3-node-etcd-cluster) y proteja el tráfico de clientes y pares. La demostración de un solo nodo no puede tolerar la pérdida de su único miembro. La propiedad actual `EtcdEndpoint` acepta una única cadena de endpoint; proporcione una URL de cliente accesible para su despliegue.

## Referencia de configuración y resolución de problemas

| Propiedad | Indicador | Variable de entorno | Valor predeterminado |
| --- | --- | --- | --- |
| `EtcdEndpoint` | `--etcdstore-endpoint` | `SPDL_ETCDSTORE_ENDPOINT` | `localhost:2379` |
| `EtcdKeyPrefix` | `--etcdstore-keyprefix` | `SPDL_ETCDSTORE_KEYPREFIX` | `/speedle_ps/` |
| `IsEmbeddedEtcd` | `--etcdstore-isembedded` | `SPDL_ETCDSTORE_ISEMBEDDED` | `false` |
| `EmbeddedEtcdDataDir` | `--etcdstore-embeddedDataDir` | `SPDL_ETCDSTORE_EMBEDDEDDATADIR` | Vacío; solo modo integrado |
| `EtcdTLSTrustedCAFile` | `--etcdstore-tls-ca` | `SPDL_ETCDSTORE_TLS_CA` | Vacío |
| `EtcdTLSCertFile` | `--etcdstore-tls-cert` | `SPDL_ETCDSTORE_TLS_CERT` | Vacío |
| `EtcdTLSKeyFile` | `--etcdstore-tls-key` | `SPDL_ETCDSTORE_TLS_KEY` | Vacío |
| `EtcdTLSServerName` | `--etcdstore-tls-serverName` | `SPDL_ETCDSTORE_TLS_SERVERNAME` | Vacío |
| `EtcdTLSInsecureSkipVerify` | `--etcdstore-tls-insecureSkipVerify` | `SPDL_ETCDSTORE_TLS_INSECURESKIPVERIFY` | `false` |

Los indicadores explícitos anulan las variables de entorno no vacías, luego los archivos de configuración y finalmente los valores predeterminados. Seleccione el backend con `--store-type etcd` o `SPDL_STORE_TYPE=etcd`. Opciones TLS adicionales son `EtcdTLSAllowedCN` (`--etcdstore-tls-allowedCN`) y `EtcdTLSCRLFile` (`--etcdstore-tls-crlFile`).

El modo integrado (`IsEmbeddedEtcd: true`) está destinado al desarrollo/pruebas locales y utiliza el puerto etcd local fijo. Esta guía mantiene etcd independiente de PMS/ADS para que su directorio de datos y su ciclo de vida se gestionen explícitamente.

| Síntoma | Comprobación |
| --- | --- |
| Conexión rechazada o inicio bloqueado | Estado de salud del endpoint etcd, puerto de cliente, dirección de host/contenedor y firewall. |
| PMS escribe pero ADS no muestra el servicio | Mismo endpoint y prefijo de clave, incluida la barra diagonal final. |
| Los cambios no llegan a ADS | Compruebe si watch está habilitado, la conectividad y errores de watch en los registros de ADS; reinicie ADS para recargar tras una recuperación. |
| Error de nombre de host o CA en TLS | Endpoint HTTPS, SAN del certificado, ruta de CA de confianza y configuración del nombre del servidor. |
| Permiso denegado | Identidad del certificado de cliente, roles RBAC habilitados y permisos de prefijo. |

Vuelva a [Almacenamiento conectable](/es/docs/store/) para conocer las interfaces del backend y la precedencia de configuración.

Los fallos de almacenamiento siguen la misma política que cualquier almacenamiento integrado: ADS devuelve de forma predeterminada una denegación con error tras detectar la falta de disponibilidad, permanece en ejecución y reintenta, y se reanuda tras una recarga completa exitosa. Consulte [fallos del almacenamiento y autorización en caché](/es/docs/store/#fallos-del-almacenamiento-y-autorización-en-caché) para el período de gracia finito opcional y su demora en la revocación.
