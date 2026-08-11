# internal/updater 文件夹索引

## 架构说明

业务层的 Registry Source 更新事务。把多个技能原件的 staging、领域校验、
批量提交与失败回滚收敛为一个 all-or-nothing 边界；具体的 Registry metadata
写入与 lint 策略由调用方通过 hooks 提供。

## 文件清单

### transaction.go

- **地位**: Source 更新事务核心
- **功能**: `Apply` 对多个更新目标执行 staging、Prepare、Validate、commit 和 rollback
- **依赖**: `internal/fsutil`
- **被依赖**: `cmd/update`

### transaction_test.go

- **功能**: 覆盖成功提交、校验失败零副作用、提交失败回滚和目标重叠保护

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
