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
</p>

<p align="right">
<a href="README.zh-cn.md">中文版</a>
</p>

# Speedle+

Speedle+ is a general purpose authorization engine. It allows users to construct their policy model with user-friendly policy definition language and get authorization decision in milliseconds based on the policies. Speedle is very user-friendly, efficient, and extremely scalable. 

Speedle+ open source project consits of a policy definition language, policy management module, authorization runtime module, commandline tool, and integration samples with popular systems.

Speedle+ is based on Speedle open source project which is hosted at https://github.com/oracle/speedle under UPL.

## Who are we

We are the founding members of Speedle project. For some reasons, we all left Oracle and are consequently not contributors of Speedle project on GitHub anymore. But we still stay with Speedle project. We create a new repo under https://github.com/teramoby/speedle-plus and maintain the new project here now.

## Documentation

Latest documentations are available at <https://speedle.io/docs>.

## Get Started

See Getting Started at <https://speedle.io/quick-start/>.

## Build

### Prerequisites

-   GO 1.10.1 or greater <https://golang.org/doc/install>
-   Set `GOROOT` and `GOPATH` properly

### Step

```
$ go get github.com/teramoby/speedle-plus/cmd/...
$ ls $GOPATH/bin
spctl  speedle-ads  speedle-pms
```

## Test

```
$ cd $GOPATH/src/github.com/teramoby/speedle-plus
$ make test
```

## Get Help

-   Join us on Slack: [#speedle-users](https://join.slack.com/t/speedleproject/shared_invite/enQtNTUzODM3NDY0ODE2LTg0ODc0NzQ1MjVmM2NiODVmMThkMmVjNmMyODA0ZWJjZjQ3NDc2MjdlMzliN2U4MDRkZjhlYzYzMDEyZTgxMGQ)
-   Mailing List: speedle-users@googlegroups.com

## Get Involved

-   Learn how to [contribute](CONTRIBUTING.md)
-   See [issues](https://github.com/oracle/speedle/issues) for issues you can help with
