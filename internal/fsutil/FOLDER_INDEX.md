# internal/fsutil 文件夹索引

## 架构说明
工具层，提供文件系统底层操作（拷贝、遍历等）。
是最底层的无状态工具包，被 registry 与 installer 复用以避免重复实现拷贝逻辑。

## 文件清单

### fsutil.go
- **地位**: 文件系统工具的唯一实现
- **功能**: CopyDir（递归拷贝目录，要求 dest 不存在）等基础文件操作
- **依赖**: （无内部包依赖）
- **被依赖**: internal/registry（copyDir）、internal/installer（copy 实体落地）、cmd/install（copySkillDir）

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
