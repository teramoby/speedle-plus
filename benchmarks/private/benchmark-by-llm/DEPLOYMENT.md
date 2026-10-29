# 部署与运行情况

本次是在 charming 上部署临时测试程序和回环 HTTP 服务。正式测量于 2026-10-29 09:19:29–10:17:58（Asia/Shanghai）完成，没有修改生产服务或发布引擎版本。

## 构建与上传

最初在 shrek（macOS 27.0.1 / Go 1.27.2）手动交叉编译：GOOS=linux、GOARCH=amd64、CGO_ENABLED=0。程序为 authzbench.test、authzd、loadgen、watchcheck、opa、speedle-ads，固定模块版本见 [build-info.txt](results/retest-20261029/charming/build-info.txt)。

原工作路径为 `/Users/LOCAL_USER/cc/authz-bench`。归档中的 [build-linux.sh](build-linux.sh) 是后来抽出的同等构建入口：在同一源码目录重建的六个程序与原测试包的 SHA-256 完全相同，见 [重建记录](results/retest-20261029/reproduction-binary-check.json)。二进制带有 Go 编译路径信息；换工作目录、工具链或平台重建不保证字节哈希相同，固定依赖与逻辑仍可复现。

最初总包为 `authz-bench-linux-20261029.tar.gz`，97289997 字节。完整哈希与源码快照来源见 [build-artifacts.json](provenance/build-artifacts.json)。本公开副本不包含总包或可执行程序，包含用于复建的源码、依赖锁定和策略。

以下是本次实际执行命令的可读重述，个人目录标识已脱敏。`charming.local` 是本轮 SSH 目标别名，访问使用密钥和 BatchMode；归档不含私钥材料。

```bash
scp -q -o BatchMode=yes dist/authz-bench-linux-20261029.tar.gz \
  charming.local:/tmp/authz-bench-linux-20261029-codex.tar.gz
ssh -o BatchMode=yes charming.local '
  test ! -e /tmp/authz-bench-codex-20261029 &&
  mkdir /tmp/authz-bench-codex-20261029 &&
  tar xzf /tmp/authz-bench-linux-20261029-codex.tar.gz \
    -C /tmp/authz-bench-codex-20261029 --strip-components=1'
ssh -o BatchMode=yes charming.local '
  cd /tmp/authz-bench-codex-20261029 &&
  test ! -e run.pid &&
  (nohup ./run-all.sh > run-all.out 2>&1 < /dev/null & echo $! > run.pid)'
```

临时目录与旧 `/tmp/authz-bench-linux` 分开，未删除旧结果。最初 Linux 解包有 LIBARCHIVE.xattr 警告，程序正常运行；公开源码快照已去除 tar 的账户和 xattr 元数据。新构建脚本使用 --no-xattrs。

## 服务配置

| 配置 | 回环 HTTP 入口 | 实现 |
|---|---|---|
| thin-casbin | 127.0.0.1:19000/check | 同一 net/http 封装，默认 matcher 的 SyncedEnforcer |
| thin-opa | 127.0.0.1:19000/check | 同一封装，PreparedEvalQuery |
| thin-speedle | 127.0.0.1:19000/check | 同一封装，NewFromFile，关闭监听 |
| opa-server | 127.0.0.1:18181/v1/data/authz/allow | opa run --server，log-level error |
| speedle-ads | 127.0.0.1:16735/authz-check/v1/is-allowed | file store，insecure=true，log-level / auditlog-level error，enable-watch false，gRPC 默认关闭 |

各配置顺序执行，不同时占用 19000。服务端 taskset=0,2，生成器=1,3，两者 GOMAXPROCS=2；两个线程属于各自的同一个物理核。每档预热 3 秒、测量 15 秒，1/16/64 并发，整套交错 3 轮。服务启动日志中的参数结构同时打印 flag 的默认值与实际值，应以实际 127.0.0.1 入口和脚本为准。

[run-all.sh](run-all.sh) 只结束自身启动的 PID / 后代；[stress.sh](stress.sh) 只结束 ACTIVE_PID。未使用全局 pkill 停止他人进程。CI Worker 每 5 秒采样，步骤前空闲冷却 10 秒；本轮没有检测到 CI，重跑分支未被实际触发。

## 结束与证据

- [nohup 原始输出](results/retest-20261029/deployment/run-all.out)、[PID 记录](results/retest-20261029/deployment/run.pid)
- [Linux 小参数冒烟](results/retest-20261029/deployment/results-linux-smoke.md)、[冒烟 stderr](results/retest-20261029/deployment/results-linux-smoke.err)：不计入正式性能结果
- [正式步骤日志](results/retest-20261029/charming/run-all.log)、[steps.tsv](results/retest-20261029/charming/steps.tsv)、[COMPLETE](results/retest-20261029/charming/COMPLETE)
- [结束后远端核验](results/retest-20261029/final-remote-verification.json)：本轮进程与监听端口均为空；六个二进制仍与初始记录、本地原包和本地重建一致

这是结束时的校验快照，不是持续运行的主机监控。原测试目录与旧目录保留；本次归档发布在测量完成之后，不会把归档 PR 的 CI 活动混入测量窗口。
