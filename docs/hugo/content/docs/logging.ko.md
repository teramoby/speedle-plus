+++
title = "로깅 프레임워크"
description = "Speedle 로깅"
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

## 개요

v1.8.0 다음 릴리스는 애플리케이션 및 감사 로그에 Go `log/slog`를, 파일 순환에 `gopkg.in/natefinch/lumberjack.v2`를 사용합니다. v1.8.0과 이전 버전은 기존 API를 사용합니다.

## 로그 설정

기존 `--log-*`, `--auditlog-*` 옵션, `SPDL_*` 환경 변수와 JSON 키는 유지됩니다. 우선순위는 명시적 옵션 > 환경 변수 > 파일입니다. 애플리케이션 기본값은 info, text, stderr이며 호출자 정보는 꺼져 있습니다. 감사 기본값은 info와 JSON이며 `--auditlog-*`로 별도 설정합니다.

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

이 옵션은 문자열 플래그이므로 `--flag=value` 형식으로 값을 지정합니다(예: `--log-compress=true`). 옵션 이름만 지정하면 다음 인수가 그 값으로 사용되므로 `--log-compress --help`는 `--help`를 `--log-compress`의 값으로 처리합니다.

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


## 레벨과 출력 형식

이름은 대소문자를 구분하지 않습니다. `trace`, `debug`, `info`, `warn`, `warning`, `error`, `fatal`, `panic`을 지원합니다. trace는 debug로 통합되므로 debug 설정은 기존 trace 메시지도 출력합니다. 나머지 임계값 순서는 유지됩니다. 알 수 없는 비어 있지 않은 값은 해당 값을 포함하는 오류를 반환합니다.

`text`는 key=value를, `json`은 줄마다 JSON 객체 하나를 출력합니다. `time`, `level`, `msg`와 호출자 정보가 켜진 경우 줄 번호를 포함한 `file`, 함수 이름 `func`를 유지합니다. 레벨은 소문자이며 warn은 `warning`으로 출력합니다. 오류는 문자열로, 다른 구조화 값은 구조를 유지하여 출력합니다. 메타데이터와 이름이 겹치는 사용자 필드에는 `fields.` 접두사를 붙입니다.

```text
time=2026-11-15T09:00:00.000+08:00 level=info msg="Started observing beach" animal=walrus number=8
```

```json
{"time":"2026-11-15T09:00:00+08:00","level":"info","msg":"Started observing beach","animal":"walrus","number":8}
```


## 로그 수집기 마이그레이션

text의 따옴표, 순서, 색상과 시간 정밀도는 logrus와 다를 수 있습니다. ANSI 색상을 출력하지 않으며 시간은 RFC3339와 호환됩니다. 기존 파서를 확인하거나 키로 처리하는 JSON을 사용하세요. API 이름 바로 앞의 `[AUDIT]`, `tenantid`, `target_index`, 요청/응답 값과 성공/오류 레벨은 유지됩니다.

새 파일의 기본 권한은 `0644`에서 소유자 전용 `0600`으로 바뀝니다(umask 적용). 기존 파일의 권한은 순환 시에도 유지됩니다. 수집기를 로그 소유자로 실행하거나 업데이트 전에 접근 권한을 명시적으로 설정하세요.

## Go API 마이그레이션

`InitLogInstance(logrusLogger, config)`를 `logger, err := logging.NewLogger(config)`로 바꾸세요. `AuditLog()`는 `*slog.Logger`를 반환합니다. logrus setter/hook 대신 네이티브 slog 메서드와 `SetAuditLog(logger)`를 사용하세요. `LogConfig.RotationConfig`에 lumberjack v2를 가져와야 하며 v1 포인터는 컴파일되지 않습니다. `InitLog`와 `InitAuditLog`는 생략된 설정을 유지하고 nil 또는 빈 설정은 변경하지 않습니다. 잘못된 설정은 기존 logger를 교체하지 않습니다.

`logging.Default()`를 사용하거나 `logging.SetDefault(logger)`로 호스트 소유 handler를 설치하세요. `slog.Default()`와 호스트의 표준 `log` 출력은 변경하지 않습니다. `logging.Fatal(message)`는 기록 후 상태 1로 종료하고 `logging.Panic(message)`는 기록 후 panic합니다. 출력이 필터링되어도 종료 동작은 실행됩니다.

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


## 컨테이너 로그

파일 이름을 생략하면 stderr와 컨테이너 런타임의 로그 순환을 사용합니다. lumberjack 파일 설정은 필요 없습니다. 내장 etcd는 logrus와 zap에 간접적으로 의존할 수 있지만 Engine 코드는 slog를 사용합니다.
