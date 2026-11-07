+++
draft= false
title = "支持"
description = "关于 Speedle 的常见问题解答"
toc = false
tocheading = "h2"
tocsidebar = false
icon = "8. Community.svg"
+++

## 常见问题

### Speedle 适合谁使用？

Speedle 是一个开源授权引擎，适合需要在应用中实现访问控制的团队。你可以把它嵌入系统，也可以作为独立服务运行，让授权策略集中管理，而不是写死在应用代码里。

### 为什么还需要另一个策略引擎？

[快速入门](/zh/quick-start/)用大约 15 分钟走完整个流程，是了解 Speedle 策略模型和授权决策方式、并与其他方案比较的最快途径。

### 如何参与贡献？

请参阅[贡献指南](/zh/contribute/#代码贡献)。

### Speedle 的发展计划是什么？

Speedle 以开源项目的形式持续维护，工作包括修复缺陷、更新文档，以及根据社区需要增加功能。

### Speedle 与 OPA、Kubernetes RBAC、Istio RBAC 有什么区别？

Speedle、OPA、Kubernetes RBAC 和 Istio RBAC 都提供授权或访问控制能力。Kubernetes RBAC 和 Istio RBAC 分别解决 Kubernetes 和 Istio 内部的授权问题；Speedle 和 OPA 是通用授权方案，Casbin 和 Ladon 也是常见的通用选择。比较时可以考虑：

1. 策略语言是否易用？这些产品都提供策略定义语言，可以各写一条策略进行对比。
2. 单次授权决策需要多长时间？
3. 扩展性如何？
4. 支持的策略模型是否足够灵活？

### 为什么 Speedle 不内置认证机制？

Speedle 专注于授权，不提供认证功能。

### 如何接入自己的后端存储？

请参阅[可插拔存储](/zh/docs/store/)。

### 如何接入自己的令牌断言服务？

请参阅[令牌断言服务](/zh/docs/assertor/)。

<div class="row" style="padding-top: 50px">
    <div class="col col-2"></div>
    <div class="col col-8 center" style="font-size: 25px">
    <div>
        <p>还没找到需要的答案？</p>
        <p>加入 Speedle Discord 社区，向社区提问和交流。</p>
        <p>
          <a class="button primary started" style="font-size:18px" href="https://discord.gg/uaY2xVsbbT">
            <span>加入 Discord →</span>
          </a>
        </p>
    </div>
    </div>
    <div class="col col-2 right">
      <a title="Discord" href="https://discord.gg/uaY2xVsbbT">
        <img class="svg" src="/img/speedle/Discord-color.svg" />
      </a>
    </div>
</div>
