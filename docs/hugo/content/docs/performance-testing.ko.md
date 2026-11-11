+++
title = "성능 테스트 가이드"
description = "AI 코딩 도구를 사용해 자신의 머신에서 공정하고 재현 가능한 인가 엔진 비교를 실행합니다."
weight = 15
draft = false
toc = true
singlelinetitle = true
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "performance", "benchmark", "policy"]
categories = ["docs"]
bref = "자신의 하드웨어와 정책으로 Speedle과 다른 인가 엔진을 측정합니다"
lastmod = 2026-11-11T12:00:00+08:00
+++

이 가이드는 자신의 머신에서 Speedle과 다른 인가 엔진을 비교하도록 돕습니다. 성능 보고서는 게시하지 않습니다. 수치는 하드웨어, 정책 집합, 고정한 엔진 버전에 따라 달라지므로, 신뢰할 수 있는 결과는 직접 측정한 결과뿐입니다.

이 가이드는 공유 시나리오, 네 가지 엔진의 동등한 정책, 방법론 체크리스트, 그리고 Claude Code, ChatGPT Codex, DeepSeek Harness 같은 AI 코딩 도구에 붙여 넣을 수 있는 프롬프트를 제공합니다. 프롬프트는 도구가 스크립트를 작성하고 실행하며 원시 출력을 자신의 머신에 남기도록 요청합니다. 도구가 작성한 내용은 게시된 결과가 아니라 검토해야 할 코드로 취급하세요.

## 이 가이드에서 얻는 것

- Speedle, Open Policy Agent(OPA), Casbin, Cedar의 동등한 RBAC 및 ABAC 규칙과 각 요청의 예상 결정.
- 자신의 머신에서 생성하고 실행하는 실행 가능한 벤치마크 스크립트, 정확한 명령, 고정한 엔진 버전.
- 지연 시간 백분위수, 처리량, 메모리 사용량, 규모에 따른 변화. 정책 로딩은 안정 상태 평가와 분리해 측정합니다.
- 원시 출력, 환경 정보, 결과 템플릿. 모두 로컬에 남습니다. 이 가이드는 아무것도 업로드하지 않습니다.

후보 엔진은 출발점이며 순위가 아닙니다. 선택한 각 엔진 버전이 이 시나리오를 지원하는지 확인하고, 제외한 사례와 이유를 기록하며, 제외한 사례를 해당 엔진의 결과로 제시하지 마세요.

## 시작하기 전에

- 직접 관리할 수 있고 실행 중에는 유휴 상태로 둘 수 있는 머신을 사용합니다. 부하가 큰 다른 프로세스를 종료하고 CPU 모델, 코어 수, 메모리, 운영 체제를 기록합니다.
- 비교에 필요한 도구를 설치합니다. Speedle과 Go 기반 엔진에는 Git과 Go 1.26 이상이 필요하며, 다른 엔진은 각 공식 빠른 시작의 요구 사항을 따릅니다. Speedle 릴리스 태그를 포함해 모든 엔진 버전을 고정합니다.
- 임시 디렉터리에 파일을 만들고 셸 명령을 실행할 수 있는 AI 코딩 도구를 사용합니다. 실행 전에 생성된 스크립트를 검토하고 원시 출력을 포함한 디렉터리 전체를 기록으로 남깁니다.
- 작은 정책 집합으로 처음 비교하는 데 30~60분을 예상합니다. 정책 수와 동시 실행 수를 늘리면 더 오래 걸립니다.

빈 디렉터리에서 작업합니다. Speedle 체크아웃 안에서 비교를 실행하지 말고, 생성한 스크립트, 정책, 로그, 결과를 저장소와 분리해 보관합니다.

## 공유 시나리오

비교는 하나의 업무 규칙 집합을 사용합니다.

- 직원(employee)은 자기 부서의 문서를 읽을 수 있습니다.
- 편집자(editor)는 자기 부서의 문서를 읽고 편집할 수 있습니다.
- Bob은 편집자이지만 4분기 재무 보고서 편집이 명시적으로 금지되어 있습니다.
- 사용자가 해당 역할을 가지고 있어도 다른 부서에 대한 요청은 거부됩니다.

