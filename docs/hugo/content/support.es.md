+++
draft= false
title = "Soporte"
description = "Respuestas a preguntas comunes sobre Speedle"
toc = false
tocheading = "h2"
tocsidebar = false
icon = "8. Community.svg"
lastmod = 2026-11-07T20:54:01+08:00
+++

## Preguntas frecuentes

### ¿Para quién es Speedle?

Speedle es un motor de autorización de código abierto para aplicaciones que necesitan control de acceso. Puede integrarlo en su sistema o ejecutarlo como un servicio, de modo que las políticas de autorización se administren de forma centralizada en lugar de estar codificadas de forma rígida en el código de la aplicación.

### ¿Por qué otro motor de políticas?

El [inicio rápido](/es/quick-start/) recorre el flujo de trabajo completo en unos 15 minutos. Es la forma más rápida de comparar el modelo de políticas y el flujo de decisiones de Speedle con otras alternativas.

### ¿Cómo contribuyo?

Consulte la [guía de contribución](/es/contribute/#code).

### ¿Cuál es el plan para Speedle?

Speedle se mantiene como un proyecto de código abierto. El trabajo cubre correcciones de errores, documentación y nuevas características según lo necesite la comunidad.

### ¿En qué se diferencia de OPA, Kubernetes RBAC e Istio RBAC?

Speedle, OPA, Kubernetes RBAC e Istio RBAC proporcionan autorización o control de acceso. Kubernetes RBAC e Istio RBAC abordan la autorización dentro de Kubernetes e Istio respectivamente; Speedle y OPA son soluciones de autorización de propósito general. Casbin y Ladon son otras opciones generales. Al compararlos, considere:

1. ¿Es utilizable el lenguaje de políticas?
2. ¿Cuánto tiempo toma una decisión de autorización?
3. ¿Qué tan bien escala?
4. ¿Es suficientemente flexible el modelo de políticas admitido?

### ¿Por qué Speedle no tiene mecanismos de autenticación integrados?

Speedle se centra en la autorización (Authorization). No proporciona autenticación (Authentication).

### ¿Cómo conecto mi propio almacenamiento backend?

Consulte [Almacenamiento conectable](/es/docs/store/).

### ¿Cómo conecto mi propio evaluador de tokens?

Consulte [Evaluador de tokens](/es/docs/assertor/).

<div class="row" style="padding-top: 50px">
    <div class="col col-2"></div>
    <div class="col col-8 center" style="font-size: 25px">
    <div>
        <p>¿No encuentra la respuesta que busca?</p>
        <p>Únase a la comunidad de Discord de Speedle</p>
        <span>
          <a class="button primary started" style="font-size:18px" href="https://discord.gg/uaY2xVsbbT">
            <span>Unirse a Discord →</span>
          </a>
        </span>
    </div>
    </div>
    <div class="col col-2 right">
      <a title="Discord" href="https://discord.gg/uaY2xVsbbT">
        <img class="svg" src="/img/speedle/Discord-color.svg" />
      </a>
    </div>
</div>
