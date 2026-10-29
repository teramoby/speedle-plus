/** Display text only; names, IDs and policy content stay as they are. */
export const messages = {
  "ADS evaluates the request with the policies it has loaded and shows which policies applied.": "ADS 使用已加载的策略评估该请求，并列出生效的策略。",
  "Action": "操作",
  "Actions": "操作",
  "Add at least one permission.": "请至少添加一项权限。",
  "Add permission": "添加权限",
  "Administrator": "管理员",
  "All granted permissions": "全部已授予权限",
  "Allowed": "允许",
  "Always": "始终",
  "Attributes": "属性",
  "Attributes must be a JSON array.": "属性必须是 JSON 数组。",
  "Cancel": "取消",
  "Choose grant or deny.": "请选择 grant 或 deny。",
  "Condition": "条件",
  "Condition false": "条件不成立",
  "Create": "创建",
  "Created service {name}.": "已创建服务 {name}。",
  "Cycle: {roles}": "循环：{roles}",
  "Delete": "删除",
  "Delete service": "删除服务",
  "Delete {name}?": "确定删除 {name}？",
  "Deleted service {name}.": "已删除服务 {name}。",
  "Deleted {name}.": "已删除 {name}。",
  "Denied": "拒绝",
  "Deny role policies that can remove it": "可能撤销该角色的拒绝型角色策略",
  "Development mode: there is no sign-in and everyone is an administrator. Never expose this console.": "开发模式：无需登录，所有人都是管理员。切勿对外开放此控制台。",
  "Diagnose": "诊断",
  "Diagnose a request": "诊断请求",
  "Each attribute needs a name, a type and a value.": "每个属性都需要 name、type 和 value。",
  "Edit as JSON": "以 JSON 编辑",
  "Edit as form": "以表单编辑",
  "Effect": "效果",
  "Enter a JSON object.": "请输入一个 JSON 对象。",
  "Enter a name.": "请输入名称。",
  "Enter a resource or a resource expression.": "请输入资源或资源表达式。",
  "Enter a service.": "请输入服务。",
  "Enter at least one action.": "请至少输入一个操作。",
  "Enter at least one role.": "请至少输入一个角色。",
  "Enter role names without a type, such as admin.": "角色名不带类型前缀，例如 admin。",
  "Expression": "表达式",
  "Filter": "筛选",
  "Filter services": "筛选服务",
  "Granted roles": "获得的角色",
  "ID": "ID",
  "Ignored": "未生效",
  "Include role policies of the global service": "包含 global 服务的角色策略",
  "JSON": "JSON",
  "Loading…": "正在加载…",
  "Main": "主导航",
  "Name": "名称",
  "New policy": "新建授权策略",
  "New role policy": "新建角色策略",
  "New service": "新建服务",
  "No authorization policy applied.": "没有适用的授权策略。",
  "No authorization policy names this role.": "没有授权策略引用该角色。",
  "No cycles, undefined or unused roles.": "没有循环、未定义或未使用的角色。",
  "No permissions.": "没有权限。",
  "No principal is granted this role.": "没有主体被授予该角色。",
  "No role policy applied.": "没有适用的角色策略。",
  "No roles.": "没有角色。",
  "No services.": "暂无服务。",
  "Nothing here yet.": "暂无内容。",
  "One alternative per line; a line matches when all its principals, separated by commas, apply. Leave empty for everyone.": "每行一个备选条件；同一行内用逗号分隔的主体须同时满足。留空表示所有人。",
  "One per line, such as user:alice or idd=corp:group:staff. Leave empty for an anonymous subject.": "每行一个，例如 user:alice 或 idd=corp:group:staff。留空表示匿名主体。",
  "One per line. ADS works out the roles, conditions included.": "每行一个。由 ADS 计算角色，包括条件判断。",
  "One principal per line, such as user:alice, group:ops or role:manager. Leave empty for everyone.": "每行一个主体，例如 user:alice、group:ops 或 role:manager。留空表示所有人。",
  "One role per line, such as admin.": "每行一个角色，例如 admin。",
  "Only when a condition or resource scope applies": "仅当条件成立或资源范围匹配时",
  "Optional JSON array for conditions.": "可选，供条件使用的 JSON 数组。",
  "Optional SPDL condition, such as request_time > 9.": "可选的 SPDL 条件，例如 request_time > 9。",
  "Optional SPDL condition.": "可选的 SPDL 条件。",
  "Optional regular expressions, one per line.": "可选的正则表达式，每行一个。",
  "Optional; passed to the token asserter.": "可选；会传给令牌断言服务。",
  "Optional; roles scoped to resources need one.": "可选；限定了资源范围的角色需要填写。",
  "Optional; the role applies only to these resources.": "可选；角色只在这些资源上生效。",
  "Permissions": "权限",
  "Policies": "授权策略",
  "Policies that use it": "引用该角色的授权策略",
  "Policy console": "策略管理控制台",
  "Principal": "主体",
  "Principals": "主体",
  "Read only": "只读",
  "Reason": "原因",
  "Remove": "移除",
  "Resource": "资源",
  "Resource expressions": "资源表达式",
  "Resource kind": "资源类型",
  "Resources": "资源",
  "Role graph": "角色关系图",
  "Role policies": "角色策略",
  "Role {name}": "角色 {name}",
  "Role {name} is granted but no policy or role uses it.": "角色 {name} 已被授予，但没有任何策略或角色使用它。",
  "Role {name} is used but no role policy grants it.": "角色 {name} 被引用，但没有角色策略授予它。",
  "Roles": "角色",
  "Roles of a subject": "主体拥有的角色",
  "Save": "保存",
  "Saved {name}.": "已保存 {name}。",
  "Service": "服务",
  "Service name": "服务名称",
  "Service sections": "服务分区",
  "Services": "服务",
  "Show roles": "查看角色",
  "Sign in": "登录",
  "Sign in with a token your identity provider issued. The console checks it with the token asserter.": "请使用身份提供方签发的令牌登录，控制台会通过令牌断言服务验证它。",
  "Sign out": "退出登录",
  "Skip to content": "跳到正文",
  "Solid arrows grant a role, red ones deny it, dotted ones have a condition or resource scope. Select a role to see who holds it.": "实线箭头表示授予角色，红色表示拒绝，虚线表示带有条件或资源范围。选择一个角色可查看谁持有它。",
  "Someone changed this item after you opened it. Reload it and make your change again.": "打开后已有人修改了此项。请重新加载后再修改。",
  "Status": "状态",
  "Subject principals": "主体身份",
  "The JSON is not valid.": "JSON 格式不正确。",
  "The authorization service is unavailable.": "授权服务不可用。",
  "The policy service is unreachable.": "无法连接策略服务。",
  "The token was not accepted.": "令牌未通过验证。",
  "These policies decide who may use the console. A mistake here can lock you out.": "这些策略决定谁可以使用控制台。改错可能导致你自己无法再登录。",
  "This deletes service {name} with all its policies and role policies. Type the service name to confirm.": "此操作会删除服务 {name} 及其全部授权策略和角色策略。请输入服务名称确认。",
  "This service has no role policies.": "该服务没有角色策略。",
  "Through": "经由",
  "Token": "令牌",
  "Token type": "令牌类型",
  "Took effect": "已生效",
  "Type": "类型",
  "When": "生效条件",
  "Who holds role {name}": "谁持有角色 {name}",
  "Write principals as type:name, such as user:alice, group:ops or role:admin.": "主体格式为 类型:名称，例如 user:alice、group:ops 或 role:admin。",
  "Write subject principals as type:name with type user, group or entity, such as user:alice.": "主体格式为 类型:名称，类型为 user、group 或 entity，例如 user:alice。",
  "You are a bootstrap administrator. Write the console policies in service {service}, then remove --bootstrap-admins.": "你是引导管理员。请在服务 {service} 中写好控制台策略，然后去掉 --bootstrap-admins 参数。",
  "You are signed in, but no policy lets you read the console.": "你已登录，但没有任何策略允许你读取控制台。",
  "You do not have permission to do this.": "你没有执行此操作的权限。",
  "all": "全部",
  "everyone": "所有人",
  "{count} items": "共 {count} 项",
  "{roles} roles, {edges} grants": "{roles} 个角色，{edges} 条授予关系",
};

const STORAGE_KEY = 'speedle-console-locale';
let locale = 'en';

function storage() {
  try { return globalThis.localStorage; } catch { return null; }
}

/** Chooses the saved language, else the browser's. */
export function initLocale() {
  let saved = null;
  try { saved = storage()?.getItem(STORAGE_KEY); } catch { saved = null; }
  const preferred = saved || globalThis.navigator?.language || 'en';
  locale = String(preferred).toLowerCase().startsWith('zh') ? 'zh' : 'en';
  return locale;
}

export function getLocale() { return locale; }

export function setLocale(value) {
  locale = value === 'zh' ? 'zh' : 'en';
  try { storage()?.setItem(STORAGE_KEY, locale); } catch { /* not saved */ }
  if (globalThis.document?.documentElement) document.documentElement.lang = locale === 'zh' ? 'zh-CN' : 'en';
}

/** Marks a string for translation where it is defined; t() translates it where it is shown. */
export const N_ = (s) => s;

/** Translates key and fills {name} placeholders from vars. */
export function t(key, vars = {}) {
  const text = locale === 'zh' && messages[key] !== undefined ? messages[key] : key;
  return text.replace(/\{(\w+)\}/g, (m, name) => (vars[name] !== undefined ? String(vars[name]) : m));
}
