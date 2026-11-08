+++
title = "Rust SDK"
description = "添加 Speedle Rust SDK，向本地 Speedle 或 Speedle Cloud 发起授权检查。"
weight = 50
aliases = ["/zh/docs/api/sdk-rust/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "用 cargo add 添加 speedle 0.2.0，再向本地 Speedle 或 Speedle Cloud 调用 is_allowed 与 check。"
+++

Rust SDK 需要 Rust 1.75 或更高版本，基于 `tokio` 和 `reqwest`（rustls）实现异步调用；下面使用的阻塞客户端由 `blocking` 特性提供。crate：crates.io 上的 [`speedle`](https://crates.io/crates/speedle) 0.2.0。参考文档：[speedle-sdk/rust](https://github.com/teramoby/speedle-sdk/tree/main/rust)。

示例使用的策略见 [SDK 概览](/docs/sdks/#示例策略)。

## 添加依赖

新建包并添加依赖：

```bash
cargo new speedle-example
cd speedle-example
cargo add speedle@0.2.0 --features blocking
```

## 调用本地 Speedle

按 [SDK 概览](/docs/sdks/#示例策略)中的说明启动 Speedle 并创建示例策略。将 `src/main.rs` 替换为：

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

运行：

```bash
cargo run
```

输出：

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

## 调用 Speedle Cloud

> **Speedle Cloud 即将上线。**

程序从 `SPEEDLE_SERVICE` 读取服务名，从 `SPEEDLE_TOKEN` 读取该服务的令牌。`SPEEDLE_ENDPOINT` 可覆盖默认端点 `https://cloud.speedle.io`。将 `src/main.rs` 替换为：

```rust
use std::env;

use speedle::blocking::Client;
use speedle::{Auth, Backend};

fn main() {
    if let Err(e) = run() {
        eprintln!("Speedle error: {e}");
        std::process::exit(1);
    }
}

fn run() -> Result<(), speedle::Error> {
    let endpoint = env::var("SPEEDLE_ENDPOINT").unwrap_or_else(|_| "https://cloud.speedle.io".to_string());
    let token = env::var("SPEEDLE_TOKEN").unwrap_or_default();
    let service = env::var("SPEEDLE_SERVICE").unwrap_or_default();
    let speedle = Client::builder(Backend::SpeedleCloud, &endpoint, Auth::bearer(token), service).build()?;

    let allowed = speedle.is_allowed(&"alice".into(), "get", "/docs/public", &[])?;
    println!("alice get /docs/public: allowed={allowed}");

    let d = speedle.check(&"alice".into(), "delete", "/docs/public", &[])?;
    println!("alice delete /docs/public: allowed={} reason={:?}", d.allowed, d.reason);
    Ok(())
}
```

设置环境变量后运行。`read -rs` 不会把令牌写入 shell 历史，也不会显示在屏幕上：

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
cargo run
```

服务中已有示例策略时，输出为：

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

令牌是服务端密钥，切勿放进浏览器、移动应用或桌面客户端。

## 错误处理

授权请求返回 Deny 并不代表调用出错：`is_allowed` 返回 `false`，`check` 返回 `allowed` 为 false 的结果及原因，如上文的 `delete` 调用。在默认设置下，调用无法得出授权结果时，调用返回 `Err(speedle::Error)`。示例用 `?` 把错误交给 `main`，向标准错误输出 `Speedle error: <消息>`，并以状态码 1 退出。
