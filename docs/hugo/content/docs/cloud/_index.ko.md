+++
title = "Speedle Cloud"
description = "Speedle Cloud 문서: 빠른 시작, 계정 및 정책 관리, 애플리케이션 토큰, 인가 API 프리뷰, 제한, 오류 및 다운로드"
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-08T17:11:18+08:00
+++

Speedle Cloud는 오픈 소스 Speedle 정책 엔진을 기반으로 구축된 개발자용 호스팅형 접근 제어 서비스입니다. 자체 인가 서비스를 직접 운영할 필요 없이 SPDL 인가 정책 및 역할 정책을 관리하고, 플레이그라운드에서 결정을 테스트하며, 애플리케이션 서버에서 인가 API를 호출할 수 있도록 설계되었습니다.

**Speedle Cloud는 출시 예정입니다.** Cloud 헤더에는 **Coming soon(출시 예정)** 배지가 표시되어 있으며, [cloud.speedle.io](https://cloud.speedle.io/ko/)에서의 프로덕션 로그인은 아직 시작되지 않았습니다. 여기에 설명된 콘솔 워크플로 및 인증된 인가 결정 서비스(ADS)는 공개된 서비스 계약의 프리뷰입니다. 인증된 ADS가 보류 중인 동안 토큰을 발급하더라도 서버 측 인가 호출이 활성화되지 않습니다. 선택한 단계에서의 배포 및 ID 공급자의 가용성을 확인하십시오.

무료 공개 베타는 시험 사용, 학습 및 평가를 목적으로 합니다. SLA는 제공되지 않으며 프로덕션 환경에서의 사용은 권장되지 않습니다. 또한 기능과 제한 사항은 변경될 수 있습니다.

## 문서

- [빠른 시작](/docs/cloud/getting-started/): GitHub 또는 Google 로그인(출시 예정), 계정 관리, 서비스, SPDL 정책, 세션 관리 요청 및 4가지 세션 플레이그라운드 작업.
- [애플리케이션 토큰](/docs/cloud/tokens/): 최근 재인증 요구 사항, 1회 한정 토큰 공개, 메타데이터 페이지네이션, 교체 및 취소.
- [인가 API 프리뷰](/docs/cloud/authorization-api/): IsAllowed 요청 및 응답 계약, Node.js 22 클라이언트 예제, 정책 스냅샷 및 통합 제약 사항.
- [제한 및 오류](/docs/cloud/limits-and-errors/): 계정 쿼터, 요청 제한, 오류 응답 구조, 레이트 및 쿼터 헤더, 재시도 지침.

## 다운로드

- [한국어 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ko.md): 4개의 Cloud 장을 모은 오프라인 문서.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): 저장소에 체크인된 정식 Cloud API 사양.
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): 동일한 YAML 소스 바이트에서 생성.
- [제공 빌드 버전](https://cloud.speedle.io/build.json): 서비스를 제공하는 빌드를 식별(ID 공급자나 엔드포인트의 가용성을 증명하지는 않음).

사양에는 4가지 세션 플레이그라운드 작업과 인증된 ADS 계약이 포함됩니다. 사양 공개나 가이드 다운로드가 보류 중인 엔드포인트를 활성화하거나 로그인을 시작하지는 않습니다. YAML 및 생성된 JSON은 해당 봉인된 Cloud 빌드와 함께 버전 관리, 프로모션 및 복원됩니다. 한국어, 일본어, 영어, 중국어 Markdown 다운로드는 이 Engine 문서 페이지와 함께 유지 관리됩니다.

Cloud의 [이용약관](https://cloud.speedle.io/ko/terms) 및 [개인정보 처리방침](https://cloud.speedle.io/ko/privacy)을 읽어보십시오.
