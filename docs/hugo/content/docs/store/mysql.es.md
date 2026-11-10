+++
title = "Configuración de MySQL"
description = "Configure el almacenamiento de políticas en MySQL 8.4 para PMS y ADS paso a paso."
weight = 10
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "mysql"]
categories = ["docs"]
bref = "Cree una base de datos, configure PMS y ADS, luego verifique la autorización y la sincronización automática de políticas."
lastmod = 2026-10-31T22:36:36+08:00
+++

Esta guía utiliza **MySQL 8.4 LTS** y dos procesos nativos de Speedle: PMS escribe políticas y ADS lee la misma base de datos con una cuenta independiente de solo lectura. El ejemplo local utiliza Docker y HTTP en texto plano en la interfaz de bucle invertido (loopback). Para un despliegue remoto, utilice su servidor MySQL existente y consulte la [guía de Seguridad](/es/docs/security/).

## 1. Compilar Speedle con el backend MySQL

Instale Git, Bash, curl, Docker y una versión compatible de Go (Go 1.26.10 o Go 1.27.3). Docker debe estar en ejecución. Compile desde el código fuente actual para que ambos binarios contengan el almacenamiento `mysql`; una versión anterior instalada podría no incluirlo.

<!-- mysql-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

Ejecute los comandos restantes desde este directorio `speedle-plus`. Mantenga libres los puertos 3306, 6733 y 6734, o modifique las direcciones correspondientes en cada comando y archivo de configuración a continuación.

## 2. Iniciar MySQL 8.4

Las contraseñas fijas a continuación son **credenciales de demostración local**. Utilice sus propias credenciales y mecanismos de gestión de secretos al desplegar. El volumen con nombre de Docker conserva la base de datos cuando este contenedor se detiene o se vuelve a crear.

<!-- mysql-step: database -->
```bash
docker run --name speedle-mysql --detach \
  --publish 127.0.0.1:3306:3306 \
  --volume speedle-mysql-data:/var/lib/mysql \
  --env MYSQL_ROOT_PASSWORD=SpeedleDemoRoot84 \
  mysql:8.4
```

Espere a que finalice la inicialización:

<!-- mysql-step: ready -->
```bash
until docker exec speedle-mysql mysqladmin ping \
  --host=127.0.0.1 --user=root --password=SpeedleDemoRoot84 --silent
do
  sleep 1
done
```

Si ya dispone de MySQL 8.4, omita estos comandos de Docker y ejecute el código SQL del siguiente paso utilizando el cliente de su administrador de base de datos. La imagen de Docker inicializa credenciales únicamente cuando su directorio de datos está vacío; cambiar una variable de entorno no restablece la contraseña en un volumen existente.

## 3. Crear la base de datos y las cuentas de servicio

Speedle crea las tablas de forma predeterminada, pero **no crea la base de datos**. Utilice una cuenta de administrador para crear `speedle` y dos cuentas:

<!-- mysql-step: accounts -->
```bash
docker exec -i speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 <<'SQL'
CREATE DATABASE speedle CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
CREATE USER 'speedle_pms'@'%' IDENTIFIED BY 'SpeedleDemoPms84';
CREATE USER 'speedle_ads'@'%' IDENTIFIED BY 'SpeedleDemoAds84';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, REFERENCES ON speedle.* TO 'speedle_pms'@'%';
GRANT SELECT ON speedle.* TO 'speedle_ads'@'%';
SQL
```

PMS necesita `CREATE` y `REFERENCES` para la inicialización automática del esquema, además de `SELECT`, `INSERT`, `UPDATE` y `DELETE` para la gestión de políticas. ADS solo necesita `SELECT` y debe tener la inicialización automática del esquema desactivada. El host de cuenta `%` permite conexiones desde el host a través de Docker; restrinja los hosts de las cuentas a las máquinas reales donde se ejecutan PMS/ADS en despliegues remotos.

## 4. Escribir los archivos de configuración de PMS y ADS

