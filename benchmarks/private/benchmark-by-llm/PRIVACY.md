# 公开范围、脱敏与完整性

目标仓库公开，private只是路径名称。用户在测量完成后明确要求归档至该仓库；公开副本遵循最小必要范围。

- `/Users/` 与 `/home/` 下的本机账号目录分别替换为 LOCAL_USER / REMOTE_USER；主机代号、CPU型号、时间、PID、性能和CI/换页计数保留，以便审计环境。
- GitHub API快照中的认证相关 permissions 字段移除。查询所需仓库/贡献者标识、贡献次数和统计保留，移除无关头像/关注链接、issue正文、release正文与上传者资料；未包含API密钥、令牌、SSH私钥、认证配置或用户目录内容。
- 源码快照重新封装，tar uid/gid置0、uname/gname清空，不保留macOS xattr/PAX账户元数据；成员文本使用同样的目录脱敏。
- PNG/SVG图表和不含上述内容的源文件保留原字节；测量数值、请求判定、轮次、模块版本、原始二进制哈希未改。
- 大型可执行文件和总包没有提交，原始SHA-256保留在provenance/build-artifacts.json；可从固定源码与依赖重建。

[publication-source-records.json](provenance/publication-source-records.json) 记录已复制文件原始/公开哈希和变换；[archive-entry-hashes.json](provenance/archive-entry-hashes.json) 覆盖两个压缩源码/历史归档的成员；[MANIFEST.json](MANIFEST.json) 覆盖最终公开文件（清单自身由Git commit覆盖）。

本目录不宣称公开副本与未脱敏原件字节相同。Go运行时源码、固定依赖与两个正式运行脚本另外与开跑前源码快照核对；后处理脚本和报告在测量后完善，具体修正另有记录。
