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
+++

## 개요

로그 메시지 처리를 위해 다음 두 개의 오픈 소스 프로젝트가 사용됩니다:

```
1. https://github.com/sirupsen/logrus
2. https://github.com/natefinch/lumberjack
```

Logrus는 Go(golang)용 구조화된 로거이며 golang 표준 라이브러리 로거와 완전한 API 호환성을 갖추고 있습니다. Logrus는 로그 롤링을 기본적으로 지원하지 않으므로 롤링 로거를 제공하는 두 번째 프로젝트인 lumberjack을 통합했습니다. Logrus는 `io.Writer` 인터페이스를 사용하여 lumberjack과 연동합니다. 아래 다이어그램을 참조하십시오.

<img src="/img/speedle/logger.jpg" alt="Speedle Logging" width="370" height="230" loading="lazy">

## 로깅 구성

Speedle 로깅은 명령줄 플래그와 구성 파일의 두 가지 방식으로 구성할 수 있습니다.

### 명령줄 인수를 통한 로깅 구성

사용 가능한 모든 명령줄 인수는 다음과 같습니다. 레벨, 포맷터, reportcaller 설정은 logrus를 구성하고, 파일/롤링 설정은 lumberjack을 구성합니다.

```text
--log-level string                로그 레벨(panic, fatal, error, warn, info, debug) (default "info")
--log-formatter string            로그 포맷터(text, json) (default "text")
--log-reportcaller                호출자의 메서드 이름과 파일 이름을 포함할지 여부
--log-file string                 로그 파일 경로
--log-file-maxsize int            로그 롤링 전 최대 크기(MB 단위) (default 100)
--log-file-maxage int             오래된 로그 파일을 보관할 최대 일수 (default 0)
--log-file-maxbackups int         보관할 이전 로그 파일의 최대 수 (default 0)
--log-file-localtime              백업 파일의 타임스탬프에 로컬 시간을 사용할지 여부
--log-file-compress               롤링된 로그 파일을 gzip으로 압축할지 여부
```

### 구성 파일을 통한 로깅 구성

구성 파일을 사용하여 로깅을 설정할 수도 있습니다. 구성 예제는 다음과 같습니다:

```json
{
  "logConfig": {
    "level": "info",
    "formatter": "json",
    "reportCaller": false,
    "fileConfig": {
      "filename": "/var/log/speedle.log",
      "maxSize": 100,
      "maxAge": 30,
      "maxBackups": 10,
      "localTime": true,
      "compress": true
    }
  }
}
```
