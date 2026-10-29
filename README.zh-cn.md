<p align="center">
    <img src="/docs/images/sp_logo.png" height="50%" width="50%" class="center"/>
</p>
<p align="center">
    <a href="https://discord.gg/uaY2xVsbbT">
        <img src="https://img.shields.io/badge/Discord-Speedle-5865F2?logo=discord&amp;logoColor=white">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/tags">
        <img src="https://img.shields.io/github/tag/teramoby/speedle-plus.svg">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/issues">
        <img src="https://img.shields.io/github/issues/teramoby/speedle-plus.svg">
    </a>
    <a href="https://996.icu/#/zh_CN">
        <img src="https://img.shields.io/badge/link-996.icu-red.svg">
    </a>
</p>

<p align="right">
<a href="README.md">English Version</a>
</p>

# Speedle+

Speedle+ 是一个用 Go 编写的开源授权引擎。应用向它询问某个主体能否对某个资源执行某个操作，它根据集中管理的策略在数毫秒内给出允许或拒绝，让授权逻辑独立于应用代码。它支持基于角色（RBAC）和基于属性（ABAC）的访问控制；拒绝策略优先于授权策略，没有授权的一律拒绝。用户可以在 Speedle 里管理百万规模级的授权策略。

Speedle+ 包括 Speedle 策略定义语言（Speedle Policy Definition Language，SPDL）、策略管理服务（PMS，即策略管理点 PAP）、授权决策服务（ADS，即策略决策点 PDP，也可以嵌入 Go 应用）、命令行工具 `spctl`，以及 Docker、Kubernetes、Istio 的集成示例。这些组件和常见授权术语的对应关系见[概览](https://speedle.io/zh/docs/overview/)。

Speedle+项目基于Speedle项目<https://github.com/oracle/speedle>，两个项目都遵守UPL协议。

## 我们是谁

我们是Speedle项目的创始团队。因为大家知道的某些原因，我们都离开了Oracle，现在也不是Speedle项目的贡献者了。但是我们还在继续维护这个项目，所以建立了一个新项目Speedle+。

## 文档

请参阅 <https://speedle.io/zh/docs/usecases/>。

## Get Started

请参阅 <https://speedle.io/quick-start/>。   

更详细的内容可以查阅[这里](https://github.com/teramoby/speedle-plus/tree/master/docs/%E4%B8%AD%E6%96%87%E8%B5%84%E6%96%99)。

## 嵌入 Go 应用

授权决策服务可以直接运行在 Go 进程里。先把策略保存为 `policies.spdl`：

```text
[service.library]
[policy]
grant role librarian read,update book
[rolepolicy]
grant user alice librarian
```

用 `go get github.com/teramoby/speedle-plus` 引入模块，然后评估一个请求：

```go
package main

import (
	"fmt"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file" // registers the file store
)

func main() {
	evaluator, err := eval.NewFromFile("policies.spdl", false)
	if err != nil {
		panic(err)
	}
	for _, user := range []string{"alice", "bob"} {
		allowed, reason, err := evaluator.IsAllowed(ads.RequestContext{
			Subject:     &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: user}}},
			ServiceName: "library",
			Resource:    "book",
			Action:      "update",
		})
		fmt.Println(user, allowed, reason, err)
	}
}
```

输出：

```text
alice true GRANT_POLICY_FOUND <nil>
bob false NO_APPLICABLE_POLICIES <nil>
```

详见[嵌入模式](https://speedle.io/zh/docs/deployment-embeded/)。

## 安装

需要 Go 1.25 或更新版本：

```
go install github.com/teramoby/speedle-plus/cmd/...@latest
```

这会把 `spctl`、`speedle-pms` 和 `speedle-ads` 安装到 `$(go env GOPATH)/bin`；如果设置了 `$GOBIN`，则安装到 `$GOBIN`。

## 从源码构建

### 前期准备

-   Go 1.25 或更新版本 <https://go.dev/doc/install>。Speedle+ 支持最近的三个 Go 版本。Go 会自动设置 `GOROOT` 和 `GOPATH`，不需要手动配置。

### 步骤

```
git clone https://github.com/teramoby/speedle-plus.git
cd speedle-plus
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
ls bin
```

输出为 `spctl  speedle-ads  speedle-pms`。

## 运行测试

运行单元测试，不需要任何外部服务：

```
go test $(go list ./pkg/... ./cmd/... ./api/... | grep -v -e adsgrpc -e pmsgrpc -e pmsrest)
```

## 社区

-   Discord：加入 [Speedle 社区](https://discord.gg/uaY2xVsbbT)
-   QQ 群：643201591

## 参与

如果您喜欢Speedle项目并愿意为它做些事情，我们将非常欢迎。您可以：

0. 下载并使用Speedle+，这是对Speedle+项目的最大支持
1. 在<https://github.com/teramoby/speedle-plus>右上角，为Speedle+项目加颗星星
2. 帮助推广Speedle项目，向您的同事，同学，朋友介绍Speedle+
3. 如果不介意的话，您可以告诉我们您如何在项目里使用Speedle+
4. 通过<https://github.com/teramoby/speedle-plus/issues>告诉我们您使用过程中发现的问题
5. 通过<https://github.com/teramoby/speedle-plus/issues>告诉我们您希望在Speedle中出现的新功能
6. 参与Speedle+的开发，通过 Discord 联系我们，我们将告诉您接下来的步骤

