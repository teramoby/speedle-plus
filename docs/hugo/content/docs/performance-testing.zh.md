+++
title = "性能测试指南"
description = "借助 AI 编程工具，在你自己的机器上公平、可重复地对比授权引擎的性能。"
weight = 15
draft = false
toc = true
singlelinetitle = true
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "performance", "benchmark", "policy"]
categories = ["docs"]
bref = "在你自己的硬件和策略集上测量 Speedle 和其他授权引擎"
lastmod = 2026-11-11T12:00:00+08:00
+++

本指南帮助你在自己的机器上对比 Speedle 和其他授权引擎。本指南不发布性能报告。数字取决于你的硬件、策略集和固定的引擎版本，因此值得信任的结果是你自己测出来的。

本指南提供一个共享场景、四种引擎的等价策略、方法清单，以及一段可以粘贴到 Claude Code、ChatGPT Codex、DeepSeek Harness 或类似 AI 编程工具中的提示词。提示词要求工具编写脚本、运行脚本，并把原始输出留在你的机器上。把工具写出的内容当作需要审查的代码，而不是已经发布的结果。

## 本指南提供什么

- Speedle、Open Policy Agent（OPA）、Casbin 和 Cedar 的等价 RBAC 与 ABAC 规则，以及每个请求的预期决策。
- 在你的机器上生成并运行的可执行基准脚本、准确命令和固定的引擎版本。
- 延迟分位数、吞吐量、内存占用和规模变化，其中策略加载与稳态评估分开测量。
- 原始输出、环境信息和结果模板，全部留在本地。本指南不会上传任何内容。

候选引擎只是起点，不是排名。确认你选择的每个引擎版本支持该场景，记录你排除的用例及原因，不要把被排除的用例当作该引擎的结果。

## 开始之前

- 使用一台你能控制、并能在一段时间内保持空闲的机器。关闭其他占用资源较多的进程，记录 CPU 型号、核数、内存和操作系统。
- 安装对比所需的工具：Speedle 和基于 Go 的引擎需要 Git 与 Go 1.26 或更新版本，其他引擎按各自官方快速入门的要求安装。固定每个引擎的版本，包括 Speedle 的发布标签。
- 使用一个能在临时目录中创建文件并执行 shell 命令的 AI 编程工具。运行前审查它生成的脚本，并保留整个目录（包括原始输出）作为记录。
- 第一次用小策略集对比大约需要 30–60 分钟；增加策略数量和并发数时需要更多时间。

在一个空目录中工作。不要在 Speedle 代码仓库里运行对比；把生成的脚本、策略、日志和结果与任何仓库分开保存。

## 共享场景

对比使用同一组业务规则：

- 员工可以读取本部门的文档。
- 编辑可以读取和编辑本部门的文档。
- Bob 是编辑，但被明确禁止编辑第四季度财务报告。
- 跨部门请求即使用户拥有对应角色也会被拒绝。

请求由应用携带用户的 `department` 和资源的 `owner_department` 两个属性。所有引擎接收相同的请求，并且必须给出相同的决策。

| 用户 | 角色 | 操作 | 资源 | department | owner_department | 预期 |
| --- | --- | --- | --- | --- | --- | --- |
| alice | employee | read | `/docs/finance/report` | finance | finance | 允许 |
| alice | employee | read | `/docs/hr/salaries` | finance | hr | 拒绝 |
| bob | editor | edit | `/docs/finance/report` | finance | finance | 允许 |
| bob | editor | edit | `/docs/finance/q4` | finance | finance | 拒绝（显式拒绝） |
| carol | editor | edit | `/docs/finance/q4` | hr | finance | 拒绝（部门条件） |
| carol | editor | read | `/docs/hr/handbook` | hr | hr | 允许 |

本指南的示例已在 Speedle v1.6.0、OPA v1.22.3、Casbin v2.123.0 和 Cedar CLI v4.14.1 上验证。请固定你安装的版本；这些只是示例，不代表只支持这些版本。

