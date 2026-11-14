+++
title = "Configuración de MongoDB"
description = "Configure el almacenamiento de políticas en MongoDB para PMS y ADS paso a paso."
weight = 30
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "mongodb"]
categories = ["docs"]
bref = "Ejecute un conjunto de réplicas de MongoDB, conecte PMS y ADS, verifique las actualizaciones de políticas en tiempo real y guarde una copia de seguridad."
lastmod = 2026-10-31T22:36:36+08:00
+++

Esta guía paso a paso utiliza **MongoDB 7.0.16** como un conjunto de réplicas (replica set) de nodo único con un volumen persistente de Docker. PMS escribe políticas y ADS sigue la misma base de datos a través de un flujo de cambios (change stream). El ejemplo escucha en la interfaz de bucle invertido (loopback) sin autenticación para desarrollo local. La autenticación para producción, TLS y la configuración del conjunto de réplicas se detallan más adelante.

MongoDB debe ejecutarse como un **conjunto de réplicas o un clúster particionado (sharded cluster)**. ADS observa la base de datos con un flujo de cambios y el reemplazo del almacén de políticas completo se ejecuta en una transacción; un proceso `mongod` independiente no admite ninguna de las dos cosas. Un conjunto de réplicas de nodo único es suficiente para desarrollo.

## 1. Compilar Speedle

Prepare Bash, Git, curl, Docker y Go 1.27.4. Inicie Docker y luego compile los servicios nativos:

<!-- mongodb-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

Ejecute los comandos posteriores desde este repositorio descargado. Mantenga disponibles los puertos locales 27017, 6733 y 6734, o actualice los comandos y la configuración en consecuencia.

## 2. Iniciar MongoDB

El volumen con nombre conserva los datos de MongoDB. `--replSet` inicia `mongod` como miembro del conjunto de réplicas `speedle-rs`; solo el puerto de la base de datos se publica, en la interfaz loopback.

<!-- mongodb-step: database -->
```bash
docker run --name speedle-mongodb --detach \
  --publish 127.0.0.1:27017:27017 \
  --volume speedle-mongodb-data:/data/db \
  mongo:7.0.16 \
  --replSet speedle-rs --bind_ip_all
```

Espere a que el servidor acepte comandos:

<!-- mongodb-step: ready -->
```bash
until docker exec speedle-mongodb mongosh --quiet \
  --eval 'db.adminCommand({ping: 1}).ok' >/dev/null 2>&1
do
  sleep 1
done
```

Si ya dispone de un conjunto de réplicas de MongoDB, omita los comandos del contenedor y el paso 3, y utilice su cadena de conexión a continuación. Consulte la [imagen oficial de Docker](https://hub.docker.com/_/mongo) para más detalles sobre el contenedor.

## 3. Inicializar el conjunto de réplicas

Inicialice el conjunto de réplicas una sola vez, con este servidor como su único miembro, y espere hasta que se convierta en primario:

<!-- mongodb-step: replica-set -->
```bash
docker exec speedle-mongodb mongosh --quiet --eval \
  'rs.initiate({_id: "speedle-rs", members: [{_id: 0, host: "127.0.0.1:27017"}]})'
until docker exec speedle-mongodb mongosh --quiet \
  --eval 'db.hello().isWritablePrimary' | grep -qx true
do
  sleep 1
done
```

La dirección de miembro `127.0.0.1:27017` es a la que se indica que se conecten los clientes, por lo que debe ser accesible desde PMS y ADS. Funciona en este caso porque el puerto se publica en la misma dirección loopback. La configuración se almacena en el volumen; no ejecute `rs.initiate` de nuevo tras un reinicio.

## 4. Configurar el almacenamiento compartido

Cree un archivo de configuración para este par local de PMS/ADS:

<!-- mongodb-step: configuration -->
```bash
mkdir -p mongodb-demo
cat > mongodb-demo/config.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mongodb",
    "storeProps": {
      "MongoURI": "mongodb://127.0.0.1:27017/?replicaSet=speedle-rs",
      "MongoDatabase": "speedle_demo"
    }
  },
  "enableWatch": true
}
JSON
```

PMS y ADS deben utilizar el **mismo despliegue y base de datos**. Utilice una base de datos independiente para cada almacén de políticas independiente; el almacenamiento mantiene los servicios y funciones en sus colecciones `services` y `functions`. MongoDB crea la base de datos y las colecciones en la primera escritura, por lo que no se requiere inicialización de esquema. `enableWatch: true` habilita actualizaciones de ADS a través del flujo de cambios; no se requiere configurar ningún intervalo de sondeo.

## 5. Iniciar PMS

En la terminal A, inicie PMS y déjelo en ejecución:

<!-- mongodb-step: pms -->
```bash
./bin/speedle-pms --config-file mongodb-demo/config.json --endpoint 127.0.0.1:6733 --insecure=true
```

Espere a que se inicie su escucha REST. HTTP en texto plano se vincula a loopback para esta demostración; consulte [Seguridad](/es/docs/security/) para la configuración de TLS y la autorización de API de Speedle.

## 6. Crear una política

En la terminal B, desde el mismo directorio, conceda permiso a Alice para leer `book`:

<!-- mongodb-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"mongodb-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

PMS devuelve el servicio y el ID de política generado. La política ya se encuentra en MongoDB.

## 7. Iniciar ADS y verificar la autorización

En la terminal C, inicie ADS con el mismo archivo y déjelo en ejecución:

<!-- mongodb-step: ads -->
```bash
./bin/speedle-ads --config-file mongodb-demo/config.json --endpoint 127.0.0.1:6734 --insecure=true
```

Espere a que se complete el inicio y luego ejecute estas comprobaciones en la terminal B. Alice está autorizada:

<!-- mongodb-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"mongodb-demo","resource":"book","action":"read"}'
```

Esperado: `{"allowed":true,"reason":0}`. Bob no tiene ninguna concesión coincidente:

<!-- mongodb-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"mongodb-demo","resource":"book","action":"read"}'
```

