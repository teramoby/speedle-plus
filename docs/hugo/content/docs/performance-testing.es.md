+++
title = "Guía de pruebas de rendimiento"
description = "Ejecute una comparación justa y reproducible de motores de autorización en su propia máquina con ayuda de una herramienta de IA."
weight = 15
misc = true
draft = false
toc = false
singlelinetitle = true
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tags = ["v0.1.0", "performance", "benchmark", "policy"]
categories = ["docs"]
lastmod = 2026-11-11T12:00:00+08:00
bref = "Pegue este prompt en su agente de programación con IA para comparar Speedle con los proyectos de autorización que elija en su propia máquina."
+++

Aquí no publicamos ningún informe de rendimiento; los resultados dependen de su hardware, sus políticas y las versiones fijadas.

```text
Primero pregúnteme qué proyectos de autorización de código abierto quiero comparar con Speedle. Pregunte también por los tamaños de los conjuntos de políticas y el tiempo disponible, proponiendo por defecto 10, 100 y 1,000 políticas y 30 minutos. Espere mi respuesta antes de hacer cualquier otra cosa.

Después, trabaje en un directorio local nuevo y haga una comparación justa en la misma máquina dentro de ese tiempo. Fije todas las versiones de motores y herramientas. Cree políticas RBAC y ABAC equivalentes y solicitudes comunes con decisiones esperadas de permitir/denegar; valide cada motor contra esas decisiones antes de medir. Documente las diferencias semánticas y excluya los casos no compatibles de las comparaciones directas.

Mida el escalado con las cantidades de políticas acordadas. Separe la carga de políticas de la evaluación en estado estable, y las mediciones embebidas de las de red/API. Use las mismas solicitudes, concurrencia y configuración de caché, caliente cada motor y ejecute al menos cinco repeticiones. Informe de la latencia p50/p95/p99 y del rendimiento con unidades, duración y variabilidad. Informe solo de cifras medidas; marque los casos no ejecutados y los fallos. Guarde localmente scripts, políticas, comandos exactos, versiones, detalles del entorno y salida sin procesar para poder reproducir los resultados.
```

Copie el prompt, péguelo en su agente y responda a sus preguntas.
