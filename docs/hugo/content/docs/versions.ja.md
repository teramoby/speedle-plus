+++
title = "バージョン"
description = "Speedle のリリース履歴とセマンティックバージョニング"
weight = 400
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["releases", "versions"]
categories = ["docs"]
bref = "リリース — Speedle はセマンティックバージョニングを採用しています"
lastmod = 2026-11-10T14:45:43+08:00
+++

## Speedle のリリース

公開されたバージョンとリリースノートについては、[GitHub Releases](https://github.com/teramoby/speedle-plus/releases) ページを参照してください。

- [v1.9.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.9.0): カスタム関数の失敗が決定の失敗になる、関数のパラメーターと結果の型宣言、MySQL ストアのスキーマバージョン 2、js/wasm ビルドでのホスト側カスタム関数呼び出しの登録、アプリケーションログと監査ログの Go `log/slog` への移行。
- [v1.8.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.8.0): 実験的なポリシー配布機能の削除、ソースビルドに Go 1.27.4 以降が必要、コンテナイメージへの公開 CA 証明書バンドルの同梱、日本語・韓国語・スペイン語ドキュメントの追加。
- [v1.7.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.7.0): MySQL ポリシーストレージ、オフラインの `spctl check`、コマンドヘルプの全オプション表示、panic とストア障害時のフェイルクローズ、Docker Hub と GHCR の単一マルチアーキテクチャイメージ。

[v1.9.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.9.0) は、現在の組み込み Go ウォークスルーで固定されているバージョンです。再現可能なモジュール依存関係のために、明示的にリリースされたバージョンを使用してください。[組み込みガイド](../deployment/embedded/) で設定方法を説明しています。

## 以前の Oracle Speedle リリース

バージョン 0.1.0 は、ポリシー管理、認可決定、コマンドラインツール、および統合サンプルを含む最初のオープンソースリリースでした。過去の 0.2.0 計画メモは、現在の Speedle リリースロードマップではありません。