Esperado: `{"allowed":false,"reason":3}`.

## 8. Verificar los documentos almacenados y las actualizaciones automáticas

Inspeccione el documento del servicio; el nombre del servicio es su `_id`:

<!-- mongodb-step: documents -->
```bash
docker exec speedle-mongodb mongosh speedle_demo --quiet --eval \
  'db.services.find({}, {"policies.name": 1, "policies.effect": 1}).toArray()'
```

La salida incluye `_id: 'mongodb-demo'` con la concesión `alice-read`. Trate los documentos como datos administrados por el backend y realice cambios de políticas a través de PMS.

Agregue una denegación mientras ADS continúa en ejecución:

<!-- mongodb-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/mongodb-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

Repita la solicitud de Alice del paso 7. Tras la actualización asíncrona del flujo de cambios, debería devolver `{"allowed":false,"reason":1}`. Repita la solicitud si la sincronización aún está en curso. ADS no necesita reiniciarse para esta actualización ordinaria de políticas.

## 9. Guardar una copia de seguridad y verificar la persistencia

Descargue la base de datos de políticas a un archivo y cópielo fuera del contenedor:

<!-- mongodb-step: backup -->
```bash
docker exec speedle-mongodb mongodump --quiet \
  --db speedle_demo --archive=/tmp/speedle-demo.archive
docker cp speedle-mongodb:/tmp/speedle-demo.archive ./mongodb-demo/speedle-demo.archive
```

