+++
title = "Marco de registro"
description = "Registro de Speedle"
weight = 320
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "logging"]
categories = ["docs"]
bref = ""
lastmod = 2026-11-08T08:33:54+08:00
+++

## Descripción general

Se utilizan los siguientes dos proyectos de código abierto para procesar mensajes de registro:

```
1. https://github.com/sirupsen/logrus
2. https://github.com/natefinch/lumberjack
```

Logrus es un registrador estructurado para Go (golang), totalmente compatible a nivel de API con el registrador de la biblioteca estándar de Go. Tenga en cuenta que logrus no admite la rotación de registros, y esa es exactamente la razón por la que incorporamos el segundo proyecto, lumberjack, que proporciona un registrador con rotación continua. Logrus se comunica con lumberjack mediante la interfaz `io.Writer`; consulte el siguiente diagrama:

<img src="/img/speedle/logger.jpg" alt="Speedle Logging" width="370" height="230" loading="lazy">

## Configuración del registro

Existen dos formas de configurar el registro en Speedle: mediante argumentos de línea de comandos o mediante un archivo de configuración.

### Configurar el registro mediante argumentos de línea de comandos

Todos los argumentos de línea de comandos disponibles se enumeran a continuación. Las opciones de nivel, formateador y reportcaller configuran logrus; las opciones de archivo/rotación configuran lumberjack.

```
--log-level string
--log-formatter string
--log-reportcaller bool
--log-filename string
--log-maxsize int
--log-maxbackups int
--log-maxage int
--log-localtime bool
--log-compress bool
```

La información detallada de cada elemento se describe en la siguiente tabla. Tenga en cuenta que cualquiera de los siguientes elementos de configuración es opcional. Si no se configura ninguno, los mensajes de registro con nivel info o superior se escribirán en stderr en formato de texto de forma predeterminada.

| Elemento de configuración | Descripción | Valor predeterminado |
| --- | --- | --- |
| `--log-level` | Nivel de registro; los niveles disponibles son panic, fatal, error, warn, info y debug | info |
| `--log-formatter` | Formateador de registro; los valores disponibles son text y json | text |
| `--log-reportcaller` | Controla si se incluye la información del emisor (archivo, línea y función) en las entradas de registro | false |
| `--log-filename` | Nombre del archivo de registro | Sin valor por defecto. Si no se configura, los mensajes se escriben en stderr |
| `--log-maxsize` | Tamaño máximo en megabytes del archivo de registro antes de rotarse. Solo es válido cuando se configura `--log-filename` | 100 |
| `--log-maxbackups` | Número máximo de archivos de registro antiguos que se conservan. Solo es válido cuando se configura `--log-filename` | 0, lo que significa conservar todos los archivos antiguos (aunque MaxAge aún puede hacer que se eliminen) |
| `--log-maxage` | Número máximo de días para conservar archivos de registro antiguos. Solo es válido cuando se configura `--log-filename` | 0, lo que significa no eliminar archivos antiguos según su antigüedad |
| `--log-localtime` | Controla si se utiliza la hora local para formatear las marcas de tiempo en los archivos de respaldo. Solo es válido cuando se configura `--log-filename` | false, lo que significa utilizar la hora UTC |
| `--log-compress` | Controla si los archivos de registro rotados deben comprimirse. Solo es válido cuando se configura `--log-filename` | false, lo que significa no realizar compresión |

Los siguientes dos ejemplos muestran los formatos text y json respectivamente:

```
# text
time="2015-03-26T01:27:38-04:00" level=debug msg="Started observing beach" animal=walrus number=8
```

```
# json
{"animal":"walrus","level":"info","msg":"A group of walrus emerges from the
ocean","size":10,"time":"2014-03-10 19:57:38.562264131 -0400 EDT"}
```

### Configurar el registro mediante un archivo de configuración

A continuación se muestra un ejemplo. Del mismo modo, cualquier elemento de configuración es opcional.

```json
{
    "logConfig": {
        "level": "info",
        "formatter": "text",
        "setReportCaller": false,
        "rotationConfig": {
            "filename": "/mnt/logs/speedle.log",
            "maxSize": 20,
            "maxBackups": 5,
            "maxAge": 0,
            "LocalTime": false,
            "compress": false
        }
    }
}
```

## Prácticas recomendadas para configurar el registro

Si Speedle se ejecuta como contenedores Docker, resulta más natural utilizar el controlador de registro json-file de Docker en lugar del proyecto de código abierto natefinch/lumberjack para rotar archivos de registro; por consiguiente, se recomienda el controlador de registro json-file para este caso. Solo se necesita sirupsen/logrus; en otras palabras, los usuarios no necesitan configurar nada para natefinch/lumberjack en este caso.

Todos los elementos de configuración relacionados con Docker, incluido el registro, se configuran en `/etc/docker/daemon.json`. Consulte [docker json-file](https://docs.docker.com/config/containers/logging/json-file/) para obtener información más detallada sobre cómo configurar el controlador de registro json-file. El siguiente fragmento es un ejemplo:

```
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "20m",
    "max-file": "10"
  }
}
```

En la mayoría de los casos, los usuarios tampoco necesitan configurar sirupsen/logrus. El nivel de registro predeterminado es info y el formato de registro predeterminado es text. Por lo tanto, generalmente los usuarios solo necesitan configurar el registro si desean cambiar el nivel o el formato de registro; de lo contrario, no necesitan configurar nada, ya que la configuración predeterminada es adecuada.

## Cómo agregar nuevos mensajes de registro

Primero, importe el paquete de registro como se muestra a continuación:

```go
import (
  log "github.com/sirupsen/logrus"
)
```

En segundo lugar, emita cualquier mensaje de registro donde lo necesite. Por ejemplo:

```go
  log.Debug("This is a debug log entry")
  log.Info("This is a info log entry")
  log.Warn("This is a warning log entry")
  log.Warning("This is a warning log entry again")
  log.Error("This is a error log entry")
```

Consulte [speedle/logging/demo/demo.go](https://github.com/teramoby/speedle-plus/blob/master/pkg/logging/demo/demo.go) para obtener información más detallada.

## Integración con recopiladores de registros de terceros

Los registros de Speedle se pueden integrar fácilmente con recopiladores de registros de terceros, como fluentd o filebeat. Lo que los usuarios deben hacer es configurar fluentd o filebeat para monitorear los archivos de registro generados por Speedle. El siguiente fragmento de configuración es un ejemplo de cómo utilizar el complemento de entrada `in_tail` de fluentd para recopilar los registros de contenedores Docker, incluidos los registros de Speedle si este se ejecuta en contenedores:

```
    <source>
      @type tail
      path /var/lib/docker/containers/*/*.log
      pos_file /data/fluentd/fluentd.log.pos
      tag speedle.log
      <parse>
        @type json
        time_key time
        keep_time_key true
      </parse>
      refresh_interval 5
    </source>
```

Consulte los sitios web oficiales de [fluentd](https://www.fluentd.org/) y [filebeat](https://www.elastic.co/products/beats/filebeat) para obtener más información.
