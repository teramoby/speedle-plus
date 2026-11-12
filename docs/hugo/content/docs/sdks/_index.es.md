+++
title = "SDK"
description = "Bibliotecas cliente para comprobaciones de autorización de Speedle en Java, Python, Go, .NET, Rust y Node.js/TypeScript."
weight = 305
draft = false
toc = true
linksection = true
tocheading = "h2"
tags = ["sdk"]
categories = ["docs"]
bref = "Bibliotecas cliente para la API de decisiones de Speedle en seis lenguajes. Compatibles con un Speedle ADS autohospedado y con Speedle Cloud (próximamente)."
lastmod = 2026-11-10T14:45:43+08:00
+++

Los SDK de Speedle son bibliotecas cliente para la API de decisiones `POST /authz-check/v1/is-allowed`. Funcionan tanto con un Speedle ADS autohospedado como con Speedle Cloud. Cada página de lenguaje muestra cómo agregar el paquete y proporciona un programa completo y probado para ambos entornos. El código fuente y la referencia completa se encuentran en [teramoby/speedle-sdk](https://github.com/teramoby/speedle-sdk). Para ejecutar PMS y ADS para estos ejemplos, consulte [Despliegue](/es/docs/deployment/) y la [Imagen de Docker](/es/docs/deployment/docker/).

## Paquetes

Java, Python, Go, .NET y Rust están en la versión 0.2.0. `@speedle/sdk` está publicado en npm. Cada página de lenguaje muestra el comando de instalación correspondiente.

- [Java](/es/docs/sdks/java/): Maven Central `io.speedle:speedle-sdk`, publicado.
- [Python](/es/docs/sdks/python/): PyPI `speedle`, publicado.
- [Go](/es/docs/sdks/go/): Módulo de Go `github.com/teramoby/speedle-sdk/go`, publicado.
- [C#/.NET](/es/docs/sdks/dotnet/): NuGet `Speedle.Sdk`, publicado.
- [Rust](/es/docs/sdks/rust/): crates.io `speedle`, publicado.
- [Node.js / TypeScript](/es/docs/sdks/node/): npm `@speedle/sdk`, publicado.

## Funcionalidades

| Funcionalidad | Estado |
| --- | --- |
| `isAllowed`: devuelve la decisión de autorización | Los seis SDK |
| `check`: devuelve la decisión y el motivo (p. ej., ninguna política aplicable) | Los seis SDK |
| Speedle ADS autohospedado | Los seis SDK |
| Speedle Cloud | Próximamente |

Los nombres de los métodos siguen las convenciones de cada lenguaje: `is_allowed` y `check` en Python y Rust, `IsAllowed` y `Check` en Go, `IsAllowedAsync` y `CheckAsync` en .NET.

Para otras operaciones, como la consulta de permisos concedidos, diagnóstico y gestión de políticas, utilice las API REST: la [API de decisiones de autorización](/es/docs/api/decision_api/) y la [API de gestión de políticas](/es/docs/api/management_api/).

## Política de ejemplo <a id="example-policy"></a>

Todos los ejemplos comprueban si el usuario `alice` puede realizar las acciones `get` y `delete` sobre el recurso `/docs/public` en el servicio `docs`. Para obtener los resultados documentados desde un Speedle local, compílelo e inicie PMS en el puerto 6733 y ADS en el puerto 6734 como se describe en el [Inicio rápido](/es/quick-start/), luego cree el servicio y la política:

```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

Los ejemplos locales se conectan a `http://localhost:6734` sin credenciales. Los SDK solo aceptan `http://` en texto plano para hosts de bucle invertido (loopback); utilice HTTPS para cualquier otro ADS.

Los ejemplos de Speedle Cloud se conectan a `https://cloud.speedle.io` y leen el nombre del servicio y una clave API de cuenta con ámbito `ads` desde las variables de entorno `SPEEDLE_SERVICE` y `SPEEDLE_TOKEN`. Cree la misma política en ese servicio para obtener las mismas respuestas. La clave API es un secreto del lado del servidor: nunca la exponga en navegadores, aplicaciones móviles ni clientes de escritorio.
