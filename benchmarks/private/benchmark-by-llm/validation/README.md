# 归档阶段的校验记录

这些检查发生于2026-10-29归档整理阶段，不是重新执行的性能测试，也不冒充最初准备阶段的原始终端日志。

- `python3 verify_archive.py`：逐文件SHA、107×5基准、90个HTTP窗口、13清洁步骤、734采样、6个二进制哈希、15个实际执行源码/脚本一致性、本目录Markdown链接与高置信度凭据模式检查通过。工具自身在清单中，MANIFEST自身由Git commit覆盖。
- 在本目录执行 `/opt/homebrew/bin/go vet ./...`：exit 0，无输出；固定Go1.27.2与原依赖，无新增引擎实验。
- `bash -n run-all.sh stress.sh build-linux.sh`：exit 0。
- 主模块隔离检查：见[parent-module-isolation.txt](parent-module-isolation.txt)。普通全仓库go list发现两个既有GOPATH样例导入错误；未修改它们。带-e列出包验证归档嵌套模块不被主模块./...包含。
- 两张PNG已在本次会话中打开查看；SVG同时保留。没有截图驱动的额外性能数据。
- 仓库公开；归档路径/认证元数据已脱敏，原/公开哈希和变换分别保留。该模式检查是归档完整性检查，不宣称完整安全审计。
