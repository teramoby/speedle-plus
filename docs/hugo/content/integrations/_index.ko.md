+++
title = "연동"
noindex = false
categories = ["docs"]
description = "Speedle을 애플리케이션 및 인프라와 연결."
aliases = ["/ko/integrations/istio/", "/ko/integrations/kubernetes/", "/ko/integrations/docker/"]
+++

Speedle은 인가 결정을 수행합니다. 애플리케이션, 게이트웨이 또는 플랫폼의 정책 시행 지점(PEP)은 결정마다 Speedle을 호출하고 그 결과를 적용합니다. 현재 지원되는 인터페이스는 다음과 같습니다.

## REST 및 gRPC API

정책 관리 서비스(PMS)를 통해 서비스, 정책 및 역할을 관리하고, 인가 결정 서비스(ADS)에서 런타임 결정을 요청합니다. 둘 다 REST API를 노출하며, `--grpc-endpoint`를 지정하여 서버를 시작하면 gRPC API가 활성화됩니다.

- [정책 관리 API](/ko/docs/api/management_api/)
- [인가 결정 API](/ko/docs/api/decision_api/)
- [토큰 어설션 웹훅](/ko/docs/api/asserter_api/)
- [API 개요(PMS 및 ADS proto 포함)](/ko/docs/api/)

## 임베디드 Go

별도의 PMS 또는 ADS 프로세스 없이 Go 애플리케이션 내에서 평가 엔진을 직접 실행합니다. [임베디드 모드 가이드](/ko/docs/deployment/embedded/)에서는 Go 모듈을 사용한 완전한 예제를 구축합니다.

## SDK

Java, Go, Python, Node.js/TypeScript, Rust, C#/.NET용 클라이언트 SDK가 제공됩니다. [SDK](/ko/docs/sdks/)를 참조하세요.

## 정책 스토리지

내장 파일, etcd, MySQL, MongoDB 스토리지를 사용하거나 다른 백엔드를 위한 스토리지 인터페이스를 구현할 수 있습니다. [스토리지 확장](/ko/docs/store/)을 참조하세요.

## 이전 가이드

이전 Istio, Kubernetes, Docker 샘플 가이드는 더 이상 게시되지 않습니다. 새로운 연동에는 위의 인터페이스를 사용하세요.
