+++
title = "API"
description = "Open API 사양 문서"
weight = 300
draft = false
toc = false
tocheading = "h4"
tocsidebar = false
tags = ["v0.1.0", "api", "guide", "core"]
categories = ["docs"]
bref = "Open API 사양 문서. API를 사용하여 단순하거나 복잡한 정책을 선언적으로 모델링하고, 결정을 쿼리하며, 정책을 동적으로 발견하고 정책 평가를 디버깅합니다."
+++

Java, Python, Go, .NET, Rust, Node.js/TypeScript용 클라이언트 라이브러리는 [SDK](/docs/sdks/)에서 제공됩니다.

## REST API

이는 **구성된 PMS/ADS 또는 어서터(Asserter) 엔드포인트**의 참조 사양입니다. `speedle.io`는 문서와 브라우저 플레이그라운드를 제공하며 호스팅형 PMS/ADS 서비스를 제공하지 않습니다. `Content-Type: application/json`으로 JSON을 제공하고 [TLS/mTLS 및 접근 제어](/docs/security/)를 구성하십시오.

- [정책 관리 API](/docs/api/management_api/)
- [인가 결정 API](/docs/api/decision_api/)
- [토큰 어설션 웹훅](/docs/api/asserter_api/)

정책 및 역할 정책 ID는 문자열입니다. PMS 응답은 플랫 객체이며 인가 권한에는 `permissions`(복수형)를 사용합니다. REST 결정 요청은 형식화된 속성 배열을 사용하지만 진단 응답 속성 및 웹훅 속성은 객체입니다.

## gRPC API

- [PMS proto](/protobuf/pms.proto): `pb.PolicyManager`
- [ADS proto](/protobuf/ads.proto): `pb.Evaluator`
