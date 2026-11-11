+++
title = "パフォーマンステストガイド"
description = "AI コーディングツールを使って、自分のマシンで公平かつ再現可能な認可エンジンの比較を行います。"
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
bref = "自分のハードウェアとポリシーで Speedle と他の認可エンジンを測定する"
lastmod = 2026-11-11T12:00:00+08:00
+++

このガイドは、自分のマシンで Speedle と他の認可エンジンを比較するためのものです。パフォーマンスレポートは公開しません。数値はハードウェア、ポリシーセット、固定したエンジンのバージョンによって変わるため、信頼できる結果は自分で測定した結果だけです。

このガイドでは、共通シナリオ、4 つのエンジンの等価なポリシー、方法論のチェックリスト、そして Claude Code、ChatGPT Codex、DeepSeek Harness などの AI コーディングツールに貼り付けられるプロンプトを提供します。プロンプトは、ツールにスクリプトを書かせて実行させ、生の出力を自分のマシンに残すよう指示します。ツールが書いたものは公開された結果ではなく、レビューすべきコードとして扱ってください。

## このガイドで得られるもの

- Speedle、Open Policy Agent（OPA）、Casbin、Cedar の等価な RBAC・ABAC ルールと、各リクエストの期待される判定。
- 自分のマシンで生成・実行する実行可能なベンチマークスクリプト、正確なコマンド、固定したエンジンのバージョン。
- レイテンシのパーセンタイル、スループット、メモリ使用量、規模による変化。ポリシーの読み込みは定常状態の評価と分けて測定します。
- 生の出力、環境情報、結果テンプレート。すべてローカルに残ります。このガイドは何もアップロードしません。

候補のエンジンは出発点であり、ランキングではありません。選んだ各エンジンのバージョンがこのシナリオをサポートすることを確認し、除外したケースとその理由を記録し、除外したケースをそのエンジンの結果として提示しないでください。

## 始める前に

- 自分で管理でき、実行中はアイドル状態にできるマシンを使います。負荷の高い他のプロセスを閉じ、CPU のモデル、コア数、メモリ、オペレーティングシステムを記録します。
- 比較に必要なツールをインストールします。Speedle と Go ベースのエンジンには Git と Go 1.26 以降が必要です。それ以外のエンジンは各公式クイックスタートの要件に従います。Speedle のリリースタグを含め、すべてのエンジンのバージョンを固定します。
- 一時ディレクトリにファイルを作成してシェルコマンドを実行できる AI コーディングツールを使います。実行前に生成されたスクリプトを確認し、生の出力を含むディレクトリ全体を記録として残します。
- 小さなポリシーセットでの最初の比較には 30〜60 分を見込みます。ポリシー数と同時実行数を増やす場合はさらに時間が必要です。

空のディレクトリで作業します。Speedle のチェックアウト内で比較を実行しないでください。生成したスクリプト、ポリシー、ログ、結果はリポジトリと分けて保存します。

## 共通シナリオ

比較では次の 1 組の業務ルールを使います。

- 従業員（employee）は自分の部署のドキュメントを読めます。
- 編集者（editor）は自分の部署のドキュメントを読んで編集できます。
- Bob は編集者ですが、第 4 四半期の財務レポートの編集を明示的に禁止されています。
- ユーザーが対応するロールを持っていても、他部署へのリクエストは拒否されます。

リクエストには、アプリケーションがユーザーの `department` とリソースの `owner_department` を属性として渡します。すべてのエンジンが同じリクエストを受け取り、同じ判定を返さなければなりません。

| ユーザー | ロール | アクション | リソース | department | owner_department | 期待 |
| --- | --- | --- | --- | --- | --- | --- |
| alice | employee | read | `/docs/finance/report` | finance | finance | 許可 |
| alice | employee | read | `/docs/hr/salaries` | finance | hr | 拒否 |
| bob | editor | edit | `/docs/finance/report` | finance | finance | 許可 |
| bob | editor | edit | `/docs/finance/q4` | finance | finance | 拒否（明示的な拒否） |
| carol | editor | edit | `/docs/finance/q4` | hr | finance | 拒否（部署の条件） |
| carol | editor | read | `/docs/hr/handbook` | hr | hr | 許可 |

このガイドの例は Speedle v1.6.0、OPA v1.22.3、Casbin v2.123.0、Cedar CLI v4.14.1 で検証しています。インストールするバージョンは固定してください。これは例であり、対応するバージョンがこれだけという意味ではありません。

### Speedle（SPDL）

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

Speedle では、該当する拒否が許可より優先されます。条件が参照する属性がリクエストにない場合、その条件は一致しないため、アプリケーションは毎回 `department` と `owner_department` を送る必要があります。ポリシーファイルは `spctl check` で検証できます。文法は [SPDL](../spdl/)、サービスとロールポリシーは[ポリシー管理](../pms/policy-mgmt/)を参照してください。

### OPA（Rego）

ポリシー：

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

データ（`data.json`）：

