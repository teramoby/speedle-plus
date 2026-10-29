# Casbin、OPA、Speedle+ 授权引擎实测对比

测试日期：2026-10-29，Asia/Shanghai。全部正式测试在 charming 从头运行；本报告不使用 shrek 或旧 charming 运行的数字。委托人是 Speedle+ 维护者；以下判断以本次观测为依据，固定版本的缺陷与今天的项目现状分别说明。

## 1. 结论

**功能。** 9 个场景的 30 个用例，三个引擎各自全部通过。各实现的数据映射不同，30/30 仅证明这些具体用例的判定一致，不足以比较通用关系授权能力。

**性能。** 在 plain 的 10000 角色/100000 用户混合请求中，单次决策中位数为 Speedle+ 6.093 µs、OPA 19.656 µs、Casbin tuned 2.258 ms、Casbin 默认 5.077 ms。少角色的 manyusers 100000 用户中，对应为 6.713 µs、37.436 µs、33.845 µs、48.837 µs；应同时看这两种形状。相同精简 HTTP 封装、并发 16 的三轮中位数，plain 为 Speedle+ 30,373、OPA 21,273、Casbin 默认 99 请求/s；manyusers 则为 30,096、15,766、11,012 请求/s。

**资源。** 100000 角色/1000000 用户加载时，保留 Go 堆为 Casbin 1024.1 MiB、OPA 197.2 MiB、Speedle+ 500.4 MiB；加载耗时分别为 8.551s、1.819s、2.056s。这是一次加载的观察，堆数据不等于整个进程的 RSS。

**易用性。** 9 个场景的 SPDL 合计 51 行，Casbin 为 121 行、OPA 为 139 行，但模型样板与关系语义的取舍影响这个数量；Casbin 的模型与策略分离方便复用，但 matcher 顺序会影响性能；OPA 的 Rego 更通用，需要额外学习声明式求值。Speedle+ 固定版本在 Linux 上的原子文件替换导致宿主退出，坏条件则直到请求时才暴露，使用嵌入模式前必须考虑这两个行为。

**生态。** 本次查询三者 stars 为 20473、12411、62；历史贡献者条目为 173、625、8。这些是社区规模线索，不是质量评分；Speedle+ 今天已有可被 Go 代理识别的 v1.6.0，交接文档中“没有正式 Go 版本”的旧结论不再成立。

## 2. 测试环境与证据

| 项目 | 本次条件 |
|---|---|
| CPU | Intel Core i5-7360U @ 2.30 GHz，2 物理核、4 逻辑线程 |
| 内存与 swap | 15 GiB 内存，2 GiB swap；开始时约 10 GiB available，swap 已用约 2 GiB |
| 磁盘 | APPLE SSD AP0128J NVMe，约 113 GiB；/tmp 位于根分区，开始时可用约 23 GiB |
| 系统 | Linux Mint 22.3 Zena，Ubuntu noble 基础；内核 7.0.0-38-generic，x86_64 |
| CPU 调频 / 睿频 | 四个线程均为 powersave；intel_pstate/no_turbo=0，即未禁用睿频；未修改系统配置 |
| 编译主机 | shrek.local，Apple M1 Pro、16 GiB / macOS 27.0.1 (26A434)；Go 1.27.2 |
| 编译参数 | GOOS=linux GOARCH=amd64 CGO_ENABLED=0，未使用额外性能补丁 |
| Casbin | github.com/casbin/casbin/v3 v3.11.0 |
| OPA | github.com/open-policy-agent/opa v1.21.4；进程内 v1/rego |
| Speedle+ | github.com/teramoby/speedle-plus v0.0.0-20261028230603-b27604c077ce，固定 commit b27604c077ce |
| 进程内 CPU | 顺序基准 -test.cpu 2；Parallel 为 1、2、4；无额外 taskset 限制，允许在线 CPU 0–3 |
| HTTP CPU | 服务 taskset 0,2（同一物理核的两个线程），压测 taskset 1,3；两者 GOMAXPROCS=2 |
| 运行窗口 | 2026-10-29T09:19:29+08:00 至 2026-10-29T10:17:58+08:00 |
| CI 与负载 | 734 条活动采样，Worker 最大 0；1 分钟负载 0.31–3.96；CI 污染尝试 0 次 |
| 换页 | 全程累计换入增加 1115 页，换出增加 35 页；起始 swap 占用和本次换页活动分开记录 |

