+++
title = "Performance testing guide"
description = "Run a fair, reproducible authorization-engine comparison on your own machine with the help of an AI coding tool."
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
bref = "Paste this prompt into your AI coding agent to compare Speedle with authorization projects you choose on your own machine."
+++

We publish no performance report here; results depend on your hardware, policies and pinned versions.

```text
First ask me which open-source authorization projects I want to compare with Speedle. Also ask for policy-set sizes and a time budget, offering defaults of 10, 100 and 1,000 policies and 30 minutes. Wait for my answer before doing anything else.

Then work in a fresh local directory and run a fair comparison on the same machine within that budget. Pin all engine and tool versions. Build equivalent RBAC and ABAC policies and shared requests with expected allow/deny decisions; validate every engine against those decisions before timing. Document semantic differences and exclude unsupported cases from direct comparisons.

Measure scaling across the agreed policy counts. Separate policy loading from steady-state evaluation, and embedded measurements from network/API measurements. Use the same requests, concurrency and cache settings, warm up each engine and run at least five repetitions. Report latency p50/p95/p99 and throughput with units, run durations and variability. Report only measured numbers; mark unrun cases and failures explicitly. Save the scripts, policies, exact commands, versions, environment details and raw output locally so I can reproduce the results.
```

Copy the prompt, paste it into your agent and answer its questions.