```json
{
  "roles": {
    "alice": ["employee"],
    "bob": ["editor"],
    "carol": ["editor"]
  }
}
```

Rego には拒否優先の組み込みルールがないため、上記のポリシーは 2 つの許可ルールから明示的な拒否を差し引いています。ロールデータは OPA のデータドキュメントの一部です。固定した OPA のバージョンに対応する Rego 構文を使ってください（例は OPA 1.x の Rego v1 構文です）。

### Casbin

モデル（`model.conf`）：

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

ポリシー（`policy.csv`）：

```csv
p, employee, /docs/*, read, allow
p, editor, /docs/*, read, allow
p, editor, /docs/*, edit, allow
p, bob, /docs/finance/q4, edit, deny
g, alice, employee
g, bob, editor
g, carol, editor
```

Casbin はモデル、ポリシー、ロール割り当てを 3 つの入力に分けます。ABAC の部署チェックはマッチャ式にあるため、拒否の行にも適用されます。このシナリオの拒否リクエストは部署属性が一致するため、期待される判定は変わりません。無条件の拒否を表すためにマッチャを変更した場合は、その変更を記録してください。

### Cedar

スキーマ（`schema.cedar`）：

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

ポリシー（`policies.cedar`）：

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

エンティティ（`entities.json`）：

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

Cedar では forbid が permit より優先され、すべてのリクエストはデフォルトで拒否されます。ロールとドキュメントの階層はポリシーのテキストではなくエンティティストアのデータなので、スキーマとエンティティも比較の一部です。サンプルのエンティティ一覧には、リクエスト表で使うすべてのドキュメントを含める必要があります。

### 記録すべき意味の違い

同じ業務ルールが、常に同じエンジンの概念に対応するとは限りません。計測の前に、4 つの実装が期待される判定で一致することを確認し、次の点を記録します。

- 許可と拒否が同時に一致したときの各エンジンの解決方法、および拒否が無条件か ABAC 条件を共有するか。
- リクエストに `department` または `owner_department` がないときの各エンジンの動作。
- リソースパターンの表現方法（SPDL の `expr:` 正規表現、Casbin のマッチャ、OPA の補助ルール、Cedar のエンティティ階層）。
- ポリシーのテキスト以外に必要なアプリケーションコードや設定（Cedar のスキーマとエンティティ、Casbin のモデルとロールデータ、OPA のデータドキュメント、Speedle のサービスとロールポリシー）。
- 選んだバージョンで表現できないケース。ランキング対象から除外し、その旨を明記します。

## 正しいものを測る

すべてのエンジンで同じマシン、同じリクエストデータ、同じポリシーセットの規模を使います。バージョンを固定し、生成したデータセットを保持して、再実行時に同じ作業を測れるようにします。

- **組み込み評価とサービス／API テストを分ける。** プロセス内の関数呼び出しと HTTP リクエストは別のものを測っています。別々の表で報告し、互いに順位を付けないでください。
- **ウォームアップして繰り返す。** 各エンジンを安定した結果が得られるまで十分に実行し、ウォームアップを除外し、計測全体を少なくとも 3 回繰り返して実行間のばらつきを報告します。
- **レイテンシとスループットを報告する。** エンジンのツールが対応していれば、p50、p95、p99 レイテンシと 1 秒あたりのリクエスト数を含めます。失敗とタイムアウトも記録します。
- **メモリを測る。** ピーク常駐メモリ、またはエンジン自身の一貫して測れるメモリ指標を記録します。
- **ポリシー数と同時実行数を変える。** 小規模から始め、ポリシーセットと同時呼び出し数を増やします。最も良い点だけでなく、すべてのデータ点を記録します。
- **ポリシーの読み込みと起動を別に測る。** 各エンジンが定常ループの前にポリシーセットを解析・読み込みする時間を測り、そのコストを最初のリクエストに隠さないでください。
- **キャッシュと準備を明示する。** インデックス、コンパイル済みポリシー、判定キャッシュ、事前計算があれば記録し、ウォームかコールドかを示します。
- **生の出力を残す。** 各実行の正確なコマンド、設定、ポリシーファイル、エンジンのバージョン、環境、完全な出力を保存します。

## AI コーディングツールで比較を実行する

Claude Code、ChatGPT Codex、DeepSeek Harness など、ファイルを作成してコマンドを実行できるツールで空のディレクトリを開きます。次のプロンプトをそのまま貼り付けてください。ツールに環境の記録、バージョンの固定、共通シナリオの構築、期待される判定の検証、計測の実行、ローカルの結果テンプレートの作成を指示します。

