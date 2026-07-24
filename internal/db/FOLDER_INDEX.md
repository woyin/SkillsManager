# internal/db 文件夹索引

## 架构说明
数据层，基于 SQLite（modernc.org/sqlite，纯 Go 无 CGO）持久化安装历史与项目记录。
采用 repository 风格，DB 结构体封装连接并提供 Upsert/查询方法；不依赖其他内部包。

## 文件清单

### db.go
- **地位**: SQLite 状态库的唯一访问入口
- **功能**: installations 表（安装历史追加）、projects 表（项目记录 upsert）、查询接口
- **依赖**: （无内部包依赖）
- **被依赖**: cmd/install（RecordInstallation/UpsertProject）、cmd/dbutil（连接）、cmd/status（查询历史）

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
