+++
title = "버전"
description = "Speedle 릴리스 내역 및 시맨틱 버저닝"
weight = 400
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["releases", "versions"]
categories = ["docs"]
bref = "릴리스 — Speedle은 시맨틱 버저닝을 따릅니다"
lastmod = 2026-11-10T14:45:43+08:00
+++

## Speedle 릴리스

게시된 버전 및 릴리스 노트는 [GitHub Releases](https://github.com/teramoby/speedle-plus/releases) 페이지를 참조하세요.

- [v1.9.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.9.0): 사용자 지정 함수 실패 시 결정 실패, 함수의 매개변수 및 결과 타입 선언, MySQL 저장소 스키마 버전 2, js/wasm 빌드의 호스트 사용자 지정 함수 호출기, 애플리케이션 및 감사 로깅의 Go `log/slog` 전환.
- [v1.8.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.8.0): 실험적 정책 배포 기능 제거, 소스 빌드에 Go 1.27.4 이상 필요, 컨테이너 이미지에 공개 CA 인증서 번들 포함, 일본어·한국어·스페인어 문서 추가.
- [v1.7.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.7.0): MySQL 정책 저장소, 오프라인 `spctl check`, 명령 도움말의 전체 옵션 목록, panic 및 저장소 장애 시 기본 거부, Docker Hub와 GHCR의 단일 멀티 아키텍처 이미지.

[v1.9.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.9.0)은 현재 임베디드 Go 안내서에서 고정된 버전입니다. 재현 가능한 모듈 의존성을 위해 명시적으로 릴리스된 버전을 사용하십시오. 설정 방법은 [임베디드 가이드](../deployment/embedded/)에 설명되어 있습니다.

## 이전 Oracle Speedle 릴리스

버전 0.1.0은 정책 관리, 인가 결정, 명령줄 도구 및 연동 샘플을 포함한 초기 오픈 소스 릴리스였습니다.
