# 策略管理控制台 UI 设计 v0

Issue #2（策略管理控制台）第一期的界面设计。截图来自分支 `claude/policy_management_console` 上已实现的控制台：真实的 PMS、ADS 和 `speedle-console`，加上示例策略和一个模拟的令牌断言服务。分辨率 1440×900，Chromium 截取整页。

## 示例数据

| 服务 | 内容 |
|---|---|
| `library` | 5 条授权策略、7 条角色策略。角色链 alice → manager → librarian → reader，其中 librarian → manager 构成一个循环；group:staff 有条件地获得 reader；user:bob 被拒绝 reader；user:dave 只在 `/books/rare` 上获得 archivist |
| `speedle-console` | 控制台自身的权限策略：alice 通过角色 console-admin 获得 read 和 write，group:auditors 获得 read |
| `hr-portal` | 1 条授权策略、1 条角色策略 |
| `global` | 1 条角色策略：group:compliance 获得 auditor，作用于所有服务 |

登录用户：alice（管理员）、eve（auditors 组，只读）、mallory（没有任何权限）。

## 页面

| 文件 | 页面 | 说明 |
|---|---|---|
| `01-login.png` | 登录 | 输入令牌和可选的令牌类型，由令牌断言服务验证 |
| `02-services.png` | 服务列表 | 每个服务的授权策略和角色策略数量；支持筛选和新建服务 |
| `03-service-delete-confirm.png` | 删除服务 | 必须输入完整的服务名称，删除按钮才可用 |
| `04-policies.png` | 授权策略列表 | 主体中，`\|` 表示“或”，`&` 表示“且”；`~` 表示资源表达式 |
| `05-policy-editor.png` | 编辑授权策略 | 主体每行一个备选条件，同一行内用逗号分隔表示“且”；权限可以是资源或表达式 |
| `06-policy-editor-json.png` | JSON 视图 | 在表单和 JSON 之间切换；metadata 由 PMS 维护，不在这里编辑 |
| `07-policy-validation.png` | 校验错误 | 错误显示在对应字段下方，并聚焦第一个出错的字段 |
| `08-role-policies.png` | 角色策略列表 | 角色、主体、资源范围和条件 |
| `09-role-policy-editor.png` | 编辑角色策略 | 带条件的角色策略 |
| `10-roles.png` | 角色关系分析 | 分层关系图：实线表示授予，红线表示拒绝，虚线表示带条件或资源范围。上方列出循环、未定义和未使用的角色。选中 reader 后显示谁持有它、经由哪些角色、拒绝策略和引用它的授权策略；右侧向 ADS 查询某个主体的角色 |
| `11-diagnose.png` | 诊断 | ADS 的结果、原因、获得的角色，以及每条生效的角色策略和授权策略及其条件结果，名称可跳转到编辑器 |
| `12-edit-conflict.png` | 修改冲突 | 打开之后有人修改了同一策略（ETag 不一致，HTTP 412），提示重新加载，不会覆盖别人的修改 |
| `13-read-only.png` | 只读用户 | eve 只有 read 权限：所有字段不可编辑，没有保存或删除按钮 |
| `14-no-access.png` | 没有权限 | mallory 已登录，但没有任何策略允许读取控制台 |

`zh/` 目录下是同样页面的中文界面：01、02、04、05、08、10、11。

## v0 已知问题

- `10-roles.png`：librarian → manager 这条回边从节点上方绕回，但正好穿过 auditor 节点，看起来像是 auditor 连向 librarian。回边需要绕开中间的节点。
- 名称字段带有 HTML 的 `required` 属性，名称为空时，显示的是浏览器自带的提示，而不是与其他字段一致的红色错误文字。
- 列表中“1 items”之类的英文复数没有区分单复数。
