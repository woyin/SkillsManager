# internal/fsutil 文件夹索引

## 架构说明
工具层，提供文件系统底层操作（拷贝、遍历等）。
是最底层的无状态工具包，被 registry 与 installer 复用以避免重复实现拷贝逻辑。

## 文件清单

### fsutil.go
- **地位**: 文件系统工具的唯一实现
- **功能**: CopyDir（递归拷贝目录；创建或合并至 dest，跳过版本控制与构建产物）等基础文件操作
- **依赖**: （无内部包依赖）
- **被依赖**: internal/registry（copyDir）、internal/installer（copy 实体落地）、cmd/install（copySkillDir）

### fsutil_test.go
- **功能**: 覆盖递归拷贝、权限保持、符号链接、跳过目录和错误路径；包含 `BenchmarkCopyDirRecursive` 性能回归基准

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
