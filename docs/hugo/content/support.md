+++
draft= false
title = "Support"
description = "Answers to common questions about Speedle"
toc = false
tocheading = "h2"
tocsidebar = false
icon = "8. Community.svg"
lastmod = 2026-11-07T20:54:01+08:00
+++

## FAQs

### Who is Speedle for?

Speedle is an open-source authorization engine for applications that need access control. You can embed it in your system or run it as a service, so authorization policies stay centrally managed instead of being hard-coded in application code.

### Why another policy engine?

The [Quick Start](/quick-start/) walks through the full workflow in about 15 minutes. It is the quickest way to compare Speedle's policy model and decision flow with the alternatives.

### How do I contribute?

See the [contribution guide](/contribute/#code).

### What is the plan for Speedle?

Speedle is maintained as an open-source project. Work covers bug fixes, documentation and new features as the community needs them.

### How is it different from OPA, Kubernetes RBAC, Istio RBAC?

Speedle, OPA, Kubernetes RBAC and Istio RBAC all provide authorization or access control. Kubernetes RBAC and Istio RBAC address authorization inside Kubernetes and Istio respectively; Speedle and OPA are general-purpose authorization solutions. Casbin and Ladon are other general-purpose options. When comparing them, consider:

1. Is the policy language usable? Each product provides one, so create a policy in each and compare.
2. How long does one authorization decision take?
3. How well does it scale?
4. Is the supported policy model flexible enough?

### Why does Speedle have no built-in authentication mechanisms?

Speedle focuses on authorization. It does not provide authentication.

### How do I plug in my own backend store?

See [Pluggable Storage](/docs/store/).

### How do I plug in my own token assertor?

See [token assertor](/docs/assertor/).

<div class="row" style="padding-top: 50px">
    <div class="col col-2"></div>
    <div class="col col-8 center" style="font-size: 25px">
    <div>
        <p>Can’t see the answer you’re looking for?</p>
        <p>Join the Speedle Discord community</p>
        <span>
          <a class="button primary started" style="font-size:18px" href="https://discord.gg/uaY2xVsbbT">
            <span>Join Discord →</span>
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
