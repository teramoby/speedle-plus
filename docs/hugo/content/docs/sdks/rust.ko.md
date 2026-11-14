+++
title = "Rust SDK"
description = "Speedle Rust SDK를 추가하고 로컬 Speedle에 대해 인가 확인 수행"
weight = 50
aliases = ["/ko/docs/api/sdk-rust/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "cargo add로 speedle 0.2.0 크레이트를 추가하고 로컬 Speedle에 대해 is_allowed 및 check를 호출합니다."
lastmod = 2026-11-07T19:43:15+08:00
+++

Rust SDK는 Rust 1.75 이상이 필요합니다. `tokio`와 `reqwest`(rustls)를 기반으로 한 비동기 구성을 제공합니다. `blocking` 기능 플래그를 통해 아래 예제에서 사용하는 동기식 블로킹 클라이언트가 추가됩니다. 크레이트: crates.io의 [`speedle`](https://crates.io/crates/speedle) 0.2.0. 참조 문서: [speedle-sdk/rust](https://github.com/teramoby/speedle-sdk/tree/main/rust).

예제에서는 [SDK 개요](/docs/sdks/#example-policy)에서 설명한 정책을 확인합니다.

## 의존성 추가

새 패키지를 생성하고 의존성을 추가합니다:

```bash
cargo new speedle-example
cd speedle-example
cargo add speedle@0.2.0 --features blocking
```

## 로컬 Speedle 호출

[SDK 개요](/docs/sdks/#example-policy)의 설명에 따라 Speedle을 시작하고 예제 정책을 생성합니다. `src/main.rs`를 다음과 같이 대체합니다:

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

실행합니다:

```bash
cargo run
```

출력:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```


## 오류 처리

거부된 요청은 오류가 아닙니다. 위의 `delete` 호출과 같이 `is_allowed`는 `false`를 반환하고, `check`는 `allowed`가 false이며 이유를 포함하는 결정을 반환합니다. 기본 구성에서 호출이 결정을 반환할 수 없는 경우 `Err(speedle::Error)`를 반환합니다. 예제에서는 오류를 `?`로 `main`에 전달하고 표준 오류로 `Speedle error: <message>`를 출력하여 종료 상태 1로 종료합니다.
