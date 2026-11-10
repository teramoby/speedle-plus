+++
title = "プラグイン可能なストレージ"
description = "ファイル、etcd、MySQL、または MongoDB を使用してポリシーを保存するか、インターフェースを実装して独自のストレージバックエンドを接続"
weight = 130
draft = false
toc = true
layout = "single"
linksection = true
tocheading = "h2"
tags = ["storage", "pluggable"]
categories = ["docs"]
bref = "Speedle にはファイル、etcd、MongoDB、MySQL のポリシーストアが含まれています。このドキュメントでは、それらの構成と、別のストアを実装または登録する方法について説明します。"
+++

## 組み込みストアと拡張コントラクト

スタンドアロンバイナリは、`pkg/store` 配下にファイル、etcd、MongoDB、MySQL の実装を登録します。利用可能であることは同じ運用上の保証を意味するわけではありません。デプロイにあたっては、バックエンドの認証、永続化、トランザクション、および watch の動作を個別に確認してください。以下の設定例はプロパティ名を示すためのものであり、etcd/MongoDB デプロイの実行はこのローカルサンプルの対象外です。

完全なバックエンドは [pms.PolicyStoreManager](https://github.com/teramoby/speedle-plus/blob/master/api/pms/interface.go) を実装します（サービス、認可ポリシー、ロールポリシー、関数の管理、スナップショット読み取り、watch/stop）。ADS はより限定的な読み取り/watch インターフェースである `PolicyStoreManagerADS` を受け入れます。新しいバックエンドを実装する際は、過去のドキュメントの不完全なメソッドリストをコピーするのではなく、実際のインターフェースを確認してください。

ステップバイステップのデプロイガイド: [MySQL セットアップ](/ja/docs/store/mysql/)、[etcd セットアップ](/ja/docs/store/etcd/)、[MongoDB セットアップ](/ja/docs/store/mongodb/)。

<a id="store-failures-and-cached-authorization"></a>
## ストア障害とキャッシュされた認可

ファイル、etcd、MySQL、MongoDB は同じ ADS 障害ポリシーを使用します。デフォルトの `storeFailureGracePeriod` は `"0s"` です。ストアの障害が検出されると、認可は `allowed:false` と評価エラーを返します。ADS は実行を継続し、再試行を続けます。接続が読み取り可能になっただけでは認可は復元されず、完全なスナップショットの再読み込みが成功する必要があります。watch が有効な場合、失敗したサブスクリプションをまず再確立する必要があります。watch が壊れている間のデータ読み取り成功は、障害/猶予期間の期限をリセットしません。ファイルが削除された場合も、ADS を終了させるのではなく再試行されます。

権限失効の遅延を明示的に許容するデプロイでは、ADS JSON 設定のトップレベルで有限の期間（例: `"storeFailureGracePeriod": "5s"`）を設定できます。同等のフラグは `--store-failure-grace-period=5s`、環境変数は `SPDL_STORE_FAILURE_GRACE_PERIOD` です。通常の「フラグ → 環境変数 → 設定ファイル → デフォルト値」の優先順位が適用されます。障害が繰り返されても期限は延長されません。猶予期間内の決定は最後の完全なスナップショットを使用するため、同期が回復するまで失効は有効になりません。部分的なキャッシュ障害や評価パニックに猶予期間が適用されることはありません。

組み込みストアには軽量なヘルスプローブがあり、1 秒のプローブタイムアウトで約 1 秒に 1 回チェックされます。watch はより早く障害を報告する場合があります。実行中のストア読み取りにもそれぞれのバックエンドタイムアウトがあります。障害検出と通常のポリシー伝播は非同期であり、即座の失効保証はありません。`enableWatch:false` の場合でも、リクエストはストアのヘルスをチェックします。通常のポリシー更新には明示的なリフレッシュが必要です。障害発生後のリカバリでは、そのモードであっても完全なスナップショットが読み込まれます。初期読み込みの失敗には猶予期間を使用できません。カスタムストアは `pms.StoreHealthChecker` を実装し、`pms.STORE_UNAVAILABLE` イベントを発行して同じ障害ポリシーに参加できます。

ファイルストアの PMS 書き込みは、完全にシリアル化され同期された一時ファイルを公開し、同じディレクトリ内でアトミックなリネームを行います。したがって、独立した ADS リーダーは常に完全な古いスナップショットまたは新しいスナップショットを参照します。ライターには親ディレクトリへの書き込みアクセス権が必要です。既存のパーミッションビットおよび構成されたシンボリックリンクは保持されます。置換ファイルはライターアカウントによって作成されるため、PMS/ADS で別個のユーザーを使用する場合は、ディレクトリのグループ/ACL 継承を設定してください。コンテナはポリシーディレクトリを共有し、置換が両方のサービスから見えるようにする必要があります。直接新規作成されたポリシーファイルはパーミッション 0600 を使用します。通常のファイルストアビルダーは、以前と同様に見つからないストアを初期化します。

## MySQL 8.4 ポリシーストア

[ステップバイステップの MySQL セットアップガイド](/ja/docs/store/mysql/) に従って、データベースを作成し、個別の PMS/ADS アカウントを設定して、ポリシーの同期を確認してください。

`mysql` バックエンドは、PMS および ADS 向けの完全なポリシーストアインターフェースを実装します。検出リクエストの収集とポリシー生成はサポートされていません。純粋な Go 製の `go-sql-driver/mysql` ドライバーを使用します。データベースはあらかじめ作成されている必要があります。

```json
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle:replace-me@tcp(db.example.com:3306)/speedle?tls=true",
      "MySQLInitSchema": true,
      "MySQLPollInterval": "1s",
      "MySQLQueryTimeout": "10s"
    }
  },
  "enableWatch": true
}
```

| プロパティ | フラグ | デフォルト |
| --- | --- | --- |
| `MySQLDSN` | `--mysqlstore-dsn` | 必須（データベース名を含む） |
| `MySQLInitSchema` | `--mysqlstore-init-schema` | `true` |
| `MySQLPollInterval` | `--mysqlstore-poll-interval` | `1s` |
| `MySQLQueryTimeout` | `--mysqlstore-query-timeout` | `10s` |

環境変数も同様のマッピングを使用します（例: `SPDL_MYSQLSTORE_DSN`）。期間は正の文字列でなければなりません。`MySQLInitSchema` は JSON のブール値またはブール文字列を受け入れます。明示的なフラグは空でない環境変数をオーバーライドし、次に設定ファイルの値、最後にデフォルト値の順で優先されます。組み込み用途の場合、`pkg/store/mysql` をインポートし、これらのプロパティを `store.NewStore` に直接渡し、完了時に返された `*mysql.Store` を閉じてください。`eval.NewFromConfig` 経由で作成されたエバリュエーターは `io.Closer` で閉じることができます。エバリュエーターを閉じると、提供されたクローズ可能なストアも閉じられます。

### スキーマとプロビジョニング

1 つのデータベースが 1 つのポリシーストアとなり、5 つの固定 InnoDB テーブル（`speedle_services`、`speedle_policies`、`speedle_role_policies`、`speedle_functions`、`speedle_store_meta`）を使用します。複雑な配列やメタデータには JSON カラムを使用します。ポリシーテーブルはカスケード削除でサービスを参照します。サービス名/関数名およびポリシー ID は大文字小文字を区別し、パディングを行わない比較を使用し、最大 255 文字の Unicode 文字をサポートします。ポリシー名は一意である必要はありません。

デフォルトでは、起動時に [schema.sql](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/mysql/schema.sql) を使用して欠落しているテーブルが作成されます。データベースの作成や既存スキーマの変更は行いません。事前構築されたデプロイの場合、プロビジョニングアカウントを使用して選択したデータベースに対して同じ SQL を実行し、`MySQLInitSchema` を `false` に設定します。起動時にスキーマバージョン、必須カラム、および InnoDB エンジンがチェックされます。サポートされていないスキーマバージョンの場合は起動に失敗します。将来のアップグレードには明示的なマイグレーションが必要です。

プロビジョニングにはテーブル作成および初期化の権限が必要です。事前構築されたスキーマの場合、PMS アカウントにはポリシーデータベースに対する `SELECT`、`INSERT`、`UPDATE`、および `DELETE` 権限が必要です。ADS は `SELECT` 権限のみを持ち、自動スキーマ作成を無効にした個別のアカウントを使用できます。リモートデータベースにはドライバーの DSN TLS オプションを使用してください。本番環境のパスワードをコミットされた設定やコマンドライン履歴に含めないでください。

### トランザクション、同期、およびリカバリ

各書き込みは単一のリビジョン行をロックし、リビジョンのインクリメントとともにデータの変更をコミットします。これにより、PMS インスタンス間でのポリシー書き込みがシリアル化されます。ネストされたポリシーを持つサービスの作成、カスケード削除、およびストア全体の置換はアトミックです。インポートでは指定されたポリシー ID が保持され、欠落している場合は ID が割り当てられます。スナップショットの読み取りには 1 つの反復可能読み取り（repeatable-read）トランザクションを使用します。

`enableWatch` が有効な場合、ADS はリビジョンをポーリングし、変更時に完全で一貫性のあるスナップショットを受け取ります。また、初回読み込みと watch 登録の間の更新をカバーするために、起動時にもスナップショットが送信されます。伝播は非同期であり、通常はポーリング間隔にクエリ/キャッシュ再構築時間を加えた時間内に行われます。データベースの停止中、ADS はデータを保持しますが、デフォルトではキャッシュされた認可を一時停止し、読み取りを再試行し、再開する前に完全なスナップショットを再読み込みします。上記の共有障害/猶予ポリシーが適用されます。リビジョンをインクリメントせずに SQL で直接行われた変更は自動的には検出されません。ポリシーの書き込みには PMS またはストアインターフェースを使用してください。

操作タイムアウトは、接続プールの待機およびトランザクションロックの待機も制限します。失敗した書き込みはロールバックされます。コミット中の接続切断により結果が不確実になる可能性があるため、呼び出し元は冪等でない作成を再試行する前に保存された状態を確認する必要があります。`StopWatch` および `Close` は冪等であり、watch リソースを解放します。`Close` は接続プールも閉じます。

## ビルダーの実装と登録

[store.StoreBuilder](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/store.go) は、`NewStore(map[string]interface{}) (pms.PolicyStoreManager, error)` および `GetStoreParams() map[string]string` を提供します。後者はフラグ名を設定プロパティ名にマッピングします。`init()` 内または `store.NewStore` を呼び出す前に一意のストアタイプを登録してください。重複登録はエラーになります。

この完全なサンプルは、ファクトリコントラクトを実証するために既存のファイルバックエンドに委任します。新しいデータベースの実装を主張するものではありません:

<!-- docs-check: store -->
```bash
go run ./website/tests/examples/store
```

<!-- docs-source: website/tests/examples/store/main.go -->
```go
package main

import (
	"fmt"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/store/file"
	"log"
	"os"
	"path/filepath"
)

// This example delegates to the file backend to demonstrate registration.
// A new backend must implement the complete PolicyStoreManager contract.
type Builder struct{ file.FileStoreBuilder }

func main() {
	dir, err := os.MkdirTemp("", "speedle-store-example-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store.Register("myfile", Builder{})
	policies, err := store.NewStore("myfile", map[string]interface{}{"FileLocation": filepath.Join(dir, "policies.json")})
	if err != nil {
		log.Fatal(err)
	}
	snapshot, err := policies.ReadPolicyStore()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Registered myfile store; services=%d\n", len(snapshot.Services))
}
```

## 設定の正しいマージ

スタンドアロンコマンドの場合、明示的なフラグは空でない環境変数をオーバーライドし、次に設定ファイルのプロパティ、最後にデフォルト値の順で優先されます。フラグ名を変換するには、`SPDL_` を追加し、`-` を `_` に置き換えて大文字にします（例: `etcdstore-endpoint` は `SPDL_ETCDSTORE_ENDPOINT` になります）。値はフラグや環境変数からの文字列、または型付けされた JSON 値として届く場合があります。意図して検証および変換を行ってください。

```json
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "localhost:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": true,
      "EmbeddedEtcdDataDir": "./speedle.etcd"
    }
  },
  "enableWatch": true
}
```

解析前に `pflag` でカスタムフラグを定義し、`GetStoreParams` からフラグとプロパティのマッピングを返します。組み込みの呼び出し元は設定マップを `store.NewStore`/`eval.NewFromConfig` に直接渡します。ファクトリ単体ではコマンドラインや環境の設定は読み取られません。

## バックエンドのリンクと更新の処理

登録が `init()` 内で行われる場合、両方のスタンドアロンバイナリ（または組み込みアプリケーション）で副作用インポートを使用します:

```go
import (
    _ "github.com/teramoby/speedle-plus/pkg/store/etcd"
    _ "github.com/teramoby/speedle-plus/pkg/store/file"
    _ "github.com/teramoby/speedle-plus/pkg/store/mongodb"
    _ "github.com/teramoby/speedle-plus/pkg/store/mysql"
)
```

`Watch()` を実装して、`pms.StorageChangeChannel` 経由で正しいスナップショット/変更イベントを返すようにします。watch リソースをリークすることなく `StopWatch()` をサポートします。ADS はランタイムキャッシュを非同期に更新します。watch の喪失/切断、リカバリ、書き込み中の障害、および同時実行リーダーをテストしてください。バックエンドが watch をサポートしているというだけで、瞬時の伝播を保証してはなりません。