完整 `lscpu`、内核、governor、磁盘、内存、二进制 SHA-256 和 `go version -m` 见 [machine.txt](results/retest-20261029/charming/machine.txt)。charming 未安装 Go，模块元数据是在 shrek 对同一 Linux 二进制读取，再由 charming 记录 SHA-256；详见 [build-info.txt](results/retest-20261029/charming/build-info.txt) 与 [编译主机记录](results/retest-20261029/charming/compiler-host.txt)。

相同 Go 源码还通过新增构建脚本在 shrek 重建，六个 Linux 程序的 SHA-256 均与实际测试包逐字节一致，见 [重建校验](results/retest-20261029/reproduction-binary-check.json)。

原始记录：[activity.log](results/retest-20261029/charming/activity.log)、[步骤与重跑状态](results/retest-20261029/charming/steps.tsv)、[run-all.log](results/retest-20261029/charming/run-all.log)。活动每 5 秒采样一次，步骤前无 Worker 并冷却 10 秒；检测到 Worker 的步骤最多重跑两次。采样不能排除小于 5 秒的短任务。桌面会话和其他常驻进程保留，未停止他人进程。

## 3. 方法

plain：n 个角色、每角色一条权限和 10 个用户；角色 i 可读 `/books/bookI`。规模为 100、1000、10000、100000 角色；100000 规模的四个非缓存变体分别运行独立进程。ABAC 每条权限增加两个相等条件；通配资源采用等价的 `/books/typeI/` 前缀。

manyusers：10 个角色、每角色 10 条权限，共 100 条权限；用户数分别为 1000 和 100000，用户 U 属于 role((U-1)%10+1)。混合请求约 80% 访问自己的角色允许的书，20% 访问相邻角色的书。新生成器另用 101 个用户、所有 10 条权限逐一核对允许与拒绝，五个变体全部通过。

顺序基准单 goroutine，`-test.benchtime 1s -test.count 5 -test.cpu 2`；CPU 参数限制 Go 并发能力，不表示一次顺序决策用了两核。RBACMixed/ManyUsers 使用固定种子的 4096 请求池循环；耗时很高的变体可能在一次测量中只覆盖池的一个前缀。混合基准的非缓存变体在每次校准/测量前检查前 32 个请求，缓存变体预热全部 4096 个；计时循环核对实际判定和错误。固定允许/拒绝请求在计时前也核对。准备原生请求的 `Prepare` 不计入进程内决策时间，HTTP 包含请求转换和 JSON/网络开销。

| 变体 | 含义 |
|---|---|
| casbin | 默认 matcher：角色查找在前；Parallel/HTTP 使用 SyncedEnforcer |
| casbin-tuned | 先比资源与动作，最后查角色；改变表达式顺序，授权语义相同 |
| casbin-cached | tuned 模型的 CachedEnforcer；已预热的重复请求命中，仅表示有限请求池/重复请求缓存效果 |
| casbin-matcher | ABAC 条件写在 matcher，和每条规则 eval(p.cond) 的默认实现对照 |
| opa | 用户角色按 key 查找，PrepareForEval 后求值；性能取决于这份 Rego/数据形状 |
| speedle | eval.NewFromFile 嵌入评估器，性能测试关闭文件监听 |

每项延迟给出五次中位数；最小值、最大值、B/op、allocs/op 全部保留在 [summary.md](results/retest-20261029/charming/summary.md)。内存是两次 GC 后仍保留的 Go 堆增量，原表写 MB，实际以 2^20 字节计算，以下按 MiB 解读。加载成本每项只测一次，不当作五次中位数。

HTTP 为同机回环、keep-alive、闭环负载，5 种配置 × 2 数据集 × 3 并发 × 3 轮；每轮跑完全部配置再进入下一轮，每次预热 3 秒、测量 15 秒。同一轮同一配置复用一个服务，依次运行并发 1/16/64；每档重新预热，换配置或轮次时重启服务。所有 HTTP 服务只绑定 127.0.0.1；Speedle+ gRPC 默认关闭，审计日志与普通日志均为 error，OPA 为 error。端口为 19000、18181、16735，避开原有服务。每个完整落在测量窗口的响应核对预期；边界跨越请求不计入延迟/吞吐。`srv cores`、`gen cores` 来自 `/proc/<pid>/stat` 的用户态+内核态 CPU 秒除以墙钟秒，CLK_TCK 由 getconf 获取；1.0 表示平均一个逻辑 CPU 的时间，不等于一个独占物理核。

