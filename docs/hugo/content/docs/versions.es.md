+++
title = "Versiones"
description = "Historial de versiones de Speedle y control de versiones semántico"
weight = 400
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["releases", "versions"]
categories = ["docs"]
bref = "Versiones — Speedle sigue el control de versiones semántico"
lastmod = 2026-11-10T14:45:43+08:00
+++

## Versiones de Speedle

Consulte la página de [GitHub Releases](https://github.com/teramoby/speedle-plus/releases) para ver las versiones publicadas y las notas de la versión.

- [v1.9.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.9.0): los fallos de las funciones personalizadas hacen fallar la decisión, las funciones pueden declarar tipos de parámetros y de resultado, el almacén MySQL usa el esquema versión 2, la compilación js/wasm acepta un llamador de funciones personalizado del host y los registros de aplicación y auditoría pasan a Go `log/slog`.
- [v1.8.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.8.0): se elimina la función experimental de distribución de políticas, las compilaciones desde el código fuente requieren Go 1.27.4 o posterior, la imagen de contenedor incluye el paquete de CA públicos y la documentación añade japonés, coreano y español.
- [v1.7.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.7.0): almacenamiento de políticas MySQL, `spctl check` sin conexión, la lista completa de opciones en la ayuda de los comandos, denegación por defecto ante pánicos y fallos del almacén, y una única imagen multiarquitectura en Docker Hub y GHCR.

[v1.9.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.9.0) es la versión fijada por el recorrido Go embebido actual. Utilice una versión publicada explícita para dependencias de módulo reproducibles; la [guía de modo embebido](../deployment/embedded/) explica la configuración.

## Versión histórica de Oracle Speedle

La versión 0.1.0 fue el lanzamiento original de código abierto, que incluía gestión de políticas, decisiones de autorización, la herramienta de línea de comandos y muestras de integración.
