# 原始结果汇总

状态：全矩阵完成。
生成时间：2026-10-29T10:18:32.352447+08:00
来源目录：`results/retest-20261029/charming`。缺失项目保留为空，不使用其他机器的数据。

## 功能和策略体量

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


## 能力与文件监听

`capabilities.txt`
```text
=== RUN   TestBrokenPolicyBehavior
time="2026-10-29T09:20:02+08:00" level=error msg="Error happens in parsing condition (amount <=): Unexpected end of expression"
speedle load err=<nil>
recompile condition for policy: &{juyfj5iq2bjrv2ovwqh5  grant [0x21f6eef9a3c0] [[role:manager]] amount <= map[]}
time="2026-10-29T09:20:02+08:00" level=error msg="Error happens in parsing condition (amount <=): Unexpected end of expression"
speedle approve amount=1 -> allowed=false reason=ERROR_IN_EVALUATION err=Unexpected end of expression
recompile condition for policy: &{juyfj5iq2bjrv2ovwqh5  grant [0x21f6eef9a3c0] [[role:manager]] amount <= map[]}
time="2026-10-29T09:20:02+08:00" level=error msg="Error happens in parsing condition (amount <=): Unexpected end of expression"
speedle approve amount=1e+09 -> allowed=false reason=ERROR_IN_EVALUATION err=Unexpected end of expression
speedle read (valid sibling policy) -> allowed=true err=<nil>
casbin typo model: load err=<nil>
casbin typo model: first Enforce -> allowed=false err=No parameter 'p_ob' found.
--- PASS: TestBrokenPolicyBehavior (0.00s)
=== RUN   TestListPermissions
casbin  GetImplicitPermissionsForUser(alice) = [[admin /data1 read] [admin /data1 write]], err=<nil>
opa     data.authz.permissions (a rule written for it) = [map[action:read resource:/data1] map[action:write resource:/data1]], err=<nil>
speedle GetAllGrantedPermissions(alice) = [/data1:[read write]], err=<nil>
--- PASS: TestListPermissions (0.00s)
=== RUN   TestExplain
casbin  EnforceEx -> allowed=false explain=[kim /hr/salaries read deny] err=<nil>
opa     allowed=false err=<nil>; trace has 81 events / 81 lines; first lines:
        query:1         Enter data.authz.allow = _
        query:1         | Eval data.authz.allow = _
        query:1         | Unify data.authz.allow = _
        query:1         | Index data.authz.allow (matched 1 rule, early exit)
        opa.rego:7      | Enter data.authz.allow
        opa.rego:8      | | Eval __local9__ = input.user
        opa.rego:8      | | Unify __local9__ = input.user
        opa.rego:8      | | Unify "kim" = __local9__
speedle IsAllowed -> allowed=false reason=DENY_POLICY_FOUND err=<nil>
speedle Diagnose -> grantedRoles=[staff] policy{{Status:takeEffect ID:e6wrs33zdwtowyfv7jbi Name: Effect:deny Permissions:[{Resource:/hr/salaries ResourceExpression: Actions:[read]}] Principals:[[user:kim]] Condition:<nil>}} policy{{Status:ignored ID:wzs5tkuo47z6hsdl2w5v Name: Effect:grant Permissions:[{Resource: ResourceExpression:/hr/.* Actions:[read]}] Principals:[[role:staff]] Condition:<nil>}}
--- PASS: TestExplain (0.00s)
=== RUN   TestRuntimeUpdate
casbin  AddPolicy in-process: allowed=true after 38.228µs (err=<nil>)
opa     storage write in-process: allowed=true after 80.994µs (err=<nil>)
time="2026-10-29T09:20:02+08:00" level=info msg="Watch policy store."
time="2026-10-29T09:20:02+08:00" level=info msg="Enter Watch..."
time="2026-10-29T09:20:02+08:00" level=info msg="Reloading the file store..."
speedle embedded file watch: allowed=true after 5ms
--- PASS: TestRuntimeUpdate (0.01s)
=== RUN   TestErrorMessages
casbin  unbalanced paren in matcher: Unbalanced parenthesis
casbin  typo p.ob in matcher: No parameter 'p_ob' found.
opa     missing closing brace: 1 error occurred: bad1.rego:14: rego_parse_error: unexpected contains keyword: expected \n or ; or }
	permissions contains perm if {
	            ^
opa     typo permm.action: bundle activation failed: 1 error occurred: bad2.rego:10: rego_unsafe_var_error: var permm is unsafe
speedle missing resource: Not found permission
GRANT ROLE admin read
                     ^
speedle typo GRAN: Not found valid effect
GRAN ROLE admin read /data1
^
time="2026-10-29T09:20:02+08:00" level=error msg="Error happens in parsing condition (amount <=): Unexpected end of expression"
speedle incomplete condition: <nil>
--- PASS: TestErrorMessages (0.00s)
PASS
```

