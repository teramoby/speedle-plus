+++
title = "Speedle Cloud"
description = "호스팅 인가, 계정 API 키, 요청 제한과 연동 안내."
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T21:00:00+08:00
+++

Speedle Cloud는 오픈 소스 Speedle 엔진 기반의 호스팅 인가 서비스입니다. 서비스, SPDL 인가 정책과 역할 정책을 관리하고 Playground에서 판정을 테스트한 뒤 서버에서 API를 호출합니다.

**Speedle Cloud는 출시 예정입니다.** 이 가이드는 프로덕션 로그인과 인증된 인가 API가 제공될 때의 사용 절차를 설명합니다. 예제는 실제 Cloud 환경에서 실행하지 않았습니다.

콘솔 사용과 API 키 생성에는 로그인이 가능해야 합니다. 무료 공개 베타는 시험, 학습, 평가용이며 SLA가 없고 프로덕션 사용은 권장하지 않습니다. 기능과 한도는 변경될 수 있습니다.

## 문서

- [빠른 시작](/ko/docs/cloud/getting-started/): 로그인한 뒤 콘솔에서 서비스와 정책, API 키를 생성하고 애플리케이션에서 인가를 확인합니다.
- [API 키](/ko/docs/cloud/tokens/): 계정 자격 증명, 고정 범위, 필수 만료 기간, 생성과 폐기, 한 번의 비밀 표시와 페이지 조회.
- [인가 API](/ko/docs/cloud/authorization-api/): IsAllowed 요청, 호출자 신원, Node.js 22와 전송 제약.
- [API 레퍼런스](/ko/docs/cloud/api-reference/): API key를 사용한 인가 확인과 정책 관리. 요청은 서버에서 보내며, 이 페이지는 읽기 전용입니다.
- [제한과 오류](/ko/docs/cloud/limits-and-errors/): 계정 한도, 크기, 버스트, 일일 할당량, 오류 코드와 초기화 시각.

## 다운로드

- [한국어 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ko.md): Cloud 네 장을 합친 오프라인 안내.
- [OpenAPI (YAML)](https://cloud.speedle.io/openapi.public.yaml)
- [OpenAPI (JSON)](https://cloud.speedle.io/openapi.public.json)

API 레퍼런스의 기계 판독 형식으로, 코드 생성기와 API 클라이언트에서 사용할 수 있습니다. 다운로드해도 로그인이 열리지는 않습니다.

Cloud 문서를 확인하세요: [이용 약관](https://cloud.speedle.io/ko/terms) / [개인정보 처리방침](https://cloud.speedle.io/ko/privacy).
