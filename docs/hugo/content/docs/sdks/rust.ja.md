+++
title = "Rust SDK"
description = "Speedle Rust SDK を追加し、ローカルの Speedle に対して認可チェックを実行"
weight = 50
aliases = ["/ja/docs/api/sdk-rust/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "cargo add で speedle 0.2.0 クレートを追加し、ローカルの Speedle に対して is_allowed と check を呼び出します。"
lastmod = 2026-11-07T19:43:15+08:00
+++

Rust SDK は Rust 1.75 以降が必要です。`tokio` と `reqwest` (rustls) をベースにした非同期構成です。`blocking` 機能により、以下で使用するブロッキングクライアントが追加されます。クレート: crates.io の [`speedle`](https://crates.io/crates/speedle) 0.2.0。リファレンス: [speedle-sdk/rust](https://github.com/teramoby/speedle-sdk/tree/main/rust)。

サンプルでは、[SDK 概要](/ja/docs/sdks/#example-policy) で説明されているポリシーをチェックします。

## 依存関係の追加

新しいパッケージを作成します:

```bash
cargo new speedle-example
cd speedle-example
cargo add speedle@0.2.0 --features blocking
```

## ローカルの Speedle の呼び出し

[SDK 概要](/ja/docs/sdks/#example-policy) の説明に従って Speedle を起動し、サンプルポリシーを作成します。`src/main.rs` を以下に置き換えます:

```rust
use speedle::blocking::Client;
use speedle::{Auth, Backend};

fn main() {
    if let Err(e) = run() {
        eprintln!("Speedle error: {e}");
        std::process::exit(1);
    }
}

fn run() -> Result<(), speedle::Error> {
    let speedle = Client::builder(Backend::Speedle, "http://localhost:6734", Auth::none(), "docs").build()?;

    let allowed = speedle.is_allowed(&"alice".into(), "get", "/docs/public", &[])?;
    println!("alice get /docs/public: allowed={allowed}");

    let d = speedle.check(&"alice".into(), "delete", "/docs/public", &[])?;
    println!("alice delete /docs/public: allowed={} reason={:?}", d.allowed, d.reason);
    Ok(())
}
```

実行します:

```bash
cargo run
```

出力:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```


## エラー処理

拒否されたリクエストはエラーではありません。上記の `delete` 呼び出しのように、`is_allowed` は `false` を返し、`check` は `allowed` が false で理由を含む決定を返します。デフォルト設定では、呼び出しが決定を返せない場合、`Err(speedle::Error)` を返します。サンプルではエラーを `?` で `main` に渡し、標準エラー出力に `Speedle error: <message>` を出力して終了ステータス 1 で終了します。
