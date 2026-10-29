# 本轮执行状态

- 用户已于 2026-10-29 明确授权 charming 全量测试。
- 本机工作目录：/Users/LOCAL_USER/cc/authz-bench（不属于 Git 仓库）。
- 旧代码/报告备份：/Users/LOCAL_USER/cc/authz-bench-backups/20261029-0914。
- 固定版本不变：Casbin v3.11.0、OPA v1.21.4、Speedle+ v0.0.0-20261028230603-b27604c077ce。
- 新工具已实现 manyusers、CPU accounting、taskset 交错 3 轮、CI 采样重跑、summarize.py；新增生成器功能测试、计时循环检查判定。
- 本地 go vet、生成器 5 变体验证、100x ManyUsers smoke、两种形状×5服务 smoke 均通过；Linux 10 配置 smoke 均零错误/判错，核数记录有效。
- Linux 二进制包：dist/authz-bench-linux-20261029.tar.gz；build-info.txt 含全部模块版本，source.tar.gz 含本轮 Go 源码快照。
- charming 独立目录：/tmp/authz-bench-codex-20261029；未删除旧目录、未停止他人进程。
- 正式 run-all.sh PID：394288（启动时间 09:19:29 附近，开始 machine 步骤 09:19:39 +08:00），run.pid 存在远端根目录。
- 结果目标：results/retest-20261029/charming；COMPLETE 表示整个矩阵完成；不要将旧 results/charming 或 shrek 的数字混入。
- 09:37:47，machine / functional / capabilities / watchcheck / loadcost / bench-small 完成；进入 bench-manyusers，然后四个独立 100k 引擎、parallel 1/2/4、stress 90窗口。
- 已确认 Linux watchcheck inplace 11ms/exit0，rename fatal/exit1；坏条件加载 nil、请求 false/ERROR_IN_EVALUATION；运行时更新38.228µs/80.994µs/5ms。
- 全量结果等待器：本机 exec session 36950，运行 /tmp/authz-bench-wait-20261029.py；每2分钟只读检查，COMPLETE 后 scp 取回、summarize、build_report 自动生成报告。可用 write_stdin 查看新增输出。
- 构建报告：python3 build_report.py，要求 COMPLETE、107基准组各5次、30HTTP组各3轮，拒绝不完整结果；REPORT.md 目前仍是旧草稿，README 已注明。
- 可选图：/tmp/authz-bench-report-venv-20261029/bin/python plot_results.py results/retest-20261029/charming；之后再 python3 build_report.py 嵌入图。matplotlib 已安装在独立临时 venv。需 view_image 检查两张 PNG。
- 生态查询已保存 research/：GitHub repos/release/search、contributors anon=1（173/625/8）、SO 标签（33/270/176/0）；Speedle+ 当前 v1.6.0 已在 Go 代理，有效 commit17da10eb；当前README已更新，旧交接无正式版本/README仍旧的描述不能照抄。
- 所有 SSH 用 BatchMode=yes；不读私钥，不推送、不发布。若用户叫停，只终止本轮 PID 394288 及其自有后代；脚本 TERM/EXIT trap 会清理自有子进程。
- 结束前：完整性检查、图形 QA、报告数字/链接校对、README/TEST_PLAN状态更新、确认本轮进程结束。正式数据尚未完成时不宣称完成。

## 09:55 更新

全部107个进程内基准组已验证各5次并以PASS结束；HTTP于09:50:32开始，09:54:56完成14/90窗口（第一轮plain），全部零错误/判错。用户问当前结果，已在commentary提供功能30/30、三种形状混合延迟、100k加载成本和第一轮HTTP数值，明确不是最终三轮结果；原任务继续。

summarize.py 现对缺少COMPLETE的目录显示“进行中”，避免把单轮HTTP误标成三轮最终中位数。plot_results.py 增加 --mixed-only，可展示已经完成的进程内图；其余仍需全矩阵完成。最终画两张图再重建REPORT。

## 最终完成

2026-10-29 10:17:58 +08:00，COMPLETE 已产生。107 基准组各5次、30 HTTP组各3轮均完整；90窗口零错误/判错，13步骤一次通过，734活动采样无CI Worker，未触发重跑。累计换入1115页、换出35页，已披露。正式REPORT已替换旧草稿，PNG/SVG图已生成并查看。6个实际远端程序SHA与初始包、machine记录、本地重建均一致。本轮测试进程和监听端口均已退出，未停止他人进程、未推送/发布。README/TEST_PLAN已更新为完成状态。
