+++
title = "아키텍처"
description = "Speedle 구성 요소"
weight = 10
draft = false
toc = false
tocheading = "h2"
tocsidebar = false
bref = "Speedle 아키텍처"
+++

## 모듈

Speedle은 다음 구성 요소로 이루어진 인가 엔진입니다:

- 정책 관리 서비스 (PMS) API - 인가 및 역할 정책과 그 생성 대상 객체를 관리합니다.
- 정책 리포지토리 - 모든 정책 아티팩트를 저장합니다. JSON 파일 또는 데이터베이스, etcd 같은 영구 스토리지를 사용할 수 있습니다.
- 인가 결정 서비스 (ADS) API - 적용 가능한 정책에 대해 인가 요청을 평가하고 GRANT/DENY 결정을 반환합니다.

<img src="/img/speedle/spdlarch.jpg" width="524" height="241" loading="lazy" />

XACML 모델에 익숙하다면 정책 관리 서비스(PMS)는 PAP, 인가 결정 서비스(ADS)는 PDP 역할을 합니다.
