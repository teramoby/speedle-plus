+++
title = "Arquitectura"
description = "Componentes básicos de Speedle"
weight = 10
draft = false
toc = false
tocheading = "h2"
tocsidebar = false
bref = "Arquitectura de Speedle"
lastmod = 2026-11-08T08:33:54+08:00
+++

## Módulos

Speedle es un motor de autorización compuesto por estos componentes:

- API del Servicio de Gestión de Políticas (PMS): gestiona las políticas de autorización y de roles, y los objetos a partir de los cuales se crean.
- Repositorio de políticas: almacena todos los artefactos de políticas. Puede ser un archivo JSON o un almacén persistente como una base de datos o etcd.
- API del Servicio de Decisiones de Autorización (ADS): evalúa las solicitudes de autorización frente a las políticas aplicables y devuelve decisiones GRANT/DENY.

<img src="/img/speedle/spdlarch.jpg" width="524" height="241" loading="lazy" />

Si está familiarizado con el modelo XACML, el Servicio de Gestión de Políticas (PMS) sirve como PAP y el Servicio de Decisiones de Autorización (ADS) sirve como PDP.