요청에는 애플리케이션이 사용자의 `department`와 리소스의 `owner_department`를 속성으로 전달합니다. 모든 엔진은 같은 요청을 받고 같은 결정을 내려야 합니다.

| 사용자 | 역할 | 작업 | 리소스 | department | owner_department | 예상 |
| --- | --- | --- | --- | --- | --- | --- |
| alice | employee | read | `/docs/finance/report` | finance | finance | 허용 |
| alice | employee | read | `/docs/hr/salaries` | finance | hr | 거부 |
| bob | editor | edit | `/docs/finance/report` | finance | finance | 허용 |
| bob | editor | edit | `/docs/finance/q4` | finance | finance | 거부(명시적 거부) |
| carol | editor | edit | `/docs/finance/q4` | hr | finance | 거부(부서 조건) |
| carol | editor | read | `/docs/hr/handbook` | hr | hr | 허용 |

이 가이드의 예제는 Speedle v1.6.0, OPA v1.22.3, Casbin v2.123.0, Cedar CLI v4.14.1에서 검증했습니다. 설치하는 버전을 고정하세요. 이는 예시일 뿐 지원되는 유일한 버전이라는 뜻이 아닙니다.

### Speedle(SPDL)

```spdl
[service.documents]

[policy]
grant role employee read expr:/docs/.* if department == owner_department
grant role editor read,edit expr:/docs/.* if department == owner_department
deny user bob edit /docs/finance/q4

[rolepolicy]
grant user alice role employee
grant user bob role editor
grant user carol role editor
```

Speedle에서는 적용되는 거부가 허용보다 우선합니다. 조건이 참조하는 속성이 요청에 없으면 그 조건은 일치하지 않으므로, 애플리케이션은 모든 요청에 `department`와 `owner_department`를 보내야 합니다. 정책 파일은 `spctl check`로 검증합니다. 문법은 [SPDL](../spdl/), 서비스와 역할 정책은 [정책 관리](../pms/policy-mgmt/)를 참고하세요.

### OPA(Rego)

정책:

```rego
package documents

import rego.v1

default allow := false

allow if {
	not denied
	input.action == "read"
	"employee" in roles
	input.attributes.department == input.attributes.owner_department
}

allow if {
	not denied
	input.action in {"read", "edit"}
	"editor" in roles
	input.attributes.department == input.attributes.owner_department
}

roles contains role if {
	some role in data.roles[input.user]
}

denied if {
	input.user == "bob"
	input.action == "edit"
	input.resource == "/docs/finance/q4"
}
```

데이터(`data.json`):

```json
{
  "roles": {
    "alice": ["employee"],
    "bob": ["editor"],
    "carol": ["editor"]
  }
}
```

Rego에는 거부 우선 규칙이 내장되어 있지 않으므로 위 정책은 두 허용 규칙에서 명시적 거부를 빼냅니다. 역할 데이터는 OPA 데이터 문서의 일부입니다. 고정한 OPA 버전에 맞는 Rego 문법을 사용하세요(예제는 OPA 1.x의 Rego v1 문법입니다).

### Casbin

모델(`model.conf`):

```ini
[request_definition]
r = sub, obj, act, dept, ownerDept

[policy_definition]
p = sub, obj, act, eft

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (r.sub == p.sub || g(r.sub, p.sub)) && keyMatch(r.obj, p.obj) && regexMatch(r.act, p.act) && r.dept == r.ownerDept
```

정책(`policy.csv`):

```csv
p, employee, /docs/*, read, allow
p, editor, /docs/*, read, allow
p, editor, /docs/*, edit, allow
p, bob, /docs/finance/q4, edit, deny
g, alice, employee
g, bob, editor
g, carol, editor
```

