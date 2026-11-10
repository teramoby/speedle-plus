+++
title = "SPDL: Speedle ポリシー定義言語"
description = "SPDL は Speedle Policy Definition Language の略であり、Speedle ポリシーおよびロールポリシーを記述するための言語です。"
weight = 5
draft = false
toc = true
singlelinetitle = true
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "core", "policy", "spdl"]
categories = ["docs"]
bref = "SPDL の基礎"
+++

SPDL は Speedle ポリシーおよびロールポリシーを記述するための言語です。このページでは構文、命名規則、およびサンプルを説明します。
<!--more-->

<aside class="docs-callout" aria-labelledby="spdl-callout-title">
  <p class="docs-callout-eyebrow">Speedle ポリシーモデラー · オープンソーススキル</p>
  <p class="docs-callout-title" id="spdl-callout-title">AI で SPDL を作成</p>
  <p>オープンソースの Speedle ポリシーモデラースキルは、要件やポリシー文書を許可・拒否のテストケースを含む SPDL モデルに変換し、<code>spctl check</code> で検証します。</p>
  <p class="docs-callout-actions"><a class="button primary" href="https://ai.speedle.io/ja/#policy-modeler">ポリシーモデラースキルを試す</a> <a class="button secondary" href="https://github.com/teramoby/ai.speedle.io/tree/main/skills/speedle-policy-modeler">GitHub でソースを見る</a></p>
</aside>

## 構文

<picture><source srcset="/img/speedle/spdl-syntax.webp" type="image/webp"><img src="/img/speedle/spdl-syntax.png" alt="基本ポリシー定義構文" width="2222" height="971" fetchpriority="high"></picture>

### キーワード

`grant`、`deny`、`user`、`group`、`entity`、`role`、`from`、`on`、`if`、`in` はそれぞれの構文上の位置で特別な意味を持ちます。ステートメントのキーワードは大文字と小文字を区別しません。条件内では `in` または `IN` を使用し、関数名は大文字と小文字を区別します。プリンシパル、アクション、およびリソース名も大文字と小文字を区別します。現在のパーサーは一部の名前の位置でキーワードに類似した名前を受け入れますが、一律の禁止を検証ルールとして前提にしないでください。レビューとクライアント間の相互運用性を高めるため、曖昧さのない名前を推奨します。

### 命名規則

シンプルで安定した識別子を推奨します。空白や区切り文字を含む名前は、パーサーがサポートする単一引用符または二重引用符で囲み、実際の入力でテストしてください。過去の Java 正規表現の文字クラスは、現在の Go パーサーの検証コントラクトではありません。リソースプレフィックス `expr:` はシェルの glob ではなく Go の正規表現を使用します。条件内の属性および関数名については後述します。

### 構文定義

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

## サンプル

ロール継承: ロールポリシーのサブジェクトは別のロールにすることができるため、`manager` は `employee` に割り当てられたすべてのロールも保持します。

```text
grant role manager employee
```

拒否の優先: 該当する拒否（deny）は許可（grant）よりも優先されます。以下の例では、`carol` は `staff` に属していますが、`journal` に対する `delete` は拒否されます。

```text
grant group staff read,delete journal
deny user carol delete journal
```

条件: 条件が真の場合にのみポリシーが適用されます。

```text
grant user bob read report if department == 'finance'
```

これらは SPDL ステートメントの例であり、完全なサービス設定ではありません。サービス、リソーススコープのロール、および完全な決定例については [ポリシー管理](../pms/policy-mgmt/) を参照してください。

<a id="condition"></a>
## 条件（Condition）

### 1. 概要

認可ポリシーおよびロールポリシーの両方で条件がサポートされています。条件が満たされた場合に*のみ*、ポリシーまたはロールポリシーが有効になります。

ポリシー適用ポイント (PEP) は、ポリシーが参照するすべてのカスタム属性を提供する必要があります。`request_user` や `request_time` などの組み込み属性は ADS によって提供されます。PEP から提供される属性は、エンドユーザーから直接取得するのではなく、呼び出し元の信頼できるアプリケーションまたはサービスから提供される必要があり、PEP は送信する値に責任を持ちます。PEP から提供される属性は、同じ名前の組み込み属性よりも優先されます。特に、信頼できないエンドユーザーの入力を `request_user`、`request_time`、またはその他の組み込み属性名として転送しないでください。

リクエストに存在しない属性を参照する条件は一致しません。したがって、条件付きの `deny` は、その属性が存在しない場合には適用されません。拒否条件が使用する属性を PEP が常に提供していることを確認し、それらのリクエストをテストしてください。

