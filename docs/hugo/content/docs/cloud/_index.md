+++
title = "Speedle Cloud"
description = "Speedle Cloud documentation: getting started, account and policy management, application tokens, the authorization API preview, limits, errors and downloads."
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-08T17:11:18+08:00
+++

Speedle Cloud is a hosted access-control service for developers, built on the open-source Speedle policy engine. It is designed to let you manage SPDL authorization and role policies, test decisions in a Playground, and call an authorization API from your application server without operating your own authorization service.

**Speedle Cloud is coming soon.** The Cloud header displays a **Coming soon** badge, and production sign-in is not open on [cloud.speedle.io](https://cloud.speedle.io/) yet. The console workflow and authenticated Authorization Decision Service (ADS) described here are previews of the published service contract. Issuing a token will not enable server-side authorization calls while authenticated ADS remains pending. Verify deployment and identity-provider availability in your chosen stage.

The free public beta is intended for trials, learning and evaluation. It has no SLA, is not recommended for production use, and its features and limits may change.

## Documentation

- [Getting started](/docs/cloud/getting-started/): Sign in, create a service and policy in the console, create an API key, and check authorization from your application.
- [Application tokens](/docs/cloud/tokens/): recent-authentication requirements, one-time token disclosure, metadata pagination, rotation and revocation.
- [Authorization API preview](/docs/cloud/authorization-api/): the IsAllowed request and response contract, a Node.js 22 example, policy snapshots and integration constraints.
- [Limits and errors](/docs/cloud/limits-and-errors/): account quotas, request limits, error responses, rate and quota headers, and retry guidance.

## Downloads

- [English Markdown](https://engine.speedle.io/docs/cloud/quickstart.en.md): all four Cloud chapters in one offline document.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): the canonical checked-in Cloud API specification.
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): generated from the same YAML source bytes.
- [Serving build version](https://cloud.speedle.io/build.json): identifies the serving build; it does not certify identity-provider or endpoint availability.

The schemas include the four session Playground operations and the authenticated ADS contract. Publishing a schema or downloading a guide does not enable a pending endpoint or open sign-in. The YAML and generated JSON are versioned, promoted and restored with the corresponding sealed Cloud build. The English and Chinese Markdown downloads are maintained with these Engine documentation pages.

Read the Cloud [Terms](https://cloud.speedle.io/terms) and [Privacy Policy](https://cloud.speedle.io/privacy).
