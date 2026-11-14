+++
title = "ロギングフレームワーク"
description = "Speedle のロギング"
weight = 320
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "logging"]
categories = ["docs"]
bref = ""
lastmod = 2026-11-08T08:33:54+08:00
+++

## 概要

v1.8.0 の次のリリースでは、アプリケーションと監査ログに Go `log/slog`、ファイルローテーションに `gopkg.in/natefinch/lumberjack.v2` を使用します。v1.8.0 以前は従来の API を使用します。

## ログ設定

既存の `--log-*`、`--auditlog-*`、`SPDL_*` 環境変数と JSON キーは維持されます。優先順位は明示的なオプション > 環境変数 > ファイルです。アプリケーションの既定値は info、text、stderr、呼び出し元情報なしです。監査の既定値は info と JSON で、`--auditlog-*` により独立して設定できます。

```text
--log-level string
--log-formatter string
--log-reportcaller bool
--log-filename string
--log-maxsize int
--log-maxbackups int
--log-maxage int
--log-localtime bool
--log-compress bool
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--log-level` | `info` | Minimum severity |
| `--log-formatter` | `text` | `text` or `json` |
| `--log-reportcaller` | `false` | Include caller file, line and function |
| `--log-filename` | unset | Stderr unless a file is configured |
| `--log-maxsize` | `100` | Maximum file size in MB |
| `--log-maxbackups` | `0` | Backup count; 0 retains all, subject to max age |
| `--log-maxage` | `0` | Backup age in days; 0 disables age deletion |
| `--log-localtime` | `false` | Local time for backup filenames, otherwise UTC |
| `--log-compress` | `false` | Compress backups |

```json
{
  "logConfig": {
    "level": "info",
    "formatter": "text",
    "setReportCaller": false,
    "rotationConfig": {
      "filename": "/mnt/logs/speedle.log",
      "maxSize": 20,
      "maxBackups": 5,
      "maxAge": 0,
      "LocalTime": false,
      "compress": false
    }
  },
  "auditLogConfig": {
    "level": "info",
    "formatter": "json"
  }
}
```


## レベルと出力形式

名前の大文字小文字は区別しません。`trace`、`debug`、`info`、`warn`、`warning`、`error`、`fatal`、`panic` を受け付けます。trace は debug に統合され、debug の設定では従来の trace メッセージも出力されます。他のしきい値の順序は変わりません。不明な空でない値は、その値を含むエラーになります。

`text` は key=value、`json` は 1 行に 1 JSON オブジェクトを出力します。`time`、`level`、`msg`、呼び出し元情報を有効にした場合の行番号付き `file` と `func` を維持します。レベルは小文字で、warn は `warning` と出力します。エラーは文字列、その他の構造化値は構造を保ちます。メタデータと同名のユーザーフィールドには `fields.` を付けます。

```text
time=2026-11-15T09:00:00.000+08:00 level=info msg="Started observing beach" animal=walrus number=8
```

```json
{"time":"2026-11-15T09:00:00+08:00","level":"info","msg":"Started observing beach","animal":"walrus","number":8}
```


## ログ収集の移行

text の引用符、順序、色、時刻の精度は logrus と異なる場合があります。ANSI 色は出力せず、時刻は RFC3339 互換です。解析ルールを確認するか、キーで処理できる JSON を使用してください。API 名直前の `[AUDIT]`、`tenantid`、`target_index`、要求と応答の値、成功とエラーのレベルは維持します。

新規ファイルの既定権限は `0644` から所有者専用の `0600` に変わります（umask の影響を受けます）。既存ファイルの権限はローテーション時も保持します。収集プロセスを所有者として実行するか、更新前にアクセス権を明示的に設定してください。

## Go API の移行

`InitLogInstance(logrusLogger, config)` を `logger, err := logging.NewLogger(config)` に置き換えます。`AuditLog()` は `*slog.Logger` を返します。logrus の setter/hook の代わりに slog のメソッドと `SetAuditLog(logger)` を使用します。`LogConfig.RotationConfig` は lumberjack v2 をインポートしてください。v1 ポインタはコンパイルできません。`InitLog` と `InitAuditLog` は省略した設定を維持し、nil は変更しません。不正な設定は既存の logger を置き換えません。

`logging.Default()` を使用するか、`logging.SetDefault(logger)` で独自の handler を設定します。`slog.Default()` とホストの標準 `log` 出力は変更しません。`logging.Fatal(message)` は記録後に終了コード 1 で終了し、`logging.Panic(message)` は記録後に panic します。出力がフィルターされても終了動作は実行します。

```go
import (
    "github.com/teramoby/speedle-plus/pkg/logging"
    "gopkg.in/natefinch/lumberjack.v2"
)

logger, err := logging.NewLogger(&logging.LogConfig{
    Level: "info", Formatter: "json",
    RotationConfig: &lumberjack.Logger{Filename: "engine.log", MaxSize: 20},
})
if err != nil {
    return err
}
logger.Info("Started observing beach", "animal", "walrus", "number", 8)
```


## コンテナのログ

ファイル名を省略すると stderr を使用し、コンテナランタイムにローテーションを任せられます。lumberjack のファイル設定は不要です。組み込み etcd の推移的依存として logrus と zap は残る場合がありますが、Engine のコードは slog を使用します。
