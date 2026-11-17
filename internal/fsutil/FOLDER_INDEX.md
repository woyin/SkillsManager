# internal/fsutil 文件夹索引

## 架构说明
工具层，提供文件系统底层操作（拷贝、遍历等）。
是最底层的无状态工具包，被 registry 与 installer 复用以避免重复实现拷贝逻辑。

## 文件清单

### fsutil.go
- **地位**: 文件系统工具的唯一实现
- **功能**: CopyDir（递归拷贝目录；创建或合并至 dest，跳过版本控制与构建产物）、ResolveWriteTarget（跟随文件 symlink 链到最终 referent，允许悬空；目录/环/超跳拒绝）、WriteFileAtomic（同目录临时文件→保留权限→rename；写 referent 保留链接、悬空链接创建 referent、只读目标包裹 os.ErrPermission、新建文件由内核应用 umask）
- **依赖**: （无内部包依赖）
- **被依赖**: internal/registry（copyDir）、internal/installer（copy 实体落地、项目文件原子写）、internal/project（.sm.json）、internal/lockfile（skills-lock.json）、cmd/install（copySkillDir）

### fsutil_test.go / fsutil_atomic_test.go
- **功能**: fsutil_test.go 覆盖递归拷贝、权限保持、符号链接、跳过目录和错误路径；fsutil_atomic_test.go 覆盖原子写的权限保留、临时文件清理、只读目标拒绝与 symlink 目标写入。包含 `BenchmarkCopyDirRecursive` 性能回归基准

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
