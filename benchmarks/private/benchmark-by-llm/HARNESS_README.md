# authz-bench

Casbin、OPA、Speedle+ 用同一组策略做功能、性能、压力和易用性对比。2026-10-29 charming 全量重测已完成，结论见 [REPORT.md](REPORT.md)，交接用的完整测试计划见 [TEST_PLAN.md](TEST_PLAN.md)。

## 版本

| 引擎 | 版本 | 用法 |
|---|---|---|
| Casbin | `github.com/casbin/casbin/v3` v3.11.0 | 进程内库 |
| OPA | `github.com/open-policy-agent/opa` v1.21.4 | 进程内库（`v1/rego`），以及 `opa run --server` |
| Speedle+ | `github.com/teramoby/speedle-plus` v0.0.0-20261028230603-b27604c077ce | 进程内库（`pkg/eval`），以及 `speedle-ads` |

上表是 2026-10-29 归档那轮的固定版本；当前 `go.mod` 固定 `v1.6.1-0.20261102051349-0722cd595f4c`（2026-11-02），`go.may.mod` 固定 `v0.0.0-20260116081120-8fb1481d715e`。

## Cedar 与 May 2026 Speedle+ 构建（2026-11 新增）

归档之后 harness 又加入了 Cedar 和 2026-05 的 Speedle+ 构建；上面的 2026-10-29 归档数字不含它们，需要重新在基准机上跑一轮。

- `engines.go` 有两个 Cedar 变体：`cedar`（cedar-go，一项权限一条 permit）和 `cedar-data`（整个集合一条 permit，权限放在实体数据 `resource.readers` 里）。
- `cmd/cedar-rs` 用 Cedar 的 Rust 参考实现（`cedar-policy` 4.14.0）回答完全相同的请求。`TestExportCedarSpecs` 把请求导出成 JSON；`cedar-rs check` 校验，`cedar-rs bench` / `parallel` 计时，`cedar-rs serve` 提供与 `authzd` 相同的精简 HTTP API。
- 9 个场景各有 `cedar.cedar` + `cedar.entities.json`（组、角色层级和嵌套目录用实体父子关系表达）；生成的大规模集合也有 Cedar 文件，尺寸从 10 开始。
- `go.may.mod` 固定 2026-05 时的 Speedle+ 构建（`8fb1481d`，2026-01-16 起一直是 master，直到 2026-06-24；`SPEEDLE_LABEL=speedle-may`）；`build-package.sh` 打包两套 Go 测试/服务、`cedar-rs` 和两个 `speedle-ads`。

```bash
CEDAR_SPEC_DIR=/tmp/cedar-specs go test -run '^TestExportCedarSpecs$' -v
(cd cmd/cedar-rs && cargo build --release --locked)
cmd/cedar-rs/target/release/cedar-rs check /tmp/cedar-specs/scenarios/*.json
```

`run-all.sh` 的对应步骤是 `cedar-specs`、`bench-100k-cedar`、`bench-100k-cedar-data`、`may-*` 和 `rs-*`。

## 目录

- `scenarios/sN_*/`：9 个场景，每个场景四份策略：`casbin.conf` + `casbin.csv`、`opa.rego` + `opa.json`、`cedar.cedar` + `cedar.entities.json`、`speedle.spdl`
- `engines.go`：四个引擎（Casbin、OPA、Cedar、Speedle+）的统一接口 `Engine.Allowed(Req)`
- `scenarios_test.go`：场景用例和通过矩阵（`TestScenarios`）、策略体量（`TestPolicySize`）
- `capabilities_test.go`、`broken_test.go`：列权限、解释决策、运行时改策略、错误信息、坏策略的行为
- `gen.go`：生成大规模策略（plain：n 个角色，每角色 10 用户；manyusers：10 角色、100 权限、n 用户），四种格式各一份
- `bench_test.go`：进程内延迟基准、并发基准、加载时间与内存（`TestLoadCost`）
- `cmd/authzd`：同一个精简 HTTP 服务包住任一引擎
- `cmd/cedar-rs`：同一批请求的 Cedar Rust 参考实现，输出 Go benchmark 格式
- `cmd/loadgen`：闭环压测客户端，输出吞吐和 p50/p90/p99/p99.9
- `cmd/watchcheck`：验证 Speedle+ 文件监听在“写临时文件再改名”时的行为
- `stress.sh`：HTTP 压测（精简服务覆盖各引擎和变体、`opa run --server`、`speedle-ads`、`cedar-rs serve`）
- `results/`：原始结果

