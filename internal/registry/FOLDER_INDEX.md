# internal/registry 文件夹索引

## 架构说明
数据层，sm 的磁盘技能与 MCP 注册表（`~/.sm/registry/`）的核心实现。
分层约束：底层包，不导入 internal/tool；特殊目录字面量与 tool catalog 保持一致。
提供技能增删改查、git 克隆、技能发现、frontmatter 解析、lint、MCP 定义与来源解析。

## 文件清单

### types.go
- **地位**: 注册表类型基础
- **功能**: Registry 结构、ItemDetail/DiscoveredSkill/SkillMatch 类型、特殊目录常量与集合、New/skillsDir/mcpDir/IsSpecialDir/Dir
- **依赖**: path/filepath
- **被依赖**: 本包其余文件、installer、cmd/（广泛使用）

### skill.go
- **地位**: 技能增删改查主实现
- **功能**: AddSkill/AddSkillWithOptions（git 克隆或本地拷贝入库）、RemoveSkill、FindSkillDir/FindSkillWithCategory/FindSkillCategories（按名检索+歧义检测）、ListSkills/ListSkillDetails、GetSkillPath
- **依赖**: bytes, fmt, os, path/filepath, sort, strings
- **被依赖**: installer、cmd/（add/install/update/rm/list 等）

### source.go
- **地位**: 来源解析
- **功能**: 解析 source 字符串为标准 URL/路径、SkillNameFromPath
- **依赖**: （标准库）
- **被依赖**: skill.go、cmd/install

### git.go
- **地位**: git 操作封装
- **功能**: IsGitURL、ParseTreeURL、NormalizeGitURL、CloneRepoWithBranch、CloneToTemp/RemoveCloneTemp
- **依赖**: os, os/exec, path/filepath
- **被依赖**: skill.go（cloneAndAdd/cloneAndExtract）、cmd/source_cache

### clone.go
- **地位**: 仓库克隆辅助
- **功能**: 克隆到临时目录与后续清理的共享逻辑
- **依赖**: git.go
- **被依赖**: skill.go、cmd/source_cache

### discovery.go
- **地位**: 技能发现
- **功能**: DiscoverSkills（扫描目录/仓库下含 SKILL.md 的技能，过滤内部技能）
- **依赖**: os, path/filepath
- **被依赖**: skill.go、cmd/install

### frontmatter.go
- **地位**: frontmatter 解析
- **功能**: 解析 SKILL.md 的 YAML frontmatter（name/description）
- **被依赖**: skill.go（命名）、cmd/install

### lint.go
- **地位**: 技能 lint
- **功能**: LintSkill（校验技能目录结构/frontmatter）、返回错误与告警
- **依赖**: （标准库）
- **被依赖**: skill.go、cmd/update（刷新后校验）、cmd/lint

### score.go
- **地位**: 技能评分
- **功能**: SkillScore（按质量/完整度评分）
- **被依赖**: cmd/lint、web/handler

### mcp.go
- **地位**: MCP 定义管理
- **功能**: MCP 定义增删查、GetMCPPath
- **被依赖**: installer（installMCP）、cmd/（add/list）

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引