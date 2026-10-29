# LLM 辅助授权引擎测试归档：2026-10-29

本目录归档 Codex 接手 Claude Code 交接后，在 charming 上对 Casbin、OPA、Speedle+ 执行的固定版本比较。委托人是 Speedle+ 维护者。目录名 `private` 不提供访问控制：本目录属于公开仓库，公开副本已做个人目录标识与认证元数据脱敏，见 [PRIVACY.md](PRIVACY.md)。

正式测试窗口：2026-10-29 09:19:29–10:17:58，Asia/Shanghai。

| 入口 | 内容 |
|---|---|
| [REPORT.md](REPORT.md) | 完整中文报告、环境、方法、性能、能力、生态、缺陷与局限 |
| [PROMPTS.md](PROMPTS.md) | 本次用户任务指令、授权、后续问题，以及原始 Claude 交接输入 |
| [TEST_PLAN.md](TEST_PLAN.md) | 已执行的测试矩阵与参数；原交接版本也单独保留 |
| [ENVIRONMENT.md](ENVIRONMENT.md) | 两台机器的职责、编译器、固定依赖、绑核、负载与换页 |
| [DEPLOYMENT.md](DEPLOYMENT.md) | 静态 Linux 包、SSH 上传、启动、服务参数与结束状态 |
| [EVIDENCE.md](EVIDENCE.md) | 原始证据索引、证明范围、未落盘信息与排除项 |
| [CORRECTIONS.md](CORRECTIONS.md) | 交接中过时描述，以及关系授权能力判断的修正 |
| [RECOMMENDATION.md](RECOMMENDATION.md) | 性能与能力的适用范围分析；不把未测方案列为不支持 |
| [HARNESS_README.md](HARNESS_README.md) | 原工作目录的运行说明与构建入口 |
| [完整数据汇总](results/retest-20261029/charming/summary.md) | 全部五次范围、分配、三轮 HTTP 与能力输出 |
| [MANIFEST.json](MANIFEST.json) | 本公开副本的逐文件 SHA-256；Git commit 覆盖清单本身 |

三个引擎各通过 30/30 功能用例。107 组进程内基准各 5 次；30 组 HTTP 配置各 3 轮，共 90 个窗口，均零 HTTP 错误、零判错。734 条活动采样未发现 Runner.Worker，13 步均第一次完成；未触发 CI 重跑。换入 1115 页、换出 35 页，已披露，不能称为完全没有后台干扰。

固定基线为 Casbin v3.11.0、OPA v1.21.4、Speedle+ v0.0.0-20261028230603-b27604c077ce。所有正式数字来自本轮 charming，未混用原 shrek 草稿或旧 charming 数据。关系样例只测了选定实现；attribute、role hierarchy、custom function 的等价关系建模方案未比较，不能据此排名产品能力。

源码、策略和脚本保持原独立 Go 模块布局。这个嵌套模块使用固定的外部 Speedle+ 基线，不改变仓库主模块或生产引擎；主模块的 `go list ./...` 不自动包含嵌套模块。

```bash
cd benchmarks/private/benchmark-by-llm
python3 verify_archive.py                 # 校验归档与测量矩阵，不启动服务
python3 summarize.py results/retest-20261029/charming
python3 build_report.py                   # 从同一数据重建报告
# 可选：隔离 Python 环境安装 plot-requirements.txt 后绘图
python3 plot_results.py results/retest-20261029/charming
# 如需重跑，先按部署说明准备独占测试窗口；下面命令只编译
./build-linux.sh
```

可执行二进制和 97 MB 的 Linux 总包未写入 Git；原包哈希、六个二进制哈希、相同源码重建逐字节一致的证明，以及小型源码快照均在 [provenance/](provenance/)。没有密钥、凭据、本机配置或完整用户目录。历史交接归档中的旧 macOS 报告只用于过程审计，不是最终报告的数据来源。
