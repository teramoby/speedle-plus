+++
title = "Speedle 개요: 세분화된 접근 제어를 위한 인가 엔진"
linktitle = "개요"
description = "Speedle이 일반적인 인가 용어(PAP, PDP, PEP, PIP, RBAC, ABAC, PBAC, XACML, 제로 트러스트, ID, IAM)와 어떻게 대응하는지 설명합니다."
weight = 2
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["core", "pdp", "identity", "spdl"]
categories = ["docs"]
bref = "인가 및 접근 제어 어휘에서 Speedle의 위치"
lastmod = 2026-11-10T14:45:43+08:00
+++

Speedle은 오픈 소스 인가 엔진입니다. 애플리케이션으로부터 "이 사용자가 이 리소스에 대해 이 작업을 수행할 수 있는가?"라는 질의를 수신하고 중앙에서 관리되는 정책에 따라 허용(Allow) 또는 거부(Deny)를 반환합니다. 이 페이지에서는 일반적인 인가 용어와 이를 구현하는 Speedle 구성 요소를 연결하고 각각의 상세 가이드 링크를 제공합니다.

아래 예제에서는 `library`라는 이름의 서비스를 사용합니다. 각 정책은 정책 관리 서비스(PMS)의 REST API를 통해 생성되었으며, 각 결정은 인가 결정 서비스(ADS)의 REST API에서 반환되었습니다. 이유 코드는 [인가 결정](../decisions/)에 나열되어 있습니다.

## 인가 엔진

인가 엔진은 정책과 대조하여 접근 요청을 평가하고 결정을 반환함으로써 인가 로직이 애플리케이션 코드 외부에 분리되도록 합니다. Speedle은 [정책 관리 서비스(PMS), 정책 저장소 및 인가 결정 서비스(ADS)](../architecture/)로 구성됩니다. ADS는 독립 실행형 서비스로 실행하거나 [Go 애플리케이션에 임베드](../deployment/embedded/)할 수 있습니다.

## 인가와 접근 제어

인증(Authentication)은 호출자가 누구인지 확인하고, 인가(Authorization)는 해당 호출자가 무엇을 할 수 있는지 결정합니다. 접근 제어(Access control)는 이러한 결정의 강제 및 집행입니다. Speedle은 인가만을 처리합니다. 이미 인증된 주체를 전달받아 결정을 반환하며, 호출하는 애플리케이션이나 게이트웨이가 이를 적용합니다. [인가 결정](../decisions/)을 참조하십시오.

## 세분화된 접근 제어 (FGA)

세분화된 접근 제어(Fine-grained access control)는 애플리케이션 전체에 대한 광범위한 접근을 허용하는 대신 사용자별, 리소스별, 작업별로 접근을 결정하며 필요에 따라 요청 컨텍스트도 고려합니다. Speedle 정책에서는 주체, 작업, 리소스를 지정하고 조건을 추가할 수 있습니다. 역할 정책에서는 특정 리소스에 대해서만 역할을 할당할 수 있습니다. [SPDL](../spdl/)을 참조하십시오.

## PAP: 정책 관리 포인트

PAP(Policy Administration Point)는 정책이 생성되고 관리되는 장소입니다. Speedle에서는 [REST API](../api/management_api/), gRPC API 또는 `spctl` 명령줄 도구를 통해 사용되는 PMS가 이에 해당합니다. [정책 관리](../pms/policy-mgmt/)를 참조하십시오.

## PDP: 정책 결정 포인트

PDP(Policy Decision Point)는 요청을 정책과 대조하여 평가하고 결정을 반환합니다. Speedle에서는 ADS 또는 Go 프로세스 내에서 실행되는 임베디드 평가 엔진이 이에 해당합니다. 결정 요청 예제는 다음과 같습니다:

