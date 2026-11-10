+++
title = "Speedle 概要: きめ細かなアクセス制御のための認可エンジン"
linktitle = "概要"
description = "Speedle が一般的な認可用語（PAP、PDP、PEP、PIP、RBAC、ABAC、PBAC、XACML、ゼロトラスト、ID、IAM）とどのように対応するか。"
weight = 2
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["core", "pdp", "identity", "spdl"]
categories = ["docs"]
bref = "認可とアクセス制御の語彙における Speedle の位置づけ"
+++

Speedle はオープンソースの認可エンジンです。アプリケーションから「このユーザーはこのリソースに対してこのアクションを実行できますか？」という問い合わせを受け取り、一元管理されたポリシーに基づいて許可（Allow）または拒否（Deny）を返します。このページでは、一般的な認可用語とそれらを実装する Speedle コンポーネントを関連付け、それぞれのガイドへのリンクを示します。

以下の例では `library` という名前のサービスを使用しています。各ポリシーはポリシー管理サービス (PMS) の REST API を通じて作成され、各決定は認可決定サービス (ADS) の REST API から返されたものです。理由コードは [認可決定](../decisions/) に一覧表示されています。

## 認可エンジン

認可エンジンはポリシーに照らしてアクセスリクエストを評価し、決定を返します。これにより、認可ロジックがアプリケーションコードの外部に分離されます。Speedle は [ポリシー管理サービス (PMS)、ポリシーリポジトリ、および認可決定サービス (ADS)](../architecture/) で構成されています。ADS はスタンドアロンサービスとして実行することも、[Go アプリケーションに組み込む](../deployment/embedded/) こともできます。

## 認可とアクセス制御

認証（Authentication）は呼び出し元が誰であるかを確立し、認可（Authorization）はその呼び出し元が何を行ってよいかを決定します。アクセス制御（Access control）はそれらの決定の強制・執行です。Speedle は認可のみを処理します。すでに認証済みのサブジェクトを受け取り、決定を返し、呼び出し元のアプリケーションまたはゲートウェイがそれを適用します。[認可決定](../decisions/) を参照してください。

## きめ細かなアクセス制御 (FGA)

きめ細かなアクセス制御（Fine-grained access control）は、アプリケーション全体への大まかなアクセスを許可するのではなく、ユーザーごと、リソースごと、アクションごとにアクセスを決定し、必要に応じてリクエストコンテキストも考慮します。Speedle ポリシーでは、プリンシパル、アクション、リソースを指定し、条件を追加できます。ロールポリシーでは、特定のリソースに対してのみロールを割り当てることができます。[SPDL](../spdl/) を参照してください。

## PAP: ポリシー管理ポイント

PAP（Policy Administration Point）は、ポリシーが作成および管理される場所です。Speedle ではこれが PMS に該当し、[REST API](../api/management_api/)、gRPC API、または `spctl` コマンドラインツールを通じて使用されます。[ポリシー管理](../pms/policy-mgmt/) を参照してください。

## PDP: ポリシー決定ポイント

PDP（Policy Decision Point）は、リクエストをポリシーに照らして評価し、決定を返します。Speedle ではこれが ADS、または Speedle が Go プロセス内で実行される際の組み込み評価器に該当します。決定リクエストは以下のようになります:

