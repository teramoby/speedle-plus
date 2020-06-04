<p align="center">
    <img src="/docs/images/Speedle_logo_b.svg" height="50%" width="50%" class="center"/>
</p>
<p align="center">
    <a href="https://join.slack.com/t/speedleproject/shared_invite/enQtNTUzODM3NDY0ODE2LTg0ODc0NzQ1MjVmM2NiODVmMThkMmVjNmMyODA0ZWJjZjQ3NDc2MjdlMzliN2U4MDRkZjhlYzYzMDEyZTgxMGQ">
        <img src="https://img.shields.io/badge/slack-speedle-red.svg">
    </a>
    <a href="https://github.com/teramoby/SpeedlePlus/tags">
        <img src="https://img.shields.io/github/tag/teramoby/SpeedlePlus.svg">
    </a>
    <a href="https://github.com/teramoby/SpeedlePlus/issues">
        <img src="https://img.shields.io/github/issues/teramoby/SpeedlePlus.svg">
    </a>
    <a href="https://goreportcard.com/report/github.com/teramoby/SpeedlePlus">
        <img src="https://goreportcard.com/badge/github.com/teramoby/SpeedlePlus">
    </a>
    <a href="https://996.icu/#/zh_CN">
        <img src="https://img.shields.io/badge/link-996.icu-red.svg">
    </a>
</p>

<p align="right">
<a href="README.md">English Version</a>
</p>

# Speedle+

Speedle+是一个通用授权引擎。用户可以使用它非常易用的策略定义语言构建自己的授权模型，并能在数毫秒之内得到授权结果。Speedle非常易用，非常高效，而且可扩展性能力非常强。用户可以在Speedle里面管理百万规模级的授权策略。

作为一个开源项目，Speedle包括策略定义语言（SPDL)，策略管理模块，授权决策模块，命令行工具，以及数个和流行系统集成的示例。

Speedle+项目基于Speedle项目<https://github.com/oracle/speedle>，两个项目都遵守UPL协议。

## 我们是谁

我们是Speedle项目的创始团队。因为大家都知道的某些原因，我们都离开了Oracle，现在也不是Speedle项目的贡献者了。但是我们还在继续维护这个项目，所以建立了一个新项目Speedle+。

## 文档

请参阅 <https://speedle.io/docs>. 请注意目前我们还在紧张地准备中文版文档。您可以先参阅英文版，如果有问题可以在Slack或者QQ群里讨论。

## Get Started

请参阅 <https://speedle.io/quick-start/>。   

更详细的内容可以查阅[这里](https://github.com/oracle/speedle/tree/master/docs/%E4%B8%AD%E6%96%87%E8%B5%84%E6%96%99)。

## 构建

### 前期准备

-   GO 1.10.1 or greater <https://golang.org/doc/install>
-   设置环境变量 `GOROOT` 和 `GOPATH` 

### 步骤

```
$ go get github.com/teramoby/speedle-plus/cmd/...
$ ls $GOPATH/bin
spctl  speedle-ads  speedle-pms
```

## 运行测试

```
$ cd $GOPATH/src/github.com/teramoby/speedle-plus
$ make test
```

## 社区

-   我们推荐大家使用Slack，Slack是一个非常优秀的沟通工具，Speedle的Slack社区很活跃，里面的每一个问题都会在24小时内得到回复。[#speedle-chinese](https://join.slack.com/t/speedleproject/shared_invite/enQtNTUzODM3NDY0ODE2LTg0ODc0NzQ1MjVmM2NiODVmMThkMmVjNmMyODA0ZWJjZjQ3NDc2MjdlMzliN2U4MDRkZjhlYzYzMDEyZTgxMGQ)
-   如果大家访问Slack有困难，可以加入QQ群。群号：643201591

## 参与

如果您喜欢Speedle项目并愿意为它做些事情，我们将非常欢迎。您可以：

0. 下载并使用Speedle+，这是对Speedle+项目的最大支持
1. 在<https://github.com/teramoby/speedle-plus>右上角，为Speedle+项目加颗星星
2. 帮助推广Speedle项目，向您的同事，同学，朋友介绍Speedle+
3. 如果不介意的话，您可以告诉我们您如何在项目里使用Speedle+
4. 通过<https://github.com/teramoby/speedle-plus/issues>告诉我们您使用过程中发现的问题
5. 通过<https://github.com/teramoby/speedle-plus/issues>告诉我们您希望在Speedle中出现的新功能
6. 参与Speedle+的开发，通过Slack联系我们，我们将告诉您接下来的步骤

