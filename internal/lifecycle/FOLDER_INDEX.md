# internal/lifecycle 文件夹索引

## 架构说明
业务层，安装与删除的有界业务单元：安装任务构建/去重与 Placement 批处理结果、
删除引用扫描与清理。所有入口只接收显式 Agent 目录、项目路径与已知项目，
不读 cmd 全局变量，也不做策略决策——flags、确认、输出与 ADR 0017 的
“有引用拒绝/--force”留在 cmd。

## 文件清单

### install.go
- **地位**: Direct Install 编排的唯一实现
- **功能**: `InstallJob`；`BuildJobs` 构建 (agent 目录 × skill) 任务（含 Eve --subagent 展开：root/./空 = 根目录，具名 = agent/subagents/<name>/skills）；`DedupeJobs` 按目标路径保留首个任务（共享 .agents/skills 防竞态）；`ApplyJobs` 调用 `installer.Placement.PlaceMany` + `Commit`，返回 `InstallResult{Installed, Failures}`，单项失败不阻止其他任务
- **依赖**: internal/installer, internal/registry, internal/tool
- **被依赖**: cmd/install

### removal.go
- **地位**: 删除引用扫描与清理的唯一实现
- **功能**: `ScanDirs`（全局可选 + 项目级）；`CollectReferences` 收集全局同名条目/指向链接与项目同名条目，IO 错误进 errors 与 Inaccessible（不得当成“无引用”）；`CleanupInstalls` 删除可访问条目并清 lock entries（无 ProjectDir 时按 IncludeGlobal 仍清全局），返回 `CleanupResult{Removed, Failures, Inaccessible}`；`RemoveLockEntry` 返回锁文件错误
- **依赖**: internal/lockfile, internal/symlink, internal/tool
- **被依赖**: cmd/rm

### install_test.go / removal_test.go
- **覆盖**: 共享项目目录去重、Eve 子代理展开、成功率与单任务失败结果；扫描目录范围、全局+项目引用收集、不可访问项目错误、不可访问清理、空 ProjectDir 的全局清理、坏锁文件错误

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
