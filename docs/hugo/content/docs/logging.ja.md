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

ログメッセージの処理には、以下の2つのオープンソースプロジェクトが使用されています:

```
1. https://github.com/sirupsen/logrus
2. https://github.com/natefinch/lumberjack
```

Logrus は Go (golang) 向けの構造化ロガーであり、golang 標準ライブラリロガーと完全な API 互換性があります。Logrus はログローテーションをサポートしていないため、ローリングロガーを提供する第2のプロジェクトである lumberjack を導入しています。Logrus は `io.Writer` インターフェースを使用して lumberjack と連携します。以下の図を参照してください。

<img src="/img/speedle/logger.jpg" alt="Speedle Logging" width="370" height="230" loading="lazy">

## ロギング構成

Speedle のロギングを構成する方法には、コマンドライン引数による方法と構成ファイルによる方法の2つがあります。

### コマンドライン引数によるロギング構成

使用可能なすべてのコマンドライン引数を以下に示します。レベル、フォーマッター、reportcaller 設定は logrus を構成し、ファイル/ローテーション設定は lumberjack を構成します。

```text
--log-level string                ログレベル（panic, fatal, error, warn, info, debug） (default "info")
--log-formatter string            ログフォーマッター（text, json） (default "text")
--log-reportcaller                呼び出し元のメソッド名とファイル名を含めるかどうか
--log-file string                 ログファイルパス
--log-file-maxsize int            ログローテーション前の最大サイズ（メガバイト） (default 100)
--log-file-maxage int             古いログファイルを保持する最大日数 (default 0)
--log-file-maxbackups int         保持する古いログファイルの最大数 (default 0)
--log-file-localtime              バックアップファイルのタイムスタンプにローカル時間を使用するかどうか
--log-file-compress               ローテーションされたログファイルを gzip で圧縮するかどうか
```

### 構成ファイルによるロギング構成

構成ファイルを使用してロギングを設定することもできます。以下は構成例です:

```json
{
  "logConfig": {
    "level": "info",
    "formatter": "json",
    "reportCaller": false,
    "fileConfig": {
      "filename": "/var/log/speedle.log",
      "maxSize": 100,
      "maxAge": 30,
      "maxBackups": 10,
      "localTime": true,
      "compress": true
    }
  }
}
```