`watchcheck.txt`
```text
time="2026-10-29T09:20:12+08:00" level=info msg="Watch policy store."
time="2026-10-29T09:20:12+08:00" level=info msg="Enter Watch..."
time="2026-10-29T09:20:12+08:00" level=info msg="Reloading the file store..."
mode=inplace: survived, change visible after 11ms
mode=inplace exit_code=0
time="2026-10-29T09:20:12+08:00" level=info msg="Watch policy store."
time="2026-10-29T09:20:12+08:00" level=info msg="Enter Watch..."
time="2026-10-29T09:20:12+08:00" level=info msg="Operation \"CHMOD\" was detected on the policy file \"/tmp/watchcheck-4111682893/policy.spdl\""
time="2026-10-29T09:20:12+08:00" level=fatal msg="The policy file \"/tmp/watchcheck-4111682893/policy.spdl\" has already been removed, please double check"
mode=rename exit_code=1
```

## 加载成本

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


## 进程内决策延迟

| benchmark/size/variant-CPU | repeats | median | min | max | median B/op | median allocs/op |
|---|---|---|---|---|---|---|
| BenchmarkABAC/n=1000/casbin-2 | 5 | 678.529 µs | 674.320 µs | 687.079 µs | 179918 | 4446 |
| BenchmarkABAC/n=1000/casbin-matcher-2 | 5 | 317.489 µs | 315.866 µs | 324.185 µs | 105779 | 3016 |
| BenchmarkABAC/n=1000/opa-2 | 5 | 27.049 µs | 25.072 µs | 28.008 µs | 11120 | 213 |
| BenchmarkABAC/n=1000/speedle-2 | 5 | 5.266 µs | 5.187 µs | 5.312 µs | 2880 | 48 |
| BenchmarkABAC/n=10000/casbin-2 | 5 | 8.530 ms | 8.022 ms | 8.975 ms | 1.67708e+06 | 42128 |
| BenchmarkABAC/n=10000/casbin-matcher-2 | 5 | 3.395 ms | 3.309 ms | 3.439 ms | 1.04543e+06 | 30016 |
| BenchmarkABAC/n=10000/opa-2 | 5 | 28.776 µs | 27.163 µs | 29.800 µs | 11120 | 213 |
| BenchmarkABAC/n=10000/speedle-2 | 5 | 5.674 µs | 5.640 µs | 5.829 µs | 2880 | 48 |
| BenchmarkManyUsers/n=1000/casbin-2 | 5 | 41.766 µs | 40.671 µs | 41.820 µs | 13492 | 385 |
| BenchmarkManyUsers/n=1000/casbin-cached-2 | 5 | 179.80 ns | 179.50 ns | 195.00 ns | 58 | 3 |
| BenchmarkManyUsers/n=1000/casbin-tuned-2 | 5 | 28.495 µs | 28.013 µs | 29.135 µs | 9112 | 201 |
| BenchmarkManyUsers/n=1000/opa-2 | 5 | 33.443 µs | 31.901 µs | 37.126 µs | 13874 | 281 |
| BenchmarkManyUsers/n=1000/speedle-2 | 5 | 4.552 µs | 4.502 µs | 4.840 µs | 2819 | 46 |
| BenchmarkManyUsers/n=100000/casbin-2 | 5 | 48.837 µs | 48.249 µs | 49.182 µs | 13459 | 383 |
| BenchmarkManyUsers/n=100000/casbin-cached-2 | 5 | 245.30 ns | 239.50 ns | 246.80 ns | 107 | 3 |
| BenchmarkManyUsers/n=100000/casbin-tuned-2 | 5 | 33.845 µs | 33.050 µs | 34.094 µs | 9082 | 200 |
| BenchmarkManyUsers/n=100000/opa-2 | 5 | 37.436 µs | 36.062 µs | 39.862 µs | 13871 | 281 |
| BenchmarkManyUsers/n=100000/speedle-2 | 5 | 6.713 µs | 6.547 µs | 6.778 µs | 2819 | 46 |
| BenchmarkParallel/n=1000/casbin | 5 | 391.303 µs | 387.637 µs | 400.012 µs | 105728 | 3016 |
| BenchmarkParallel/n=1000/casbin-2 | 5 | 197.708 µs | 195.417 µs | 202.238 µs | 105728 | 3016 |
| BenchmarkParallel/n=1000/casbin-4 | 5 | 189.988 µs | 186.431 µs | 230.120 µs | 105733 | 3016 |
| BenchmarkParallel/n=1000/casbin-tuned | 5 | 260.319 µs | 259.065 µs | 264.380 µs | 65880 | 1519 |
| BenchmarkParallel/n=1000/casbin-tuned-2 | 5 | 134.963 µs | 134.336 µs | 140.040 µs | 65880 | 1519 |
| BenchmarkParallel/n=1000/casbin-tuned-4 | 5 | 128.226 µs | 126.085 µs | 128.343 µs | 65884 | 1519 |
| BenchmarkParallel/n=1000/opa | 5 | 23.643 µs | 22.807 µs | 24.208 µs | 8264 | 153 |
| BenchmarkParallel/n=1000/opa-2 | 5 | 11.819 µs | 11.597 µs | 12.745 µs | 8264 | 153 |
| BenchmarkParallel/n=1000/opa-4 | 5 | 9.876 µs | 9.777 µs | 10.037 µs | 8265 | 153 |
| BenchmarkParallel/n=1000/speedle | 5 | 6.537 µs | 6.487 µs | 6.585 µs | 2864 | 47 |
| BenchmarkParallel/n=1000/speedle-2 | 5 | 3.538 µs | 3.515 µs | 3.682 µs | 2864 | 47 |
| BenchmarkParallel/n=1000/speedle-4 | 5 | 3.112 µs | 3.049 µs | 3.172 µs | 2864 | 47 |
| BenchmarkParallel/n=10000/casbin | 5 | 4.014 ms | 3.899 ms | 4.059 ms | 1.04526e+06 | 30016 |
| BenchmarkParallel/n=10000/casbin-2 | 5 | 2.130 ms | 2.089 ms | 2.157 ms | 1.04526e+06 | 30016 |
| BenchmarkParallel/n=10000/casbin-4 | 5 | 1.846 ms | 1.806 ms | 1.876 ms | 1.04526e+06 | 30016 |
| BenchmarkParallel/n=10000/casbin-tuned | 5 | 2.679 ms | 2.583 ms | 2.739 ms | 645336 | 15019 |
| BenchmarkParallel/n=10000/casbin-tuned-2 | 5 | 1.420 ms | 1.378 ms | 1.451 ms | 645336 | 15019 |
| BenchmarkParallel/n=10000/casbin-tuned-4 | 5 | 1.294 ms | 1.276 ms | 1.302 ms | 645337 | 15019 |
| BenchmarkParallel/n=10000/opa | 5 | 28.447 µs | 27.589 µs | 28.767 µs | 8264 | 153 |
| BenchmarkParallel/n=10000/opa-2 | 5 | 12.791 µs | 12.596 µs | 13.264 µs | 8264 | 153 |
| BenchmarkParallel/n=10000/opa-4 | 5 | 11.176 µs | 10.907 µs | 11.434 µs | 8264 | 153 |
| BenchmarkParallel/n=10000/speedle | 5 | 7.447 µs | 7.414 µs | 7.594 µs | 2864 | 47 |
| BenchmarkParallel/n=10000/speedle-2 | 5 | 3.797 µs | 3.760 µs | 3.931 µs | 2864 | 47 |
| BenchmarkParallel/n=10000/speedle-4 | 5 | 3.303 µs | 3.292 µs | 3.408 µs | 2864 | 47 |
| BenchmarkRBACAllow/n=100/casbin-2 | 5 | 33.799 µs | 32.128 µs | 33.945 µs | 11615 | 316 |
| BenchmarkRBACAllow/n=100/casbin-cached-2 | 5 | 149.60 ns | 147.90 ns | 158.90 ns | 56 | 3 |
| BenchmarkRBACAllow/n=100/casbin-tuned-2 | 5 | 24.024 µs | 23.584 µs | 25.215 µs | 8084 | 169 |
| BenchmarkRBACAllow/n=100/opa-2 | 5 | 19.508 µs | 17.068 µs | 20.746 µs | 8267 | 153 |
| BenchmarkRBACAllow/n=100/speedle-2 | 5 | 4.563 µs | 4.455 µs | 4.618 µs | 2864 | 47 |
| BenchmarkRBACAllow/n=1000/casbin-2 | 5 | 307.863 µs | 304.666 µs | 314.397 µs | 105745 | 3016 |
| BenchmarkRBACAllow/n=1000/casbin-cached-2 | 5 | 173.00 ns | 172.40 ns | 177.40 ns | 112 | 3 |
| BenchmarkRBACAllow/n=1000/casbin-tuned-2 | 5 | 206.979 µs | 203.659 µs | 208.680 µs | 65891 | 1519 |
| BenchmarkRBACAllow/n=1000/opa-2 | 5 | 18.367 µs | 17.030 µs | 19.729 µs | 8264 | 153 |
| BenchmarkRBACAllow/n=1000/speedle-2 | 5 | 4.787 µs | 4.760 µs | 4.811 µs | 2864 | 47 |
| BenchmarkRBACAllow/n=10000/casbin-2 | 5 | 3.345 ms | 3.328 ms | 3.378 ms | 1.04542e+06 | 30016 |
| BenchmarkRBACAllow/n=10000/casbin-cached-2 | 5 | 205.00 ns | 202.20 ns | 216.20 ns | 112 | 3 |
| BenchmarkRBACAllow/n=10000/casbin-tuned-2 | 5 | 2.253 ms | 2.170 ms | 2.265 ms | 645447 | 15019 |
| BenchmarkRBACAllow/n=10000/opa-2 | 5 | 19.952 µs | 19.423 µs | 21.610 µs | 8264 | 153 |
| BenchmarkRBACAllow/n=10000/speedle-2 | 5 | 5.715 µs | 5.649 µs | 5.978 µs | 2864 | 47 |
| BenchmarkRBACAllow/n=100000/casbin-2 | 5 | 32.634 ms | 31.812 ms | 35.512 ms | 1.0407e+07 | 300016 |
| BenchmarkRBACAllow/n=100000/casbin-tuned-2 | 5 | 20.522 ms | 18.782 ms | 20.739 ms | 6.40713e+06 | 150019 |
| BenchmarkRBACAllow/n=100000/opa-2 | 5 | 19.965 µs | 18.568 µs | 20.503 µs | 8264 | 153 |
| BenchmarkRBACAllow/n=100000/speedle-2 | 5 | 5.358 µs | 4.607 µs | 5.530 µs | 2864 | 47 |
| BenchmarkRBACDeny/n=100/casbin-2 | 5 | 67.367 µs | 67.014 µs | 67.948 µs | 20001 | 615 |
| BenchmarkRBACDeny/n=100/casbin-cached-2 | 5 | 171.80 ns | 170.00 ns | 175.20 ns | 56 | 3 |
| BenchmarkRBACDeny/n=100/casbin-tuned-2 | 5 | 49.895 µs | 49.419 µs | 50.526 µs | 12881 | 319 |
| BenchmarkRBACDeny/n=100/opa-2 | 5 | 21.036 µs | 18.240 µs | 21.404 µs | 8608 | 163 |
| BenchmarkRBACDeny/n=100/speedle-2 | 5 | 5.294 µs | 5.220 µs | 5.400 µs | 2648 | 45 |
| BenchmarkRBACDeny/n=1000/casbin-2 | 5 | 670.255 µs | 663.397 µs | 683.703 µs | 193728 | 6015 |
| BenchmarkRBACDeny/n=1000/casbin-cached-2 | 5 | 211.40 ns | 207.50 ns | 211.80 ns | 112 | 3 |
| BenchmarkRBACDeny/n=1000/casbin-tuned-2 | 5 | 440.093 µs | 388.647 µs | 451.942 µs | 113892 | 3019 |
| BenchmarkRBACDeny/n=1000/opa-2 | 5 | 20.887 µs | 19.055 µs | 21.918 µs | 8608 | 163 |
| BenchmarkRBACDeny/n=1000/speedle-2 | 5 | 5.266 µs | 5.198 µs | 5.311 µs | 2648 | 45 |
| BenchmarkRBACDeny/n=10000/casbin-2 | 5 | 6.971 ms | 6.891 ms | 7.079 ms | 1.92557e+06 | 60015 |
| BenchmarkRBACDeny/n=10000/casbin-cached-2 | 5 | 213.00 ns | 203.10 ns | 220.70 ns | 112 | 3 |
| BenchmarkRBACDeny/n=10000/casbin-tuned-2 | 5 | 4.543 ms | 4.485 ms | 4.630 ms | 1.12555e+06 | 30019 |
| BenchmarkRBACDeny/n=10000/opa-2 | 5 | 21.279 µs | 19.500 µs | 23.679 µs | 8608 | 163 |
| BenchmarkRBACDeny/n=10000/speedle-2 | 5 | 5.244 µs | 4.789 µs | 5.322 µs | 2648 | 45 |
| BenchmarkRBACDeny/n=100000/casbin-2 | 5 | 72.371 ms | 67.519 ms | 73.682 ms | 1.9207e+07 | 600015 |
| BenchmarkRBACDeny/n=100000/casbin-tuned-2 | 5 | 41.735 ms | 39.198 ms | 43.260 ms | 1.12071e+07 | 300019 |
| BenchmarkRBACDeny/n=100000/opa-2 | 5 | 20.657 µs | 18.385 µs | 23.294 µs | 8608 | 163 |
| BenchmarkRBACDeny/n=100000/speedle-2 | 5 | 4.864 µs | 4.323 µs | 5.321 µs | 2648 | 45 |
| BenchmarkRBACMixed/n=100/casbin-2 | 5 | 51.350 µs | 50.577 µs | 52.024 µs | 13452 | 380 |
| BenchmarkRBACMixed/n=100/casbin-cached-2 | 5 | 210.90 ns | 202.80 ns | 216.10 ns | 55 | 2 |
| BenchmarkRBACMixed/n=100/casbin-tuned-2 | 5 | 32.679 µs | 32.444 µs | 33.451 µs | 9109 | 201 |
| BenchmarkRBACMixed/n=100/opa-2 | 5 | 20.794 µs | 19.786 µs | 23.210 µs | 8333 | 155 |
| BenchmarkRBACMixed/n=100/speedle-2 | 5 | 5.794 µs | 5.642 µs | 6.055 µs | 2820 | 46 |
| BenchmarkRBACMixed/n=1000/casbin-2 | 5 | 491.909 µs | 453.511 µs | 922.410 µs | 125110 | 3677 |
| BenchmarkRBACMixed/n=1000/casbin-cached-2 | 5 | 316.00 ns | 312.70 ns | 331.70 ns | 106 | 2 |
| BenchmarkRBACMixed/n=1000/casbin-tuned-2 | 5 | 231.503 µs | 224.492 µs | 234.208 µs | 75948 | 1832 |
| BenchmarkRBACMixed/n=1000/opa-2 | 5 | 27.532 µs | 25.555 µs | 28.822 µs | 8333 | 155 |
| BenchmarkRBACMixed/n=1000/speedle-2 | 5 | 8.248 µs | 8.199 µs | 8.409 µs | 2820 | 46 |
| BenchmarkRBACMixed/n=10000/casbin-2 | 5 | 5.077 ms | 5.013 ms | 7.437 ms | 1.23671e+06 | 36543 |
| BenchmarkRBACMixed/n=10000/casbin-cached-2 | 5 | 252.90 ns | 190.60 ns | 312.30 ns | 111 | 2 |
| BenchmarkRBACMixed/n=10000/casbin-tuned-2 | 5 | 2.258 ms | 2.198 ms | 2.291 ms | 740055 | 17978 |
| BenchmarkRBACMixed/n=10000/opa-2 | 5 | 19.656 µs | 19.033 µs | 20.567 µs | 8333 | 155 |
| BenchmarkRBACMixed/n=10000/speedle-2 | 5 | 6.093 µs | 5.835 µs | 6.692 µs | 2820 | 46 |
| BenchmarkRBACMixed/n=100000/casbin-2 | 5 | 48.887 ms | 47.065 ms | 51.337 ms | 1.17749e+07 | 346646 |
| BenchmarkRBACMixed/n=100000/casbin-tuned-2 | 5 | 23.176 ms | 23.014 ms | 24.241 ms | 7.2528e+06 | 176446 |
| BenchmarkRBACMixed/n=100000/opa-2 | 5 | 21.859 µs | 20.815 µs | 22.186 µs | 8333 | 155 |
| BenchmarkRBACMixed/n=100000/speedle-2 | 5 | 7.765 µs | 7.468 µs | 7.856 µs | 2821 | 46 |
| BenchmarkWildcard/n=1000/casbin-2 | 5 | 324.840 µs | 320.314 µs | 336.613 µs | 105792 | 3018 |
| BenchmarkWildcard/n=1000/casbin-tuned-2 | 5 | 266.869 µs | 263.239 µs | 272.821 µs | 93887 | 2519 |
| BenchmarkWildcard/n=1000/opa-2 | 5 | 21.875 µs | 20.777 µs | 23.170 µs | 8896 | 167 |
| BenchmarkWildcard/n=1000/speedle-2 | 5 | 11.150 µs | 10.915 µs | 11.306 µs | 6576 | 70 |
| BenchmarkWildcard/n=10000/casbin-2 | 5 | 3.119 ms | 2.964 ms | 3.301 ms | 1.04548e+06 | 30018 |
| BenchmarkWildcard/n=10000/casbin-tuned-2 | 5 | 2.803 ms | 2.631 ms | 2.862 ms | 925467 | 25019 |
| BenchmarkWildcard/n=10000/opa-2 | 5 | 23.535 µs | 21.094 µs | 27.933 µs | 8872 | 166 |
| BenchmarkWildcard/n=10000/speedle-2 | 5 | 10.053 µs | 9.968 µs | 10.382 µs | 6576 | 70 |

