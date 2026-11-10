+++
title = "SPDL: Speedle 정책 정의 언어"
description = "SPDL은 Speedle Policy Definition Language의 약자로, Speedle 정책 및 역할 정책을 작성하기 위한 언어입니다."
weight = 5
draft = false
toc = true
singlelinetitle = true
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "core", "policy", "spdl"]
categories = ["docs"]
bref = "SPDL의 기초"
lastmod = 2026-11-08T20:01:50+08:00
+++

SPDL은 Speedle 정책 및 역할 정책을 작성하기 위한 언어입니다. 이 페이지에서는 구문, 명명 규칙 및 예제를 설명합니다.
<!--more-->

<aside class="docs-callout" aria-labelledby="spdl-callout-title">
  <p class="docs-callout-eyebrow">Speedle 정책 모델러 · 오픈 소스 스킬</p>
  <p class="docs-callout-title" id="spdl-callout-title">AI로 SPDL 작성</p>
  <p>오픈 소스 Speedle 정책 모델러 스킬은 요구 사항과 정책 문서를 허용 및 거부 테스트 케이스를 포함한 SPDL 모델로 변환하고 <code>spctl check</code>로 검증합니다.</p>
  <p class="docs-callout-actions"><a class="button primary" href="https://ai.speedle.io/ko/#policy-modeler">정책 모델러 스킬 체험하기</a> <a class="button secondary" href="https://github.com/teramoby/ai.speedle.io/tree/main/skills/speedle-policy-modeler">GitHub에서 소스 보기</a></p>
</aside>

## 구문

<picture><source srcset="/img/speedle/spdl-syntax.webp" type="image/webp"><img src="/img/speedle/spdl-syntax.png" alt="기본 정책 정의 구문" width="2222" height="971" fetchpriority="high"></picture>

### 키워드

`grant`, `deny`, `user`, `group`, `entity`, `role`, `from`, `on`, `if`, `in`은 각각의 구문 위치에서 특별한 의미를 가집니다. 정책 문의 키워드는 대소문자를 구분하지 않습니다. 조건 내에서는 `in` 또는 `IN`을 사용하고 함수 이름은 대소문자를 구분합니다. 주체, 작업 및 리소스 이름도 대소문자를 구분합니다. 현재 파서는 일부 이름 위치에서 키워드와 유사한 이름을 허용하지만 일률적인 금지를 검증 규칙으로 전제하지 마십시오. 검토와 클라이언트 간 상호 운용성을 높이기 위해 모호하지 않은 이름을 권장합니다.

### 명명 규칙

단순하고 안정적인 식별자를 권장합니다. 공백이나 구분 기호가 포함된 이름은 파서가 지원하는 작은따옴표나 큰따옴표로 감싸고 실제 입력으로 테스트하십시오. 과거의 Java 정규식 문자 클래스는 현재 Go 파서의 엄격한 검증 규칙이 아닙니다. 리소스 접두사 `expr:`은 셸 와일드카드가 아닌 Go 정규식을 사용합니다. 조건 내의 속성 및 함수 이름에 대해서는 아래에서 설명합니다.

### 구문 정의

<pre>
POLICY = EFFECT SUBJECT ACTION RESOURCE [if CONDITION]
EFFECT = grant | deny
SUBJECT = AND_PRINCIPALS (, AND_PRINCIPALS)*
AND_PRINCIPALS = PRINCIPAL | \( PRINCIPAL_LIST \)
PRINCIPAL_LIST = PRINCIPAL (, PRINCIPAL)*
PRINCIPAL = PRINCIPAL_TYPE PRINCIPAL_NAME [PRINCIPAL_IDD]
PRINCIPAL_TYPE = user|group|entity|role
PRINCIPAL_IDD = from IDD_IDENTIFIER
IDD_IDENTIFIER = NAME
ACTION = (ACTION_IDENTIFIER)(, ACTION_IDENTIFIER)*
RESOURCE = RESOURCE_IDENTIFIER
PRINCIPAL_NAME = NAME
ACTION_IDENTIFIER = NAME
RESOURCE_IDENTIFIER = NAME
</pre>
<pre>
ROLE_POLICY = EFFECT SUBJECT ROLE (on RESOURCE)? [if CONDITION]
EFFECT = grant | deny
SUBJECT = PRINCIPAL (, PRINCIPAL)*
PRINCIPAL = PRINCIPAL_TYPE PRINCIPAL_NAME [PRINCIPAL_IDD]
PRINCIPAL_TYPE = user|group|entity|role
PRINCIPAL_IDD = from IDD_IDENTIFIER
IDD_IDENTIFIER = NAME
ROLE = (role)? SUBJECT_IDENTIFIER
RESOURCE = RESOURCE_IDENTIFIER
SUBJECT_IDENTIFIER = NAME
RESOURCE_IDENTIFIER = NAME
</pre>

