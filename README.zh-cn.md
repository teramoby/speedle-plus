<p align="center">
    <img src="/docs/images/sp_logo.png" height="50%" width="50%" class="center"/>
</p>
<p align="center">
    <a href="https://join.slack.com/t/speedleproject/shared_invite/enQtNTUzODM3NDY0ODE2LTg0ODc0NzQ1MjVmM2NiODVmMThkMmVjNmMyODA0ZWJjZjQ3NDc2MjdlMzliN2U4MDRkZjhlYzYzMDEyZTgxMGQ">
        <img src="https://img.shields.io/badge/slack-speedle-red.svg">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/tags">
        <img src="https://img.shields.io/github/v/tag/teramoby/speedle-plus">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/issues">
        <img src="https://img.shields.io/github/issues/teramoby/speedle-plus.svg">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/actions/workflows/ci.yml">
        <img src="https://github.com/teramoby/speedle-plus/actions/workflows/ci.yml/badge.svg">
    </a>
    <a href="https://goreportcard.com/report/github.com/teramoby/speedle-plus">
        <img src="https://goreportcard.com/badge/github.com/teramoby/speedle-plus">
    </a>
    <a href="https://pkg.go.dev/github.com/teramoby/speedle-plus">
        <img src="https://pkg.go.dev/badge/github.com/teramoby/speedle-plus.svg">
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

我们是Speedle项目的创始团队。因为大家知道的某些原因，我们都离开了Oracle，现在也不是Speedle项目的贡献者了。但是我们还在继续维护这个项目，所以建立了一个新项目Speedle+。

## 系统架构

Speedle+ 由三个核心组件组成：

```
                    +-----------+
                    |   spctl   |  命令行工具，用于管理策略
                    +-----+-----+
                          |
              +-----------+-----------+
              |                       |
              v                       v
      +-------+------+        +-------+------+
      |     PMS      |        |     ADS      |
      |  策略管理服务 |  推送  |  授权决策服务 |
      |              +------->+              |
      +-------+------+        +-------+------+
              |                       |
              v                       v
      +-------+------+        +-------+------+
      |   策略存储   |        |   策略存储   |
      | (file/etcd/  |<-------+   (只读)     |
      |  mongodb)    |  监听  |              |
      +--------------+        +--------------+
```

- **spctl**: 命令行工具，用于创建、查询、更新和删除策略、服务和函数。通过 REST API 与 PMS 通信。
- **PMS (策略管理服务)**: 提供 REST/gRPC API，管理策略生命周期，将策略写入后端存储。
- **ADS (授权决策服务)**: 实时评估授权请求。将策略加载到内存缓存中，并监听存储变更以保持同步。

## PDL 语法示例

Speedle 使用易于阅读的策略定义语言（PDL）。以下是一些基本示例：

**定义服务:**
```pdl
service bookstore {
    // 授权策略：允许 "employee" 组用户读取书籍
    grant policy readBooks {
        principals: ["group:employee"]
        permissions: [
            {resource: "book", actions: ["read"]}
        ]
    }

    // 拒绝策略：禁止用户 "bob" 删除书籍
    deny policy bobCannotDelete {
        principals: ["user:bob"]
        permissions: [
            {resource: "book", actions: ["delete"]}
        ]
    }

    // 角色策略：将 "reader" 角色授予 "intern" 组
    grant role policy readerRole {
        principals: ["group:intern"]
        roles: ["reader"]
        resources: ["book"]
    }

    // 带条件表达式的策略
    grant policy weekdayAccess {
        principals: ["user:*"]
        permissions: [
            {resource: "book", actions: ["read"]}
        ]
        condition: request_weekday != "Saturday" && request_weekday != "Sunday"
    }
}
```

## 配置文件示例

Speedle 使用 JSON 配置文件：

```json
{
    "storeConfig": {
        "storeType": "file",
        "storeProps": {
            "FileLocation": "/etc/speedle/policies.json"
        }
    },
    "enableWatch": true,
    "serverConfig": {
        "endpoint": "0.0.0.0:6733",
        "insecure": "true"
    },
    "logConfig": {
        "level": "info",
        "formatter": "json",
        "rotationConfig": {
            "filename": "/var/log/speedle/speedle.log",
            "maxSize": 10,
            "maxBackups": 5,
            "maxAge": 30
        }
    },
    "auditLogConfig": {
        "level": "info",
        "formatter": "json",
        "rotationConfig": {
            "filename": "/var/log/speedle/audit.log",
            "maxSize": 10,
            "maxBackups": 5,
            "maxAge": 90
        }
    }
}
```

## 文档

- 请参阅 <https://speedle.io/zh/docs/usecases/>
- Go 包参考文档: <https://pkg.go.dev/github.com/teramoby/speedle-plus>

## Get Started

请参阅 <https://speedle.io/quick-start/>。   

更详细的内容可以查阅[这里](https://github.com/teramoby/speedle-plus/tree/master/docs/%E4%B8%AD%E6%96%87%E8%B5%84%E6%96%99)。

## 构建

### 前期准备

- Go 1.22 或更高版本 <https://go.dev/doc/install>

### 步骤

```
$ go install github.com/teramoby/speedle-plus/cmd/spctl@latest
$ go install github.com/teramoby/speedle-plus/cmd/speedle-ads@latest
$ go install github.com/teramoby/speedle-plus/cmd/speedle-pms@latest
$ ls $(go env GOPATH)/bin
spctl  speedle-ads  speedle-pms
```

或者克隆仓库并从源码构建：

```
$ git clone https://github.com/teramoby/speedle-plus.git
$ cd speedle-plus
$ make build
```

## 运行测试

```
$ cd speedle-plus
$ make test
```

## 社区

- 我们推荐大家使用Slack，Slack是一个非常优秀的沟通工具，Speedle的Slack社区很活跃，里面的每一个问题都会在24小时内得到回复。[#speedle-chinese](https://join.slack.com/t/speedleproject/shared_invite/zt-72fgiyuo-QKJAhHAqVbn17KRFbd7aZw)
- 如果大家访问Slack有困难，可以加入QQ群。群号：643201591

## 参与

如果您喜欢Speedle项目并愿意为它做些事情，我们将非常欢迎。您可以：

0. 下载并使用Speedle+，这是对Speedle+项目的最大支持
1. 在<https://github.com/teramoby/speedle-plus>右上角，为Speedle+项目加颗星星
2. 帮助推广Speedle项目，向您的同事，同学，朋友介绍Speedle+
3. 如果不介意的话，您可以告诉我们您如何在项目里使用Speedle+
4. 通过<https://github.com/teramoby/speedle-plus/issues>告诉我们您使用过程中发现的问题
5. 通过<https://github.com/teramoby/speedle-plus/issues>告诉我们您希望在Speedle中出现的新功能
6. 参与Speedle+的开发，通过Slack联系我们，我们将告诉您接下来的步骤
