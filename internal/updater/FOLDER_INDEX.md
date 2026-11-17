# internal/updater 文件夹索引

## 架构说明
业务层的 Registry Source 更新实现：文件系统事务（staging/校验/批量提交/回滚）、
按 Source+Ref 的 Origin 分组与刷新、原子回写 Registry 原件并写回 registry schema
的 provenance，以及项目 skills-lock.json 的哈希刷新。模块只接收显式目录参数
（RegistryDir/DataDir），不读 cmd 全局变量；CLI 文案、范围选择与退出码留在 cmd。

## 文件清单

### transaction.go
- **地位**: Source 更新事务核心
- **功能**: `Apply` 对多个更新目标执行 staging、Prepare、Validate、commit 和 rollback；直接写入已保留的空暂存目录，避免每目标一次删除/重建
- **依赖**: `internal/fsutil`
- **被依赖**: `internal/updater`（origin.go）、cmd/update

### origin.go
- **地位**: Registry Origin 刷新业务
- **功能**: `OriginTarget`/`OriginGroup`/`GroupOriginTargets` 按 Source+Ref 分组（保留首个出现顺序）；`RefreshOriginGroup` 处理 pinned（不前进）与 tracking（detached 视为 pinned，dirty/pull 失败为组级 error）并刷新 source cache；`RewriteOriginTargets` 复用 `Apply` 原子回写，Prepare 写入 `registry.WriteOrigin`，Validate 用 `LintSkill` 阻断坏内容
- **依赖**: `Apply`、`internal/registry`、`internal/sourcecache`
- **被依赖**: cmd/update（refreshOriginGroup/rewriteOriginSkills 门面）

### git.go
- **功能**: `IsDetached`（symbolic-ref 判定 pinned）、`IsDirty`（status --porcelain）、`HeadHash`（rev-parse HEAD）
- **被依赖**: origin.go、cmd/update 的 git 门面

### lock.go
- **功能**: `RefreshProjectLock` 对已存在的 lock 条目重算 `ComputedHash`、补 ref；只更新同名条目，错误通过 `LockResult.Err` 暴露，cmd 保持 warning 与退出码
- **依赖**: `internal/lockfile`
- **被依赖**: cmd/update

### transaction_test.go
- **功能**: 事务成功提交、校验/Prepare 失败零副作用、提交失败回滚、目标重叠保护与基准

### origin_test.go
- **功能**: 分组隔离与顺序、pinned 不前进、detached 视为 pinned、dirty 组级错误、整组原子回滚（lint 失败）、registry origin schema 保留

### lock_test.go
- **功能**: 只更新已存在条目与 ref 补齐；锁文件不可读时 Err 暴露

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
