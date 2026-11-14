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

La versión posterior a v1.8.0 usa Go `log/slog` para los registros de aplicación y auditoría y `gopkg.in/natefinch/lumberjack.v2` para la rotación. v1.8.0 y las versiones anteriores conservan la API anterior.

## Configuración del registro

Se conservan las opciones `--log-*` y `--auditlog-*`, las variables `SPDL_*` y las claves JSON. La precedencia es opción explícita > entorno > archivo. La aplicación usa info, text y stderr sin datos del llamador; la auditoría usa info y JSON. Configure la auditoría por separado con las opciones `--auditlog-*`.

```text
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

| Flag | Default | Meaning |
| --- | --- | --- |
| `--log-level` | `info` | Minimum severity |
| `--log-formatter` | `text` | `text` or `json` |
| `--log-reportcaller` | `false` | Include caller file, line and function |
| `--log-filename` | unset | Stderr unless a file is configured |
| `--log-maxsize` | `100` | Maximum file size in MB |
| `--log-maxbackups` | `0` | Backup count; 0 retains all, subject to max age |
| `--log-maxage` | `0` | Backup age in days; 0 disables age deletion |
| `--log-localtime` | `false` | Local time for backup filenames, otherwise UTC |
| `--log-compress` | `false` | Compress backups |

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
  },
  "auditLogConfig": {
    "level": "info",
    "formatter": "json"
  }
}
```


## Niveles y formatos

Los nombres no distinguen mayúsculas. Se aceptan `trace`, `debug`, `info`, `warn`, `warning`, `error`, `fatal` y `panic`. Trace se convierte en debug: el umbral debug incluye los antiguos mensajes trace. Los demás umbrales conservan su orden. Un valor no vacío desconocido devuelve un error que identifica el valor.

`text` genera pares key=value y `json` un objeto JSON por línea. Se conservan `time`, `level`, `msg` y, si se habilita el llamador, `file` con línea y `func`. Los niveles son minúsculos; warn se escribe `warning`. Los errores son cadenas y los demás valores conservan su estructura. Los campos del usuario que coinciden con metadatos reciben el prefijo `fields.`.

```text
time=2026-11-15T09:00:00.000+08:00 level=info msg="Started observing beach" animal=walrus number=8
```

```json
{"time":"2026-11-15T09:00:00+08:00","level":"info","msg":"Started observing beach","animal":"walrus","number":8}
```


## Migración de los recolectores

El formato de texto puede cambiar en comillas, orden, color y precisión temporal respecto a logrus. No se generan colores ANSI; las fechas son compatibles con RFC3339. Revise los analizadores o use JSON por claves. Se conservan `[AUDIT]` junto al nombre de la API, `tenantid`, `target_index`, los valores de petición/respuesta y los niveles de éxito/error.

Los archivos nuevos usan `0600` (solo propietario), sujeto a umask, en lugar de `0644`. La rotación conserva los permisos de archivos existentes. Ejecute el recolector como propietario o configure el acceso explícitamente antes de actualizar.

## Migración de la API Go

Sustituya `InitLogInstance(logrusLogger, config)` por `logger, err := logging.NewLogger(config)`. `AuditLog()` devuelve `*slog.Logger`: use sus métodos nativos y `SetAuditLog(logger)` en lugar de setters/hooks de logrus. Importe lumberjack v2 para `LogConfig.RotationConfig`; los punteros v1 ya no compilan. `InitLog` y `InitAuditLog` conservan los ajustes omitidos; nil no modifica nada. Una configuración inválida no sustituye el logger instalado.

Use `logging.Default()` o instale un handler propio con `logging.SetDefault(logger)`. No se modifica `slog.Default()` ni se redirige el `log` estándar del anfitrión. `logging.Fatal(message)` registra y sale con estado 1; `logging.Panic(message)` registra y provoca panic, incluso si se filtra el registro.

```go
import (
    "github.com/teramoby/speedle-plus/pkg/logging"
    "gopkg.in/natefinch/lumberjack.v2"
)

logger, err := logging.NewLogger(&logging.LogConfig{
    Level: "info", Formatter: "json",
    RotationConfig: &lumberjack.Logger{Filename: "engine.log", MaxSize: 20},
})
if err != nil {
    return err
}
logger.Info("Started observing beach", "animal", "walrus", "number", 8)
```


## Registros de contenedores

Omita el nombre de archivo para usar stderr y la rotación del runtime del contenedor; no necesita configurar archivos de lumberjack. Etcd integrado puede seguir dependiendo de logrus y zap de forma transitiva, pero el código del Engine usa slog.
