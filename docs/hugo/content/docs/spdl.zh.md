+++
title = "SPDL：Speedle 策略定义语言"
description = "SPDL 的全称是 Speedle Policy Definition Language（Speedle 策略定义语言），用于编写 Speedle 策略和角色策略。"
weight = 5
draft = false
toc = true
singlelinetitle = true
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "core", "policy", "spdl"]
categories = ["docs"]
bref = ""
+++

## 基本的策略定义语言

<picture><source srcset="/img/speedle/spdl-syntax.webp" type="image/webp"><img src="/img/speedle/spdl-syntax.png" alt="基本策略定义语法" width="2222" height="971" fetchpriority="high"></picture>

### 保留关键字

`grant`、`deny`、`user`、`group`、`entity`、`role`、`from`、`on`、`if`、`in` 在各自语法位置有特定含义。策略语句的关键字不区分大小写；条件中的成员运算使用 `in` 或 `IN`，函数名则区分大小写。主体、操作和资源名称也区分大小写。当前解析器在部分名称位置接受类似关键字的名称，不要据此认定所有位置都禁止关键字。为便于审查和客户端互通，建议避免使用这类名称。

### 命名规范

推荐使用简短、稳定的标识符。名称包含空格或分隔符时，使用解析器支持的单引号或双引号，并用实际输入测试。不要把历史 Java 正则字符类当作当前 Go 解析器的严格校验规则。资源前缀 `expr:` 表示 Go 正则表达式，不是 shell 通配符。条件中的属性和函数名称见下文。

### 语法

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

## 示例

角色继承：角色策略的主体可以是另一个角色，因此 `manager` 同时拥有授予 `employee` 的所有角色。

```text
grant role manager employee
```

拒绝优先于授权：适用的拒绝策略会覆盖授权策略。下例中 `carol` 属于 `staff`，但对 `journal` 的 `delete` 仍被拒绝。

```text
grant group staff read,delete journal
deny user carol delete journal
```

条件：只有条件成立时策略才生效。

```text
grant user bob read report if department == 'finance'
```

以上是 SPDL 语句示例，不是完整的服务配置；服务、资源范围角色和完整的决策示例见[策略管理](../pms/policy-mgmt/)。

## 条件（Condition）

### 1. 概述

授权策略和角色策略都支持条件。只有条件成立，策略才会生效。

PEP 必须传入策略用到的所有自定义属性（`request_user`、`request_time` 等内置属性由 ADS 自动提供）；请求里缺少的属性，相关条件不匹配。PEP 传入的同名属性优先于内置属性，PEP 负责提供可信的属性值。

本节介绍条件的组成和写法。

<a id="2-condition"></a>
### 2. 条件的组成

条件是由属性、函数、常量、运算符、比较符和括号组成的布尔表达式。

#### 2.1 数据类型 （Data Types）

属性和常量可以是 string、numeric、bool、datetime，也可以是由这些类型组成的数组。  
各类数据支持的运算符和比较符如下：

 <table class="bordered striped">
    <thead>
      <tr>
        <th>数据类型</th>
        <th>运算符</th>
        <th>比较符</th>
        <th>说明</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>string</td>
        <td>+</td>
        <td>== <br> != <br> =~ <br> > <br> >= <br> < <br> <= <br>
        </td>
        <td><code>+</code> 用于连接字符串。<br><br><code>=~</code> 用于正则表达式匹配：左边是待匹配字符串，右边是正则表达式；匹配成功返回 true，否则返回 false。</td>
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
        <td>成员运算符 <code>in</code> 的左边是单个 string、numeric、bool 或 datetime 值，右边是数组。</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

#### 2.2 属性(Attributes)

属性相当于条件中的变量，分为内置属性和用户属性。
内置属性由 Speedle 预定义，授权决策服务（ADS）在评估时为其赋值。
用户属性的值由调用方随授权请求传入。

##### 2.2.1 内置属性(Built-in Attributes)

Speedle 预定义了以下内置属性：

