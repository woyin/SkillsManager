# web 文件夹索引

## 架构说明
API 层，sm 内嵌的只读 Web 仪表盘后端：静态前端（`//go:embed`）+ JSON REST API。
独立于 CLI 命令层，只读消费 `internal/registry`、`internal/profile`、`internal/curation`、`internal/health`、`internal/db`、`internal/tool`、`internal/aivo`；由 `cmd/web` 绑定 `127.0.0.1` 启动 HTTP 服务。

## 文件清单

### handler.go
- **地位**: 仪表盘核心处理器与路由注册
- **功能**: `NewHandler` / `WithProfilesDir` / `WithCurrentProject`、`RegisterRoutes`、GET-only 门禁，以及 `/api/registry`（含每技能评分/Lint/Origin 摘要）、`/api/projects`、`/api/history`、`/api/check`（复用 `internal/health`）、`/api/tools`、`/api/aivo`
- **依赖**: `net/http`, `embed`, `internal/registry`, `internal/health`, `internal/db`, `internal/tool`, `internal/aivo`

### readonly_views.go
- **地位**: 只读扩展视图接口
- **功能**: `/api/profiles`（列举并校验 Profile 成员）、`/api/skill`（按已注册列表定位返回评分明细、Lint 发现、Origin 元数据与有界 `SKILL.md` 文本）、`/api/plan`（限定已登记项目或启动工作区的只读 Curation Plan 预览）
- **依赖**: `internal/profile`, `internal/curation`, `internal/registry`, `internal/tool`

### static/
- **地位**: 内嵌静态前端（无构建链）
- **功能**: `index.html` / `app.js` / `style.css`，提供 Overview、Registry（含检索与技能详情面板）、Profiles、Projects（含只读 Curation Plan 预览）与 History 视图

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
