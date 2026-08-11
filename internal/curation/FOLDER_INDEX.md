# internal/curation 文件夹索引

## 架构说明
业务层，Curation Core：为项目生成、预览并原子应用 Curation Plan
（ADR 0020/0021/0022/0023/0027/0028）。协调 registry（原件）、profile（预设）、
project（.sm.json 与 curation 元数据）、tool（目标代理目录）与 install 落地。
是策划/对账逻辑的深模块（ADR 0018），不被塞进 cmd/status 或 cmd/install。

## 文件清单

### plan.go
- **地位**: 领域类型与共享工具
- **功能**: `Action`/`Proposal`/`Baseline` 与 `Plan`；bootstrap 判定、baseline 成员展开
- **依赖**: 无内部依赖

### planner.go
- **地位**: 计划生成
- **功能**: `Planner` 扫描各代理项目级目录，对照分层 baseline 分类每条已装实体
  （in-baseline / owned remove / unowned cleanup / add），生成 `Plan`
- **依赖**: internal/profile, internal/project, internal/registry, internal/tool

### apply.go
- **地位**: 原子应用
- **功能**: `Plan.Apply`——bootstrap 拒绝无显式目标应用；只移除 owned Link Install
  （ADR 0023）；同步 `.sm.json#curation.managed`；可挂接安装回调落地缺失成员
- **依赖**: internal/project

### plan_test.go / apply_test.go
- **覆盖**: bootstrap 无变更、baseline 成员 leave、owned 移除、未拥有/Copy 绝不移除、
  缺员 add、profile 展开、JSON 输出；apply 的原子性、bootstrap 目标约束、
  只移除 owned、managed 同步

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