### Speedle（SPDL）

```spdl
[service.documents]

[policy]
grant role employee read expr:/docs/.* if department == owner_department
grant role editor read,edit expr:/docs/.* if department == owner_department
deny user bob edit /docs/finance/q4

[rolepolicy]
grant user alice role employee
grant user bob role editor
grant user carol role editor
```

Speedle 中生效的拒绝优先于授权。条件引用的属性在请求中缺失时不匹配，因此应用必须在每个请求中发送 `department` 和 `owner_department`。策略文件用 `spctl check` 校验；语法见 [SPDL](../spdl/)，服务和角色策略见[策略管理](../pms/policy-mgmt/)。

### OPA（Rego）

策略：

```rego
package documents

import rego.v1

default allow := false

allow if {
	not denied
	input.action == "read"
	"employee" in roles
	input.attributes.department == input.attributes.owner_department
}

allow if {
	not denied
	input.action in {"read", "edit"}
	"editor" in roles
	input.attributes.department == input.attributes.owner_department
}

roles contains role if {
	some role in data.roles[input.user]
}

denied if {
	input.user == "bob"
	input.action == "edit"
	input.resource == "/docs/finance/q4"
}
```

数据（`data.json`）：

```json
{
  "roles": {
    "alice": ["employee"],
    "bob": ["editor"],
    "carol": ["editor"]
  }
}
```

Rego 没有内置的拒绝优先规则，因此上面的策略在两个允许规则中都减去了显式拒绝。角色数据是 OPA 数据文档的一部分。使用你所固定的 OPA 版本对应的 Rego 语法（示例使用 OPA 1.x 的 Rego v1 语法）。

### Casbin

模型（`model.conf`）：

```ini
[request_definition]
r = sub, obj, act, dept, ownerDept

[policy_definition]
p = sub, obj, act, eft

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (r.sub == p.sub || g(r.sub, p.sub)) && keyMatch(r.obj, p.obj) && regexMatch(r.act, p.act) && r.dept == r.ownerDept
```

策略（`policy.csv`）：

```csv
p, employee, /docs/*, read, allow
p, editor, /docs/*, read, allow
p, editor, /docs/*, edit, allow
p, bob, /docs/finance/q4, edit, deny
g, alice, employee
g, bob, editor
g, carol, editor
```

Casbin 把模型、策略和角色分配拆成三份输入。ABAC 的部门检查写在匹配表达式里，因此它也作用于拒绝行；本场景的拒绝请求带有匹配的部门属性，所以预期决策不变。如果你为了表达无条件拒绝而修改匹配表达式，请记录这一改动。

### Cedar

Schema（`schema.cedar`）：

```cedar
entity Role;
entity User in [Role];
entity Folder;
entity Document in [Folder];

action "read" appliesTo {
  principal: [User],
  resource: [Document],
  context: { department: String, ownerDepartment: String }
};
action "edit" appliesTo {
  principal: [User],
  resource: [Document],
  context: { department: String, ownerDepartment: String }
};
```

策略（`policies.cedar`）：

```cedar
permit (
  principal in Role::"employee",
  action == Action::"read",
  resource in Folder::"/docs"
)
when { context.department == context.ownerDepartment };

permit (
  principal in Role::"editor",
  action in [Action::"read", Action::"edit"],
  resource in Folder::"/docs"
)
when { context.department == context.ownerDepartment };

forbid (
  principal == User::"bob",
  action == Action::"edit",
  resource == Document::"/docs/finance/q4"
);
```

实体数据（`entities.json`）：

