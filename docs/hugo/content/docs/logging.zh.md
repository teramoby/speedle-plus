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
+++

## 日志组件

Speedle 使用以下两个开源项目处理日志：

```
1. https://github.com/sirupsen/logrus
2. https://github.com/natefinch/lumberjack
```

Logrus 是 Go 结构化日志库，API 与 Go 标准库的 logger 兼容。Logrus 本身不负责日志滚动，Speedle 用 lumberjack 补足这部分能力。两者通过 `io.Writer` 接口连接：

<img src="/img/speedle/logger.jpg" alt="Speedle 日志组件" width="370" height="230" loading="lazy">

## 日志配置

可以通过命令行参数或配置文件设置 Speedle 日志。

### 通过命令行参数配置日志

可用参数如下。`level`、`formatter`、`reportcaller` 由 logrus 使用，文件和滚动设置由 lumberjack 使用。

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

所有参数都可省略。未配置文件日志时，日志默认输出到标准错误（stderr），默认级别为 info。

| 配置项 | 说明 | 默认值 |
| --- | --- | --- |
| --log-level | 日志级别：panic、fatal、error、warn、info、debug | info |
| --log-formatter | 日志格式：text、json | text |
| --log-reportcaller | 是否在日志中包含调用信息（文件名、行号和函数名） | false |
| --log-filename | 日志文件路径。未设置时输出到标准错误（stderr） | 空 |
| --log-maxsize | 单个日志文件的最大尺寸，仅在设置 `--log-filename` 后生效 | 100M |
| --log-maxbackups | 最多保留多少个历史日志文件，仅在设置 `--log-filename` 后生效 | 0，保留全部文件；达到 maxAge 上限的文件仍会删除 |
| --log-maxage | 历史日志文件最多保留多少天，仅在设置 `--log-filename` 后生效 | 0，不按天数删除；达到 maxBackups 上限的文件仍会删除 |
| --log-localtime | 滚动日志文件名是否使用本地时间，仅在设置 `--log-filename` 后生效 | false，使用 UTC |
| --log-compress | 是否压缩滚动后的历史日志，仅在设置 `--log-filename` 后生效 | false，不压缩 |

以下分别是 text 和 json 格式的日志示例：

```
# text
time="2015-03-26T01:27:38-04:00" level=debug msg="Started observing beach" animal=walrus number=8
```

```
# json
{"animal":"walrus","level":"info","msg":"A group of walrus emerges from the
ocean","size":10,"time":"2014-03-10 19:57:38.562264131 -0400 EDT"}
```

### 通过配置文件配置日志

配置示例如下。各项含义与同名命令行参数一致，全部可以省略。

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

## 容器日志建议

Speedle 在 Docker 容器中运行时，建议用 Docker 的 `json-file` 日志驱动管理文件大小和滚动，不再使用 lumberjack。此时 logrus 只负责日志级别和格式。

Docker 的日志配置位于 `/etc/docker/daemon.json`。具体用法见 [Docker json-file 文档](https://docs.docker.com/config/containers/logging/json-file/)，配置示例如下：

```
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "20m",
    "max-file": "10"
  }
}
```

多数场景使用默认的 info 级别和 text 格式即可。需要 JSON 等格式时，再调整 logrus 配置。

## 在代码中添加日志

先在 `*.go` 源文件中导入 logrus：

```go
import (
  log "github.com/sirupsen/logrus"
)
```

然后添加需要的日志，例如：

```go
  log.Debug("This is a debug log entry")
  log.Info("This is a info log entry")
  log.Warn("This is a warning log entry")
  log.Warning("This is a warning log entry again")
  log.Error("This is a error log entry")
```

完整示例见 [speedle/logging/demo/demo.go](https://github.com/teramoby/speedle-plus/blob/master/pkg/logging/demo/demo.go)。

## 接入第三方日志系统

Speedle 日志可以交给 fluentd、filebeat 等系统收集，只需让采集器监控 Speedle 生成的日志文件。下面的示例用 fluentd 的 `in_tail` 插件收集 Docker 容器日志；Speedle 在容器中运行时，其日志也会由这套配置采集。

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

更多配置见 [fluentd](https://www.fluentd.org/) 和 [filebeat](https://www.elastic.co/products/beats/filebeat) 文档。
