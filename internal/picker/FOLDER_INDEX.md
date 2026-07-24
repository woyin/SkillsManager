# internal/picker 文件夹索引

## 架构说明
UI 层，提供 TTY 下的交互式选择器。
独立模块，仅被 cmd 层在需要用户从多个候选中选择时调用（如多技能发现、find 命令）。

## 文件清单

### picker.go
- **地位**: 交互选择器的唯一实现
- **功能**: 从候选列表中渲染并让用户交互选择（非 TTY 时回退）
- **依赖**: （无内部包依赖）
- **被依赖**: cmd/find、cmd/install（selectSkillsForInstall）、cmd/browse_display

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
