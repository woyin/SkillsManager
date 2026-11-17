# internal/health 文件夹索引

## 架构说明
业务层，安装完整性的窄只读扫描。把“哪些符号链接失效/不受管、哪些已记录项目目录
消失”的判定收敛为一份实现，供 CLI（`sm check` 与 `--fix`）与 Web（`/api/check`）
共用。扫描不做任何写入：修复与呈现由调用方负责；IO 错误与完整性问题的返回分开，
调用方不得把 IO 错误当成健康状态。

## 文件清单

### health.go
- **地位**: 完整性扫描的唯一实现
- **功能**: `Scan(dirs, registryDir, knownProjects)` 返回 `Report{Issues}`；问题分类为 broken_symlink（仅目标真实缺失）、orphaned_symlink（目标不在 Registry 根内，lexical PointInside）、missing_project（数据库记录目录不在磁盘）；目录不存在跳过；Lstat/Readlink/Stat 的 EACCES、ELOOP 等其他错误返回 error，不折叠成 broken
- **依赖**: internal/symlink
- **被依赖**: cmd/check、web/handler

### health_test.go
- **功能**: 共享目录 fixture（broken/outside/inside/普通目录/共享目录去重）、非目录 IO 失败、缺失项目分类、项目 Stat 权限错误返回 error

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
