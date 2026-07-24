# internal/prompt 文件夹索引

## 架构说明
业务层，管理提示词（prompt）模板的存取。
与 profile 类似的文件式存储，独立于 skills/MCP 主链路，仅服务 cmd/prompt 子命令。

## 文件清单

### prompt.go
- **地位**: prompt 存取的唯一实现
- **功能**: 提示词模板的增删查、应用（apply）
- **依赖**: （无内部包依赖）
- **被依赖**: cmd/prompt（prompt list/show/apply/create/delete）

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
