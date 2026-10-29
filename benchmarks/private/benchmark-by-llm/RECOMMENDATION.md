# 性能与能力的适用范围分析

这是对本次固定版本、单机测试的解释，不是新的实验或对所有部署的选型结论。完整数据和已复核问题见[报告](REPORT.md)。

对本轮覆盖的常规RBAC/ABAC请求，Speedle+的非缓存决策延迟与同封装HTTP吞吐较好；生产采用前应处理本次b276版本实测的原子替换监听文件退出、坏条件加载未失败两个行为。OPA加载/保留堆更低；Casbin在固定少量角色/权限的形状下明显改善，缓存预热命中结果需单列，不能与所有非缓存路径直接作总体排名。

关系授权可以采用多条路线：可信attribute传入关系或祖先集合，role hierarchy映射继承，custom function执行关系判断。Speedle+官方[条件与角色策略](https://speedle.io/docs/spdl/)及[custom function](https://speedle.io/docs/custom-function/)支持这些扩展机制。OPA提供[graph.reachable](https://www.openpolicyagent.org/docs/policy-reference/builtins/graph)等现成工具；本轮没有比较等价实现的规模、更新、一致性、延迟或外部调用失败处理，因此不能据此给三个产品的关系授权能力排名。

“复杂图遍历”是此前过宽的说法。常见业务问题是按用户、组、项目、文件夹和文档的关系继承权限；它属于授权决策的一类需求，但不是每个Access Management产品都必须采用独立图引擎。s2已验证普通角色继承；s9只实现静态路径样例。用户的追问与修正记录见[PROMPTS.md](PROMPTS.md)、[CORRECTIONS.md](CORRECTIONS.md)。