このドキュメントでは、条件とは何か、および条件の記述方法に焦点を当てています。

### 2. 条件の構成

条件は、属性、関数、定数、演算子、比較演算子、または括弧を使用して構築され、ブール値を生成するブール式です。

#### 2.1 データ型

属性値および定数のデータ型は、string、numeric、bool、datetime、またはこれらの型の配列です。  
サポートされているデータ型と対応する演算子および比較演算子を以下の表に示します。

 <table class="bordered striped">
    <thead>
      <tr>
        <th>データ型</th>
        <th>演算子</th>
        <th>比較演算子</th>
        <th>説明</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>string</td>
        <td>+</td>
        <td>== <br> != <br> =~ <br> > <br> >= <br> < <br> <= <br>
        </td>
        <td><code>+</code> は文字列の連結に使用されます。<br><code>=~</code> は正規表現マッチング用です。表の下の注記を参照してください。</td>
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
        <td>所属を表す <code>in</code> 演算子: 左辺は単一の型（string、numeric、bool、datetime）、右辺は配列である必要があります</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

`"=~"` についての注記:  
`"=~"` は正規表現比較演算子です。Go 言語の標準 `regexp` 構文を使用します。左辺は候補文字列、右辺は正規表現パターンです。候補文字列が右辺で指定された正規表現パターンに一致するかどうかをブール値で返します。

- 左辺: string
- 右辺: string
- 戻り値: bool

#### 2.2 属性

条件内の属性は変数を表します。属性値はランタイムで決定され、認可決定サービス (ADS) のランタイムまたは PEP のいずれかから取得されます。  
組み込み属性は、ADS ランタイムによって自動的に設定される値です。  
カスタム属性は、認可決定を要求する際に、エンドユーザーからではなく呼び出し元の信頼できるアプリケーションまたはサービス（PEP）によって提供される値です。PEP は送信する値に責任を持ちます。PEP から提供される属性は同名の組み込み属性よりも優先されるため、PEP は信頼できないエンドユーザーの入力を組み込み属性名として転送してはなりません。

<a id="2-2-1-built-in-attributes"></a>
##### 2.2.1 組み込み属性

組み込み属性は以下のとおりです:

<table class="bordered striped">
    <thead>
      <tr>
        <th>組み込み属性名</th>
        <th>データ型</th>
        <th>サンプル値</th>
        <th>定義</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>request_user</td>
        <td>string</td>
        <td>"Alice"</td>
        <td>リソースに対する操作を要求しているサブジェクト内のユーザー情報</td>
      </tr>
      <tr>
        <td>request_groups</td>
        <td>[]string</td>
        <td>[]string{"managers"}</td>
        <td>リソースに対する操作を要求しているサブジェクト内のグループ情報</td>
      </tr>
      <tr>
        <td>request_entity</td>
        <td>string</td>
        <td>"/org1/service1"</td>
        <td>サービスまたはプログラムがリソースに対する操作を要求しているサブジェクト内のサービス名またはその他のエンティティ情報</td>
      </tr>
      <tr>
        <td>request_resource</td>
        <td>string</td>
        <td>"commercialLoans"</td>
        <td>サブジェクトが操作対象とするリソース</td>
      </tr>
      <tr>
        <td>request_action</td>
        <td>string</td>
        <td>"issue" </td>
        <td>サブジェクトがリソースに対して実行するアクション</td>
      </tr>
      <tr>
        <td>request_time</td>
        <td>datetime</td>
        <td>'2019-01-02T15:04:05-07:00'</td>
        <td>リクエストが発生した日時</td>
      </tr>
      <tr>
        <td>request_year</td>
        <td>int</td>
        <td>2019</td>
        <td>リクエストが発生した年</td>
      </tr>
      <tr>
        <td>request_month</td>
        <td>int</td>
        <td>1, 2, ... 12</td>
        <td>リクエストが発生した月</td>
      </tr>
      <tr>
        <td>request_day</td>
        <td>int</td>
        <td>1, 2, ... 31</td>
        <td>リクエストが発生した月の日</td>
      </tr>
      <tr>
        <td>request_hour</td>
        <td>int</td>
        <td>0, 1, ... 23</td>
        <td>リクエストが発生した時刻（時）</td>
      </tr>
      <tr>
        <td>request_weekday</td>
        <td>string</td>
        <td>"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"</td>
        <td>リクエストが発生した曜日</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

##### 2.2.2 カスタム属性

###### 2.2.2.1 属性名