## 多核吞吐

| benchmark | decisions/s (median ns/op converted) |
|---|---|
| BenchmarkParallel/n=1000/casbin | 2,556 |
| BenchmarkParallel/n=1000/casbin-2 | 5,058 |
| BenchmarkParallel/n=1000/casbin-4 | 5,263 |
| BenchmarkParallel/n=1000/casbin-tuned | 3,841 |
| BenchmarkParallel/n=1000/casbin-tuned-2 | 7,409 |
| BenchmarkParallel/n=1000/casbin-tuned-4 | 7,799 |
| BenchmarkParallel/n=1000/opa | 42,296 |
| BenchmarkParallel/n=1000/opa-2 | 84,610 |
| BenchmarkParallel/n=1000/opa-4 | 101,256 |
| BenchmarkParallel/n=1000/speedle | 152,975 |
| BenchmarkParallel/n=1000/speedle-2 | 282,646 |
| BenchmarkParallel/n=1000/speedle-4 | 321,337 |
| BenchmarkParallel/n=10000/casbin | 249 |
| BenchmarkParallel/n=10000/casbin-2 | 469 |
| BenchmarkParallel/n=10000/casbin-4 | 542 |
| BenchmarkParallel/n=10000/casbin-tuned | 373 |
| BenchmarkParallel/n=10000/casbin-tuned-2 | 704 |
| BenchmarkParallel/n=10000/casbin-tuned-4 | 773 |
| BenchmarkParallel/n=10000/opa | 35,153 |
| BenchmarkParallel/n=10000/opa-2 | 78,180 |
| BenchmarkParallel/n=10000/opa-4 | 89,477 |
| BenchmarkParallel/n=10000/speedle | 134,282 |
| BenchmarkParallel/n=10000/speedle-2 | 263,366 |
| BenchmarkParallel/n=10000/speedle-4 | 302,755 |

