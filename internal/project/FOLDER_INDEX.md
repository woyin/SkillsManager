# internal/project 文件夹索引

## 架构说明
业务层，管理项目级配置文件 `.sm.json`（profile 名 + 附加 skills/MCP）。
单一职责：读写项目配置；被 installer 持久化安装结果，被 cmd 层加载配置。

## 文件清单

### project.go
- **地位**: .sm.json 存取的唯一实现
- **功能**: NewManager、Load、Save；Config 结构（Profile/Skills/MCP）
- **依赖**: （无内部包依赖）
- **被依赖**: internal/installer（写 .sm.json）、cmd/install（读配置）、cmd/status

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