Casbin은 모델, 정책 행, 역할 할당을 세 가지 입력으로 나눕니다. ABAC 부서 검사는 매처 식에 있으므로 거부 행에도 적용됩니다. 이 시나리오의 거부 요청은 부서 속성이 일치하므로 예상 결정이 바뀌지 않습니다. 무조건 거부를 표현하려고 매처를 변경했다면 그 변경을 기록하세요.

### Cedar

스키마(`schema.cedar`):

```cedar
entity Role;
entity User in [Role];
entity Folder;
entity Document in [Folder];

action "read" appliesTo {
  principal: [User],
  resource: [Document],
  context: { department: String, ownerDepartment: String }
};
action "edit" appliesTo {
  principal: [User],
  resource: [Document],
  context: { department: String, ownerDepartment: String }
};
```

정책(`policies.cedar`):

```cedar
permit (
  principal in Role::"employee",
  action == Action::"read",
  resource in Folder::"/docs"
)
when { context.department == context.ownerDepartment };

permit (
  principal in Role::"editor",
  action in [Action::"read", Action::"edit"],
  resource in Folder::"/docs"
)
when { context.department == context.ownerDepartment };

forbid (
  principal == User::"bob",
  action == Action::"edit",
  resource == Document::"/docs/finance/q4"
);
```

엔터티(`entities.json`):

```json
[
  {"uid": {"type": "User", "id": "alice"}, "attrs": {}, "parents": [{"type": "Role", "id": "employee"}]},
  {"uid": {"type": "User", "id": "bob"}, "attrs": {}, "parents": [{"type": "Role", "id": "editor"}]},
  {"uid": {"type": "User", "id": "carol"}, "attrs": {}, "parents": [{"type": "Role", "id": "editor"}]},
  {"uid": {"type": "Folder", "id": "/docs"}, "attrs": {}, "parents": []},
  {"uid": {"type": "Document", "id": "/docs/finance/report"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/finance/q4"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/hr/salaries"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/hr/handbook"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]}
]
```

Cedar에서는 forbid가 permit보다 우선하며 모든 요청은 기본적으로 거부됩니다. 역할과 문서 계층은 정책 텍스트가 아니라 엔터티 저장소의 데이터이므로 스키마와 엔터티도 비교의 일부입니다. 예제 엔터티 목록에는 요청 표에 나오는 모든 문서가 포함되어야 합니다.

### 기록해야 할 의미 차이

같은 업무 규칙이 항상 같은 엔진 개념으로 대응되지는 않습니다. 측정 전에 네 구현이 예상 결정에 합의하는지 확인하고 다음을 기록하세요.

- 허용과 거부가 동시에 일치할 때 각 엔진이 해결하는 방식, 그리고 거부가 무조건인지 ABAC 조건을 공유하는지.
- 요청에 `department` 또는 `owner_department`가 없을 때 각 엔진의 동작.
- 리소스 패턴을 표현하는 방식(SPDL의 `expr:` 정규식, Casbin 매처, OPA 보조 규칙, Cedar 엔터티 계층).
- 정책 텍스트 외에 필요한 애플리케이션 코드나 구성(Cedar 스키마와 엔터티, Casbin 모델과 역할 데이터, OPA 데이터 문서, Speedle 서비스와 역할 정책).
- 선택한 버전에서 표현할 수 없는 사례. 순위 사례에서 제외하고 그 이유를 밝힙니다.

## 올바른 것을 측정하기

모든 엔진에 같은 머신, 같은 요청 데이터, 같은 정책 집합 크기를 사용합니다. 버전을 고정하고 생성한 데이터 세트를 보관해 다시 실행할 때 같은 작업을 측정합니다.

