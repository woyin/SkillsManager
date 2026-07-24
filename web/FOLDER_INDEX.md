# web 文件夹索引

## 架构说明
API 层，sm 内嵌的 Web 仪表盘后端：静态前端（`//go:embed`）+ JSON REST API。
独立于 CLI 命令层，通过 registry 包读取数据；由 cmd/web 启动 HTTP 服务。

## 文件清单

### handler.go
- **地位**: Web 仪表盘的唯一处理器
- **功能**: RegisterRoutes（静态资源 + API）、JSON 接口（技能列表/详情、MCP、安装历史）、webPort 配置
- **依赖**: net/http, embed, internal/registry
- **被依赖**: cmd/web（启动服务）

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引