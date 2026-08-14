# internal/installer 文件夹索引

## 架构说明
业务层，把 profile + 临时附加项解析为具体的技能安装（symlink/copy）与 MCP 合并。
是核心聚合层，协调 registry（原件）、profile（预设）、project（.sm.json）、tool（目标目录）与 placement（落地契约）。

## 文件清单

### installer.go
- **地位**: 安装业务的核心入口
- **功能**: Install（profile 模式，先 gatherAndPreflight 全量预检、写入失败 rollback 回滚，ADR 0012）、GatherAndPreflight（导出供测试/预检）、InstallFromRegistry（按名从本地库秒装）、createSymlinks（委托 placement）、installMCP（合并 .mcp.json）
- **依赖**: internal/profile, internal/project, internal/registry, internal/tool, placement.go
- **被依赖**: cmd/install（构造并调用）

### placement.go
- **地位**: 可复用的落地深模块，隔离目标目录解析与文件系统副作用
- **功能**: `TargetDirectories`（项目/全局 scope，并对多 agent 共享目录去重）、`Placement.Place`（symlink/copy、冲突策略、可选 symlink→copy fallback）、`PlacementResult.Commit/Rollback`（替换快照与事务回滚）
- **被依赖**: Direct Install 已直接构造 `Placement`，把 source/destination 交给同一并发落地契约

### placement_test.go / installer_test.go
- **覆盖**: 幂等 symlink、交互冲突与恢复、copy 替换与回滚、symlink fallback、冲突策略、scope 目录解析、共享目录去重与 Registry Install 幂等落地、批量落地；包含 `BenchmarkPlacementPlaceMany` 批量安装基准

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