## HTTP 压测：三轮中位数

| dataset | setup | conc | rounds | req/s | p50 µs | p90 µs | p99 µs | p99.9 µs | max µs | errors (sum) | wrong (sum) | srv cores | gen cores |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| manyusers n=100000 | opa-server | 1 | 3 | 4189 | 211 | 266 | 824 | 1966 | 4754 | 0 | 0 | 0.944 | 0.472 |
| manyusers n=100000 | opa-server | 16 | 3 | 11604 | 1123 | 2467 | 4489 | 15916 | 59126 | 0 | 0 | 1.991 | 0.907 |
| manyusers n=100000 | opa-server | 64 | 3 | 11396 | 4906 | 9760 | 15195 | 22745 | 118722 | 0 | 0 | 1.995 | 0.911 |
| manyusers n=100000 | speedle-ads | 1 | 3 | 7316 | 128 | 139 | 221 | 1763 | 4383 | 0 | 0 | 0.821 | 0.743 |
| manyusers n=100000 | speedle-ads | 16 | 3 | 25909 | 497 | 1049 | 3152 | 6408 | 11291 | 0 | 0 | 1.881 | 1.438 |
| manyusers n=100000 | speedle-ads | 64 | 3 | 26330 | 1946 | 4699 | 10556 | 15376 | 30208 | 0 | 0 | 1.959 | 1.518 |
| manyusers n=100000 | thin-casbin | 1 | 3 | 4092 | 214 | 287 | 630 | 8343 | 19445 | 0 | 0 | 1.032 | 0.445 |
| manyusers n=100000 | thin-casbin | 16 | 3 | 11012 | 928 | 2540 | 11490 | 23507 | 33886 | 0 | 0 | 1.989 | 0.835 |
| manyusers n=100000 | thin-casbin | 64 | 3 | 10755 | 3856 | 14024 | 34917 | 50944 | 76533 | 0 | 0 | 1.997 | 0.826 |
| manyusers n=100000 | thin-opa | 1 | 3 | 5320 | 169 | 215 | 584 | 1833 | 5451 | 0 | 0 | 0.923 | 0.557 |
| manyusers n=100000 | thin-opa | 16 | 3 | 15766 | 802 | 1763 | 3691 | 14767 | 47739 | 0 | 0 | 1.980 | 1.067 |
| manyusers n=100000 | thin-opa | 64 | 3 | 15469 | 3464 | 7131 | 11458 | 60894 | 139887 | 0 | 0 | 1.985 | 1.053 |
| manyusers n=100000 | thin-speedle | 1 | 3 | 8454 | 111 | 120 | 188 | 1478 | 3675 | 0 | 0 | 0.799 | 0.831 |
| manyusers n=100000 | thin-speedle | 16 | 3 | 30096 | 441 | 875 | 2524 | 6257 | 17678 | 0 | 0 | 1.725 | 1.655 |
| manyusers n=100000 | thin-speedle | 64 | 3 | 30957 | 1719 | 3878 | 8285 | 12042 | 25732 | 0 | 0 | 1.798 | 1.737 |
| plain n=10000 | opa-server | 1 | 3 | 4965 | 178 | 228 | 515 | 1878 | 5668 | 0 | 0 | 0.938 | 0.536 |
| plain n=10000 | opa-server | 16 | 3 | 14621 | 905 | 1731 | 4180 | 14934 | 56406 | 0 | 0 | 1.989 | 1.030 |
| plain n=10000 | opa-server | 64 | 3 | 14290 | 3875 | 7851 | 12214 | 19047 | 117149 | 0 | 0 | 1.995 | 1.033 |
| plain n=10000 | speedle-ads | 1 | 3 | 7322 | 129 | 139 | 220 | 1321 | 4092 | 0 | 0 | 0.819 | 0.741 |
| plain n=10000 | speedle-ads | 16 | 3 | 25953 | 502 | 1055 | 3005 | 4943 | 10757 | 0 | 0 | 1.875 | 1.431 |
| plain n=10000 | speedle-ads | 64 | 3 | 25815 | 2018 | 4844 | 10060 | 14983 | 30157 | 0 | 0 | 1.963 | 1.497 |
| plain n=10000 | thin-casbin | 1 | 3 | 58 | 16097 | 31369 | 60271 | 63789 | 73321 | 0 | 0 | 1.368 | 0.011 |
| plain n=10000 | thin-casbin | 16 | 3 | 99 | 15414 | 501821 | 1.47524e+06 | 2.16936e+06 | 2.52739e+06 | 0 | 0 | 1.972 | 0.018 |
| plain n=10000 | thin-casbin | 64 | 3 | 98 | 22359 | 1.30018e+06 | 4.35319e+06 | 6.29212e+06 | 7.13806e+06 | 0 | 0 | 1.991 | 0.019 |
| plain n=10000 | thin-opa | 1 | 3 | 6829 | 134 | 146 | 373 | 1583 | 4879 | 0 | 0 | 0.850 | 0.672 |
| plain n=10000 | thin-opa | 16 | 3 | 21273 | 559 | 1174 | 3736 | 13701 | 46244 | 0 | 0 | 1.957 | 1.223 |
| plain n=10000 | thin-opa | 64 | 3 | 21020 | 2604 | 5399 | 8775 | 33373 | 121018 | 0 | 0 | 1.979 | 1.223 |
| plain n=10000 | thin-speedle | 1 | 3 | 8416 | 113 | 122 | 178 | 1122 | 3251 | 0 | 0 | 0.795 | 0.825 |
| plain n=10000 | thin-speedle | 16 | 3 | 30373 | 439 | 869 | 2424 | 4473 | 22534 | 0 | 0 | 1.742 | 1.645 |
| plain n=10000 | thin-speedle | 64 | 3 | 30933 | 1744 | 3876 | 7818 | 11894 | 25052 | 0 | 0 | 1.836 | 1.757 |

