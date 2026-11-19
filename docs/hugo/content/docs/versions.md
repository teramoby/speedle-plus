+++
title = "Versions"
description = "Speedle release history and semantic versioning"
weight = 400
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["releases", "versions"]
categories = ["docs"]
bref = "Releases — Speedle follows semantic versioning"
lastmod = 2026-11-10T14:45:43+08:00
+++

## Speedle releases

Use the [GitHub Releases](https://github.com/teramoby/speedle-plus/releases) page for published versions and release notes.

- [v1.9.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.9.0): custom-function failures now fail the decision, functions can declare parameter and result types, the MySQL store uses schema version 2, the js/wasm build accepts a host custom-function caller, and application and audit logging moved to Go `log/slog`.
- [v1.8.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.8.0): removed the experimental policy-distribution feature, source builds require Go 1.27.4 or newer, the container image includes the public CA bundle, and the documentation gained Japanese, Korean and Spanish.
- [v1.7.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.7.0): MySQL policy storage, offline `spctl check`, the full option list in command help, fail-closed authorization on panics and store outages, and one multi-architecture image on Docker Hub and GHCR.

[v1.9.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.9.0) is the version pinned by the current embedded Go walkthrough. Use an explicit released version for reproducible module dependencies; the [embedded guide](../deployment/embedded/) explains the setup.

## Historical Oracle Speedle release

Version 0.1.0 was the original open-source release, including policy management, authorization decisions, the command-line tool and integration samples. The historical 0.2.0 planning note is not the current Speedle release roadmap.