```bash
curl --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"library","resource":"book","action":"update"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

以下の RBAC ポリシーが設定されている場合、レスポンスは `{"allowed":true,"reason":0}` となります。[決定 API](../api/decision_api/) を参照してください。

## PEP: ポリシー施行ポイント

PEP（Policy Enforcement Point）はアクセス試行をインターセプトし、PDP に決定を問い合わせてそれを強制・執行します。Speedle は汎用の PEP を同梱していません。PEP は、REST または gRPC API を通じて ADS を呼び出す、あるいは Go に評価器を組み込むアプリケーションまたはゲートウェイ自身です。サポートされているインターフェースについては [連携](../../integrations/) を参照してください。

## PIP: ポリシー情報ポイント

PIP（Policy Information Point）は、ポリシーの評価に必要な追加情報を提供します。Speedle では、この情報はリクエスト属性や [組み込み属性](../spdl/#2-2-1-built-in-attributes)、ID トークンをサブジェクトに変換する [トークンアサーター](../assertor/)、評価中に外部 REST エンドポイントを呼び出す [カスタム関数](../custom-function/) から提供されます。

## XACML

XACML（eXtensible Access Control Markup Language）は、ポリシー言語と PAP/PDP/PEP/PIP アーキテクチャの両方を定義する OASIS 標準です。Speedle は同じ役割分担に従っていますが、そのポリシー言語は XACML ではなく SPDL であり、XACML ポリシーを直接インポートすることはできません。

## RBAC: ロールベースのアクセス制御

RBAC では、権限がロールに付与され、ロールがユーザーまたはグループに割り当てられます。Speedle はポリシーを使用してロールに権限を付与し、ロールポリシーを使用してロールを割り当てます:

```text
grant role librarian read,update book
grant user alice librarian
```

`alice` は `book` を更新できます（理由 0: 認可ポリシーが存在）。ロールを持たない `bob` は拒否されます（理由 3: 適用可能なポリシーなし）。ロールは特定のリソースに対してのみ割り当てることも可能です。[ポリシー管理](../pms/policy-mgmt/) および [グローバルポリシー](../pms/global-policy/) を参照してください。

## ABAC: 属性ベースのアクセス制御

ABAC では、サブジェクト、リソース、アクション、または環境の属性に応じて決定が行われます。Speedle では、任意のポリシーまたはロールポリシーに属性に対する `if` 条件を付加できます:

```text
grant user bob read report if department == 'finance'
```

文字列属性 `department` が `finance` に設定された `bob` からのリクエストは許可されます（理由 0）。`sales` である場合、または属性がない場合は拒否されます（理由 3）。[条件式](../spdl/#condition) を参照してください。

## PBAC: ポリシーベースのアクセス制御

PBAC は、各アプリケーションに埋め込まれた権限ではなく、一元的に定義されたポリシーを通じてアクセスを管理します。すべての Speedle ポリシーは PMS に保持され、2つの固定ルールで評価されます: 拒否は許可をオーバーライドし、適用可能な認可がないリクエストは拒否されます。

```text
grant group staff read,delete journal
deny user carol delete journal
```

`staff` のメンバーである `carol` は `journal` に対する `delete` が拒否されます（理由 1: 拒否ポリシーが存在）。同じく `staff` に属する `erin` は許可されます（理由 0）。

## ID と IAM

アイデンティティおよびアクセス管理（IAM）は、ユーザーが誰であるかを証明することと、ユーザーが何を行ってよいかを決定することの両方を網羅します。Speedle は ID プロバイダーではなく、ユーザーの認証は行いません。Speedle は ID を消費します: プリンシパルは [ID ドメイン](../idd/) によって修飾でき、ID トークンは [トークンアサーター](../assertor/) によって解決できます。

```text
grant user dave from github read code
```

`github` ID ドメインの `dave` は `code` の読み取りが許可されます（理由 0）。`corp` の `dave`、または ID ドメインのない `dave` は拒否されます（理由 3）。

## ゼロトラスト

ゼロトラストはネットワークの場所に基づいてアクセスを許可せず、すべてのリクエストを検証します。Speedle は決定サービスとしてこれをサポートします: ADS は受信した各リクエストを評価し、明示的に許可されていないものはすべて拒否されます。認証、デバイストラスト、ネットワーク制御は Speedle のスコープ外です。

## SPDL: Speedle Policy Definition Language

SPDL は Speedle Policy Definition Language（Speedle ポリシー定義言語）の略です。これは Speedle ポリシーおよびロールポリシーを記述するために使用される言語です（例: `grant role librarian read,update book`）。[SPDL リファレンス](../spdl/) を参照してください。

## 連携

他のアプリケーションは、[REST または gRPC API](../api/) を介して ADS を呼び出すか、Go に評価器を組み込みます。[デプロイ](../deployment/) を参照してください。[連携](../../integrations/) ページには、現在提供されている API、SDK、組み込みモード、およびポリシーストアが一覧表示されています。