## 运行

正式数据只在 charming 上测；shrek 用于构建、冒烟验证和汇总。

```bash
# shrek：从归档源码提交重建 2026-10-29 那轮的六程序包，不会启动远端测试
./build-linux.sh
# 构建机：四引擎新一轮（含 Cedar 和 May 2026 构建），Linux x86-64，需要 Go 与 Cargo
./build-package.sh
# charming：解压包后，等待机器空闲，执行完整矩阵
./run-all.sh
# shrek：取回本轮结果后
python3 summarize.py results/<run>/charming
python3 build_report.py  # 仅重建 2026-10-29 三引擎归档报告
```

`run-all.sh` 的默认步骤包含 `may-*` 和 `rs-*`，它们需要 `build-package.sh` 产出的第二套 Go 测试/服务和 `cedar-rs`；用 `build-linux.sh` 打的归档包只适合重跑三引擎那轮，可用 `STEPS=` 选择步骤。

`build-linux.sh` 从归档提交（`bca2c4e5`）而非当前工作树构建，因为 Cedar 新增内容与已变化的 `go.mod` Speedle+ 版本无法复现归档二进制。`build-package.sh` 要求 Linux x86-64 构建机，因为 `cedar-rs` 按构建机编译而不是交叉编译。

完整运行会等待 CI 空闲，受 CI 污染的步骤最多重跑两次。`build_report.py` 只适用于 2026-10-29 那轮三引擎归档：运行目录、引擎表与期望组数都是固定的（COMPLETE、107 组进程内基准各 5 次、30 组 HTTP 配置各 3 轮），否则不会替换旧报告；四引擎新一轮在扩展报告生成器之前用 `summarize.py` 汇总。参数与逐步说明见 [TEST_PLAN.md](TEST_PLAN.md)。

可选绘图：在隔离 Python 环境中安装 `plot-requirements.txt`，然后运行 `python plot_results.py results/retest-20261029/charming`，最后重建报告。仅查看已完成进程内结果可加 `--mixed-only`。

## 2026-10-29 重测工具

- `manyusers` 基准：1000 / 100000 用户；HTTP 两组为 plain 10000 角色和 manyusers 100000 用户。
- `stress.sh`：默认三轮交错、并发 1/16/64、预热 3 秒、测量 15 秒。Linux 服务 CPU 0,2，压测 CPU 1,3（实际分配写入 stress.err）；CPU 列来自进程自身 CPU 时间差。
- `run-all.sh`：每 5 秒采样 CI、负载、前三进程、累计换页；步骤前空闲冷却 10 秒；受 CI 污染最多重跑两次，原始记录留在 contaminated/。
- `summarize.py results/<run>/charming`：只汇总指定目录，不混入旧结果。
- 二进制模块信息在 shrek 上用 `go version -m` 读取并打包为 build-info.txt（charming 没有 Go）。
- 只通过记录的 PID 清理本次启动的进程。默认服务端口为 127.0.0.1:19000、18181、16735。

## 本轮结果

正式窗口：2026-10-29 09:19:29–10:17:58（Asia/Shanghai）。三个引擎功能用例各 30/30；107 组进程内基准各 5 次；30 组 HTTP 配置各 3 轮，全部零错误、零判错。734 条活动采样中没有 CI Worker，未触发重跑。环境换页活动已在报告披露。

- [完整汇总](results/retest-20261029/charming/summary.md)
- [运行日志](results/retest-20261029/charming/run-all.log)
- [重建二进制一致性](results/retest-20261029/reproduction-binary-check.json)
- [结束后远端校验](results/retest-20261029/final-remote-verification.json)

固定版本的两个 Speedle+ 问题已在 Linux 复核：原子覆盖监听文件使宿主退出、不完整条件直到请求时才报错。当前项目的 v1.6.0 和 README 状态已单独复核，未将过时交接描述当成当前结论；没有升级本次被测版本。
