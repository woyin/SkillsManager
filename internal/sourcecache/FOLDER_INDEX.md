# internal/sourcecache 文件夹索引

## 架构说明

数据层深模块，集中管理持久化 Git source cache。调用方只需提供 data
目录、source、ref 与 offline 选择；模块负责稳定 key、缓存路径、tree URL
和 ref 克隆策略、离线命中以及 metadata 的原子读写。

## 文件清单

### sourcecache.go

- **地位**: 持久 source cache 的唯一实现与接口
- **功能**: `Store`/`New`、`Acquire`、key/path 派生、metadata 读写、HEAD 提取与 git ref checkout
- **依赖**: 标准库、`internal/registry`
- **被依赖**: `cmd/source_cache.go`、`cmd/add.go`，后续 install/use/update 迁移

### sourcecache_test.go

- **地位**: 模块接口测试
- **功能**: key、cache hit、offline exact key、metadata roundtrip

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
