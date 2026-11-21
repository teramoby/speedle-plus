+++
title = "自定义函数"
description = "在 SPDL 条件中调用自定义函数，扩展授权规则的计算能力。"
date = 2019-01-21T10:43:02+08:00
lastmod = 2026-11-17T10:00:00+08:00
weight = 600
draft = false
bref = ""
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
+++

## 为什么使用自定义函数？

当内置运算或函数不能表达规则时，SPDL 条件可以调用已配置的函数。Speedle 向其 REST 端点发送 `POST {"params":[...]}`，预期返回 `{"result":...}` 或 `{"error":"..."}`。参数顺序、类型和返回类型必须与条件一致。函数失败是评估错误，应用应拒绝访问。

## 启动本地示例

示例接受 100 至 50000 之间的数值。参数类型错误或列表为空时返回 HTTP 错误。只有回环 IP 可以使用明文，其他监听地址必须提供 TLS。

> **注意：** 本页使用的 `website/tests/examples/` 文件来自克隆的仓库（[快速入门](/quick-start/)的 Route B）；`go install` 不包含这些文件。

在单独终端运行：

<!-- docs-check: function-server -->
```bash
go run ./website/tests/examples/custom-function
```


<!-- docs-source: website/tests/examples/custom-function/main.go -->
```go
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"time"
)

type request struct {
	Params []interface{} `json:"params"`
}
type response struct {
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func isValid(w http.ResponseWriter, r *http.Request) {
	send := func(status int, value response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		send(405, response{Error: "POST required"})
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input request
	if err := decoder.Decode(&input); err != nil || len(input.Params) == 0 {
		send(400, response{Error: "params must contain at least one number"})
		return
	}
	valid := true
	for _, param := range input.Params {
		value, ok := param.(float64)
		if !ok {
			send(400, response{Error: "all params must be numbers"})
			return
		}
		if value < 100 || value > 50000 {
			valid = false
		}
	}
	send(200, response{Result: valid})
}
func main() {
	listen := flag.String("listen", "127.0.0.1:23456", "Function listen address")
	cert := flag.String("cert", "", "TLS server certificate")
	key := flag.String("key", "", "TLS server key")
	flag.Parse()
	if (*cert == "") != (*key == "") {
		log.Fatal("provide both --cert and --key")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/func/isValid", isValid)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	if *cert != "" {
		log.Fatal(server.ListenAndServeTLS(*cert, *key))
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("plaintext examples require a loopback IP; provide TLS certificates for other addresses")
	}
	log.Fatal(server.ListenAndServe())
}
```


## 运行示例前

先按[快速入门](/quick-start/)编译仓库根目录下的 `./bin/spctl`、`./bin/speedle-pms` 和 `./bin/speedle-ads`，需要 Go 1.26.0（也可使用同一大版本的更新补丁版本）、Bash、curl 和 Python 3。下面各终端都从仓库根目录运行。为本页使用一个新的策略文件；仅在两个服务都停止后重建该文件。

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
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

在第三个终端运行后续命令。PMS/ADS 共享同一个文件，策略更新通过 watch 异步到达 ADS；先查询确认已更新，再依赖新策略做决策。`--insecure=true` 仅用于回环地址上的本地示例，生产连接见[安全配置](/docs/security/)。示例显式指定 PMS 地址，不修改 `~/.spctlconfig`。完成后在两个服务终端按 Ctrl-C。


## 注册并调用函数

以下定义关闭结果缓存，使每次决策反映本次请求。先创建服务，再创建策略。

<!-- docs-check: function-fixture -->
```bash
cat > function.json <<'JSON'
{"name":"isValid","funcURL":"http://127.0.0.1:23456/func/isValid","resultCachable":false}
JSON
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create function --json-file function.json
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service service1
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user Ally access library if isValid(attr1)" --service-name service1
```


以下两条请求分别返回 Allow（`200` 在范围内）、Deny（`50` 不在范围内）：

<!-- docs-check: function-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Ally"}]},"serviceName":"service1","resource":"library","action":"access","attributes":[{"name":"attr1","type":"numeric","value":200}]}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: function-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Ally"}]},"serviceName":"service1","resource":"library","action":"access","attributes":[{"name":"attr1","type":"numeric","value":50}]}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


## 函数失败

自定义函数调用可能失败：请求超时或无法连接，端点返回非 2xx 状态或无法读取的响应，响应包含 `{"error":"..."}`，或者值不符合声明的类型（见下文）。这些情况都是评估错误，而不是条件不成立。

同一函数收到字符串参数时返回 HTTP 错误，因此决策返回错误响应（`500`），而不是拒绝：

<!-- docs-check: function-failure -->
```bash
test "$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Ally"}]},"serviceName":"service1","resource":"library","action":"access","attributes":[{"name":"attr1","type":"string","value":"invalid"}]}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed)" = 500
```


`is-allowed`、`all-granted-roles` 和 `all-granted-permissions` 返回错误而不是决策结果。REST 端点返回 `500 Internal Server Error` 和 `{"error":"..."}` 响应体，gRPC 方法返回非 OK 状态。`diagnose` 端点在错误响应中报告失败的调用；Go 的 `Diagnose` API 同时返回评估结果，原因为 `ERROR_IN_EVALUATION`。失败的调用不会写入缓存，可缓存函数会在下次请求时重新调用。

其他行为保持不变。特别是，请求未携带某属性时，引用该属性的条件仍然不匹配，带此条件的 `deny` 策略不生效。

## 声明的参数类型和返回类型

函数定义可以声明参数类型和返回类型：

```json
{"name":"isValid","funcURL":"http://127.0.0.1:23456/func/isValid","params":["numeric"],"result":"bool"}
```

`params` 按调用顺序列出参数类型，`result` 是返回值类型，两个字段都是可选的。类型取值为 `string`、`numeric`、`bool`、`datetime`，或列表形式 `string[]`、`numeric[]`、`bool[]`、`datetime[]`。PMS 拒绝其他取值。

声明后，Speedle 在调用前检查参数、调用后检查返回值，不匹配即为函数失败。`numeric` 接受整数和浮点数；`datetime` 接受 Unix 时间戳或 RFC 3339 字符串。列表参数既接受一个列表值，也接受列表值的各个元素——条件把列表值作为多个独立参数传给函数。未声明这两个字段的函数不做这些检查。

## TLS 与缓存

TLS 端点先生成新的[本地测试证书](/docs/security/)，然后运行 `go run ./website/tests/examples/custom-function --cert ./tls/server.crt --key ./tls/server.key`。注册时 `funcURL` 为 `https://localhost:23456/func/isValid`，`ca` 填写 `tls/ca.crt` 中的 **PEM 证书内容**，不是文件路径。生产证书必须覆盖实际主机。无需使用过期的硬编码证书或私钥。

`resultCachable` 默认为 false；启用缓存后，`resultTTL` 表示结果缓存秒数。只有当结果在相应时间和参数组合下仍然正确时才应缓存，例如账号状态等外部数据可能在 TTL 到期前变化。定义管理见 [PMS 函数 API](/docs/api/management_api/)。浏览器 Playground 不支持调用外部自定义函数。


## 示例：零常驻权限（ZSP）

SPDL 条件和自定义函数可以用来实现零常驻权限（ZSP）：凭证短期有效、权限范围受限，应用负责校验并原子消费令牌。自托管 Speedle 的[可运行 ZSP 示例](https://github.com/teramoby/ai.speedle.io/tree/main/samples/zsp)用自定义函数检查令牌是否过期、是否处于生效窗口，以及是否已经使用。令牌仍由应用签发、校验和原子消费。
