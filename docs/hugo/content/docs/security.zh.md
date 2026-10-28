+++
title = "安全"
description = "配置 REST/gRPC TLS、双向 TLS 和调用方访问边界"
weight = 310
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["security", "guide", "tls", "https"]
categories = ["docs"]
bref = ""
+++

## 同时保护两种 API 传输

独立运行的 PMS 与 ADS 都提供 **REST 和 gRPC**。现在两者共同使用 `--insecure`、`--cert`、`--key`、`--client-cert` 和 `--force-client-cert` 配置。证书或信任配置无效时启动失败，安全模式不会降级为明文。

TLS 加密连接并认证服务端。若要强制认证客户端，必须通过 `--client-cert` 提供客户端 CA，**同时**设置 `--force-client-cert=true`。只提供 CA 时，仅在客户端主动发送证书的情况下验证该证书。TLS 本身不决定谁有管理员权限，也不提供 Bearer token 校验。管理、诊断、发现接口需要认证网关或等价的访问控制。`--enable-authz` 参数不提供内置的端点授权策略。

## 监听地址与兼容性

| 服务 | 默认 REST 地址 | 默认 gRPC 地址 |
| --- | --- | --- |
| PMS | `0.0.0.0:6733` | REST 的主机地址，端口 `50001` |
| ADS | `0.0.0.0:6734` | REST 的主机地址，端口 `50002` |

因此，`--endpoint 127.0.0.1:6733` 也会把 PMS 默认 gRPC 监听限制在 `127.0.0.1:50001`。可用 `--grpc-endpoint` 指定另一网卡或端口；`127.0.0.1:0` 可为隔离测试选择临时端口。需要 IPv6 时明确使用 `--endpoint '[::]:6733'` 或 `--grpc-endpoint '[::]:50001'`。与旧版本不同，限制或加密 REST 后，gRPC 不再悄悄保持全部网卡监听或明文传输。

配置优先级为命令行、`SPDL_*` 环境变量、JSON 配置、默认值。gRPC 对应 `--grpc-endpoint`、`SPDL_GRPC_ENDPOINT` 和 `serverConfig.grpcEndpoint`。[API 总览](/docs/api/)提供当前 proto 契约及客户端生成说明。

## 生成本地测试证书

先按[快速入门](/quick-start/)编译二进制，然后从仓库根目录使用 Go 1.26 或更高版本生成本地 CA、独立的服务端与客户端证书。服务端证书的 SAN 包括 `localhost`、`127.0.0.1`、`::1`，私钥权限为 0600。这些是**有效期 24 小时的演示证书**，不能作为生产凭据。工具拒绝覆盖已有文件；重新生成时使用新的输出目录。

<!-- docs-check: certificates -->
```bash
go run ./samples/docs/certificates --out ./tls
```


## 启动双向 TLS 服务

先停止两个服务，再初始化新的策略文件；只执行一次：

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```


终端一启动 PMS：

<!-- docs-check: secure-pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json \
  --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:50001 \
  --cert ./tls/server.crt --key ./tls/server.key \
  --client-cert ./tls/ca.crt --force-client-cert=true
```


终端二启动 ADS：

<!-- docs-check: secure-ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json \
  --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:50002 \
  --cert ./tls/server.crt --key ./tls/server.key \
  --client-cert ./tls/ca.crt --force-client-cert=true
```


两个终端都从同一仓库根目录运行并保持打开。完成后分别按 Ctrl-C 停止。生产证书必须覆盖实际域名/IP，并来自组织信任的 CA。

## 使用 spctl 和 curl

<!-- docs-check: secure-cli -->
```bash
./bin/spctl --pms-endpoint https://127.0.0.1:6733/policy-mgmt/v1/ \
  --skipverify=false --cacert ./tls/ca.crt --cert ./tls/client.crt --key ./tls/client.key \
  get service --all
```


<!-- docs-check: secure-curl -->
```bash
curl --fail --silent --show-error --cacert ./tls/ca.crt \
  --cert ./tls/client.crt --key ./tls/client.key \
  https://127.0.0.1:6733/policy-mgmt/v1/service
```


空存储返回 `[]`。不带客户端证书时 TLS 握手失败。`spctl` 的 `--cacert` 用来验证服务器，`--cert`/`--key` 提供客户端身份；`--client-cert` 和 `--force-client-cert` 是服务端参数。保持 `--skipverify=false`（默认值）。

gRPC 客户端通道同样需要 CA、与证书匹配的服务端名称、客户端证书与私钥。PMS 的服务名称是 `pms_pb.PolicyManager`，ADS 是 `ads_pb.Evaluator`。明文通道无法调用上述 TLS 端点。

## 本地明文开发

只有显式 `--insecure=true` 才会为两种传输启用明文。此时 REST/gRPC 都应绑定回环地址。不要公开这些未认证端点，也不要信任非可信调用者填写的主体名或属性。[令牌断言服务](/docs/assertor/)验证已配置的身份令牌，并不能自动认证每一个 PMS/ADS 调用方或替代端点访问控制。
