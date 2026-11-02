# 基准测试

## 运行基准测试
```bash
go test -bench .
```

## 测试报告

[eval_b_test.go](eval_b_test.go)用来得到策略决策的时间开销。测试平台为：

2 * `Intel(R) Xeon(R) Platinum 8167M CPU @ 2.00GHz 2核, 4线程`

该表格展示了API `IsAllowed`的时间开销。`op`的意思是对`IsAllowed`的一次调用。

用例|模型|时间开销 (μs/op)
----|----|----
One|1 policy, 1 role policy, 1 user|6.6
Tiny|10 policies, 10 role policies, 10 users/role|7.3
Small|100 policies, 100 role policies, 10 users/role|8.5
Medium|1K policies, 1K role policies, 10 users/role|8.6
Large|10K policies, 10K role policies, 10 users/role|8.7
Huge|100K policies, 100K role policies, 10 users/role|8.7
LargeExp|10K policies with resource expression, 10K role policies, 10 users/role|43.0
LargeCond|10K policies with condition, 10K role policies, 10 users/role|8.0

以下表格展示了API `GetAllGrantedPermissions`的时间开销。

用例|模型|时间开销 (μs/op)
----|----|----
LargePerm|10K policies, 10K role policies, 10 users/role|8.9

## CI 每周对比
每周的 CI（`scripts/ci/run-go-tests.sh weekly`，任一 Speedle runner 均可接单，报告中注明运行机器）会在同一台机器上，交替运行当前代码和基准版本
（2026 年 5 月时的 master，`8fb1481d`）的这些基准测试各五轮，比较每个用例的 ns/op 中位数。任一用例比基准慢 20% 以上，
`performance` 检查失败，由 weekly 报告分析并开 issue 跟踪。每份 weekly 报告都附有对比表，详见
[scripts/ci/benchmark.py](../scripts/ci/benchmark.py)。