移植性の高い条件参照のため、`[A-Za-z][A-Za-z0-9_]*` に適合する名前を推奨し、組み込みの属性名や関数名との重複を避けてください。REST レイヤーは最大 256 文字の ASCII 文字、数字、アンダースコア、ハイフン、およびドットを受け入れます。ハイフンやドットを含む式参照は、演算子として誤認されないよう実際にテストしてください。

###### 2.2.2.2 属性値

カスタム属性が条件で使用されている場合、クライアントは isAllowed 結果をリクエストする際にカスタム属性値を渡す必要があります。リクエストに条件が参照する属性が含まれていない場合、条件は一致しません。その場合、条件付きの `deny` は適用されません。拒否条件が使用するすべての属性を PEP が提供していることを確認し、それらのリクエストをテストしてください。

- Go API での属性値の受け渡し  
  Go API にカスタム属性値を渡す際は、以下の規則に従ってください:

  - bool 属性値には Go の bool 型を使用します。
  - string 属性値には Go の string 型を使用します。
  - 数値属性値には Go の float64 型を使用します。
  - datetime 属性値には、その日時の Unix 時間を表す Go の float64 表現（`time.Time.Unix()` を使用）を使用します。
  - 配列属性値には Go の []interface{} を使用します。

- REST API での属性値の受け渡し  
  REST API にカスタム属性値を渡す際は、以下の規則に従ってください:
  - 属性は、属性の名前、型、値を含む構造体です。詳細については REST API を参照してください。
  - 属性の型は "string"、"numeric"、"bool"、または "datetime" のみ指定できます。
  - 属性値は単一の値またはスライスにすることができます。

#### 2.3 定数

サポートされているデータ型:

- string: 単一引用符、'foobar'
- numeric: 10、3.1415926
- bool: true または false
- datetime: 単一引用符、RFC3339 に準拠。RFC3339 形式の日時は YYYY-MM-DDTHH:mm:SS[.sssssssss]Z（Z は [+|-]HH:mm）です。
- array: string、numeric、bool、datetime 型の配列

以下の表は定数のサンプルを示しています。

<table class="bordered striped">
    <thead>
      <tr>
        <th>データ型</th>
        <th>定数の例</th>
        <th>定数配列の例</th>
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

#### 2.4 関数

既存の演算子や比較演算子では要件を満たせない場合に関数を使用します。

##### 2.4.1 組み込み関数

Speedle は以下の組み込み関数を提供します。

<table class="bordered striped">
    <thead>
      <tr>
        <th>組み込み関数名</th>
        <th>機能</th>
        <th>入力とデータ型</th>
        <th>出力とデータ型</th>
        <th>使用例</th>
      </tr>
    </thead>
    <tbody>
       <tr>
        <td>Sqrt</td>
        <td>数値の平方根を計算</td>
        <td>1 つの数値パラメータ</td>
        <td>numeric</td>
        <td>Sqrt(x)<br>Sqrt(64)</td>
      </tr>
       <tr>
        <td>Max</td>
        <td>集合内の最大値を取得</td>
        <td>1 つ以上の数値パラメータ</td>
        <td>numeric</td>
        <td>Max(1, 4, x)</td>
      </tr>
       <tr>
        <td>Min</td>
        <td>集合内の最小値を取得</td>
        <td>1 つ以上の数値パラメータ</td>
        <td>numeric</td>
        <td>Min(x, 5, z)</td>
      </tr>
       <tr>
        <td>Sum</td>
        <td>数値の集合の合計を取得</td>
        <td>1 つ以上の数値パラメータ</td>
        <td>numeric</td>
        <td>Sum(1, 3, 5, 7, x)</td>
      </tr>
      <tr>
        <td>Avg</td>
        <td>数値の集合の平均値を取得</td>
        <td>1 つ以上の数値パラメータ</td>
        <td>numeric</td>
        <td>Avg(x, 8, 10)</td>
      </tr>
      <tr>
        <td>IsSubSet</td>
        <td>最初の集合/配列が 2 番目の集合/配列の部分集合であるか確認</td>
        <td>同じデータ型の要素を持つ 2 つの集合/配列</td>
        <td>bool</td>
        <td>IsSubSet(s1, s2)</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

数値組み込み関数は、非有限の入力/結果および負の平方根に対して `BuiltInFuncError` を返します。関数の失敗は、他のランタイム条件エラーと同様に条件を不一致にします。`Sum()` および `Avg()` は、空の入力に対してゼロを返す従来の動作を保持しています。`IsSubSet` は重複要素を無視し、サポートされていない要素型や混在した要素型を拒否し、空の集合に対して false を返す従来の動作を保持しています。SPDL で引用符で囲まれた名前はリテラルのバックスラッシュを保持します。名前に引用符が含まれる場合は、別の種類の引用符を使用してください。ステートメントは完全に消費される必要があり、末尾の余計なテキスト、不完全な条件、および閉じられていない名前の引用符は拒否されます。

