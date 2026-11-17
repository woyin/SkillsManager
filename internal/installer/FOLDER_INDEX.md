# internal/installer 文件夹索引

## 架构说明
业务层，把 profile + 临时附加项解析为具体的技能安装（symlink/copy）与 MCP 合并。
是核心聚合层，协调 registry（原件）、profile（预设）、project（.sm.json）、tool（目标目录）与 placement（落地契约）。

## 文件清单

### installer.go
- **地位**: 安装业务的核心入口
- **功能**: Install（兼容包装：BeginInstall → StageProjectConfig → Commit）、GatherAndPreflight（导出供测试/预检）、InstallFromRegistry（普通批量按名从本地库秒装，保持 warning/continue 契约）、createSymlinks（委托 placement）、loadMCPConfig/mergeRegistryMCP（.mcp.json 内存合并且拒绝非法既有 JSON）
- **依赖**: internal/profile, internal/project, internal/registry, internal/tool, placement.go, transaction.go
- **被依赖**: cmd/install、cmd/plan（构造并调用）

### transaction.go
- **地位**: 延迟安装事务的唯一实现（plan v4 接口协议 v2）
- **功能**: BeginInstall/BeginRegistryInstall（全量预检 + 落地 skills + MCP 仅内存合并，不写项目文件）；InstallTransaction.Result/ChangedSkills/ChangedPlacements（只含实际 changed 的 Agent/Skill/Source/Destination/ActualMode/Fallback）；StageProjectConfig（暂存最终配置与写前快照）；Commit（固定顺序 .mcp.json → .sm.json → 释放备份，前两步失败补偿本次实体与已写文件）；Rollback（逆序恢复，幂等）；CommitCleanupError（内容已提交、仅清理失败，含保留备份路径）；ErrTransactionCommitted/ErrTransactionRolledBack
- **依赖**: internal/fsutil（WriteFileAtomic）、internal/project、placement.go
- **被依赖**: cmd/install（Install 包装）、Curation（保留句柄到 ownership 保存完成后 Commit）

### placement.go
- **地位**: 可复用的落地深模块，隔离目标目录解析与文件系统副作用
- **功能**: `TargetDirectories`（项目/全局 scope，并对多 agent 共享目录去重）、`Placement.Place`（symlink/copy、冲突策略、可选 symlink→copy fallback）、`PlacementResult.Commit/Rollback`（替换快照与事务回滚）
- **被依赖**: Direct Install 已直接构造 `Placement`，把 source/destination 交给同一并发落地契约

### placement_test.go / installer_test.go
- **覆盖**: 幂等 symlink、交互冲突与恢复、copy 替换与回滚、symlink fallback、冲突策略、scope 目录解析、共享目录去重与 Registry Install 幂等落地、批量落地；包含 `BenchmarkPlacementPlaceMany` 批量安装基准

### installer_atomic_test.go / transaction_test.go / transaction_symlink_test.go
- **覆盖**: MCP 与配置末次写失败的完整恢复、后续 MCP 合并失败、非法既有 .mcp.json 原字节保留、category 成员失败整体拒绝、恢复失败保留唯一备份、strict 预检零写入、MCP 仅内存合并、ChangedPlacements 的 Agent/Mode、Commit 各步骤失败补偿、cleanup 失败的已提交语义、幂等终态、普通批量部分失败契约；悬空 symlink 下 MCP/配置写入与失败后只删本次 referent、保留原链接

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