El archivo solo contiene la base de datos `speedle_demo`. Guarde las copias de seguridad en un almacenamiento protegido independiente. Para restaurarla, detenga los emisores de políticas y ejecute `mongorestore --drop --archive=...` contra el nodo primario; consulte [Copia de seguridad y restauración con herramientas de MongoDB](https://www.mongodb.com/docs/manual/tutorial/backup-and-restore-tools/).

Detenga PMS y ADS con Ctrl+C antes de esta comprobación de reinicio:

<!-- mongodb-step: restart -->
```bash
docker restart speedle-mongodb
```

Ejecute ambos bucles de espera de los pasos 2 y 3, omitiendo `rs.initiate`, reinicie PMS y ADS con la misma configuración y luego repita la solicitud de Alice. Todavía debería devolver `{"allowed":false,"reason":1}`. El volumen con nombre conserva los datos y la configuración del conjunto de réplicas tras el reinicio del contenedor.

Tras una restauración, reinicie ADS para que lea el almacenamiento restaurado antes de emitir decisiones. Vuelva a comprobar la autorización tras la recuperación; una caché que ya estuviera en ejecución no es prueba de que contenga el conjunto de políticas restaurado.

## Producción: autenticación, TLS y conjuntos de réplicas

Habilite el control de acceso, proporcione usuarios independientes para PMS y ADS y conéctese a través de TLS. PMS necesita el rol `readWrite` en la base de datos de políticas. ADS solo lee y observa, por lo que el rol integrado `read` es suficiente, y su usuario no puede modificar políticas:

```javascript
use speedle
db.createUser({user: "speedle-pms", pwd: passwordPrompt(), roles: [{role: "readWrite", db: "speedle"}]})
db.createUser({user: "speedle-ads", pwd: passwordPrompt(), roles: [{role: "read", db: "speedle"}]})
```

Luego proporcione a cada servicio su propia cadena de conexión:

```json
{
  "storeConfig": {
    "storeType": "mongodb",
    "storeProps": {
      "MongoURI": "mongodb://speedle-ads:replace-me@mongo1.example.com:27017,mongo2.example.com:27017,mongo3.example.com:27017/?replicaSet=speedle-rs&authSource=speedle&tls=true&tlsCAFile=/etc/speedle/mongodb/ca.pem",
      "MongoDatabase": "speedle"
    }
  },
  "enableWatch": true
}
```

La ruta de la CA corresponde a la máquina donde se ejecuta PMS/ADS. Las credenciales en una URI deben codificarse con porcentaje y el archivo de configuración que las contenga debe ser legible únicamente por la cuenta de servicio; también puede proporcionar la URI mediante `SPDL_MONGOSTORE_URI`. Cada opción de la [cadena de conexión de MongoDB](https://www.mongodb.com/docs/manual/reference/connection-string/) se pasa al controlador Go sin modificaciones, incluidos los certificados de cliente (`tlsCertificateKeyFile`) y otros mecanismos de autenticación. TLS hacia MongoDB se configura por separado de TLS en los agentes de escucha REST/gRPC de Speedle.

El control de acceso en un conjunto de réplicas también requiere autenticación entre los miembros, con un archivo de clave o certificados X.509. Consulte las guías oficiales para [desplegar un conjunto de réplicas autogestionado con autenticación de archivo de clave](https://www.mongodb.com/docs/manual/tutorial/deploy-replica-set-with-keyfile-access-control/) y [configurar TLS](https://www.mongodb.com/docs/manual/tutorial/configure-ssl/).

Para alta disponibilidad, utilice al menos tres miembros que contengan datos y enumere varios de ellos en la cadena de conexión. La demostración de un solo nodo no puede tolerar la pérdida de su único miembro. Mientras se elige un nuevo primario, las escrituras a través de PMS pueden fallar; reinténtelas tras la elección.

## Referencia de configuración y resolución de problemas

| Propiedad | Indicador | Variable de entorno | Valor predeterminado |
| --- | --- | --- | --- |
| `MongoURI` | `--mongostore_uri` | `SPDL_MONGOSTORE_URI` | `mongodb://localhost:27017` |
| `MongoDatabase` | `--mongostore_database` | `SPDL_MONGOSTORE_DATABASE` | `speedleplus` |

Los indicadores explícitos anulan las variables de entorno no vacías, luego los archivos de configuración y finalmente los valores predeterminados. Seleccione el backend con `--store-type mongodb` o `SPDL_STORE_TYPE=mongodb`. Para uso integrado, importe `pkg/store/mongodb` y pase estas propiedades a `store.NewStore`.

Comportamientos importantes al operar el almacenamiento:

- El reemplazo del almacenamiento completo (`WritePolicyStore`) se ejecuta en una sola transacción. Conserva los IDs de políticas suministrados y genera los ausentes. En un servidor independiente falla sin modificar los datos existentes.
- Eliminar todos los servicios o funciones conserva las colecciones, por lo que el flujo de cambios permanece abierto y ADS elimina las entradas de la caché.
- Leer el almacenamiento completo lee los servicios y las funciones en una sola instantánea en un conjunto de réplicas o clúster particionado.
- Si el flujo de cambios finaliza, ADS lo vuelve a abrir con un retroceso (backoff) de hasta 30 segundos y luego recarga el almacenamiento completo, por lo que los cambios realizados en el intervalo no se pierden. Hasta entonces continúa sirviendo sus últimas políticas cargadas.
- La recopilación de solicitudes de Discover y la generación de políticas no son compatibles; están disponibles con los almacenes de archivos y etcd.

| Síntoma | Comprobación |
| --- | --- |
| Tiempo de espera en la selección de servidor al iniciar | Conjunto de réplicas inicializado y primario, host miembro en `rs.conf()` accesible desde PMS/ADS, host y puerto de URI, firewall. |
| ADS finaliza con `The $changeStream stage is only supported on replica sets` | El despliegue es un `mongod` independiente; utilice un conjunto de réplicas o un clúster particionado. |
| PMS escribe pero ADS no muestra el servicio | Mismo despliegue de URI y mismo nombre `MongoDatabase` en ambas configuraciones. |
| Los cambios no llegan a ADS | Watch habilitado; errores en el flujo de cambios y de reapertura en los registros de ADS, que se repiten hasta que ADS alcanza la base de datos de nuevo. |
| Fallo de autenticación | Usuario, codificación de contraseña, `authSource` y el rol en la base de datos de políticas. |
| Error de TLS | `tls=true`, ruta del archivo de CA y nombres de host del certificado. |

Vuelva a [Almacenamiento conectable](/es/docs/store/) para conocer las interfaces del backend y la precedencia de configuración.

Los fallos de almacenamiento siguen la misma política que cualquier almacenamiento integrado: ADS devuelve de forma predeterminada una denegación con error tras detectar la falta de disponibilidad, permanece en ejecución y reintenta, y se reanuda tras una recarga completa exitosa. Consulte [fallos del almacenamiento y autorización en caché](/es/docs/store/#fallos-del-almacenamiento-y-autorización-en-caché) para el período de gracia finito opcional y su demora en la revocación.
