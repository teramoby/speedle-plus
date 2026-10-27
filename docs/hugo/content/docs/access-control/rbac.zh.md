+++
title = "RBAC：基于角色的访问控制"
description = "使用 Speedle+ 实现基于角色的访问控制（RBAC，role-based access control）：角色分配、资源范围、角色继承及可运行的 SPDL 示例。"
weight = 10
keywords = ["RBAC", "role-based access control", "基于角色的访问控制", "授权", "Speedle+"]
bref = "通过角色授予权限，减少逐个用户重复配置"
+++

## Speedle 如何实现 RBAC

基于角色的访问控制（Role-Based Access Control，RBAC）将角色分配与操作权限分开。Speedle 的角色策略把角色分配给用户、组或其他角色；授权策略则允许角色对资源执行某项操作。评估请求时，引擎会解析当前主体适用的角色。

当多人承担相同职责时，这种方式可以减少重复配置：修改“读者”的权限，只需调整相关授权策略，无需逐个修改用户。

## 验证具有资源范围的角色

把下面两行粘贴到 [Playground](/playground/)：

```text
grant user alice reader on book
grant role reader read book
```

用户填 `alice`，资源填 `book`，操作填 `read`，结果为允许：Alice 获得该资源上的 `reader` 角色，而此角色具有读取权限。把用户换成 `bob`，同样请求会被拒绝；Alice 执行 `delete` 也会被拒绝，因为角色没有删除权限。

`on book` 限定了角色分配的资源范围，删除这一限定会改变角色的适用范围。名为 `reader` 的身份本身不等于获得角色，角色关系应通过明确的角色策略建立。

## 角色继承与显式拒绝

角色策略还可以把角色分配给另一个角色。例如，将上面的策略替换为：

```text
grant user alice editor on book
grant role editor reader on book
grant role reader read book
```

相同的 `alice` / `book` / `read` 请求会通过角色链获得允许。角色层级应保持清晰，并验证最终权限。匹配的显式拒绝策略可以覆盖允许策略，因此拥有某个角色并不保证一定允许访问。

如果决策还依赖部门、密级或资源分类，可以组合使用 [ABAC 属性条件](/docs/access-control/abac/)。[PBAC 策略访问控制](/docs/access-control/pbac/)说明管理与执行职责，[授权决策文档](/docs/decisions/)提供角色查询 API。
