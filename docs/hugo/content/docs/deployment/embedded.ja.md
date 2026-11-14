+++
title = "Speedle 組み込みモード"
description = "Go modules を使用して Speedle を Go アプリケーションに組み込み、Allow/Deny 認可決定を検証"
weight = 10
aliases = ["/ja/docs/deployment-embeded/"]
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
bref = ""
lastmod = 2026-11-10T14:45:43+08:00
+++

## 組み込みモードとは？

組み込みモードは、Speedle ポリシー決定エンジンを Go アプリケーション内で直接実行します。PMS や ADS サーバーを起動することなく、評価エンジンを直接呼び出します。アプリケーションがユーザーを認証し、各決定のためのサブジェクト、リソース、アクションを提供します。

コードを書く前に基本的なポリシーを試すには、[Playground](/ja/playground/) を開いてください。Speedle をスタンドアロンサービスとして実行するには、[クイックスタート](/ja/quick-start/) に従ってください。

## 前提条件

[クイックスタート](/ja/quick-start/) と同様に、シェルと **Go 1.27.4**（または同じリリースブランチの新しいパッチバージョン）を備えた Linux または macOS を使用します。このウォークスルーでは、Go modules と公開された **Speedle v1.6.0** リリースを使用します。

[Go 公式インストール手順](https://go.dev/doc/install) に従って Go をインストールし、バージョンを確認します:

<!-- embedded: version -->
```bash
go version
```

Go は `GOROOT` とデフォルトの `GOPATH` を自動的に構成するため、カスタム値は不要です。

## アプリケーションモジュールの作成

Speedle ソースチェックアウトの外部にある書き込み可能なディレクトリから始めます。このウォークスルー用に新しいディレクトリを使用します:

<!-- embedded: setup -->
```bash
mkdir speedle-embedded-demo
cd speedle-embedded-demo
go mod init example.com/speedle-embedded-demo
go get github.com/teramoby/speedle-plus/pkg/eval@v1.6.0
```

既存の Go モジュールでは、そのモジュールのディレクトリから最後の `go get` コマンドのみを実行します。

ここで `go get` はバージョン管理された**ライブラリ依存関係**を `go.mod` と `go.sum` に追加します。プログラムのインストールは行いません。これは、スタンドアロンサービスの実行可能ファイルをビルドするために `git clone` と `go build` を使用するクイックスタートとは異なります。どちらも Go modules を使用します。`go.mod` と `go.sum` はバージョン管理下に保持してください。

## ポリシーファイルの作成

`speedle-embedded-demo` 内で、この内容を `policies.spdl` として保存します。サービス `mysvc` 内のリソース `res1` に対する `alice` の `get` 権限を付与します:

<!-- embedded: policies -->
```ini
[service.mysvc]
[policy]
grant user alice get res1
```

コンストラクタ `eval.NewFromFile` は、SPDL または JSON ポリシーを含むローカルファイルを読み込みます。このウォークスルーでは SPDL を使用します。

## 評価エンジンの初期化と決定リクエスト

同じディレクトリに以下の完全なプログラムを `main.go` として保存します:

<!-- embedded: program -->
```go
package main

import (
	"fmt"
	"log"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
)

func main() {
	evaluator, err := eval.NewFromFile("./policies.spdl", false)
	if err != nil {
		log.Fatal(err)
	}

	for _, user := range []string{"alice", "bob"} {
		allowed, _, err := evaluator.IsAllowed(ads.RequestContext{
			Subject: &ads.Subject{
				Principals: []*ads.Principal{{Type: ads.PRINCIPAL_TYPE_USER, Name: user}},
			},
			ServiceName: "mysvc",
			Resource:    "res1",
			Action:      "get",
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s allowed=%t\n", user, allowed)
	}
}
```

ブランクインポート `_ "github.com/teramoby/speedle-plus/pkg/store/file"` は、ファイルストアのドライバーを登録します。これを省略すると `unknown store type "file"` エラーが発生します。

`eval.NewFromFile(loc, isWatch)` は `ads.PolicyEvaluator` とエラーを返します。評価エンジンを使用する前にエラーを確認してください。`isWatch=false` の場合、ポリシーは 1 回だけ読み込まれます。`true` に設定すると、長時間実行されるアプリケーション向けにファイル監視が有効になります。

そのインスタンスで `evaluator.IsAllowed(ctx)` を呼び出します。その `ads.RequestContext` には、サブジェクトのプリンシパル、`ServiceName`、`Resource`、`Action`、およびオプションの `Attributes` が含まれます。`(allowed bool, reason ads.Reason, err error)` が返されます。評価エラーは、正常な `allowed=false` 決定とは別に処理する必要があります。

## ビルドと実行

`speedle-embedded-demo` から依存関係を解決し、アプリケーションをビルドします:

<!-- embedded: build -->
```bash
go mod tidy
go build -o ./bin/embedded-demo .
```

`./policies.spdl` が正しく解決されるように、同じディレクトリから実行します:

<!-- embedded: run -->
```bash
./bin/embedded-demo
```

認可決定の出力は次のとおりです:

```text
alice allowed=true
bob allowed=false
```

`alice` はグラントに一致します。`bob` には一致するグラントがないため拒否されます。このサンプルでは、ネットワークサービスやデータベースは必要ありません。