- **내장 평가와 서비스/API 테스트를 분리합니다.** 프로세스 내 함수 호출과 HTTP 요청은 다른 것을 측정합니다. 서로 다른 표로 보고하고 서로 순위를 매기지 마세요.
- **워밍업하고 반복합니다.** 각 엔진을 안정적인 결과가 나올 때까지 충분히 실행하고 워밍업 실행을 버리며, 전체 측정을 최소 세 번 반복해 실행 간 변동을 보고합니다.
- **지연 시간과 처리량을 보고합니다.** 엔진 도구가 지원하면 p50, p95, p99 지연 시간과 초당 요청 수를 포함합니다. 실패와 시간 초과도 기록합니다.
- **메모리를 측정합니다.** 최대 상주 메모리 또는 엔진 자체의 일관되게 측정 가능한 메모리 지표를 기록합니다.
- **정책 수와 동시 실행 수를 바꿉니다.** 작게 시작해 정책 집합과 동시 호출 수를 늘립니다. 가장 좋은 지점만이 아니라 모든 데이터 지점을 기록합니다.
- **정책 로딩과 시작을 따로 측정합니다.** 각 엔진이 안정 상태 루프 전에 정책 집합을 파싱하고 로드하는 시간을 측정하고, 그 비용을 첫 요청에 숨기지 마세요.
- **캐시와 준비를 밝힙니다.** 인덱스, 컴파일된 정책, 결정 캐시, 사전 계산이 있으면 기록하고 워밍 상태인지 콜드 상태인지 표시합니다.
- **원시 출력을 보관합니다.** 각 실행의 정확한 명령, 구성, 정책 파일, 엔진 버전, 환경, 전체 출력을 저장합니다.

## AI 코딩 도구로 비교 실행하기

Claude Code, ChatGPT Codex, DeepSeek Harness 또는 파일을 만들고 명령을 실행할 수 있는 비슷한 도구에서 빈 디렉터리를 엽니다. 아래 프롬프트를 그대로 붙여 넣으세요. 도구에 환경 기록, 버전 고정, 공유 시나리오 구성, 예상 결정 검증, 측정 실행, 로컬 결과 템플릿 작성을 요청합니다.

```text
내 머신에서 Speedle과 OPA, Casbin, Cedar의 성능을 빠르게 비교하고 싶습니다. 먼저 내 운영 체제, 하드웨어, 리소스 제한을 기록하고, 지원되는 엔진 버전을 선택해 고정하세요. 공유 데이터 세트와 예상 허용/거부 결정으로 동등한 RBAC 및 ABAC 예제를 만드세요. 의미 차이를 설명하고 지원되지 않는 사례는 직접 순위에서 제외하세요. 성능을 측정하기 전에 정확성을 검증하세요.

실행 가능한 벤치마크 스크립트와 정확한 설치/실행 명령을 만드세요. 내장 테스트와 서비스/API 테스트는 분리하세요. 동일한 워크로드를 사용하고, 정책 수와 동시 실행 수를 바꾸고, 캐시와 준비를 밝히고, 각 엔진을 워밍업하고 측정을 반복하세요. 지원되는 경우 지연 시간, 처리량, 메모리 사용량, 실행 간 변동을 원시 출력과 환경 정보와 함께 보고하세요. 실제로 측정한 수치만 보고하고, 실행할 수 없었던 항목에는 그 사실을 표시하세요.

각 엔진의 동등한 정책과 필요한 모델/스키마/애플리케이션 코드를 보여 주세요. 가독성, 배워야 할 개념, 작성 노력, 검증, 디버깅, 정책 변경의 용이성을 비교하세요. 트레이드오프와 각 엔진이 어떤 테스트 워크로드에 적합한지 설명하고, Speedle이 이긴다고 가정하지 마세요.
```

도구는 현재 디렉터리에서 작업하고 스크립트, 정책, 출력을 그곳에 남겨야 합니다. 첫 실행 전에 특히 다운로드나 설치 단계를 포함해 스크립트를 검토하세요. 도구가 환경에 엔진을 설치할 수 없다면 수치를 추정하지 말고 해당 엔진을 실행하지 않음으로 표시하도록 요청하세요.

## AI 도구의 작업 확인하기