## 예제

역할 상속: 역할 정책의 주체는 다른 역할이 될 수 있으므로, `manager`는 `employee`에 부여된 모든 역할도 함께 보유합니다.

```text
grant role manager employee
```

거부 우선: 일치하는 거부(deny) 정책은 허용(grant) 정책보다 우선합니다. 아래 예제에서 `carol`은 `staff`에 속하지만 `journal`에 대한 `delete` 작업은 거부됩니다.

```text
grant group staff read,delete journal
deny user carol delete journal
```

조건: 조건이 참인 경우에만 정책이 적용됩니다.

```text
grant user bob read report if department == 'finance'
```

위의 예제는 SPDL 문장 예제이며 완전한 서비스 구성이 아닙니다. 서비스, 리소스 범위의 역할 및 완전한 결정 예제에 대해서는 [정책 관리](../pms/policy-mgmt/)를 참조하십시오.

<a id="condition"></a>
## 조건(Condition)

### 1. 개요

인가 정책 및 역할 정책 모두에서 조건을 지원합니다. 조건이 충족되는 경우에*만* 정책 또는 역할 정책이 적용됩니다.

정책 적용 지점(PEP)은 정책에서 참조하는 모든 사용자 정의 속성을 제공해야 합니다. `request_user` 및 `request_time`과 같은 기본 제공 속성은 ADS에서 자동으로 제공합니다. PEP에서 제공하는 속성은 최종 사용자로부터 직접 가져오는 것이 아니라 호출하는 신뢰할 수 있는 애플리케이션 또는 서비스에서 제공해야 하며, PEP는 전달하는 값에 책임을 집니다. PEP에서 제공하는 속성은 동일한 이름의 기본 제공 속성보다 우선합니다. 특히 신뢰할 수 없는 최종 사용자 입력을 `request_user`, `request_time` 또는 기타 기본 제공 속성 이름으로 전달하지 마십시오.

요청에 존재하지 않는 속성을 참조하는 조건은 일치하지 않습니다. 따라서 조건부 `deny`는 해당 속성이 존재하지 않는 경우 적용되지 않습니다. 거부 조건에서 사용하는 속성을 PEP가 항상 제공하는지 확인하고 해당 요청을 테스트하십시오.

이 절에서는 조건의 구성과 작성 방법에 대해 설명합니다.

### 2. 조건의 구성

조건은 속성, 함수, 상수, 연산자, 비교 연산자 또는 괄호를 사용하여 구성되며 부울 값을 반환하는 부울 식입니다.

#### 2.1 데이터 타입