Ambos DSN deben seleccionar el **mismo servidor y base de datos**. Un DSN sigue la sintaxis del controlador `user:password@tcp(host:port)/database`.

<!-- mysql-step: configuration -->
```bash
mkdir -p mysql-demo
cat > mysql-demo/pms.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle_pms:SpeedleDemoPms84@tcp(127.0.0.1:3306)/speedle",
      "MySQLInitSchema": true,
      "MySQLPollInterval": "1s",
      "MySQLQueryTimeout": "10s"
    }
  }
}
JSON
cat > mysql-demo/ads.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle_ads:SpeedleDemoAds84@tcp(127.0.0.1:3306)/speedle",
      "MySQLInitSchema": false,
      "MySQLPollInterval": "1s",
      "MySQLQueryTimeout": "10s"
    }
  },
  "enableWatch": true
}
JSON
chmod 600 mysql-demo/pms.json mysql-demo/ads.json
```

`MySQLInitSchema: false` hace que el inicio verifique el esquema existente sin ejecutar comandos DDL. `enableWatch: true` permite a ADS seguir los cambios confirmados. No confirme estos archivos de configuración que contienen credenciales en el control de versiones.

## 5. Iniciar PMS y luego ADS

En la terminal A, inicie PMS primero para que pueda crear las tablas:

<!-- mysql-step: pms -->
```bash
./bin/speedle-pms --config-file mysql-demo/pms.json --endpoint 127.0.0.1:6733 --insecure=true
```

Deje PMS en ejecución. En la terminal B, desde el mismo directorio, confirme la inicialización:

<!-- mysql-step: tables -->
```bash
docker exec speedle-mysql mysql --user=speedle_ads --password=SpeedleDemoAds84 speedle -e 'SHOW TABLES;'
```

Debería ver cinco tablas:

| Tabla | Contenido |
| --- | --- |
| `speedle_services` | Servicios y metadatos de servicios |
| `speedle_policies` | Políticas de autorización |
| `speedle_role_policies` | Políticas de roles |
| `speedle_functions` | Definiciones de funciones personalizadas |
| `speedle_store_meta` | Versión del esquema y revisión confirmada de políticas |

Luego inicie ADS en la terminal B y déjelo en ejecución:

<!-- mysql-step: ads -->
```bash
./bin/speedle-ads --config-file mysql-demo/ads.json --endpoint 127.0.0.1:6734 --insecure=true
```

Espere a que se inicie el agente de escucha REST de cada servicio antes de continuar. Los servidores gRPC permanecen desactivados a menos que también pase `--grpc-endpoint`.

## 6. Crear una política y comprobar la autorización

En la terminal C, cree un servicio con una política que conceda a Alice permiso para leer `book`:

<!-- mysql-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"mysql-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

Espere un intervalo de sondeo (1 segundo por defecto), más el tiempo de reconstrucción de consulta/caché, para que ADS cargue el cambio. Repita la solicitud si la sincronización aún está en curso:

<!-- mysql-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"mysql-demo","resource":"book","action":"read"}'
```

Respuesta esperada: `{"allowed":true,"reason":0}`. Bob no tiene ninguna concesión coincidente:

<!-- mysql-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"mysql-demo","resource":"book","action":"read"}'
```

Respuesta esperada: `{"allowed":false,"reason":3}`.

## 7. Verificar actualizaciones automáticas y persistencia

Agregue una denegación explícita para Alice mediante PMS, sin reiniciar ADS:

<!-- mysql-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/mysql-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

Tras la sincronización, repita la solicitud de Alice del paso 6. Ahora debería devolver `{"allowed":false,"reason":1}`: la denegación explícita tiene prioridad. Puede detener PMS y ADS con Ctrl+C, reiniciarlos con los comandos del paso 5 y repetir la misma solicitud. La política y la denegación almacenadas persisten en MySQL.

