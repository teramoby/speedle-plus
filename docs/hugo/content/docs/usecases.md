+++
title = "Usecases"
keywords = ["Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "centralized authorization", "authorization policy management", "API authorization", "microservices authorization", "cloud-native authorization"]
description = "Who, What & Where?"
weight = 1
draft = false
toc = true
tocheading = "h2"
tags = ["guide", "usecase"]
categories = ["docs"]
bref = "Usecases for Speedle"
+++

## Application specific policies

Alex Chan is a developer who is implementing a simple application with golang. His application requires Authorization and he doesn’t want to re-invent the wheel. Alex can use an open source choice like Speedle for Authorization.

## Centalized policy enforcement and management

Jin Chen is a security manager of multinational company. His team monitor dozens of internal systems. Different systems use different authorization mechanisms, different policy languages. They are eager to find a solution to manage all these systems efficiently.

- Centralized policy management
- Flexible policy model, subject to verification of each system’s required semantics
- Easy to integrate with different systems, from OS to middleware to applications
- Scalable to support all the systems
- Efficient

## Single plane of glass for authorization across the fleet

TTH Inc. is a startup cloud company. They offer a cloud native environment to their customers. The environment consists of Linux, Docker, Kubernetes, and Istio. Security is top priority for TTH offerings. They define a comprehensive authorization model, create policies in Linux, Docker, Kubernetes, Istio in different terminals and using different policy languages. Can we make their lives easier?

- Centralized policy management
- Unified authorization policy model for Linux, Docker, Kubernetes, Istio
- Scalable for cloud operation
- Efficient
- Integration requires identity/attribute mapping, enforcement and validation for the actual platform version
- Open Source

## Performance scope

Measure latency, throughput and policy capacity with the actual revision, hardware, interface, policy data and cache conditions. The current REST wrapper defaults to 1000 requests/second with a burst of 200; this is not an embedded Go engine benchmark. Additional instances still require capacity and policy-propagation validation.