속성 값 및 상수의 데이터 타입은 string, numeric, bool, datetime 또는 이러한 타입의 배열입니다.
지원되는 데이터 타입과 해당 연산자 및 비교 연산자는 다음 표와 같습니다.

 <table class="bordered striped">
    <thead>
      <tr>
        <th>데이터 타입</th>
        <th>연산자</th>
        <th>비교 연산자</th>
        <th>설명</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>string</td>
        <td>+</td>
        <td>== <br> != <br> =~ <br> > <br> >= <br> < <br> <= <br>
        </td>
        <td><code>+</code>는 문자열 연결에 사용됩니다.<br><code>=~</code>는 정규식 매칭에 사용됩니다. 표 아래의 참고 사항을 참조하십시오.</td>
      </tr>
      <tr>
        <td>numeric</td>
        <td> + <br> - <br> * <br> / <br> %</td>
        <td>== <br> != <br>  > <br> >= <br> < <br> <= </td>
        <td></td>
      </tr>
      <tr>
        <td>bool</td>
        <td>&& <br> || <br> !</td>
        <td>== <br>!=</td>
        <td></td>
      </tr>
      <tr>
        <td>datetime</td>
        <td></td>
        <td>== <br> != <br> > <br> >= <br> < <br> <=</td>
        <td></td>
      </tr>
      <tr>
        <td>array</td>
        <td></td>
        <td>in</td>
        <td>포함 여부를 나타내는 <code>in</code> 연산자: 왼쪽은 단일 타입(string, numeric, bool, datetime), 오른쪽은 배열이어야 합니다.</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

`"=~"` 연산자 참고 사항:
`"=~"`는 정규식 비교 연산자입니다. Go 언어의 표준 `regexp` 구문을 사용합니다. 왼쪽은 대상 문자열, 오른쪽은 정규식 패턴입니다. 대상 문자열이 오른쪽 정규식 패턴과 일치하는지 여부를 부울 값으로 반환합니다.

- 왼쪽: string
- 오른쪽: string
- 반환 값: bool

#### 2.2 속성

조건 내의 속성은 변수를 나타냅니다. 속성 값은 런타임에 결정되며 인가 결정 서비스(ADS) 런타임 또는 PEP에서 가져옵니다.
기본 제공 속성은 ADS 런타임에 의해 자동으로 설정되는 값입니다.
사용자 정의 속성은 인가 결정을 요청할 때 최종 사용자가 아닌 호출하는 신뢰할 수 있는 애플리케이션 또는 서비스(PEP)에서 제공하는 값입니다. PEP는 전달하는 값에 책임을 집니다. PEP에서 제공하는 속성은 동일한 이름의 기본 제공 속성보다 우선하므로 PEP는 신뢰할 수 없는 최종 사용자 입력을 기본 제공 속성 이름으로 전달해서는 안 됩니다.

<a id="2-2-1-built-in-attributes"></a>
##### 2.2.1 기본 제공 속성

기본 제공 속성은 다음과 같습니다:

<table class="bordered striped">
    <thead>
      <tr>
        <th>기본 제공 속성 이름</th>
        <th>데이터 타입</th>
        <th>예제 값</th>
        <th>정의</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>request_user</td>
        <td>string</td>
        <td>"Alice"</td>
        <td>리소스에 대한 작업을 요청하는 주체(Subject)의 사용자 정보</td>
      </tr>
      <tr>
        <td>request_groups</td>
        <td>[]string</td>
        <td>[]string{"managers"}</td>
        <td>리소스에 대한 작업을 요청하는 주체(Subject)의 그룹 정보</td>
      </tr>
      <tr>
        <td>request_entity</td>
        <td>string</td>
        <td>"/org1/service1"</td>
        <td>서비스 또는 프로그램이 작업을 요청하는 주체(Subject)의 서비스 이름 또는 기타 엔티티 정보</td>
      </tr>
      <tr>
        <td>request_resource</td>
        <td>string</td>
        <td>"commercialLoans"</td>
        <td>주체가 작업 대상으로 하는 리소스</td>
      </tr>
      <tr>
        <td>request_action</td>
        <td>string</td>
        <td>"issue" </td>
        <td>주체가 리소스에 대해 수행하는 작업</td>
      </tr>
      <tr>
        <td>request_time</td>
        <td>datetime</td>
        <td>'2019-01-02T15:04:05-07:00'</td>
        <td>요청이 발생한 날짜 및 시간</td>
      </tr>
      <tr>
        <td>request_year</td>
        <td>int</td>
        <td>2019</td>
        <td>요청이 발생한 연도</td>
      </tr>
      <tr>
        <td>request_month</td>
        <td>int</td>
        <td>1, 2, ... 12</td>
        <td>요청이 발생한 월</td>
      </tr>
      <tr>
        <td>request_day</td>
        <td>int</td>
        <td>1, 2, ... 31</td>
        <td>요청이 발생한 날(일)</td>
      </tr>
      <tr>
        <td>request_hour</td>
        <td>int</td>
        <td>0, 1, ... 23</td>
        <td>요청이 발생한 시각(시)</td>
      </tr>
      <tr>
        <td>request_weekday</td>
        <td>string</td>
        <td>"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"</td>
        <td>요청이 발생한 요일</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

