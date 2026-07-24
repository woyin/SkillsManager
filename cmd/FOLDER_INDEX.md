# cmd 文件夹索引

## 架构说明
控制层（cobra 命令实现），sm CLI 的全部子命令入口与编排逻辑。
采用“命令文件 + 共享辅助”模式：每个 `*.go` 对应一个命令或一组共享工具，root.go 聚合并定义全局目录（RegistryDir/DataDir/ProfilesDir）。
向下调用 internal 各包完成业务，向用户提供 CLI 界面；是 internal 层与用户的唯一边界。

## 文件清单（按职责分组）

### 入口与全局
- **root.go** — 根命令、全局持久化标志（--registry/--data/--profiles）、Execute；导出 RegistryDir/DataDir/ProfilesDir/Version
- **dbutil.go** — 数据库连接工具（openDB、dbPath）
- **resolve.go** — 特殊目录标志（--codex/--claude 等）的 cobra 绑定与解析

### 技能安装/更新/卸载生命周期
- **install.go** — install 命令：Direct Install / Registry Install（--from-registry）/ Profile Install；scope+copy 分支
- **update.go** — update 命令：并发 git pull、origin-backed 刷新、`--in-place` 就地刷 copy 实体、orphan 提示
- **uninstall.go** — uninstall 命令：移除 agent 目录里的已装 symlink
- **check.go** — check 命令：安装完整性检查与自动修复
- **source_cache.go** — 远程 git 源缓存（DataDir/sources）的读写与命中
- **skill_origin.go** — `.sm-origin.json` provenance 元数据的读写与原子覆盖（replaceSkillDir）

### 注册表内容管理
- **add.go** — add 命令：拉取/注册技能到本地 registry（不安装）
- **rm.go** — rm 命令：从 registry 移除技能/MCP，清理 agent 目录引用
- **list.go** — list 命令：列出已装/registry 技能与 MCP
- **lint.go** — lint 命令：校验技能结构与质量
- **cache.go** — cache 命令：查看/清理远程源缓存

### 预设与提示词
- **profile.go** — profile 子命令：list/show/create/update/delete（CRUD 完整）
- **prompt.go** — prompt 子命令：list/show/apply/create/delete

### 发现与浏览
- **find.go** — find 命令：关键词搜索并交互选择已装技能
- **browse.go** / **browse_display.go** / **browse_fetch.go** — browse 命令三件套：入口路由 / 展示层（选择器+表格）/ 数据层（skills.sh API+HTML 抓取+缓存）

### 项目健康与状态
- **status.go** — status 命令：项目技能健康视图、aivo 状态、孤儿技能检测
- **doctor.go** — doctor 命令：CLI/目录/数据库/环境变量健康检查

### 配置迁移与备份
- **init.go** — init 命令：初始化项目配置/生成技能模板
- **export.go** / **import.go** — 配置导出/导入 JSON
- **backup.go** / **restore.go** — 创建/恢复 tar.gz 备份

### 启动与 UI
- **use.go** — use 命令：启动指定 agent 并加载技能
- **web.go** — web 命令：启动内嵌 Web 仪表盘
- **completion.go** — 生成 shell 自动补全脚本

### 共享工具
- **install.go 内的 installJob/installSkillsConcurrently/copySkillDir 等** — 安装并发执行与拷贝
- **profile.go 内的 formatList/splitAndTrim** — 字符串格式化

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引