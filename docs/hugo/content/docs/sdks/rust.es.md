+++
title = "Rust SDK"
description = "Añada el SDK de Speedle para Rust y compruebe autorizaciones frente a un Speedle local o Speedle Cloud."
weight = 50
aliases = ["/es/docs/api/sdk-rust/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Añada el crate speedle 0.2.0 con cargo add, y luego llame a is_allowed y check frente a un Speedle local o Speedle Cloud."
lastmod = 2026-11-07T19:43:15+08:00
+++

El SDK de Rust requiere Rust 1.75 o posterior. Es asíncrono sobre `tokio` y `reqwest` (rustls); la característica `blocking` añade el cliente bloqueante utilizado a continuación. Crate: [`speedle`](https://crates.io/crates/speedle) 0.2.0 en crates.io. Referencia: [speedle-sdk/rust](https://github.com/teramoby/speedle-sdk/tree/main/rust).

Los ejemplos comprueban la política descrita en la [descripción general de los SDK](/es/docs/sdks/#example-policy).

## Añadir la dependencia

En un nuevo paquete:

```bash
cargo new speedle-example
cd speedle-example
cargo add speedle@0.2.0 --features blocking
```

## Llamar a un Speedle local

Inicie Speedle y cree la política de ejemplo como se describe en la [descripción general de los SDK](/es/docs/sdks/#example-policy). Sustituya `src/main.rs` por:

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

Ejecución:

```bash
cargo run
```

Salida:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

## Llamar a Speedle Cloud

> **Speedle Cloud estará disponible próximamente.**

El programa lee el nombre del servicio desde `SPEEDLE_SERVICE` y una clave API de cuenta con ámbito `ads` desde `SPEEDLE_TOKEN`. `SPEEDLE_ENDPOINT` anula el endpoint predeterminado `https://cloud.speedle.io`. Sustituya `src/main.rs` por:

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

Configure las variables de entorno y ejecútelo. `read -rs` evita que la clave API quede registrada en el historial de la terminal o en pantalla:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
cargo run
```

Salida, cuando el servicio cuenta con la política de ejemplo:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

La clave API es un secreto del lado del servidor. Nunca lo distribuya a navegadores, aplicaciones móviles o clientes de escritorio.

## Gestión de errores

Una solicitud denegada no constituye un error: `is_allowed` devuelve `false`, y `check` devuelve una decisión con `allowed` en false y el motivo, tal como se muestra en la llamada a `delete` anterior. Con la configuración predeterminada, cuando una llamada no puede devolver una decisión, devuelve `Err(speedle::Error)`. Los ejemplos propagan el error a `main` con `?`, imprimen `Speedle error: <mensaje>` en la salida de error estándar y finalizan con el código de salida 1.
