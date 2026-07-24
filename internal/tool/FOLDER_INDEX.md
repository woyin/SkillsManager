# internal/tool 文件夹索引

## 架构说明
工具层，描述 sm 支持的 AI 编程助手（agent）目录配置。
单一来源契约：data.go 的 catalog 是唯一真相，tool.go 由其派生导出工具变量与查找函数，避免目录与别名漂移。

## 文件清单

### data.go
- **地位**: agent 目录的单一来源（catalog）
- **功能**: 全部支持工具的目录配置（name/agentName/skillDir/projectSkillDir/configFile/binary/specialDir）
- **依赖**: （无导出，包级 catalog）
- **被依赖**: tool.go（派生 allTools 与导出别名）

### tool.go
- **地位**: 工具查找与检测的对外入口
- **功能**: Tool 类型、AllTools/DefaultTools（{Claude,Codex,Pi} 回退集）、DetectInstalled/IsInstalled、HasSkillDir/GetSkillDir/GetProjectSkillDir/GetConfigPath、ToolByName/ToolByAgentName/ToolsByNames、NameForSpecialDir/SpecialFlagSpecs
- **依赖**: os, os/exec, path/filepath, internal/home
- **被依赖**: installer、cmd/（install/update/list/rm/uninstall/status 等广泛使用）

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引