## 4. 功能与策略体量

| Scenario | Casbin | OPA | Speedle+ |
|---|---|---|---|
| Basic RBAC | 4/4 | 4/4 | 4/4 |
| Role hierarchy | 4/4 | 4/4 | 4/4 |
| Groups | 3/3 | 3/3 | 3/3 |
| Multi-tenant roles | 4/4 | 4/4 | 4/4 |
| ABAC: approval limit | 3/3 | 3/3 | 3/3 |
| ABAC: owner can edit | 3/3 | 3/3 | 3/3 |
| Deny overrides grant | 3/3 | 3/3 | 3/3 |
| Wildcard resources | 3/3 | 3/3 | 3/3 |
| Relationship: doc in (nested) folder | 3/3 | 3/3 | 3/3 |

| Scenario | Casbin lines (chars) | OPA lines (chars) | Speedle+ lines (chars) |
|---|---|---|---|
| Basic RBAC | 15 (267) | 20 (472) | 7 (125) |
| Role hierarchy | 15 (262) | 20 (460) | 8 (140) |
| Groups | 13 (219) | 14 (286) | 5 (71) |
| Multi-tenant roles | 15 (292) | 16 (416) | 7 (181) |
| ABAC: approval limit | 13 (319) | 13 (271) | 6 (146) |
| ABAC: owner can edit | 8 (152) | 8 (104) | 3 (86) |
| Deny overrides grant | 14 (297) | 20 (474) | 7 (123) |
| Wildcard resources | 12 (230) | 13 (291) | 5 (88) |
| Relationship: doc in (nested) folder | 16 (293) | 15 (330) | 3 (48) |
| **Total** | **121 (2331)** | **139 (3104)** | **51 (1008)** |