```json
[
  {"uid": {"type": "User", "id": "alice"}, "attrs": {}, "parents": [{"type": "Role", "id": "employee"}]},
  {"uid": {"type": "User", "id": "bob"}, "attrs": {}, "parents": [{"type": "Role", "id": "editor"}]},
  {"uid": {"type": "User", "id": "carol"}, "attrs": {}, "parents": [{"type": "Role", "id": "editor"}]},
  {"uid": {"type": "Folder", "id": "/docs"}, "attrs": {}, "parents": []},
  {"uid": {"type": "Document", "id": "/docs/finance/report"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/finance/q4"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/hr/salaries"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]},
  {"uid": {"type": "Document", "id": "/docs/hr/handbook"}, "attrs": {}, "parents": [{"type": "Folder", "id": "/docs"}]}
]
```

Cedar 中拒绝（forbid）优先于允许（permit），每个请求默认拒绝。角色和文档层级是实体存储中的数据，不是策略文本，因此 schema 和实体数据也是对比的一部分。示例实体列表必须包含请求矩阵用到的每个文档。

### 需要记录的语义差异

同一条业务规则并不总能映射成相同的引擎概念。在计时之前，先让四个实现给出一致的预期决策，并记录：

- 每个引擎如何处理同时匹配允许和拒绝的请求，拒绝是无条件的，还是与 ABAC 条件共享。
- 请求缺少 `department` 或 `owner_department` 时每个引擎的行为。
- 资源模式如何表达（SPDL 的 `expr:` 正则表达式、Casbin 匹配表达式、OPA 辅助规则、Cedar 实体层级）。
- 哪些部分是策略文本之外必需的应用代码或配置（Cedar schema 与实体、Casbin 模型与角色数据、OPA 数据文档、Speedle 服务与角色策略）。
- 某个引擎在你选定的版本中无法表达的用例；把它从排名用例中排除并说明原因。

## 测量正确的内容

每个引擎都使用同一台机器、同一份请求数据和同样的策略集规模。固定版本并保留生成的数据集，这样重跑时测量的是同样的工作。

- **把嵌入式评估与服务或 API 测试分开。** 进程内函数调用和 HTTP 请求测量的不是同一件事。把它们放在不同的表里报告，不要互相排名。
- **预热并重复。** 每个引擎运行足够多次以达到稳定结果，丢弃预热运行，整个测量至少重复三次，并报告运行之间的波动。
- **报告延迟和吞吐量。** 在引擎工具支持时，包含 p50、p95 和 p99 延迟以及每秒请求数。记录失败和超时。
- **测量内存。** 记录峰值常驻内存或引擎自身的可一致测量的内存指标。
- **改变策略数量和并发数。** 从小规模开始，再增加策略集和并发调用数。记录每个数据点，不只记录最好看的。
- **把策略加载和启动单独测量。** 记录每个引擎在稳态循环之前解析和加载策略集所需的时间，不要把这项成本藏在第一个请求里。
- **说明缓存和准备工作。** 记录索引、编译后的策略、决策缓存和任何预计算，以及它们是热的还是冷的。
- **保留原始输出。** 为每次运行保存准确的命令、配置、策略文件、引擎版本、环境和完整输出。

## 用 AI 编程工具运行对比

在 Claude Code、ChatGPT Codex、DeepSeek Harness 或类似能够写文件并执行命令的工具中打开一个空目录，原样粘贴下面的提示词。它要求工具记录环境、固定版本、构建共享场景、验证预期决策、运行测量，并在本地生成结果模板。

```text
帮我在自己的机器上快速对比 Speedle 与 OPA、Casbin 和 Cedar 的性能。先记录我的操作系统、硬件和资源限制，然后选择并固定受支持的引擎版本。用共享数据集和预期的允许/拒绝决策构建等价的 RBAC 和 ABAC 示例。说明语义差异，并把不支持的用例排除在直接排名之外。在测量性能之前先验证正确性。

创建可运行的基准脚本和准确的安装/运行命令。把嵌入式测试与服务/API 测试分开。使用相同的工作负载，改变策略数量和并发数，说明缓存和准备工作，预热每个引擎并重复测量。在支持的情况下报告延迟、吞吐量、内存占用和运行之间的波动，并附上原始输出和环境信息。只报告你实际测量过的数字；标出你无法运行的内容。

展示每个引擎的等价策略以及所需的模型、schema 或应用代码。比较可读性、需要学习的概念、编写成本、验证、调试以及修改策略的难易程度。解释权衡，以及每个引擎适合哪种测试负载，不要预设 Speedle 会赢。
```

