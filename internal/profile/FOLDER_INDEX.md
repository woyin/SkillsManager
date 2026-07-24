# internal/profile 文件夹索引

## 架构说明
业务层，管理命名的 skills + MCP 预设（profile）。
以 JSON 文件（`<profiles>/<name>.json`）持久化，Save 即覆盖写；被 installer 与 cmd/profile 共用。

## 文件清单

### profile.go
- **地位**: profile 存取的唯一实现
- **功能**: Load/Save/List/Delete；Profile 结构（Skills + MCP）；Config 别名兼容
- **依赖**: （无内部包依赖）
- **被依赖**: internal/installer（载入 profile）、cmd/profile（CRUD 子命令）

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