三者各 30/30。Casbin 的组成员关系写在策略中，OPA/Speedle+ 的组来自请求；比较的是本用例输入下的判定，不包含组来源系统。Speedle+ 多租户使用 `ON expr:/tenant1/.*` 并把租户编码进资源路径。关系场景把文件夹层级编码为 `/F1/F1a/D3`，只验证了三个静态路径用例，没有覆盖对象 ID 独立于路径、文档移动、多父节点等需求。Speedle+ 支持角色继承、attribute 条件和 [custom function](https://speedle.io/docs/custom-function/)；这些都可作为关系授权的建模路线。本轮没有实现并比较这些等价方案，因此不能据此判断 Speedle+ 无法支持这些需求，或给三个产品的通用关系授权能力排名。Casbin g2 与 OPA graph.reachable 是本轮另外两种具体实现。

所有者场景使用 Speedle+ `authenticated_role` 与 `request_user`；角色常量来自固定版本 api/ads/types.go。在本次查阅的 [SPDL](https://speedle.io/docs/spdl/) 与 [嵌入说明](https://speedle.io/docs/deployment-embeded/) 中没有找到该角色名称的介绍。策略体量是这些具体实现的非空行/去空白字符数；Casbin 模型样板有重复，关系场景的建模方法不同，不能仅按行数评优劣。

## 5. 决策之外的能力

| 能力 | Casbin | OPA | Speedle+ |
|---|---|---|---|
| 列 alice 权限 | GetImplicitPermissionsForUser 返回 admin 的 read/write | 额外编写 permissions 规则，查询返回 read/write | GetAllGrantedPermissions 返回 /data1 的 read/write |
| 解释 kim 的拒绝 | EnforceEx 命中 [kim /hr/salaries read deny] | 求值轨迹 81 个事件/81 行；本测试记录前 8 行 | IsAllowed 返回 DENY_POLICY_FOUND；Diagnose 列出生效 deny 和忽略 grant |
| 运行时增加 viewer write | AddPolicy，38.228µs，随后允许 | storage write，80.994µs，随后允许 | 文件原地改写监听，5ms，随后允许 |
| 策略错误发现阶段 | 括号错误/字段拼错在第一次 Enforce 报出 | 编译时拒绝，带文件名和行号 | SPDL 缺资源/GRAN 拼错在加载时返回错误；不完整条件加载返回 nil |
| 坏条件后行为 | 拼错模型首次 Enforce 返回 false 与错误 | 这类编译错误不进入可用查询 | amount <= 的 approve 请求返回 false / ERROR_IN_EVALUATION；有效 sibling read 仍允许 |
| 嵌入式文件监听 | 本次未评估相应适配器 | 本次未评估文件监听；使用 store 更新 | 原地写入存活，watchcheck 为 11ms；原子 rename 覆盖触发 fatal，进程退出码 1 |

运行时更新时间是一次能力观察，轮询间隔为 5 ms 或 10 ms，没有统计分布；Casbin/OPA 进程内修改与 Speedle+ 文件 I/O+监听的路径不同，不据此给出机制等价的速度排名。

```text
Casbin: Unbalanced parenthesis
Casbin: No parameter 'p_ob' found.
OPA: bad1.rego:14: rego_parse_error ...
OPA: bad2.rego:10: rego_unsafe_var_error: var permm is unsafe
Speedle+: Not found permission / Not found valid effect（带文本和 ^）
Speedle+: Error happens in parsing condition (amount <=): Unexpected end of expression
```

原文、返回值和退出码见 [capabilities.txt](results/retest-20261029/charming/capabilities.txt)、[watchcheck.txt](results/retest-20261029/charming/watchcheck.txt)。这些输出包含观察型测试，`PASS` 表示测试程序完成，不表示引擎正确拒绝了所有坏策略；上表按实际输出判读。

## 6. 进程内性能

### 6.1 混合请求（主要指标）

![两种策略形状下的混合请求决策延迟；点为中位数，误差棒为最小到最大；不包含缓存变体](results/retest-20261029/charming/figures/mixed-latency.png)

| 规模 n | casbin | casbin-tuned | casbin-cached | opa | speedle |
|---|---|---|---|---|---|
| 100 | 51.350 µs | 32.679 µs | 210.9 ns | 20.794 µs | 5.794 µs |
| 1000 | 491.909 µs | 231.503 µs | 316.0 ns | 27.532 µs | 8.248 µs |
| 10000 | 5.077 ms | 2.258 ms | 252.9 ns | 19.656 µs | 6.093 µs |
| 100000 | 48.887 ms | 23.176 ms | — | 21.859 µs | 7.765 µs |

缓存列不能与非缓存引擎当作相同工作负载的无条件胜负；100000 plain 未运行 cached 变体，表中用“—”标记。

### 6.2 重复同一允许请求

| 规模 n | casbin | casbin-tuned | casbin-cached | opa | speedle |
|---|---|---|---|---|---|
| 100 | 33.799 µs | 24.024 µs | 149.6 ns | 19.508 µs | 4.563 µs |
| 1000 | 307.863 µs | 206.979 µs | 173.0 ns | 18.367 µs | 4.787 µs |
| 10000 | 3.345 ms | 2.253 ms | 205.0 ns | 19.952 µs | 5.715 µs |
| 100000 | 32.634 ms | 20.522 ms | — | 19.965 µs | 5.358 µs |

### 6.3 重复同一拒绝请求

| 规模 n | casbin | casbin-tuned | casbin-cached | opa | speedle |
|---|---|---|---|---|---|
| 100 | 67.367 µs | 49.895 µs | 171.8 ns | 21.036 µs | 5.294 µs |
| 1000 | 670.255 µs | 440.093 µs | 211.4 ns | 20.887 µs | 5.266 µs |
| 10000 | 6.971 ms | 4.543 ms | 213.0 ns | 21.279 µs | 5.244 µs |
| 100000 | 72.371 ms | 41.735 ms | — | 20.657 µs | 4.864 µs |

### 6.4 ABAC 与通配

| 规模 n | casbin | casbin-matcher | opa | speedle |
|---|---|---|---|---|
| 1000 | 678.529 µs | 317.489 µs | 27.049 µs | 5.266 µs |
| 10000 | 8.530 ms | 3.395 ms | 28.776 µs | 5.674 µs |

上表为 ABAC。下面为通配资源：

| 规模 n | casbin | casbin-tuned | opa | speedle |
|---|---|---|---|---|
| 1000 | 324.840 µs | 266.869 µs | 21.875 µs | 11.150 µs |
| 10000 | 3.119 ms | 2.803 ms | 23.535 µs | 10.053 µs |

### 6.5 少角色、多用户

| 规模 n | casbin | casbin-tuned | casbin-cached | opa | speedle |
|---|---|---|---|---|---|
| 1000 | 41.766 µs | 28.495 µs | 179.8 ns | 33.443 µs | 4.552 µs |
| 100000 | 48.837 µs | 33.845 µs | 245.3 ns | 37.436 µs | 6.713 µs |

这组固定 100 权限；与 plain 的角色/权限数随 n 增长的形状共同阅读。它更接近复用少量角色管理大量用户的做法；不能将 plain 的结果外推到所有 Casbin 部署。

### 6.6 多核吞吐

| n | GOMAXPROCS | casbin 决策/s | casbin-tuned 决策/s | opa 决策/s | speedle 决策/s |
|---|---|---|---|---|---|
| 1000 | 1 | 2,556 | 3,841 | 42,296 | 152,975 |
| 1000 | 2 | 5,058 | 7,409 | 84,610 | 282,646 |
| 1000 | 4 | 5,263 | 7,799 | 101,256 | 321,337 |
| 10000 | 1 | 249 | 373 | 35,153 | 134,282 |
| 10000 | 2 | 469 | 704 | 78,180 | 263,366 |
| 10000 | 4 | 542 | 773 | 89,477 | 302,755 |

由五次 ns/op 中位数取倒数得到整进程决策/s，单个固定允许请求；不是多核随机请求基准。两个物理核开超线程，GOMAXPROCS=4 不等于四个物理核；并发扩展还受分配、GC、共享数据结构和调频影响，不假定线性增长。

### 6.7 加载时间与保留堆

| n roles (users) | engine | load time | retained heap |
|---|---|---|---|
| 1000 (10000) | casbin | 64ms | 10.2 MB |
| 1000 (10000) | opa | 15ms | 1.9 MB |
| 1000 (10000) | speedle | 18ms | 4.9 MB |
| 10000 (100000) | casbin | 634ms | 101.0 MB |
| 10000 (100000) | opa | 144ms | 16.8 MB |
| 10000 (100000) | speedle | 180ms | 48.2 MB |
| 100000 (1000000) | casbin | 8.551s | 1024.1 MB |
| 100000 (1000000) | opa | 1.819s | 197.2 MB |
| 100000 (1000000) | speedle | 2.056s | 500.4 MB |

上表 retained heap 的 MB 按 MiB 解读；测量包含策略加载/编译，策略文件生成不计入。不是峰值内存，也未测服务常驻 RSS。

## 7. HTTP 压力测试

![两种数据集的 HTTP 吞吐；点为三轮中位数，误差棒为最小到最大](results/retest-20261029/charming/figures/http-throughput.png)

以下各项为同配置三轮中位数；90 个测量窗口的 HTTP 错误数与判错数均为 0。完整的 p90、p99.9、max（各自三轮中位数）与每轮数据见 [summary.md](results/retest-20261029/charming/summary.md) 和 [stress.md](results/retest-20261029/charming/stress.md)。

| 数据集 | 配置 | 并发 | req/s | p50 µs | p99 µs | srv cores | gen cores |
|---|---|---|---|---|---|---|---|
| manyusers n=100000 | opa-server | 1 | 4,189 | 211 | 824 | 0.944 | 0.472 |
| manyusers n=100000 | opa-server | 16 | 11,604 | 1,123 | 4,489 | 1.991 | 0.907 |
| manyusers n=100000 | opa-server | 64 | 11,396 | 4,906 | 15,195 | 1.995 | 0.911 |
| manyusers n=100000 | speedle-ads | 1 | 7,316 | 128 | 221 | 0.821 | 0.743 |
| manyusers n=100000 | speedle-ads | 16 | 25,909 | 497 | 3,152 | 1.881 | 1.438 |
| manyusers n=100000 | speedle-ads | 64 | 26,330 | 1,946 | 10,556 | 1.959 | 1.518 |
| manyusers n=100000 | thin-casbin | 1 | 4,092 | 214 | 630 | 1.032 | 0.445 |
| manyusers n=100000 | thin-casbin | 16 | 11,012 | 928 | 11,490 | 1.989 | 0.835 |
| manyusers n=100000 | thin-casbin | 64 | 10,755 | 3,856 | 34,917 | 1.997 | 0.826 |
| manyusers n=100000 | thin-opa | 1 | 5,320 | 169 | 584 | 0.923 | 0.557 |
| manyusers n=100000 | thin-opa | 16 | 15,766 | 802 | 3,691 | 1.980 | 1.067 |
| manyusers n=100000 | thin-opa | 64 | 15,469 | 3,464 | 11,458 | 1.985 | 1.053 |
| manyusers n=100000 | thin-speedle | 1 | 8,454 | 111 | 188 | 0.799 | 0.831 |
| manyusers n=100000 | thin-speedle | 16 | 30,096 | 441 | 2,524 | 1.725 | 1.655 |
| manyusers n=100000 | thin-speedle | 64 | 30,957 | 1,719 | 8,285 | 1.798 | 1.737 |
| plain n=10000 | opa-server | 1 | 4,965 | 178 | 515 | 0.938 | 0.536 |
| plain n=10000 | opa-server | 16 | 14,621 | 905 | 4,180 | 1.989 | 1.030 |
| plain n=10000 | opa-server | 64 | 14,290 | 3,875 | 12,214 | 1.995 | 1.033 |
| plain n=10000 | speedle-ads | 1 | 7,322 | 129 | 220 | 0.819 | 0.741 |
| plain n=10000 | speedle-ads | 16 | 25,953 | 502 | 3,005 | 1.875 | 1.431 |
| plain n=10000 | speedle-ads | 64 | 25,815 | 2,018 | 10,060 | 1.963 | 1.497 |
| plain n=10000 | thin-casbin | 1 | 58 | 16,097 | 60,271 | 1.368 | 0.011 |
| plain n=10000 | thin-casbin | 16 | 99 | 15,414 | 1,475,240 | 1.972 | 0.018 |
| plain n=10000 | thin-casbin | 64 | 98 | 22,359 | 4,353,191 | 1.991 | 0.019 |
| plain n=10000 | thin-opa | 1 | 6,829 | 134 | 373 | 0.850 | 0.672 |
| plain n=10000 | thin-opa | 16 | 21,273 | 559 | 3,736 | 1.957 | 1.223 |
| plain n=10000 | thin-opa | 64 | 21,020 | 2,604 | 8,775 | 1.979 | 1.223 |
| plain n=10000 | thin-speedle | 1 | 8,416 | 113 | 178 | 0.795 | 0.825 |
| plain n=10000 | thin-speedle | 16 | 30,373 | 439 | 2,424 | 1.742 | 1.645 |
| plain n=10000 | thin-speedle | 64 | 30,933 | 1,744 | 7,818 | 1.836 | 1.757 |

thin-* 使用相同 net/http 封装。thin-casbin 是默认 matcher 的 SyncedEnforcer，不是 tuned/cached；官方 OPA/Speedle+ 服务还包含各自协议和处理链，不应把它们与 thin 行的差值全归因于决策引擎。Casbin 核心库的 HTTP 入口由本项目提供，未评测独立的 Casbin Service。

CPU 分配见 [stress.err](results/retest-20261029/charming/stress.err)。服务和生成器分别最多使用一个物理核的两个线程；当 gen cores 接近 2 时，同机客户端可能约束吞吐；没有独立客户端校准，因此这些数字是该 CPU 分配下的闭环吞吐，不能称作引擎的无限制服务上限。各行 CPU 核数来自各自进程，包含测量期间 GC 和协议开销。

## 8. 易用性与学习曲线

以下是普通 Go 开发者视角的工程评估，没有招募用户测量学习时长。

| 维度 | Casbin | OPA | Speedle+ |
|---|---|---|---|
| 安装与程序 | Go 库 go get；当前 v3.11.0 release 为源码包 | Go 库、官方多平台预编译 CLI；本次统一源码编译 | 本次使用锁定伪版本编译；今天 v1.6.0 已在 Go 代理中，release 无二进制附件 |
| 浏览器试用 | [在线编辑器](https://casbin.apache.org/docs/online-editor/) | [OPA Playground](https://play.openpolicyagent.org/) | [Speedle+ Playground](https://speedle.io/playground/) |
| 本项目 Go 嵌入 | NewEnforcer + Enforce，请求为位置参数 | 读 Rego/数据、建 store、PrepareForEval、Eval、提取布尔值 | NewFromFile + IsAllowed，需构造 Subject/Principals/RequestContext |
| 需学概念 | PERM 模型、matcher/effect、角色图、adapter | Rego 声明式规则、some/every、undefined 与 false、data/input、store | SPDL 的 grant/deny/role、条件、资源路径/正则、内置属性 |
| 调试与工具 | EnforceEx、模型编辑器；错误位置较少 | eval/test/fmt/check/bench、REPL、trace；本例错误有文件与行 | spctl、Diagnose、明确拒绝原因；坏条件加载反馈不足 |

SPDL 在本次授权场景下可读性好，原生列权限/诊断接口也有帮助，并支持属性条件、角色继承与函数扩展。扩展关系建模的维护成本和端到端性能未在本轮测量。Casbin 的基本库接入调用较少，模型可复用；但 effect/matcher 语法和表达式顺序需要理解。OPA 前期要学习 Rego 与数据建模，其 CLI、trace 和规则语言提供了测试、格式化与通用策略编写工具；本轮没有全面比较三个引擎的扩展能力。

文档时效分开看：[当前 Speedle+ README](https://github.com/teramoby/speedle-plus/blob/master/README.md) 已改为 go install/go build；[Quick Start](https://speedle.io/quick-start/) 也使用现代 Go。固定 b27604c077ce 的 README 仍含旧式 GOROOT/GOPATH/go get 构建说明；当前[嵌入说明](https://speedle.io/docs/deployment-embeded/) 仍介绍 dep 和 Go 1.10，[Istio 集成](https://speedle.io/integrations/istio/) 使用 Mixer，而 [Istio 1.8 升级说明](https://istio.io/latest/news/releases/1.8.x/announcing-1.8/upgrade-notes/) 已说明不再支持 Mixer。这些旧说明会增加排查成本，不能以新 Quick Start 代表全部文档都已更新。

策略写法会影响测量结果。[Casbin 性能文档](https://casbin.apache.org/docs/performance/) 建议复用少量规则、按需求加载策略子集；[OPA 性能文档](https://www.openpolicyagent.org/docs/policy-performance) 建议对象索引等写法。本次给 Casbin 增加了 matcher 调序和少角色形状，OPA 采用按用户 key 查角色；没有评测所有可用优化路径。

## 9. 生态与成熟度

查询日期：2026-10-29，Asia/Shanghai。GitHub metadata/release/search API 与 Stack Exchange API 的原始响应和口径见 [research/](results/retest-20261029/research/)。

| 仓库 | stars | 贡献者条目 | issue 总数 | PR 总数 | 当前 latest release / UTC 发布日 |
|---|---|---|---|---|---|
| [apache/casbin](https://github.com/apache/casbin) | 20473 | 173 | 942 | 674 | [v3.11.0](https://github.com/apache/casbin/releases/tag/v3.11.0) / 2026-08-20 |
| [open-policy-agent/opa](https://github.com/open-policy-agent/opa) | 12411 | 625 | 3054 | 6354 | [v1.21.4](https://github.com/open-policy-agent/opa/releases/tag/v1.21.4) / 2026-10-23 |
| [teramoby/speedle-plus](https://github.com/teramoby/speedle-plus) | 62 | 8 | 16 | 114 | [v1.6.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.6.0) / 2026-10-29 |

issue/PR 为各自全历史数量，不把仓库 open_issues_count（含 PR）当成纯 issue 数。贡献者使用 GitHub API `anon=1` 分页，包含匿名作者条目与机器人，不表示当前活跃维护者或独立自然人人数。stars、PR 数量不直接证明产品质量。

治理：[Casbin 2026-02-07 进入 Apache 孵化器](https://incubator.apache.org/projects/casbin.html)；[OPA 2021-01-29 达到 CNCF Graduated](https://www.cncf.io/projects/open-policy-agent-opa/)；[Speedle+ 仓库](https://github.com/teramoby/speedle-plus) 描述为原 Speedle 维护者维护的分支。治理与历史规模不替代具体功能/可靠性评估。

Stack Overflow 当前标签问题数：casbin 33、open-policy-agent 270、rego 176、speedle 0。OPA 两个标签可能重叠，不能直接相加；speedle 标签为 0 也不能推出全站没有任何提及 Speedle+ 的内容。数据来自 [Stack Exchange questions API](https://api.stackexchange.com/docs/questions)，以 tagged 与 filter=total 查询，响应见 [contributors-stackoverflow.json](results/retest-20261029/research/contributors-stackoverflow.json)。

版本结论修正：当前 [Speedle+ v1.6.0 release](https://github.com/teramoby/speedle-plus/releases/tag/v1.6.0) 于 2026-10-29 发布，[Go 代理 info](https://proxy.golang.org/github.com/teramoby/speedle-plus/@v/v1.6.0.info) 返回该版本与 commit 17da10eb3d4efd80f95aee0da909640f632e6fa7。因此本报告不再使用“项目没有 Go 能识别的正式版本”描述当前状态；本次性能及缺陷结论仍严格限定于旧的 b27604c077ce，没有升级或测试 v1.6.0，也不推断新版本已修复这些问题。

## 10. 发现的问题与适用边界

1. **Speedle+ 固定版本，Linux 实测：原子替换被监听文件导致宿主退出。** watchcheck rename 记录 CHMOD 后的 Remove，进程 fatal 并返回 1；原地写入正常。固定源码 pkg/store/file/fileStore.go:341、350、354 的 Fatalf 与观测一致。嵌入模式应用若用原子保存配置，应先处理此兼容性问题。
2. **Speedle+ 固定版本，Linux 实测：不完整条件没有使加载失败。** `if amount <=` 加载返回 nil；approve 请求返回 false、ERROR_IN_EVALUATION 与错误，有效 sibling read 仍允许。pkg/eval/runtimeservice.go:94、118 等 fmt.Println 会输出策略调试信息，本次 raw output 也观察到。不能把拒绝请求等同于加载期验证完备。
3. **Casbin 固定版本，Linux 实测：matcher 字段拼错延迟到首次请求报错。** 加载返回 nil，Enforce 报 `No parameter 'p_ob' found.`，没有文件行号。OPA 的本例 Rego 错误在编译阶段定位到文件/行；这只是选定错误样本的比较。
4. **测试覆盖与文档发现成本。** 关系场景只覆盖静态路径样例，未比较 Speedle+ 通过属性、角色层级或自定义函数实现的等价方案；这是本轮覆盖范围的限制。内置角色名称在查阅页面中难以发现，嵌入与 Istio 页面还保留过时步骤。当前 README 与 release 已有更新，不能把旧文档和版本问题一概视为仍然存在。

源码定位：[fileStore.go@b27604c077ce](https://github.com/teramoby/speedle-plus/blob/b27604c077ce/pkg/store/file/fileStore.go#L339)、[runtimeservice.go](https://github.com/teramoby/speedle-plus/blob/b27604c077ce/pkg/eval/runtimeservice.go#L94)、[内置角色](https://github.com/teramoby/speedle-plus/blob/b27604c077ce/api/ads/types.go#L89)。本次未修改任何被测引擎，也未推送或发布代码。

## 11. 局限与复现

未对 attribute、role hierarchy、custom function 的等价关系授权实现做功能与性能比较，不能以是否使用现成图函数推断产品能力高低。只测一台双核笔记本 Linux 主机，两种主要策略形状；未测跨机器网络、分布式部署、存储故障、策略一致性或高可用。调频为 powersave 且允许睿频，未锁定频率/温度；三轮 HTTP 交错只能减轻时间漂移，不能消除所有热与后台负载影响。系统既有 swap 已接近占满，实际换页增量在环境表中披露；CI 仅以 5 秒采样观测。

RBACMixed 是固定随机池循环，不等于生产请求分布；CachedEnforcer 的结果仅适用于相应预热命中条件。Parallel 使用同一个允许请求，不能代替随机并发性能。OPA 写法、Casbin 模型/分片、Speedle+ 路径索引都可能改变结果。HTTP 为固定并发闭环，服务变慢时发压速率会随之降低；未测试固定到达率与开放环排队，不能据此推算生产峰值的排队 SLO。多次重复给出范围，未计算显著性或置信区间；本报告不预设跨平台或换策略后的排序。

复现入口：[TEST_PLAN.md](TEST_PLAN.md)、[run-all.sh](run-all.sh)、[stress.sh](stress.sh)、[summarize.py](summarize.py)。固定源代码快照在 `dist/authz-bench-linux-20261029/source.tar.gz`，正式运行目录为 charming 的 `/tmp/authz-bench-codex-20261029`；旧目录和其他进程保留。

```bash
# 在 charming 上、解压同一固定二进制包后运行
./run-all.sh
# 在 shrek 取回这一次结果后
python3 summarize.py results/retest-20261029/charming
python3 build_report.py
```
