+++
title = "日志"
description = "配置 Speedle 的日志级别、格式、文件滚动和第三方日志收集。"
weight = 320
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "logging"]
categories = ["docs"]
bref = ""
lastmod = 2026-11-08T11:21:16+08:00
+++

## 日志组件

v1.8.0 之后的下一个版本使用 Go `log/slog` 输出应用日志和审计日志，使用 `gopkg.in/natefinch/lumberjack.v2` 滚动日志文件。v1.8.0 及更早版本仍使用旧的日志 API。

## 日志配置

现有 `--log-*`、`--auditlog-*` 参数、`SPDL_*` 环境变量及 JSON 配置键保持兼容。显式命令行参数优先于环境变量，环境变量优先于配置文件。应用日志默认为 info、text、stderr，不包含调用位置；审计日志默认为 info、JSON。使用对应的 `--auditlog-*` 参数单独配置审计日志。

```text
--log-level string
--log-formatter string
--log-reportcaller string
--log-filename string
--log-maxsize string
--log-maxbackups string
--log-maxage string
--log-localtime string
--log-compress string
```

这些参数都是字符串类型，请使用 `--flag=value` 形式传入值（例如 `--log-compress=true`）。只写参数名时，会把下一个参数当作它的值，例如 `--log-compress --help` 会把 `--help` 当作 `--log-compress` 的值。

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
      "maxsize": 20,
      "maxbackups": 5,
      "maxage": 0,
      "localtime": false,
      "compress": false
    }
  },
  "auditLogConfig": {
    "level": "info",
    "formatter": "json"
  }
}
```


## 级别与输出格式

级别和格式名称不区分大小写。支持 `trace`、`debug`、`info`、`warn`、`warning`、`error`、`fatal`、`panic`。trace 映射到 debug，因此选择 debug 也会输出原来的 trace 消息；其他级别保持原来的过滤顺序。未知的非空值会返回包含该值的错误。

`text` 输出 key=value 记录，`json` 每行输出一个 JSON 对象。元数据键仍为 `time`、`level`、`msg`；开启调用位置后还包括带行号的 `file` 和函数名 `func`。级别值为小写，warn 输出为 `warning`。错误以字符串输出，其他结构化字段保持结构。与元数据重名的用户字段添加 `fields.` 前缀。

```text
time=2026-11-15T09:00:00.000+08:00 level=info msg="Started observing beach" animal=walrus number=8
```

```json
{"time":"2026-11-15T09:00:00+08:00","level":"info","msg":"Started observing beach","animal":"walrus","number":8}
```


## 日志收集器迁移

text 的引号、字段顺序、颜色和时间精度可能与 logrus 不同；不再输出 ANSI 颜色，时间仍兼容 RFC3339。升级前检查现有解析规则，或改用 JSON 按键解析。审计消息保留紧接 API 名称的 `[AUDIT]` 前缀，以及 `tenantid`、`target_index`、请求和响应字段、成功和失败的日志级别。

新建日志文件使用仅属主可读写的 `0600` 默认权限（受 umask 影响），不再是原来的 `0644`；已有文件滚动时保留原权限。升级前让收集器以日志属主身份运行，或明确配置访问权限，不要假定其他账号可以读取新建文件。

## Go API 迁移

将 `InitLogInstance(logrusLogger, config)` 改为 `logger, err := logging.NewLogger(config)`。`AuditLog()` 现在返回 `*slog.Logger`；使用原生 slog 方法和 `SetAuditLog(logger)` 替代 logrus 的 setter/hook。给 `LogConfig.RotationConfig` 赋值时改用 lumberjack v2 导入，v1 指针不再编译通过。`InitLog` 和 `InitAuditLog` 保留未指定的设置，nil 或空配置不作修改，非法配置不会替换现有 logger。

通过 `logging.Default()` 获取 Engine logger，或通过 `logging.SetDefault(logger)` 安装宿主提供的 slog handler。这些函数不会修改 `slog.Default()`，也不会重定向宿主的标准 `log` 输出。`logging.Fatal(message)` 记录日志后以状态 1 退出；`logging.Panic(message)` 记录日志后 panic，即使日志被过滤也会执行终止动作。

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


## 容器日志

容器中可不设置文件名，由容器运行时滚动日志，无需配置 lumberjack 文件输出。嵌入式 etcd 仍可能间接依赖 logrus 和 zap，但 Engine 代码使用 slog。
