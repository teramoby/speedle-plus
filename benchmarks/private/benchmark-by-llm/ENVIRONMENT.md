# 测试环境与版本

本页摘自完整报告，原始机器输出与编译元数据保留在链接中。主机名 shrek / charming 用作本轮机器标识；个人账号目录已替换为 LOCAL_USER / REMOTE_USER。


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