## 执行与干扰记录

```text
[2026-10-29T09:19:39+08:00] start step=machine attempt=1
[2026-10-29T09:19:41+08:00] complete step=machine attempt=1
[2026-10-29T09:19:52+08:00] start step=functional attempt=1
[2026-10-29T09:19:52+08:00] complete step=functional attempt=1
[2026-10-29T09:20:02+08:00] start step=capabilities attempt=1
[2026-10-29T09:20:02+08:00] complete step=capabilities attempt=1
[2026-10-29T09:20:12+08:00] start step=watchcheck attempt=1
[2026-10-29T09:20:13+08:00] complete step=watchcheck attempt=1
[2026-10-29T09:20:23+08:00] start step=loadcost attempt=1
[2026-10-29T09:20:39+08:00] complete step=loadcost attempt=1
[2026-10-29T09:20:50+08:00] start step=bench-small attempt=1
[2026-10-29T09:37:47+08:00] complete step=bench-small attempt=1
[2026-10-29T09:37:58+08:00] start step=bench-manyusers attempt=1
[2026-10-29T09:40:11+08:00] complete step=bench-manyusers attempt=1
[2026-10-29T09:40:22+08:00] start step=bench-100k-speedle attempt=1
[2026-10-29T09:41:06+08:00] complete step=bench-100k-speedle attempt=1
[2026-10-29T09:41:16+08:00] start step=bench-100k-opa attempt=1
[2026-10-29T09:41:56+08:00] complete step=bench-100k-opa attempt=1
[2026-10-29T09:42:07+08:00] start step=bench-100k-casbin-tuned attempt=1
[2026-10-29T09:43:29+08:00] complete step=bench-100k-casbin-tuned attempt=1
[2026-10-29T09:43:39+08:00] start step=bench-100k-casbin attempt=1
[2026-10-29T09:45:34+08:00] complete step=bench-100k-casbin attempt=1
[2026-10-29T09:45:44+08:00] start step=parallel attempt=1
[2026-10-29T09:50:22+08:00] complete step=parallel attempt=1
[2026-10-29T09:50:32+08:00] start step=stress attempt=1
[2026-10-29T10:17:58+08:00] complete step=stress attempt=1
[2026-10-29T10:17:58+08:00] done: results/charming
```

活动采样 734 行；1 分钟负载范围 0.31–3.96；CI Worker 最大值 0；换入/换出页增量 1115 / 35。