##### 2.2.2 사용자 정의 속성

###### 2.2.2.1 속성 이름

이식성 높은 조건 참조를 위해 `[A-Za-z][A-Za-z0-9_]*`에 부합하는 이름을 권장하며, 기본 제공 속성 이름이나 함수 이름과의 중복을 피하십시오. REST 계층은 최대 256자의 ASCII 문자, 숫자, 밑줄, 하이픈 및 마침표를 허용합니다. 하이픈이나 마침표가 포함된 표현식 참조는 연산자로 오인되지 않도록 실제로 테스트하십시오.

###### 2.2.2.2 속성 값

사용자 정의 속성이 조건에서 사용되는 경우 클라이언트는 isAllowed 결과를 요청할 때 해당 속성 값을 전달해야 합니다. 요청에 조건이 참조하는 속성이 포함되어 있지 않으면 조건이 일치하지 않습니다. 이 경우 조건부 `deny`는 적용되지 않습니다. 거부 조건이 사용하는 모든 속성을 PEP가 제공하는지 확인하고 해당 요청을 테스트하십시오.

- Go API에서 속성 값 전달
  Go API에 사용자 정의 속성 값을 전달할 때는 다음 규칙을 따르십시오:

  - bool 속성 값에는 Go의 bool 타입을 사용합니다.
  - string 속성 값에는 Go의 string 타입을 사용합니다.
  - numeric 속성 값에는 Go의 float64 타입을 사용합니다.
  - datetime 속성 값에는 해당 날짜/시간의 Unix 시간을 나타내는 Go의 float64 표현(`time.Time.Unix()` 사용)을 사용합니다.
  - array 속성 값에는 Go의 []interface{}를 사용합니다.

- REST API에서 속성 값 전달
  REST API에 사용자 정의 속성 값을 전달할 때는 다음 규칙을 따르십시오:
  - 속성은 속성의 이름, 타입, 값을 포함하는 구조체입니다. 자세한 내용은 REST API를 참조하십시오.
  - 속성 타입은 "string", "numeric", "bool" 또는 "datetime"만 지정할 수 있습니다.
  - 속성 값은 단일 값 또는 슬라이스가 될 수 있습니다.

#### 2.3 상수

지원되는 데이터 타입:

- string: 작은따옴표, 'foobar'
- numeric: 10, 3.1415926
- bool: true 또는 false
- datetime: 작은따옴표, RFC3339 준수. RFC3339 형식의 날짜/시간은 YYYY-MM-DDTHH:mm:SS[.sssssssss]Z(Z는 [+|-]HH:mm)입니다.
- array: string, numeric, bool, datetime 타입의 배열

다음 표는 상수의 예제를 보여줍니다.

<table class="bordered striped">
    <thead>
      <tr>
        <th>데이터 타입</th>
        <th>상수 예제</th>
        <th>상수 배열 예제</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>string</td>
        <td>'a string'</td>
        <td>('string1', 'string2')</td>
      </tr>
      <tr>
        <td>bool</td>
        <td>true <br> false</td>
        <td>(true, false, true)</td>
      </tr>
      <tr>
        <td>numeric / float</td>
        <td>2.1</td>
        <td>(1, 2, 3.1)</td>
      </tr>
      <tr>
        <td>datetime</td>
        <td>'2006-01-02T15:04:05-07:00'</td>
        <td>('2016-01-02T15:04:05-07:00', '2019-01-02T15:04:05-07:00')</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

#### 2.4 함수

기존 연산자나 비교 연산자로 요구 사항을 충족할 수 없는 경우 함수를 사용합니다.

##### 2.4.1 기본 제공 함수