- 측정을 시작하기 전에 네 구현이 모든 요청에 대해 표의 결정을 반환하는지 확인합니다. 빠르지만 틀린 답은 결과가 아닙니다.
- 설치한 버전이 보고한 버전과 일치하고, 모든 엔진에 같은 머신, 요청 데이터, 정책 집합 크기가 사용되었는지 확인합니다.
- 로딩과 시작이 안정 상태 루프와 분리되어 보고되고, 내장 측정과 서비스/API 측정이 다른 표에 있는지 확인합니다.
- 요약의 모든 수치에 원시 출력이 있는지 확인합니다. 명령과 출력이 없는 수치는 측정되지 않은 것으로 취급합니다.
- 제한 사항을 읽으세요. 사례를 실행할 수 없었던 엔진은 순위가 아니라 실행하지 않음으로 표시해야 합니다.

## 결과 템플릿

도구가 다음과 같은 로컬 표를 채우게 하고 원시 출력과 함께 보관합니다. Speedle의 성능 보고서로 게시하지 마세요.

| 엔진 | 버전 | 모드 | 정책 수 | 동시 실행 수 | p50 | p95 | p99 | 처리량 | 최대 메모리 | 정책 로딩 시간 |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Speedle | | 내장 | | | | | | | | |
| OPA | | 내장 | | | | | | | | |
| Casbin | | 내장 | | | | | | | | |
| Cedar | | 내장 | | | | | | | | |

실행 기록도 남깁니다.

| 실행 | 엔진 | 명령 | 원시 출력 | 메모 |
| --- | --- | --- | --- | --- |
| 1 | | | | |

## 사용성 비교

측정과 사용성은 별개의 관찰입니다. 같은 업무 규칙을 사용해 각 엔진이 그것을 어떻게 표현하는지 기록하세요.

- **Speedle(SPDL).** 각 규칙은 하나의 문장이고, 역할 할당은 별도의 역할 정책 섹션에 있으며, 조건은 요청 속성에 대한 불리언 식입니다. 정책은 `spctl check`로 검증할 수 있습니다.
- **OPA(Rego).** 규칙은 기본 거부 아래의 규칙 본문으로 작성하고, 역할 데이터는 별도 문서에 두며, 명시적 거부에는 자체 규칙과 허용 규칙의 제외 조건이 필요합니다.
- **Casbin.** 모델, 정책 행, 역할 할당은 별도의 입력입니다. ABAC 검사는 매처 식의 일부이므로 조건을 바꾸려면 모델을 바꿔야 합니다.
- **Cedar.** 스키마가 엔터티와 작업을 선언하고, 정책은 permit과 forbid 문이며, 역할과 리소스 계층은 엔터티 저장소에 있습니다. forbid는 permit보다 우선합니다.

각 엔진에 대해 가독성(규칙이 업무 문장에 얼마나 직접 대응하는지), 배워야 할 개념, 상용구와 애플리케이션 코드, 검증과 오류 메시지, 테스트와 디버깅, 규칙을 하나 추가하거나 변경할 때 필요한 작업을 기록합니다. 정책 줄 수는 참고가 될 수 있지만 사용성의 유일한 척도로 삼지 마세요.

## Speedle 전용 벤치마크

저장소의 [벤치마크](https://github.com/teramoby/speedle-plus/blob/master/benchmarks/README.md)는 Speedle 단독 평가를 다루며, 명시적으로 켜는 백만 정책 사례를 포함합니다. 체크아웃에서 `go test -bench .`를 실행하고, 백만 정책 사례는 README의 안내를 따르세요. 이 측정은 Speedle 단독이며 특정 하드웨어에서 얻은 것입니다. 현재의 교차 엔진 비교로 제시하지 마세요.

## 공정하게 보고하기

- 모든 엔진에 같은 시나리오, 데이터, 정책 집합 크기를 사용하고 각 엔진이 실행하지 못한 사례를 밝힙니다.
- 다른 엔진이 더 빠른 사례를 포함해 결과의 전체 범위를 보고합니다.
- 각 엔진이 어떤 테스트 워크로드에 적합한지 한계와 함께 설명합니다. 한 머신의 한 시나리오로 보편적으로 가장 빠른 엔진을 선언하지 마세요.
- 요약을 공유할 때는 원시 출력과 정확한 명령을 함께 두세요.