工具应在当前目录中工作，并把脚本、策略和输出留在那里。第一次运行前审查脚本，尤其是任何下载或安装步骤。如果工具无法在你的环境中安装某个引擎，让它把该引擎标记为未运行，而不是估算一个数字。

## 检查 AI 工具的工作

- 在开始计时之前，确认四个实现对所有请求都返回表中列出的决策。又快又错的答案不是结果。
- 确认它安装的版本与报告中的版本一致，并且每个引擎都使用了同一台机器、相同的请求数据和相同的策略集规模。
- 确认加载和启动与稳态循环分开报告，嵌入式测量与服务/API 测量在不同的表里。
- 确认摘要中的每个数字都有原始输出；没有命令和输出支撑的数字视为未测量。
- 阅读局限性说明。无法运行某个用例的引擎必须标注出来，而不是参与排名。

## 结果模板

让工具填写一份如下的本地表格，并与原始输出放在一起。不要把它作为 Speedle 的性能报告发布。

| 引擎 | 版本 | 模式 | 策略数 | 并发数 | p50 | p95 | p99 | 吞吐量 | 峰值内存 | 策略加载时间 |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Speedle | | 嵌入式 | | | | | | | | |
| OPA | | 嵌入式 | | | | | | | | |
| Casbin | | 嵌入式 | | | | | | | | |
| Cedar | | 嵌入式 | | | | | | | | |

再保留一份运行记录：

| 运行 | 引擎 | 命令 | 原始输出 | 备注 |
| --- | --- | --- | --- | --- |
| 1 | | | | |

## 易用性对比

测量和易用性是分开的观察。使用同一组业务规则，记录每个引擎如何表达它们：

- **Speedle（SPDL）。** 每条规则是一条语句，角色分配是单独的角色策略段，条件是请求属性上的布尔表达式。策略可以用 `spctl check` 检查。
- **OPA（Rego）。** 规则以默认拒绝为前提写成规则体，角色数据放在单独的数据文档中，显式拒绝需要自己的规则以及在允许规则中的排除条件。
- **Casbin。** 模型、策略行和角色分配是三份分开的输入。ABAC 检查是匹配表达式的一部分，因此修改条件意味着修改模型。
- **Cedar。** schema 声明实体和操作，策略是 permit 和 forbid 语句，角色和资源层级放在实体存储中。forbid 优先于 permit。

对每个引擎记录：可读性（规则读起来有多接近业务描述）、必须学习的概念、样板代码和应用代码、验证与错误信息、测试和调试，以及新增或修改一条规则需要做什么。策略行数可以作为参考，但不要把它当作衡量易用性的唯一标准。

## Speedle 专用基准测试

仓库中的[基准测试](https://github.com/teramoby/speedle-plus/blob/master/benchmarks/README.md)覆盖 Speedle 自身的评估，包括一个需要显式开启的百万策略用例。在代码仓库中运行 `go test -bench .`，百万策略用例按 README 的说明运行。这些测量只针对 Speedle，且来自特定硬件；不要把它们当作当前的跨引擎对比。

## 公正地报告

- 每个引擎使用相同的场景、数据和策略集规模，并说明每个引擎无法运行哪些用例。
- 报告完整的测量范围，包括其他引擎更快的用例。
- 说明每个引擎适合哪种测试负载及其局限。不要用一台机器上的一个场景宣布某个引擎普遍最快。
- 分享任何摘要时，保留原始输出和准确命令。
