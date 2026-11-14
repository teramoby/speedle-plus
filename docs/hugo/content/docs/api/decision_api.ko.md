+++
title = "인가 결정 API"
description = "인가 결정 API 참조: 접근 권한 확인, 부여된 역할 및 권한 조회, 정책 발견 및 평가 프로세스 진단"
tags = ["v0.1.0", "api", "guide", "core", "ads"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
lastmod = 2026-11-12T10:00:00+08:00
+++

구성된 ADS 인터페이스 참조 사양입니다. 요청 속성은 형식화된 배열이고 진단 응답 속성은 객체입니다.

[보안 및 엔드포인트 구성](/docs/security/). 이 문서는 인터페이스 사양을 설명하며 speedle.io는 호스팅형 백엔드를 제공하지 않습니다. Try it out은 꺼져 있습니다. 사양에 서버 호스트가 없어서, 켜 두면 요청이 이 문서 사이트로 갑니다.

{{< swagger-spec url="/swagger/policy-check.yaml" tryitout="false" >}}

## 헬스 체크 엔드포인트

`GET /healthz`는 프로세스의 실행 상태를 보고합니다. 프로세스가 실행 중인 동안 항상 `200 {"status":"ok"}`를 반환합니다.

`GET /readyz`는 인증이 필요하지 않으며 로컬 결정 서비스에서 `200 {"ready":true,"servicesTotal":0,"servicesUsable":0}`을 반환합니다. 카운트는 항상 0이며 저장된 서비스를 열거하지 않습니다.