Speedle은 다음과 같은 기본 제공 함수를 제공합니다:

<table class="bordered striped">
    <thead>
      <tr>
        <th>기본 제공 함수 이름</th>
        <th>기능</th>
        <th>입력 및 데이터 타입</th>
        <th>출력 및 데이터 타입</th>
        <th>사용 예제</th>
      </tr>
    </thead>
    <tbody>
       <tr>
        <td>Sqrt</td>
        <td>숫자의 제곱근 계산</td>
        <td>1개의 numeric 매개변수</td>
        <td>numeric</td>
        <td>Sqrt(x)<br>Sqrt(64)</td>
      </tr>
       <tr>
        <td>Max</td>
        <td>집합 내 최대값 조회</td>
        <td>1개 이상의 numeric 매개변수</td>
        <td>numeric</td>
        <td>Max(1, 4, x)</td>
      </tr>
       <tr>
        <td>Min</td>
        <td>집합 내 최소값 조회</td>
        <td>1개 이상의 numeric 매개변수</td>
        <td>numeric</td>
        <td>Min(1, 4, x)</td>
      </tr>
       <tr>
        <td>Sum</td>
        <td>집합 내 모든 수치의 합계 계산</td>
        <td>1개 이상의 numeric 매개변수</td>
        <td>numeric</td>
        <td>Sum(1, 4, x)</td>
      </tr>
       <tr>
        <td>Avg</td>
        <td>집합 내 모든 수치의 평균 계산</td>
        <td>1개 이상의 numeric 매개변수</td>
        <td>numeric</td>
        <td>Avg(1, 4, x)</td>
      </tr>
       <tr>
        <td>IsSubSet</td>
        <td>첫 번째 집합이 두 번째 집합의 부분 집합인지 확인</td>
        <td>동일한 타입(string, numeric, bool 또는 datetime)의 요소를 가진 2개의 배열</td>
        <td>bool</td>
        <td>IsSubSet(a, ('s1', 's2', 's3'))</td>
      </tr>
       <tr>
        <td>IsIntersection</td>
        <td>두 집합에 공통 요소가 있는지 확인</td>
        <td>동일한 타입(string, numeric, bool 또는 datetime)의 요소를 가진 2개의 배열</td>
        <td>bool</td>
        <td>IsIntersection(a, ('s1', 's2', 's3'))</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

##### 2.4.2 사용자 정의 함수

기본 제공 함수가 요구 사항을 충족하지 못하는 경우 사용자 정의 함수를 구현하여 Speedle에 등록할 수 있습니다. 자세한 내용은 [사용자 정의 함수](../custom-function/)를 참조하십시오.

#### 2.5 연산자 우선순위 및 결합성

다음 표는 문서화된 표현식 구문을 우선순위가 높은 순서대로 정리한 것입니다. 비교는 논리 AND/OR보다 먼저 결합됩니다. 복잡한 조건을 명확히 하려면 괄호를 사용하십시오.

<table class="bordered striped"><thead><tr><th>우선순위(높은 순)</th><th>연산자 / 비교 연산자</th><th>결합성</th></tr></thead><tbody><tr><td>1</td><td>( )</td><td>해당 없음</td></tr><tr><td>2</td><td>함수 호출</td><td>해당 없음</td></tr><tr><td>3</td><td>! 및 단항 -</td><td>오른쪽에서 왼쪽</td></tr><tr><td>4</td><td>* / %</td><td>왼쪽에서 오른쪽</td></tr><tr><td>5</td><td>+ -</td><td>왼쪽에서 오른쪽</td></tr><tr><td>6</td><td>== != =~ &gt; &gt;= &lt; &lt;= in</td><td>해당 없음</td></tr><tr><td>7</td><td>&amp;&amp;</td><td>왼쪽에서 오른쪽</td></tr><tr><td>8</td><td>||</td><td>왼쪽에서 오른쪽</td></tr></tbody></table>

#### 2.6 조건 예제

