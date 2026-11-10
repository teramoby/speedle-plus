+++
title = "Homebrew 安装"
description = "用 Homebrew 安装 speedle-spctl、speedle-pms 和 speedle-ads，并在本机跑一次演示。"
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Homebrew 安装命令行工具和两个服务，并让它们在 127.0.0.1 上共用一份策略文件。"
+++

在 macOS 或 Linux 上用 [Homebrew](https://brew.sh/) 安装 Speedle 命令行工具：

```bash
brew install teramoby/tap/spctl
```

Formula 的名字是 `spctl`。macOS 上的命令是 `speedle-spctl`，因此安装不会替换 Apple Gatekeeper 的 `/usr/sbin/spctl`。Linux 上同一个程序也可以用 `spctl` 调用。

策略管理服务和授权决策服务是单独的 formula：

```bash
brew install teramoby/tap/speedle-pms
brew install teramoby/tap/speedle-ads
```

命令行工具和两个服务可以一次装齐：

```bash
brew install teramoby/tap/speedle
```

`spctl`、`speedle-pms` 和 `speedle-ads` 各自安装对应系统的发布二进制，并用该发布的 `checksums.txt` 校验。

## 用 Homebrew 做一次演示

启动两个服务。它们监听 `127.0.0.1`，并共用一份文件策略库。

```bash
brew services start speedle-pms
brew services start speedle-ads
```

`speedle-pms` 监听 `127.0.0.1:6733`。`speedle-ads` 监听 `127.0.0.1:6734`。两者都使用 `--store-type file`，策略文件是 `$(brew --prefix)/var/speedle/policies.json`。日志在 `$(brew --prefix)/var/log/`。`--insecure=true` 只用于这次本机回环演示。TLS 见[安全配置](../../security/)。

创建服务和策略。在 Linux 上可以把 `speedle-spctl` 换成 `spctl`。

```bash
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service mysvc
speedle-spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get,del res1" --service-name=mysvc
```

向 ADS 查询 `user1` 能否 get `res1`：

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

响应里包含 `"allowed":true`。两个服务从同一份策略文件读取变更。停止服务：

```bash
brew services stop speedle-ads
brew services stop speedle-pms
```
