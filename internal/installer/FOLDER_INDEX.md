# internal/installer 文件夹索引

## 架构说明
业务层，把 profile + 临时附加项解析为具体的技能安装（symlink/copy）与 MCP 合并。
是核心聚合层，协调 registry（原件）、profile（预设）、project（.sm.json）、tool（目标目录）、symlink/fsutil（落地）。

## 文件清单

### installer.go
- **地位**: 安装业务的核心入口
- **功能**: Install（profile 模式，先 gatherAndPreflight 全量预检、写入失败 rollbackLinks 回滚，ADR 0012）、GatherAndPreflight（导出供测试/预检）、InstallFromRegistry（按名从本地库秒装）、createSymlinks（scope+copy 分支）、installMCP（合并 .mcp.json）
- **依赖**: internal/fsutil, internal/home, internal/profile, internal/project, internal/registry, internal/symlink, internal/tool
- **被依赖**: cmd/install（构造并调用）

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