<table class="bordered striped">
    <thead>
      <tr>
        <th>조건</th>
        <th>설명</th>
      </tr>
    </thead>
    <tbody>
    <tr><td>a=='abc'</td><td>속성 'a'의 값이 'abc'와 같음</td></tr>
    <tr><td>a!='abc'</td><td></td></tr>
    <tr><td>a>='abc'</td><td></td></tr>
    <tr><td>a+b=='ab'</td><td></td></tr>
    <tr><td>a=~'^get.*'</td><td>속성 'a'의 값이 정규식 '^get.*'과 일치함</td></tr>
<tr><td>a==123</td><td></td></tr>
<tr><td>a-b>123</td><td></td></tr>
<tr><td>a in (1, 2, 3) </td><td>속성 'a'의 값이 1, 2, 3 중 하나임</td></tr>
<tr><td>'manager' in a </td><td>'a'는 문자열 배열 속성이며, 'manager'는 해당 배열 요소 중 하나임</td></tr>
<tr><td>IsSubSet(e, ('s1', 's2', 's3'))</td><td>'e'는 문자열 배열/집합 속성이며, e는 ('s1', 's2', 's3')의 부분 집합임</td></tr>
<tr><td>a in (1, 2, 3) && (b==c &#124;&#124; d==3) && IsSubSet(e, ('s1', 's2', 's3'))</td><td></td></tr>
<tr><td>request_year==2019 && request_month==12 </td><td>request_year와 request_month는 기본 제공 속성입니다. 리소스에 접근한 연도가 2019이고 접근한 월이 12여야 함을 요구합니다</td></tr>
 </tbody>
    <tfoot>
    </tfoot>
  </table>

### 조건 구문 정의

조건은 유효한 부울 식이어야 합니다. 다음은 일반적인 표현식 요약입니다. 정확한 입력은 현재 파서로 검증하십시오:

```
BoolExpr: ('!')BoolExpr
          | BoolExpr ('&&'|'||') BoolExpr
		  | BoolConstant
		  | Attribute
		  | Function
		  | RelationalExpr
		  | '(' BoolExpr ')'


RelationalExpr: NumericExpr ('=='|'!='|'>'|'>='|'<'|'<=') NumericExpr
              | StringExpr ('=='|'!='|'=~'|'>'|'>='|'<'|'<=') StringExpr
			  | BoolExpr ('=='|'!=') BoolExpr
			  | DateTimeExpr ('=='|'!='|'>'|'>='|'<'|'<=') DateTimeExpr
			  | (NumericExpr|StringExpr|BoolExpr|DateTimeExpr) ('in') ArrayExpr


NumericExpr: NumericExpr('+'|'-'|'*'|'/'|'%')NumericExpr
           | NumericConstant
		   | Attribute
		   | Function
		   | '(' NumericExpr ')'

StringExpr: StringExpr('+') StringExpr
           | StringConstant
		   | Attribute
		   | Function
           | '(' StringExpr ')'

DateTimeExpr: DateTimeConstant
           | Attribute
           | Function

ArrayExpr: ArrayConstant
         | Attribute
         | Function

ArrayConstant: '('Constant [, Constant]* ')'

Constant: NumericConstant
         |StringConstant
         |BoolConstant
         |DateTimeConstant

NumericConstant: any numeric float64 data
StringConstant: single quoted string, for example, 'string1'
BoolConstant: true|false
DateTimeConstant: single quoted datetime, datetime should conform to the format defined by RFC3339: YYYY-MM-DDTHH:mm:SS[.sssssssss]Z, Z is [+|-]HH:mm. For example, '2016-01-02T15:04:05-07:00'

Attribute: attribute name should conform to  [a-zA-Z]+[a-zA-Z0-9_]*, prefer simple portable identifiers; REST attribute names are limited to 256 ASCII characters
Function: FunctionName '(' Argument [,Argument]* ')'

```

## 부록

### SPDL 전체 구문

<picture><source srcset="/img/speedle/spdl-syntax-full.webp" type="image/webp"><img src="/img/speedle/spdl-syntax-full.png" alt="전체 정책 정의 구문" width="1771" height="2281" loading="lazy"></picture>

<br/>
<br/>