<table class="bordered striped">
    <thead>
      <tr>
        <th>内置属性名</th>
        <th>数据类型</th>
        <th>示例值</th>
        <th>含义</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>request_user</td>
        <td>string</td>
        <td>"Alice"</td>
        <td>授权请求主体（Subject）中的用户信息</td>
      </tr>
      <tr>
        <td>request_groups</td>
        <td>[]string</td>
        <td>[]string{"managers"}</td>
        <td>授权请求主体（Subject）中的用户组信息</td>
      </tr>
      <tr>
        <td>request_entity</td>
        <td>string</td>
        <td>"/org1/service1"</td>
        <td>授权请求主体（Subject）中的实体信息</td>
      </tr>
      <tr>
        <td>request_resource</td>
        <td>string</td>
        <td>"commercialLoans"</td>
        <td>授权请求中的资源</td>
      </tr>
      <tr>
        <td>request_action</td>
        <td>string</td>
        <td>"issue" </td>
        <td>授权请求中对资源执行的操作</td>
      </tr>
      <tr>
        <td>request_time</td>
        <td>datetime</td>
        <td>'2019-01-02T15:04:05-07:00'</td>
        <td>发起授权请求的日期和时间</td>
      </tr>
      <tr>
        <td>request_year</td>
        <td>int</td>
        <td>2019</td>
        <td>发起授权请求的年份</td>
      </tr>
      <tr>
        <td>request_month</td>
        <td>int</td>
        <td>1, 2, ... 12</td>
        <td>发起授权请求的月份</td>
      </tr>
      <tr>
        <td>request_day</td>
        <td>int</td>
        <td>1, 2, ... 31</td>
        <td>发起授权请求的日期是当月第几天</td>
      </tr>
      <tr>
        <td>request_hour</td>
        <td>int</td>
        <td>0, 1, ... 23</td>
        <td>发起授权请求的小时</td>
      </tr>
      <tr>
        <td>request_weekday</td>
        <td>string</td>
        <td>"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"</td>
        <td>发起授权请求时是星期几</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

<a id="2-2-2-用户属性-customer-attributes"></a>
##### 2.2.2 用户属性(Custom Attributes)

###### 2.2.2.1 属性名(name)

为便于在条件中直接引用，建议属性名符合 `[A-Za-z][A-Za-z0-9_]*`，并避免与内置属性或函数重名。REST 请求层实际接受最长 256 个字符的 ASCII 字母、数字、下划线、连字符和点。名称包含连字符或点时，应测试表达式中的引用方式，不要把这些字符误当成运算符。

###### 2.2.2.2 属性值(value)

条件引用用户属性时，调用方必须把属性值随授权请求一并发给 ADS。

- 通过 Go API 传入属性值  
  遵循以下规则：

  - bool 属性使用 Go bool 类型。
  - string 属性使用 Go string 类型。
  - numeric 属性使用 Go float64 类型。
  - datetime 属性使用 Go float64 类型，即 Unix 时间（使用 `time.Time.Unix()`）。
  - 数组属性使用 Go []interface{} 类型。

- 通过 REST API 传入属性值  
  遵循以下规则：
  - 每个属性都是一个结构体（struct），包含属性名（name）、数据类型（type）和属性值（value）。详见 REST API。
  - 数据类型（type）只能是 "string"、"numeric"、"bool" 或 "datetime"。
  - 属性值可以是单个值，也可以是数组。

#### 2.3 常量(Constants)

支持以下数据类型：

- string：使用单引号，如 'foobar'
- numeric：如 10、3.1415926
- bool：true 或 false
- datetime：使用单引号，遵循 RFC3339。格式为 YYYY-MM-DDTHH:mm:SS[.sssssssss]Z，其中 Z 为 [+|-]HH:mm
- array：由 string、numeric、bool 或 datetime 组成的数组

各类常量示例如下：

<table class="bordered striped">
    <thead>
      <tr>
        <th>数据类型</th>
        <th>常量示例</th>
        <th>常量数组示例</th>
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

#### 2.4 函数(Functions)

简单的运算或比较无法满足需求时，可以调用函数。函数分为内置函数和用户自定义函数。

##### 2.4.1 内置函数(Built-in Functions)

Speedle 提供以下内置函数：

<table class="bordered striped">
    <thead>
      <tr>
        <th>内置函数</th>
        <th>作用</th>
        <th>输入及类型</th>
        <th>输出及类型</th>
        <th>示例</th>
      </tr>
    </thead>
    <tbody>
       <tr>
        <td>Sqrt</td>
        <td>求平方根</td>
        <td>一个数值参数</td>
        <td>numeric</td>
        <td>Sqrt(x)<br>Sqrt(64)</td>
      </tr>
       <tr>
        <td>Max</td>
        <td>取最大值</td>
        <td>至少一个数值参数</td>
        <td>numeric</td>
        <td>Max(1, 4, x)</td>
      </tr>
       <tr>
        <td>Min</td>
        <td>取最小值</td>
        <td>至少一个数值参数</td>
        <td>numeric</td>
        <td>Min(x, 5, z)</td>
      </tr>
       <tr>
        <td>Sum</td>
        <td>求和</td>
        <td>至少一个数值参数</td>
        <td>numeric</td>
        <td>Sum(1, 3, 5, 7, x)</td>
      </tr>
      <tr>
        <td>Avg</td>
        <td>求平均值</td>
        <td>至少一个数值参数</td>
        <td>numeric</td>
        <td>Avg(x, 8, 10)</td>
      </tr>
      <tr>
        <td>IsSubSet</td>
        <td>判断第一个参数是否为第二个参数的子集</td>
        <td>两个集合或数组，元素类型相同</td>
        <td>bool</td>
        <td>IsSubSet(s1, s2)</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

