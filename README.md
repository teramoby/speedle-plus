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
    <a href="https://github.com/teramoby/speedle-plus/workflows/Speedle%20Plus%20CI/badge.svg">
        <img src="https://github.com/teramoby/speedle-plus/workflows/Speedle%20Plus%20CI/badge.svg">
    </a>
    <a href="https://goreportcard.com/report/github.com/teramoby/speedle-plus">
        <img src="https://goreportcard.com/badge/github.com/teramoby/speedle-plus">
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

We are the founding members of Speedle project. Now we are not Oracle employees and consequently not contributors of Speedle project on GitHub anymore. But we still stay with Speedle project. We create a new repo under https://github.com/teramoby/speedle-plus and maintain the new project here now.

## Documentation

Latest documentations are available at <https://speedle.io/docs>.

## Get Started

See Getting Started at <https://speedle.io/quick-start/>.

## Build

### Prerequisites

-   GO 1.13 or greater <https://golang.org/doc/install>
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

-   Join us on Discord: [Speedle community](https://discord.gg/uaY2xVsbbT)
-   Mailing List: speedle-users@googlegroups.com

## Get Involved

-   Learn how to [contribute](CONTRIBUTING.md)
-   See [issues](https://github.com/teramoby/speedle-plus/issues) for issues you can help with
