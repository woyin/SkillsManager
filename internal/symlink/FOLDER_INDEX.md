# internal/symlink 文件夹索引

## 架构说明
工具层，封装符号链接的创建、判别、校验。
无状态工具包，定义 Link Install 的底层语义；被 installer 与 cmd 共用以落地/扫描 symlink 实体。

## 文件清单

### symlink.go
- **地位**: symlink 操作的唯一实现
- **功能**: Create（建链）、IsSymlink（判别）、Verify（校验目标）、PointInside（判定目标是否落在某根内）
- **依赖**: （无内部包依赖）
- **被依赖**: internal/installer、cmd/install、cmd/update（扫已装）、cmd/rm、cmd/uninstall

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