数值内置函数遇到非有限数值、非有限结果或负数平方根时返回 `BuiltInFuncError`。与其他运行时条件错误一样，函数失败时条件不匹配。`Sum()` 和 `Avg()` 保留历史行为：无参数时返回零。`IsSubSet` 忽略重复元素，拒绝不支持或混合的元素类型，并保留空集合返回 false 的历史行为。SPDL 名称引号中的反斜杠按字面处理；名称包含引号时，换用另一种引号包围。语句必须完整解析，解析器会拒绝尾随文本、不完整条件和未闭合的名称引号。

<a id="2-4-2-用户自定义函数-custom-functions"></a>
##### 2.4.2 用户自定义函数(Custom Functions)

可以向 Speedle 注册自己的函数，并在条件中调用。  
详见[自定义函数](../custom-function/)。

<a id="2-5-运算比较操作符的优先级-operator-comparator-precedence"></a>
#### 2.5 运算符与比较符的优先级(Operator/Comparator Precedence)

##### 2.5.1 优先顺序(Precedence order)

两个运算符共用一个操作数时，先计算优先级较高的运算符。例如，`1 + 2 * 3` 按 `1 + (2 * 3)` 计算，而 `1 * 2 + 3` 按 `(1 * 2) + 3` 计算，因为乘法优先于加法。

<a id="2-5-2-关联-associativity"></a>
##### 2.5.2 结合性(Associativity)

两个运算符优先级相同时，按结合性决定计算顺序。`72/2/3` 按 `(72/2)/3` 计算，因为 `/` 从左向右结合。有些运算符不能连续使用，例如 `(x <= y <= z)` 和 `x ++ -` 都不是有效表达式。

<a id="2-5-3-precedence-and-associativity-of-supported-operators-and-comparators"></a>
##### 2.5.3 运算符与比较符的优先级和结合性

下表列出本文语法的优先级。比较运算先于逻辑与、逻辑或；复杂条件建议加上括号。

<table class="bordered striped"><thead><tr><th>优先级（由高到低）</th><th>运算符/比较符</th><th>结合性</th></tr></thead><tbody><tr><td>1</td><td>( )</td><td>不适用</td></tr><tr><td>2</td><td>函数调用</td><td>不适用</td></tr><tr><td>3</td><td>! 和一元 -</td><td>从右到左</td></tr><tr><td>4</td><td>* / %</td><td>从左到右</td></tr><tr><td>5</td><td>+ -</td><td>从左到右</td></tr><tr><td>6</td><td>== != =~ &gt; &gt;= &lt; &lt;= in</td><td>不适用</td></tr><tr><td>7</td><td>&amp;&amp;</td><td>从左到右</td></tr><tr><td>8</td><td>||</td><td>从左到右</td></tr></tbody></table>

<a id="2-6-condition-示例"></a>
#### 2.6 条件示例

<table class="bordered striped">
    <thead>
      <tr>
        <th>条件</th>
        <th>说明</th>
      </tr>
    </thead>
    <tbody>
    <tr><td>a=='abc'</td><td>属性 'a' 的值等于 'abc'</td></tr>
    <tr><td>a!='abc'</td><td></td></tr>
    <tr><td>a>='abc'</td><td></td></tr>
    <tr><td>a+b=='ab'</td><td></td></tr>
    <tr><td>a=~'^get.*'</td><td>属性 'a' 的值匹配正则表达式 '^get.*'</td></tr>
<tr><td>a==123</td><td></td></tr>
<tr><td>a-b>123</td><td></td></tr>
<tr><td>a in (1, 2, 3) </td><td>属性 'a' 的值是 1、2、3 之一</td></tr>
<tr><td>'manager' in a </td><td>'a' 是字符串数组属性，'manager' 是其中一个元素</td></tr>
<tr><td>IsSubSet(e, ('s1', 's2', 's3'))</td><td>'e' 是字符串数组或集合属性，并且是 ('s1', 's2', 's3') 的子集</td></tr>
<tr><td>a in (1, 2, 3) && (b==c &#124;&#124; d==3) && IsSubSet(e, ('s1', 's2', 's3'))</td><td></td></tr>
<tr><td>request_year==2019 && request_month==12 </td><td>request_year 和 request_month 是内置属性。此例要求访问资源的年份为 2019，月份为 12</td></tr>
 </tbody>
    <tfoot>
    </tfoot>
  </table>

<a id="condition-定义"></a>
### 条件表达式语法

条件必须是合法的布尔表达式。下面是常用表达式的语法摘要，具体输入以当前解析器和实际测试为准：

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

## 附录

### 完整策略定义语法

<picture><source srcset="/img/speedle/spdl-syntax-full.webp" type="image/webp"><img src="/img/speedle/spdl-syntax-full.png" alt="完整策略定义语法" width="1771" height="2281" loading="lazy"></picture>

<br/>
<br/>