##### 2.4.2 カスタム関数

REST API を通じて独自の関数を公開し、条件式でカスタム関数を使用することもできます。  
詳細については [カスタム関数](../custom-function/) を参照してください。

#### 2.5 演算子および比較演算子の優先順位

##### 2.5.1 優先順位

2 つの演算子がオペランドを共有する場合、優先順位の高い演算子が先に評価されます。たとえば、乗算は加算よりも優先順位が高いため、`1 + 2 * 3` は `1 + (2 * 3)` として扱われ、`1 * 2 + 3` は `(1 * 2) + 3` として扱われます。

##### 2.5.2 結合性

式に同じ優先順位の演算子が 2 つある場合、式はその結合性に従って評価されます。`/` 演算子は左から右への結合性を持つため、`72 / 2 / 3` は `(72 / 2) / 3` として扱われます。一部の演算子は結合性を持ちません。たとえば、式 `(x <= y <= z)` や `x++--` は無効です。

##### 2.5.3 サポートされている演算子および比較演算子の優先順位と結合性

以下の表は、ドキュメント化された式の構文を優先順位の高い順にまとめたものです。比較は論理 AND/OR よりも前に結合します。複雑な条件を明確にするには括弧を使用してください。

<table class="bordered striped"><thead><tr><th>優先順位（高い順）</th><th>演算子 / 比較演算子</th><th>結合性</th></tr></thead><tbody><tr><td>1</td><td>( )</td><td>該当なし</td></tr><tr><td>2</td><td>関数呼び出し</td><td>該当なし</td></tr><tr><td>3</td><td>! および単項 -</td><td>右から左</td></tr><tr><td>4</td><td>* / %</td><td>左から右</td></tr><tr><td>5</td><td>+ -</td><td>左から右</td></tr><tr><td>6</td><td>== != =~ &gt; &gt;= &lt; &lt;= in</td><td>該当なし</td></tr><tr><td>7</td><td>&amp;&amp;</td><td>左から右</td></tr><tr><td>8</td><td>||</td><td>左から右</td></tr></tbody></table>

#### 2.6 条件のサンプル

<table class="bordered striped">
    <thead>
      <tr>
        <th>条件</th>
        <th>説明</th>
      </tr>
    </thead>
    <tbody>
    <tr><td>a=='abc'</td><td>属性 'a' の値が 'abc' に等しい</td></tr>
    <tr><td>a!='abc'</td><td></td></tr>
    <tr><td>a>='abc'</td><td></td></tr>
    <tr><td>a+b=='ab'</td><td></td></tr>
    <tr><td>a=~'^get.*'</td><td>属性 'a' の値が正規表現 '^get.*' に一致する</td></tr>
<tr><td>a==123</td><td></td></tr>
<tr><td>a-b>123</td><td></td></tr>
<tr><td>a in (1, 2, 3) </td><td>属性 'a' の値が 1、2、3 のいずれかである</td></tr>
<tr><td>'manager' in a </td><td>'a' は文字列配列属性であり、'manager' はその配列要素の 1 つである</td></tr>
<tr><td>IsSubSet(e, ('s1', 's2', 's3'))</td><td>'e' は文字列配列/集合の属性であり、e は配列/集合 ('s1', 's2', 's3') の部分集合である</td></tr>
<tr><td>a in (1, 2, 3) && (b==c &#124;&#124; d==3) && IsSubSet(e, ('s1', 's2', 's3'))</td><td></td></tr>
<tr><td>request_year==2019 && request_month==12 </td><td>request_year と request_month は組み込み属性です。リソースにアクセスした年が 2019 であり、アクセスした月が 12 であることを要求します</td></tr>
 </tbody>
    <tfoot>
    </tfoot>
  </table>

### 条件の文法定義

条件は有効なブール式である必要があります。以下は一般的な式のまとめです。正確な入力については現在のパーサーで検証してください:

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

## 付録

### SPDL 完全構文

<picture><source srcset="/img/speedle/spdl-syntax-full.webp" type="image/webp"><img src="/img/speedle/spdl-syntax-full.png" alt="完全なポリシー定義構文" width="1771" height="2281" loading="lazy"></picture>

<br/>
<br/>