Los cambios de políticas deben realizarse a través de PMS o de la interfaz de almacenamiento para que los datos y la revisión avancen en la misma transacción. Las modificaciones directas en SQL no activan automáticamente una recarga de ADS. Durante una interrupción de la base de datos, ADS permanece en ejecución y reintenta la sincronización, pero devuelve `allowed:false` y un error una vez detectado el fallo de forma predeterminada. Se debe cargar con éxito una instantánea completa antes de reanudar la autorización. Consulte la política compartida de [fallos del almacenamiento y período de gracia](/es/docs/store/#fallos-del-almacenamiento-y-autorización-en-caché). Iniciar ADS sigue requiriendo una base de datos accesible.

## Crear previamente el esquema sin privilegios DDL en tiempo de ejecución

Como alternativa a la creación automática de tablas, ejecute el script [schema.sql](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/mysql/schema.sql) del repositorio con una cuenta de aprovisionamiento antes de iniciar cualquiera de los servicios:

<!-- mysql-step: schema -->
```bash
docker exec -i speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 speedle < pkg/store/mysql/schema.sql
```

Establezca `MySQLInitSchema` en `false` en **ambos** archivos de configuración. Si siguió la demostración anterior, revoque los privilegios adicionales de inicialización:

<!-- mysql-step: revoke-ddl -->
```bash
docker exec speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 -e "REVOKE CREATE, REFERENCES ON speedle.* FROM 'speedle_pms'@'%';"
```

PMS luego se ejecuta con permisos `SELECT`, `INSERT`, `UPDATE`, `DELETE`; ADS permanece en solo lectura. Reinicie ambos servicios tras cambiar su configuración. La inicialización del esquema no migra tablas existentes; las versiones de esquema no compatibles impiden el inicio.

## Referencia de configuración y resolución de problemas

| Propiedad | Indicador | Variable de entorno | Valor predeterminado |
| --- | --- | --- | --- |
| `MySQLDSN` | `--mysqlstore-dsn` | `SPDL_MYSQLSTORE_DSN` | Obligatorio, incluido el nombre de la base de datos |
| `MySQLInitSchema` | `--mysqlstore-init-schema` | `SPDL_MYSQLSTORE_INIT_SCHEMA` | `true` |
| `MySQLPollInterval` | `--mysqlstore-poll-interval` | `SPDL_MYSQLSTORE_POLL_INTERVAL` | `1s` |
| `MySQLQueryTimeout` | `--mysqlstore-query-timeout` | `SPDL_MYSQLSTORE_QUERY_TIMEOUT` | `10s` |

El indicador de selección es `--store-type mysql` (`SPDL_STORE_TYPE=mysql`). Los indicadores explícitos anulan las variables de entorno no vacías, luego los archivos de configuración y finalmente los valores predeterminados. Para MySQL remoto, utilice un nombre de host con un certificado de servidor de confianza y agregue `?tls=true` al DSN; la configuración personalizada de CA sigue la [documentación TLS del controlador](https://github.com/go-sql-driver/mysql/tree/v1.10.1#tls).

| Síntoma | Comprobación |
| --- | --- |
| Almacenamiento o indicador `mysql` desconocido | Recompile ambos binarios desde el código fuente que contiene `pkg/store/mysql`. |
| Conexión rechazada / acceso denegado | Disponibilidad de la base de datos, host/puerto del DSN, contraseña y restricciones de host de la cuenta MySQL. |
| Base de datos desconocida | Cree la base de datos primero; Speedle solo crea tablas. |
| Faltan tablas o DDL denegado | Inicie PMS primero, o cree previamente el esquema; el proceso ADS de solo lectura debe tener `MySQLInitSchema` en `false`. |
| El cambio de política no es visible | Misma base de datos en ambos DSN, watch habilitado en ADS, retraso de sondeo y ninguna variable `SPDL_*` anulando la configuración. |

MySQL no admite la recopilación de solicitudes de Discover ni la generación de políticas. Consulte [Almacenamiento conectable](/es/docs/store/) para obtener detalles sobre transacciones, instantáneas y recuperación.