```text
自分のマシンで Speedle と OPA、Casbin、Cedar のパフォーマンスを手早く比較したいです。まず私のオペレーティングシステム、ハードウェア、リソース制限を記録し、サポートされているエンジンのバージョンを選んで固定してください。共通のデータセットと期待される許可／拒否の判定を使って、等価な RBAC と ABAC の例を作成してください。意味の違いを説明し、サポートされていないケースは直接のランキングから除外してください。パフォーマンスを測る前に正しさを検証してください。

実行可能なベンチマークスクリプトと正確なセットアップ／実行コマンドを作成してください。組み込みテストとサービス／API テストは分けてください。同じワークロードを使い、ポリシー数と同時実行数を変え、キャッシュと準備を明示し、各エンジンをウォームアップして計測を繰り返してください。対応している場合はレイテンシ、スループット、メモリ使用量、実行間のばらつきを、生の出力と環境情報とともに報告してください。実際に測定した数値だけを報告し、実行できなかったものにはその旨を記してください。

各エンジンの等価なポリシーと、必要なモデル／スキーマ／アプリケーションコードを示してください。可読性、学ぶ必要のある概念、作成の手間、検証、デバッグ、ポリシー変更のしやすさを比較してください。トレードオフと、各エンジンがどのテストワークロードに適しているかを説明し、Speedle が勝つと決めつけないでください。
```

ツールは現在のディレクトリで作業し、スクリプト、ポリシー、出力をそこに残す必要があります。最初の実行前に、特にダウンロードやインストールの手順を含めてスクリプトを確認してください。ツールが環境にエンジンをインストールできない場合は、数値を推定せず、そのエンジンを未実行として示すよう依頼してください。

## AI ツールの作業を確認する

- 計測を始める前に、4 つの実装がすべてのリクエストで表の判定を返すことを確認します。速くても間違った答えは結果ではありません。
- インストールされたバージョンが報告されたバージョンと一致し、すべてのエンジンで同じマシン、リクエストデータ、ポリシーセットの規模が使われたことを確認します。
- 読み込みと起動が定常ループと分けて報告され、組み込みとサービス／API の測定が別の表にあることを確認します。
- 要約のすべての数値に対応する生の出力があることを確認します。コマンドと出力のない数値は未測定として扱います。
- 制限事項を読んでください。ケースを実行できなかったエンジンは、ランキングではなく未実行として示さなければなりません。

## 結果テンプレート

ツールに次のようなローカルの表を埋めさせ、生の出力と一緒に保管します。Speedle のパフォーマンスレポートとして公開しないでください。

| エンジン | バージョン | モード | ポリシー数 | 同時実行数 | p50 | p95 | p99 | スループット | ピークメモリ | ポリシー読み込み時間 |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Speedle | | 組み込み | | | | | | | | |
| OPA | | 組み込み | | | | | | | | |
| Casbin | | 組み込み | | | | | | | | |
| Cedar | | 組み込み | | | | | | | | |

実行記録も残します。

| 実行 | エンジン | コマンド | 生の出力 | メモ |
| --- | --- | --- | --- | --- |
| 1 | | | | |

## 使いやすさの比較

測定と使いやすさは別の観察です。同じ業務ルールを使って、各エンジンがそれをどう表現するかを記録します。

- **Speedle（SPDL）。** 各ルールは 1 つの文で、ロールの割り当ては別のロールポリシーセクションにあり、条件はリクエスト属性に対するブール式です。ポリシーは `spctl check` で検証できます。
- **OPA（Rego）。** ルールはデフォルト拒否の下でルール本体として書き、ロールデータは別のドキュメントに置き、明示的な拒否には独自のルールと許可ルール側の除外条件が必要です。
- **Casbin。** モデル、ポリシー行、ロール割り当ては別々の入力です。ABAC チェックはマッチャ式の一部なので、条件を変えるにはモデルを変えます。
- **Cedar。** スキーマがエンティティとアクションを宣言し、ポリシーは permit と forbid の文で、ロールとリソース階層はエンティティストアにあります。forbid は permit より優先されます。

各エンジンについて、可読性（ルールが業務の文にどれだけ直接対応するか）、学ぶ必要のある概念、定型コードとアプリケーションコード、検証とエラーメッセージ、テストとデバッグ、ルールを 1 つ追加・変更する際の作業を記録します。ポリシーの行数は参考になりますが、使いやすさの唯一の尺度にしないでください。

## Speedle 専用のベンチマーク

リポジトリの[ベンチマーク](https://github.com/teramoby/speedle-plus/blob/master/benchmarks/README.md)は Speedle 単体の評価を対象とし、明示的に有効化する 100 万ポリシーのケースを含みます。チェックアウトで `go test -bench .` を実行し、100 万ポリシーのケースは README の手順に従ってください。これらの測定は Speedle 単体のもので、特定のハードウェアで取得されたものです。現在のクロスエンジン比較として提示しないでください。

## 公平に報告する

- すべてのエンジンで同じシナリオ、データ、ポリシーセットの規模を使い、各エンジンが実行できなかったケースを明記します。
- 他のエンジンが速いケースも含め、結果の全範囲を報告します。
- 各エンジンがどのテストワークロードに適しているかを制限とともに説明します。1 台のマシンでの 1 つのシナリオから、普遍的に最速のエンジンを宣言しないでください。
- 要約を共有するときは、生の出力と正確なコマンドを添えてください。
