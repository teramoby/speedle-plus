+++
title = "Almacenamiento conectable"
description = "¡Traiga su propio almacenamiento de persistencia!"
weight = 130
draft = false
toc = true
layout = "single"
linksection = true
tocheading = "h2"
tags = ["storage", "pluggable"]
categories = ["docs"]
bref = "Speedle incluye almacenes de políticas basados en archivos, etcd, MongoDB y MySQL. Este documento describe su configuración y cómo implementar o registrar un almacenamiento diferente."
+++

## Almacenes integrados y contratos de extensión

Los binarios independientes registran implementaciones de archivo, etcd, MongoDB y MySQL bajo `pkg/store`. Su disponibilidad no implica garantías operativas idénticas; revise la autenticación del backend, persistencia, transacciones y el comportamiento de observación (watch) para su despliegue. La configuración a continuación ilustra los nombres de propiedades; la ejecución de despliegues de etcd/MongoDB queda fuera de este ejemplo local.

Un backend completo implementa [pms.PolicyStoreManager](https://github.com/teramoby/speedle-plus/blob/master/api/pms/interface.go): gestión de servicios, políticas de autorización, políticas de roles y funciones, lecturas de instantáneas y watch/stop. ADS acepta la interfaz más reducida de lectura/observación `PolicyStoreManagerADS`. Compruebe las interfaces reales al implementar un backend, en lugar de copiar una lista histórica parcial de métodos.

Para guías de despliegue paso a paso, consulte [Configuración de MySQL](/es/docs/store/mysql/), [Configuración de etcd](/es/docs/store/etcd/) y [Configuración de MongoDB](/es/docs/store/mongodb/).

## Fallos del almacenamiento y autorización en caché

Archivo, etcd, MySQL y MongoDB utilizan la misma política de fallos de ADS. El valor predeterminado de `storeFailureGracePeriod` es `"0s"`: una vez detectado un fallo en el almacenamiento, la autorización devuelve `allowed:false` y un error de evaluación. ADS continúa ejecutándose y reintenta la conexión. Una conexión legible por sí sola no restaura la autorización; se requiere una recarga completa y exitosa de la instantánea. Con el modo watch habilitado, primero se debe restablecer la suscripción fallida; las lecturas exitosas de datos mientras el watch está interrumpido no restablecen el plazo de fallo/gracia. Un archivo eliminado se reintenta en lugar de finalizar el proceso ADS.

Los despliegues que aceptan explícitamente una revocación demorada pueden establecer una duración finita, por ejemplo `"storeFailureGracePeriod": "5s"` en el nivel superior de la configuración JSON de ADS. El indicador equivalente es `--store-failure-grace-period=5s`, y la variable de entorno es `SPDL_STORE_FAILURE_GRACE_PERIOD`. Se aplica la precedencia habitual de indicador → variable de entorno → archivo → valor predeterminado. Los fallos repetidos no amplían el plazo límite. Dentro del período de gracia, las decisiones utilizan la última instantánea completa, por lo que las revocaciones no surten efecto hasta que se recupera la sincronización. Los fallos de caché parcial y los pánicos de evaluación nunca reciben período de gracia.

Los almacenes integrados cuentan con sondas de estado ligeras, comprobadas aproximadamente una vez por segundo con un tiempo de espera de sonda de un segundo. Las suscripciones watch pueden notificar fallos con mayor rapidez; las lecturas en curso también tienen sus tiempos de espera del backend. La detección de fallos y la propagación ordinaria de políticas son asíncronas, no garantías de revocación inmediata. Con `enableWatch:false`, las solicitudes siguen comprobando el estado del almacenamiento; las actualizaciones ordinarias de políticas requieren una actualización explícita. Tras una interrupción, la recuperación carga una instantánea completa incluso en ese modo. Un fallo en la carga inicial no puede utilizar un período de gracia. Los almacenes personalizados pueden implementar `pms.StoreHealthChecker` y publicar eventos `pms.STORE_UNAVAILABLE` para participar en la misma política de fallos.

Las operaciones de escritura en PMS con almacenamiento de archivos publican un archivo temporal completamente serializado y sincronizado con un renombrado atómico en el mismo directorio. Por lo tanto, los lectores independientes de ADS observan una instantánea completa, ya sea antigua o nueva. El proceso de escritura requiere permisos de escritura en el directorio principal; se conservan los bits de permisos existentes y los enlaces simbólicos configurados. El archivo de reemplazo es creado por la cuenta de escritura, por lo que se debe configurar la herencia de grupo/ACL del directorio para usuarios independientes de PMS/ADS. Los contenedores deben compartir el directorio de políticas para que los reemplazos sean visibles en ambos servicios. Los nuevos archivos de políticas creados directamente utilizan permisos 0600; el generador normal de almacenamiento de archivos continúa inicializando los almacenes ausentes como de costumbre.

## Almacén de políticas MySQL 8.4

Siga la [guía de configuración paso a paso de MySQL](/es/docs/store/mysql/) para crear una base de datos, configurar cuentas independientes para PMS/ADS y verificar la sincronización de políticas.

El backend `mysql` implementa la interfaz completa de almacenamiento de políticas para PMS y ADS. La recopilación de solicitudes de descubrimiento y la generación de políticas no son compatibles. Utiliza el controlador Go puro `go-sql-driver/mysql`; la base de datos ya debe existir previamente.

```json
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle:replace-me@tcp(db.example.com:3306)/speedle?tls=true",
      "MySQLInitSchema": true,
      "MySQLPollInterval": "1s",
      "MySQLQueryTimeout": "10s"
    }
  },
  "enableWatch": true
}
```

| Propiedad | Indicador | Valor predeterminado |
| --- | --- | --- |
| `MySQLDSN` | `--mysqlstore-dsn` | Obligatorio, incluido el nombre de la base de datos |
| `MySQLInitSchema` | `--mysqlstore-init-schema` | `true` |
| `MySQLPollInterval` | `--mysqlstore-poll-interval` | `1s` |
| `MySQLQueryTimeout` | `--mysqlstore-query-timeout` | `10s` |

Las variables de entorno utilizan el mismo mapeo, por ejemplo `SPDL_MYSQLSTORE_DSN`. Las duraciones deben ser cadenas positivas; `MySQLInitSchema` acepta un booleano JSON o una cadena booleana. Los indicadores explícitos anulan las variables de entorno no vacías, luego los valores del archivo de configuración y finalmente los valores predeterminados. Para uso integrado, importe `pkg/store/mysql`, pase estas propiedades directamente a `store.NewStore` y cierre el `*mysql.Store` devuelto al finalizar. Un evaluador creado mediante `eval.NewFromConfig` se puede cerrar mediante `io.Closer`; cerrar un evaluador también cierra un almacenamiento suministrado que sea cerrable.

### Esquema y aprovisionamiento

Una base de datos corresponde a un almacén de políticas, utilizando cinco tablas InnoDB fijas: `speedle_services`, `speedle_policies`, `speedle_role_policies`, `speedle_functions` y `speedle_store_meta`. Las matrices complejas y los metadatos utilizan columnas JSON. Las tablas de políticas hacen referencia al servicio con eliminación en cascada. Los nombres de servicios/funciones y los IDs de políticas utilizan comparaciones que distinguen mayúsculas y minúsculas sin relleno (non-padding) y admiten hasta 255 caracteres Unicode; los nombres de políticas no necesitan ser únicos.

De forma predeterminada, el inicio crea las tablas ausentes utilizando [schema.sql](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/mysql/schema.sql). No crea una base de datos ni altera un esquema existente. Para un despliegue preconstruido, ejecute ese mismo SQL en la base de datos seleccionada utilizando una cuenta de aprovisionamiento y luego establezca `MySQLInitSchema` en `false`. El inicio verifica la versión del esquema, las columnas requeridas y los motores InnoDB. Una versión de esquema no compatible impedirá el inicio; las actualizaciones futuras requieren una migración explícita.

El aprovisionamiento requiere privilegios de creación e inicialización de tablas. Con un esquema preconstruido, la cuenta de PMS necesita `SELECT`, `INSERT`, `UPDATE` y `DELETE` en la base de datos de políticas; ADS puede utilizar una cuenta independiente que solo tenga `SELECT` y con la creación automática de esquema deshabilitada. Utilice las opciones TLS del DSN del controlador para bases de datos remotas. No coloque contraseñas de producción en configuraciones confirmadas o en el historial de la línea de comandos.

### Transacciones, sincronización y recuperación

Cada escritura bloquea la fila de revisión única y confirma los cambios de datos junto con un incremento de revisión. Esto serializa las escrituras de políticas entre instancias de PMS. La creación de servicios con políticas anidadas, la eliminación en cascada y el reemplazo completo del almacenamiento son atómicos. La importación conserva los IDs de políticas suministrados y asigna IDs donde falten. Las lecturas de instantáneas utilizan una única transacción de lectura repetible (repeatable-read).

Con `enableWatch`, ADS sondea la revisión y recibe una instantánea consistente completa cuando esta cambia. El inicio también envía una instantánea para cubrir las actualizaciones entre la lectura inicial y el registro de la observación. La propagación es asíncrona, normalmente dentro de un intervalo de sondeo más el tiempo de consulta/reconstrucción de la caché. Durante una interrupción de la base de datos, ADS retiene sus datos pero suspende la autorización en caché de forma predeterminada, reintenta las lecturas y recarga una instantánea completa antes de reanudar el servicio. Se aplica la política compartida de fallo/gracia descrita anteriormente. Los cambios realizados directamente en SQL sin incrementar la revisión no se descubren automáticamente; utilice PMS o la interfaz de almacenamiento para escrituras de políticas.

El tiempo de espera de operación también limita las esperas del grupo de conexiones y los bloqueos de transacciones. Las escrituras fallidas se revierten; una pérdida de conexión durante el commit puede dejar el resultado incierto, por lo que los clientes deben inspeccionar el estado almacenado antes de reintentar una creación no idempotente. `StopWatch` y `Close` son idempotentes y liberan los recursos de observación; `Close` también cierra el grupo de conexiones.

## Implementar y registrar un constructor

Un [store.StoreBuilder](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/store.go) proporciona `NewStore(map[string]interface{}) (pms.PolicyStoreManager, error)` y `GetStoreParams() map[string]string`. Este último asigna nombres de indicadores a nombres de propiedades de configuración. Registre un tipo de almacenamiento único en `init()` o antes de llamar a `store.NewStore`; el registro duplicado produce un error.

Este ejemplo completo delega en el backend de archivos existente para demostrar el contrato de fábrica. No pretende implementar una nueva base de datos:

<!-- docs-check: store -->
```bash
go run ./website/tests/examples/store
```


<!-- docs-source: website/tests/examples/store/main.go -->
```go
package main

import (
	"fmt"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/store/file"
	"log"
	"os"
	"path/filepath"
)

// This example delegates to the file backend to demonstrate registration.
// A new backend must implement the complete PolicyStoreManager contract.
type Builder struct{ file.FileStoreBuilder }

func main() {
	dir, err := os.MkdirTemp("", "speedle-store-example-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store.Register("myfile", Builder{})
	policies, err := store.NewStore("myfile", map[string]interface{}{"FileLocation": filepath.Join(dir, "policies.json")})
	if err != nil {
		log.Fatal(err)
	}
	snapshot, err := policies.ReadPolicyStore()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Registered myfile store; services=%d\n", len(snapshot.Services))
}
```


## Combinar la configuración correctamente

Para comandos independientes, los indicadores explícitos anulan las variables de entorno no vacías, luego las propiedades del archivo de configuración y finalmente los valores predeterminados. Convierta el nombre del indicador agregando `SPDL_`, reemplazando `-` por `_` y poniendo en mayúsculas: `etcdstore-endpoint` se convierte en `SPDL_ETCDSTORE_ENDPOINT`. Los valores pueden llegar como cadenas desde indicadores/entorno o como valores JSON tipados; valide y convierta de forma deliberada.

```json
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "localhost:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": true,
      "EmbeddedEtcdDataDir": "./speedle.etcd"
    }
  },
  "enableWatch": true
}
```

Defina indicadores personalizados con `pflag` antes del análisis y devuelva su mapeo de indicador a propiedad desde `GetStoreParams`. Los invocadores integrados pasan el mapa de configuración directamente a `store.NewStore`/`eval.NewFromConfig`; la fábrica por sí sola no lee la configuración de la línea de comandos o del entorno.

## Vincular el backend y gestionar actualizaciones

Utilice importaciones por efectos secundarios en ambos binarios independientes (o en su aplicación integrada) cuando el registro se realice en `init()`:

```go
import (
    _ "github.com/teramoby/speedle-plus/pkg/store/etcd"
    _ "github.com/teramoby/speedle-plus/pkg/store/file"
    _ "github.com/teramoby/speedle-plus/pkg/store/mongodb"
    _ "github.com/teramoby/speedle-plus/pkg/store/mysql"
)
```

Implemente `Watch()` para que devuelva los eventos de cambio/instantánea correctos mediante `pms.StorageChangeChannel`; admita `StopWatch()` sin fugas de recursos de observación. ADS actualiza su caché en tiempo de ejecución de manera asíncrona. Pruebe suscripciones watch perdidas/desconectadas, recuperación, fallos durante una escritura y lectores concurrentes; no prometa una propagación instantánea simplemente porque el backend admita watch.
