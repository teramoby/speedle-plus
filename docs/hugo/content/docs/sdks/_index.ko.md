+++
title = "SDK"
description = "Java, Python, Go, .NET, Rust, Node.js/TypeScript용 Speedle 인가 확인 클라이언트 라이브러리"
weight = 305
draft = false
toc = true
linksection = true
tocheading = "h2"
tags = ["sdk"]
categories = ["docs"]
bref = "Speedle 인가 결정 API용 6개 언어 클라이언트 라이브러리. 자체 호스팅 Speedle ADS 및 Speedle Cloud(출시 예정)를 지원합니다."
lastmod = 2026-11-10T14:45:43+08:00
+++

Speedle SDK는 결정 API `POST /authz-check/v1/is-allowed`의 클라이언트 라이브러리입니다. 자체 호스팅 Speedle ADS 및 Speedle Cloud 모두에서 작동합니다. 각 언어 페이지에서는 패키지 추가 방법을 설명하고 두 환경 모두에 대해 테스트를 거친 완전한 프로그램을 제공합니다. 소스 코드와 전체 참조 문서는 [teramoby/speedle-sdk](https://github.com/teramoby/speedle-sdk)에서 확인할 수 있습니다. 이 예제들에서 사용할 PMS 및 ADS 실행 방법에 대해서는 [배포](/docs/deployment/)와 [Docker 이미지](/docs/deployment/docker/)를 참조하십시오.

## 패키지

Java, Python, Go, .NET, Rust는 0.2.0 버전입니다. `@speedle/sdk`는 npm에 게시되어 있습니다. 각 언어별 설치 명령은 해당 페이지에 나와 있습니다.

- [Java](/docs/sdks/java/): Maven Central `io.speedle:speedle-sdk`, 게시 완료.
- [Python](/docs/sdks/python/): PyPI `speedle`, 게시 완료.
- [Go](/docs/sdks/go/): Go 모듈 `github.com/teramoby/speedle-sdk/go`, 게시 완료.
- [C#/.NET](/docs/sdks/dotnet/): NuGet `Speedle.Sdk`, 게시 완료.
- [Rust](/docs/sdks/rust/): crates.io `speedle`, 게시 완료.
- [Node.js / TypeScript](/docs/sdks/node/): npm `@speedle/sdk`, 게시 완료.

## 기능

| 기능 | 상태 |
| --- | --- |
| `isAllowed`: 인가 결정 반환 | 6개 SDK 모두 지원 |
| `check`: 인가 결정 및 이유(해당하는 정책 없음 등) 반환 | 6개 SDK 모두 지원 |
| 자체 호스팅 Speedle ADS | 6개 SDK 모두 지원 |
| Speedle Cloud | 출시 예정 |

메서드 이름은 각 언어의 명명 규칙을 따릅니다. Python 및 Rust는 `is_allowed`와 `check`, Go는 `IsAllowed`와 `Check`, .NET은 `IsAllowedAsync`와 `CheckAsync`입니다.

부여된 권한 조회, 진단, 정책 관리 등 기타 작업은 REST API([인가 결정 API](/docs/api/decision_api/) 및 [정책 관리 API](/docs/api/management_api/))를 사용하십시오.

## 예제 정책 <a id="example-policy"></a>

모든 예제는 `docs` 서비스 내의 리소스 `/docs/public`에 대해 사용자 `alice`가 `get` 및 `delete` 작업을 수행할 수 있는지 확인합니다. 로컬 Speedle에서 문서와 동일한 결과를 얻으려면 [빠른 시작](/quick-start/)의 설명에 따라 빌드하고 포트 6733에서 PMS를, 포트 6734에서 ADS를 시작한 후 서비스와 정책을 생성합니다:

```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

로컬 예제는 자격 증명 없이 `http://localhost:6734`에 연결합니다. SDK는 루프백 호스트에 대해서만 일반 텍스트 `http://`를 허용하며, 다른 ADS 호스트에 연결할 때는 HTTPS를 사용해야 합니다.

Speedle Cloud 예제는 `https://cloud.speedle.io`에 연결하며 환경 변수 `SPEEDLE_SERVICE` 및 `SPEEDLE_TOKEN`에서 서비스 이름과 `ads` 범위의 계정 API 키을 읽습니다. 해당 서비스에 동일한 정책을 생성하면 동일한 결과를 얻을 수 있습니다. API 키은 서버 측 비밀 값이므로 브라우저, 모바일 앱 또는 데스크톱 클라이언트에 절대 노출하지 마십시오.