```bash
curl --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"library","resource":"book","action":"update"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

아래의 RBAC 정책이 설정되어 있는 경우 응답은 `{"allowed":true,"reason":0}`이 됩니다. [결정 API](../api/decision_api/)를 참조하십시오.

## PEP: 정책 적용 포인트

PEP(Policy Enforcement Point)는 접근 시도를 가로채서 PDP에 결정을 질의하고 이를 강제 집행합니다. Speedle은 범용 PEP를 자체 포함하지 않습니다. PEP는 REST 또는 gRPC API를 통해 ADS를 호출하거나 Go에 평가 엔진을 임베드하는 애플리케이션 또는 게이트웨이 자체입니다. 지원되는 인터페이스에 대해서는 [연동](../../integrations/)을 참조하십시오.

## PIP: 정책 정보 포인트

PIP(Policy Information Point)는 정책 평가에 필요한 추가 정보를 제공합니다. Speedle에서는 이 정보가 요청 속성이나 [기본 제공 속성](../spdl/#2-2-1-built-in-attributes), ID 토큰을 주체로 변환하는 [토큰 어서터](../assertor/), 평가 도중 외부 REST 엔드포인트를 호출하는 [사용자 정의 함수](../custom-function/)에서 제공됩니다.

## XACML

XACML(eXtensible Access Control Markup Language)은 정책 언어와 PAP/PDP/PEP/PIP 아키텍처를 모두 정의하는 OASIS 표준입니다. Speedle은 동일한 역할 분담 체계를 따르지만 정책 언어는 XACML이 아닌 SPDL이며 XACML 정책을 직접 가져올 수는 없습니다.

## RBAC: 역할 기반 접근 제어

RBAC에서는 권한이 역할에 부여되고 역할이 사용자 또는 그룹에 할당됩니다. Speedle은 정책을 사용하여 역할에 권한을 부여하고 역할 정책을 사용하여 역할을 할당합니다:

```text
grant role librarian read,update book
grant user alice librarian
```

`alice`는 `book`을 업데이트할 수 있습니다(이유 0: 인가 정책 존재). 역할을 갖지 않은 `bob`은 거부됩니다(이유 3: 적용 가능한 정책 없음). 역할은 특정 리소스에 대해서만 할당할 수도 있습니다. [정책 관리](../pms/policy-mgmt/) 및 [글로벌 정책](../pms/global-policy/)을 참조하십시오.

## ABAC: 속성 기반 접근 제어

ABAC에서는 주체, 리소스, 작업 또는 환경의 속성에 따라 결정이 내려집니다. Speedle에서는 임의의 정책 또는 역할 정책에 속성에 대한 `if` 조건을 추가할 수 있습니다:

```text
grant user bob read report if department == 'finance'
```

문자열 속성 `department`가 `finance`로 설정된 `bob`의 요청은 허용됩니다(이유 0). `sales`이거나 속성이 없는 경우 거부됩니다(이유 3). [조건식](../spdl/#condition)을 참조하십시오.

## PBAC: 정책 기반 접근 제어

PBAC는 각 애플리케이션에 하드코딩된 권한이 아니라 중앙에서 정의된 정책을 통해 접근을 관리합니다. 모든 Speedle 정책은 PMS에 보관되며 두 가지 고정된 규칙으로 평가됩니다: 거부는 허용보다 우선하며 적용 가능한 인가가 없는 요청은 거부됩니다.

```text
grant group staff read,delete journal
deny user carol delete journal
```

`staff`의 구성원인 `carol`은 `journal`에 대한 `delete`가 거부됩니다(이유 1: 거부 정책 존재). 동일하게 `staff`에 속한 `erin`은 허용됩니다(이유 0).

## ID와 IAM

ID 및 접근 관리(IAM)는 사용자가 누구인지 증명하는 것과 사용자가 무엇을 할 수 있는지 결정하는 것을 모두 포괄합니다. Speedle은 ID 공급자가 아니며 사용자 인증을 수행하지 않습니다. Speedle은 ID를 사용합니다: 주체는 [ID 도메인](../idd/)으로 수식할 수 있으며 ID 토큰은 [토큰 어서터](../assertor/)에 의해 확인될 수 있습니다.

```text
grant user dave from github read code
```

`github` ID 도메인의 `dave`는 `code` 읽기가 허용됩니다(이유 0). `corp`의 `dave` 또는 ID 도메인이 없는 `dave`는 거부됩니다(이유 3).

## 제로 트러스트

제로 트러스트는 네트워크 위치에 기반하여 접근을 허용하지 않으며 모든 요청을 명시적으로 검증합니다. Speedle은 결정 서비스로서 이를 지원합니다: ADS는 수신된 각 요청을 개별 평가하며 명시적으로 허용되지 않은 모든 것은 거부됩니다. 인증, 디바이스 신뢰 및 네트워크 제어는 Speedle의 범위에 속하지 않습니다.

## SPDL: Speedle Policy Definition Language

SPDL은 Speedle Policy Definition Language(Speedle 정책 정의 언어)의 약자입니다. Speedle 정책 및 역할 정책을 작성하는 데 사용되는 도메인 특화 언어입니다(예: `grant role librarian read,update book`). [SPDL 참조](../spdl/)를 참조하십시오.

## 연동

다른 애플리케이션은 [REST 또는 gRPC API](../api/)를 통해 ADS를 호출하거나 Go 프로세스에 평가 엔진을 임베드합니다. [배포](../deployment/)를 참조하십시오. [연동](../../integrations/) 페이지에는 현재 제공되는 API, SDK, 임베디드 모드 및 정책 저장소가 나열되어 있습니다.
