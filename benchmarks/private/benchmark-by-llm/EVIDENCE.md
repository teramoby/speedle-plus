# 证据索引与证明范围

| 证据 | 可证明内容 | 限定 |
|---|---|---|
| [functional.txt](results/retest-20261029/charming/functional.txt) | 9 场景各30个用例、生成器一致性与策略体量 | 只验证所选输入/建模 |
| [capabilities.txt](results/retest-20261029/charming/capabilities.txt) | 列权限、拒绝诊断、更新与错误样本 | 部分为观察型测试，PASS不代表坏策略都正确加载失败 |
| [watchcheck.txt](results/retest-20261029/charming/watchcheck.txt) | Linux inplace / rename 输出及退出码 | 固定b276版本，本轮各一次 |
| bench-*.txt（[汇总](results/retest-20261029/charming/summary.md)） | 107组各5次，ns/op、B/op、allocs/op | 有限请求池/固定请求；缓存单列 |
| [stress.md](results/retest-20261029/charming/stress.md) | 90个窗口、延迟、吞吐、错误/判错、CPU时间 | 同机回环、固定并发闭环 |
| [stress.err](results/retest-20261029/charming/stress.err) / [服务日志](results/retest-20261029/charming/stress-logs/) | 绑核、配置、轮次、PID和服务实际参数 | 不是生产环境记录 |
| [machine.txt](results/retest-20261029/charming/machine.txt) / [build-info.txt](results/retest-20261029/charming/build-info.txt) | 硬件、系统、编译器/模块、起始二进制哈希 | go version -m 在shrek读取同一Linux程序 |
| [activity.log](results/retest-20261029/charming/activity.log) / [steps.tsv](results/retest-20261029/charming/steps.tsv) | 734采样、13清洁步骤、负载与累计换页 | 5秒采样可能漏过短活动；没有实际触发CI重跑 |
| [部署证据](results/retest-20261029/deployment/) | nohup输出、PID、Linux冒烟结果 | 冒烟不并入正式性能统计 |
| [重建一致性](results/retest-20261029/reproduction-binary-check.json) / [结束核验](results/retest-20261029/final-remote-verification.json) | 六程序本地重建与远端哈希一致、测试进程/端口已退出 | 二进制未上传，哈希保留 |
| [research/](results/retest-20261029/research/) | GitHub/StackExchange查询快照 | 日期/查询口径见JSON；认证权限字段移除 |
| [preparation/](evidence/preparation/) | shrek上小参数HTTP冒烟输出 | 非正式测试机，不作最终性能依据 |
| [executed-source.tar.gz](provenance/executed-source.tar.gz) | 开跑前固定源码快照的公开副本 | 已脱敏并规范化tar元数据；逐条哈希见下 |
| [原/公开文件映射](provenance/publication-source-records.json) / [归档条目哈希](provenance/archive-entry-hashes.json) | 原始与公开副本哈希、已做变换 | 公开副本不是未改动的逐字节原件 |
| [preparation-changes.patch](provenance/preparation-changes.patch) | 从Claude交接到实际执行工具的主要改动 | 新增生成器测试见当前源码 |

原始 shrek 报告和准备前脚本保存在 [HISTORICAL 归档](provenance/before-preparation-HISTORICAL.tar.gz)，仅用于审计交接历史。其数字不进入正式报告。原始用户粘贴内容也包含明确标为参考的旧数字。

本轮 `go vet`、本地生成器验证和100x基准曾在聊天工具中执行成功，但没有独立保存其完整原始终端日志；不补造缺失日志。Linux正式功能输出包含生成器验证；之后的归档验证另存 validation/，不能冒充09点准备阶段的日志。

没有提供完整、更早的Claude会话、自动化图关系扩展测试、分布式/高可用测试、开放环压测、峰值RSS或人类学习时间研究。没有从缺少证据推断某个产品不支持attribute/角色层级/custom function实现的能力。
