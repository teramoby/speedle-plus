+++
title = "令牌用户身份断言"
description = "配置令牌断言服务并验证带身份域的授权请求"
date = 2019-01-21T09:28:30+08:00
weight = 60
draft = false
bref = ""
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
tags = ["Identity", "Token asserter"]
+++

## 令牌断言服务负责什么？

已配置的断言服务验证令牌，并返回显式主体及其身份域，再由 Speedle 使用这些身份评估策略。生产实现必须验证签名或 introspection 结果、签发者、受众、有效期及身份提供方要求。仅有 webhook 接口并不意味着自动兼容任意 OAuth/OIDC 令牌。

[令牌断言 API](/docs/api/asserter_api/)中的 `errCode` 为整数，可选的属性是**对象**，不是带类型的属性数组。`asserterWebhookConfig.endpoint` 是完整 URL，下面的示例使用 `/v1/assert`。

## 启动确定性的本地 mock

此 mock 只识别 `githubtoken`/`github`、`googletoken`/`google`、`gitlabtoken`/`gitlab` 三组输入，分别表示对应域的 `user1`。令牌与提供方不匹配时失败。**它是测试 fixture，不验证真实令牌。** 不会记录令牌值。仅绑定回环地址，生产环境必须替换为真实身份集成。

从仓库根目录在单独终端运行：

<!-- docs-check: mock-asserter -->
```bash
go run ./samples/docs/asserter
```


<!-- docs-source: samples/docs/asserter/main.go -->
```go
// A deterministic mock for the token-assertion tutorial. Never use for real tokens.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"
)

type principal struct {
	Type string `json:"type"`
	Name string `json:"name"`
	IDD  string `json:"idd"`
}
type response struct {
	Principals []principal `json:"principals,omitempty"`
	ErrCode    int         `json:"errCode"`
	ErrMessage string      `json:"errMessage,omitempty"`
}

func assert(w http.ResponseWriter, r *http.Request) {
	send := func(status int, value response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		send(405, response{ErrCode: 405, ErrMessage: "GET required"})
		return
	}
	// Fixed fixtures model successful assertion. This is not signature validation.
	domains := map[string]string{"githubtoken": "github", "googletoken": "google", "gitlabtoken": "gitlab"}
	domain, ok := domains[r.Header.Get("x-token")]
	if !ok || domain != r.Header.Get("x-idp") {
		send(401, response{ErrCode: 401, ErrMessage: "invalid demonstration token or identity provider"})
		return
	}
	if allowed := r.Header.Get("x-allowedIDD"); allowed != "" {
		found := false
		for _, name := range strings.Split(allowed, ",") {
			found = found || strings.TrimSpace(name) == domain
		}
		if !found {
			send(403, response{ErrCode: 403, ErrMessage: "identity domain is not allowed"})
			return
		}
	}
	send(200, response{Principals: []principal{{Type: "user", Name: "user1", IDD: domain}}})
}
func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "Local demonstration endpoint")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/assert", assert)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}
```


## 运行示例前

先按[快速入门](/quick-start/)编译仓库根目录下的 `./bin/spctl`、`./bin/speedle-pms` 和 `./bin/speedle-ads`，需要 Go 1.26 或更高版本、Bash、curl 和 Python 3。下面各终端都从仓库根目录运行。为本页使用一个新的策略文件；仅在两个服务都停止后重建该文件。

初始化空策略文件：

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

终端一启动 PMS，并保持运行：

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

终端二启动 ADS，并保持运行：

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true --asserter-endpoint http://127.0.0.1:8080/v1/assert
```

在第三个终端运行后续命令。PMS/ADS 共享同一个文件，策略更新通过 watch 异步到达 ADS；先查询确认已更新，再依赖新策略做决策。`--insecure=true` 仅用于回环地址上的本地示例，生产连接见[安全配置](/docs/security/)。示例显式指定 PMS 地址，不修改 `~/.spctlconfig`。完成后在两个服务终端按 Ctrl-C。


## 创建带身份域的测试策略

<!-- docs-check: idd-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file samples/docs/idd.json
```


测试策略允许 GitHub 的 `user1` 读取、Google 的 `user1` 写入，所有域的 `user1` 租借。与[身份域示例](/docs/idd/)使用同一组规则。

## 验证令牌请求

依次预期为 Allow、Deny、Allow。第二条请求使用有效的 **GitLab** 身份，因此不能使用仅授予 GitHub 身份的读取权限。

<!-- docs-check: token-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"githubtoken","tokenType":"github"},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: token-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"gitlabtoken","tokenType":"gitlab"},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: token-any -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"gitlabtoken","tokenType":"gitlab"},"serviceName":"booksvc","resource":"book","action":"rent"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


无效令牌应返回断言错误（非 2xx），不是正常的策略 Deny：

<!-- docs-check: token-invalid -->
```bash
curl --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"invalid-token","tokenType":"google"},"serviceName":"booksvc","resource":"book","action":"write"}' \
  -w '\nHTTP %{http_code}\n' http://127.0.0.1:6734/authz-check/v1/is-allowed
```


结束后分别在 mock、PMS、ADS 终端按 Ctrl-C。

## 对接真实提供方

可用完整 JSON 配置替代本地参数，其中断言部分如下：

```json
{
  "asserterWebhookConfig": {
    "endpoint": "https://identity.example/v1/assert",
    "clientCert": "/absolute/path/client.crt",
    "clientKey": "/absolute/path/client.key",
    "caCert": "/absolute/path/issuer-ca.crt",
    "httpTimeout": 5
  }
}
```

这是服务器配置中的断言部分，部署时还要提供存储和服务端配置。`caCert` 验证 webhook 服务器，`clientCert` 与 `clientKey` 用于对方要求双向 TLS 的情况。按身份提供方文档通过 [OpenID Connect](https://openid.net/connect/) 或 [OAuth](https://oauth.net/2/) 获取令牌，并验证其实际支持的令牌类型。PMS/ADS 调用方仍需单独遵循[安全配置](/docs/security/)。
