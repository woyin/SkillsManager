# Registry-first SkillsManager 实施交接

**日期**：2026-08-01  
**状态**：产品决策已确认，领域文档已更新；功能代码尚未实施  
**工作区**：`/Users/breestealth/Documents/DevelopmentRepository/SkillsManager`

## 目标

把 SkillsManager 的核心从“兼容 `npx skills` 的项目安装器”调整为“跨项目的个人 Skill Registry”：Skill 原件统一存放在 `~/.sm`，项目和 Agent 默认通过软链接使用这些原件，Profile 批量部署 Registry 内容，Registry 中所有可跟踪 Git 来源可通过一次 `sm update` 统一升级。

对外差异化必须表述为：

> 跨项目个人 Registry + Profile 批量部署 + 一次更新所有共享原件。

不要声称“canonical copy”“软链接”或“update”本身是 `npx skills` 不具备的能力。当前 `npx skills` 同样支持项目/全局 canonical copy、软链接和更新；区别是它的 canonical copy 按项目或全局 Agent scope 分布，没有 `sm` 这种用户维护的跨项目 Registry 与 Profile 生命周期。

参考：

- <https://github.com/vercel-labs/skills/blob/main/README.md>
- 本仓库旧兼容审计：[npx-skills-1.5.20.md](../audits/npx-skills-1.5.20.md)。开始实现前注意 npm 当前版本已高于该审计基线，不能把旧审计当作最新产品比较。

## 当前工作区状态

本轮只修改了领域文档和 ADR，没有修改 Go 功能代码。未提交变更包括：

- `CONTEXT.md`
- `docs/adr/0001-direct-install-primary-path.md`
- `docs/adr/0003-installed-skills-lifecycle-surface.md`
- `docs/adr/0004-reliability-p0-lifecycle.md`
- `docs/adr/0005-registry-install-profile-project-in-place.md`
- 新增 `docs/adr/0007-...` 至 `0017-...`
- 本 handoff 文件

开始工作前运行 `git status --short`，保留上述变更，不要回退用户或其他工作者的修改。

在现有代码未修改的情况下，以下测试已通过：

```bash
go test ./internal/registry ./internal/profile ./internal/installer ./cmd
```

单个本地 `SKILL.md` 的探测确认失败；当前 `fsutil.CopyDir` 只接受目录，传文件时甚至可能用文件权限创建不可遍历的中间目录，最终返回误导性的 permission denied。

## 已确认的产品契约

### 1. Registry 是核心产品模型

- 默认路径仍为：
  - `~/.sm/registry`
  - `~/.sm/data`
  - `~/.sm/data/sources`
  - `~/.sm/profiles`
- Registry 是用户拥有的跨项目原件库，不是隐藏缓存。
- Direct Install 继续存在，但只是“注册并安装”的快捷路径。
- Source cache 是远程获取缓存，不是 Registry 原件，术语不能混用。

依据：[ADR 0007](../adr/0007-registry-is-the-primary-product-model.md)。

### 2. Register / `sm add`

- `sm add <source>` 只注册，不安装。
- 未指定 category/Agent flag 时默认写入 `global` category。
- 支持：
  - GitHub/GitLab 简写、URL、SSH Git；
  - 本地单 Skill 目录；
  - 本地多 Skill 集合；
  - 单个本地 `SKILL.md` 文件；
  - 既有 Well-Known Source 能力不得回归。
- 单个文件只接受文件名为 `SKILL.md` 且 frontmatter 有效的文件；注册后物化为 `<category>/<frontmatter.name>/SKILL.md`。
- Git 与本地目录使用相同发现规则：
  - 根目录有 `SKILL.md`：单 Skill；
  - 否则发现集合；
  - TTY 交互选择；
  - 非 TTY 必须 `--skill <name>` 或 `--all`，否则零写入失败。
- 裸 `sm add` 需要新增 `--all`。
- 本地目录/文件始终复制为独立 Snapshot，不链接回原路径；重新 `sm add` 才刷新。
- 任何 Git 形态都必须保存 Origin，包括：
  - 单 Skill 仓库；
  - tree/subdirectory URL；
  - 从多 Skill 仓库选择或 `--all` 抽取的 Skill。
- 同名且同 Source：正常刷新。
- 同名但不同 Source：默认失败；`--force` 才允许替换，并提示所有 Link Install 会立即受影响。

依据：[ADR 0009](../adr/0009-all-git-registrations-preserve-origin.md)、[ADR 0011](../adr/0011-cross-source-name-replacement-requires-force.md)。

### 3. Skill 身份和质量门槛

- `SKILL.md` frontmatter `name` 是 Registry 内全局唯一身份；category 不参与身份。
- Registry 中不同 category 不得新增同名 Skill。
- 注册必须在写入前拒绝：
  - 缺少 `SKILL.md`；
  - 缺少或无效 `name`；
  - 缺少或无效 `description`。
- `name`：1–64 个小写字母、数字或单连字符；不能以连字符开头/结尾，不能含连续连字符。
- `description`：1–1024 字符。
- 不再用来源目录名补造 `name`。
- Registry 目标目录以 frontmatter `name` 规范化；来源目录名可以不同。
- 其他 lint 规则可以继续 warning，不阻断注册。
- 旧 Registry 如果已经跨 category 同名：
  - 不自动删除、改名或选择；
  - `sm doctor` 列出所有冲突路径；
  - name-based install、Profile Install、update 等依赖唯一身份的操作必须失败；
  - 用户显式清理到只剩一个原件后恢复正常。

依据：[ADR 0010](../adr/0010-skill-names-are-globally-unique.md)。

### 4. Git ref 与 provenance

- 无 ref：跟踪默认 branch。
- ref 解析为 branch：跟踪该 branch，参与更新。
- ref 解析为 tag 或 commit：pinned，不自动前进。
- branch/tag 同名时拒绝模糊名称，要求 `refs/heads/...` 或 `refs/tags/...`。
- Origin metadata 必须保存 ref kind，不能继续用“ref 是否为空”判断 pinned。
- Snapshot 必须能与 provenance 丢失的 Orphan 区分；不要再把所有无 `.git`/origin 的本地快照都报告为 orphan。

依据：[ADR 0014](../adr/0014-git-ref-kind-controls-update.md)。

### 5. Registry Install / Direct Install 分流

- `sm install foo`：从 Registry 按全局唯一名称安装；不存在则报错并提示 `sm add`，绝不隐式联网。
- `sm install owner/repo`、Git/HTTP URL、`./path`、绝对路径：Direct Install。
- 本地目录与 Skill 同名时，用户用 `./foo` 明确表达本地 Source。
- `--from-registry` 暂时保留为弃用兼容 flag。
- `--category` 不再用于解决同名身份歧义；评估是否只为旧数据诊断保留，不能绕开全局唯一不变量。

依据：[ADR 0016](../adr/0016-bare-name-selects-registry-install.md)。

### 6. Profile

- Profile 是一组已存在的 Registry Skills 和 MCP definitions。
- `profile create/update` 保存前验证所有引用存在且唯一；失败不能改写旧 Profile。
- Profile Install 在任何写操作前完整预检 Skills 与 MCP。
- 任一引用缺失/无效时：命令失败，不能创建链接、改 `.mcp.json`、改 `.sm.json` 或写安装记录。
- 写入阶段发生文件系统错误也必须回滚本次已经产生的链接、配置和 DB 状态。
- 默认仍为 Project Scope；`--global` 显式选择 Global Scope。

依据：[ADR 0012](../adr/0012-profile-install-is-atomic.md)。

### 7. List

- `sm list`：默认列 Registry inventory。
- `sm list --installed`：列 Installed Skills。
- `--installed` 可配合 `--project`、`--global`、`--agent`、`--dir`、`--json`。
- `--mcp` 仍是 Registry view。
- 旧 `--registry` 暂时保留为弃用的默认-view alias，并打印 deprecation warning。

依据：[ADR 0015](../adr/0015-bare-list-shows-the-registry.md)。

### 8. Update

命令契约：

```text
sm update                         整个 Registry
sm update foo bar                 指定 Registry Skills
sm update --project [--dir PATH]  该项目安装引用的 Registry Skills
sm update --global                全局 Agent 安装引用的 Registry Skills
sm update --project --global      两者并集
sm update --in-place              保留既有 Copy Install 就地更新语义
```

- 旧 `--registry` 暂时作为裸默认的弃用 alias。
- tracking Git：尝试更新。
- pinned tag/commit：报告 `pinned`，健康跳过。
- Snapshot：报告 `snapshot`，健康跳过。
- Orphan：报告 metadata 损坏，计为错误。
- 以 Source 为隔离/事务边界：
  - 某 Source 获取、验证或重写失败时，其全部受影响原件保持旧的有效版本；
  - 其他 Source 继续更新；
  - 任一失败使最终退出码非零；
  - 输出 itemized summary。
- 保留现有更新后 lint 与 rollback 防护，但要扩展到同一 Source 下的多个抽取 Skill，避免只回滚一半。
- Link Install 无需逐项目复制，更新 Registry 原件后立即可见。

依据：[ADR 0008](../adr/0008-bare-update-refreshes-the-registry.md)、[ADR 0013](../adr/0013-registry-update-is-source-isolated.md)。

### 9. Remove 与 Uninstall

- `sm uninstall`：只删除 Installed Skill，不删除 Registry 原件。
- `sm rm <name>`：删除 Registry 原件。
- 如果任一已知 project/global 安装仍引用该原件：默认拒绝并列出引用。
- `sm rm <name> --force`：
  - 移除所有已知 Link Installs；
  - 清理相应项目 lock entries；
  - 再删除 Registry 原件；
  - 对数据库中已知但当前不可访问的历史项目明确报告风险。
- 不能只扫描当前项目和全局目录后就断定“无引用”。

依据：[ADR 0017](../adr/0017-remove-and-uninstall-have-separate-boundaries.md)。

## 当前实现的主要差距

### `cmd/add.go` / `internal/registry`

- `AddSkillWithOptions` 要求 category 或特殊 flag；裸 `sm add` 失败。
- add 没有 `--all`、`--force`。
- Git 集合有选择逻辑，本地集合没有同等逻辑。
- `cloneAndExtract` 把抽取 Skill 拷贝入库但不写 Origin。
- 本地文件进入 `CopyDir` 后失败。
- add 先写入再 lint，Error 仍 exit 0；目标契约要求先验证再写。
- `FindSkillCategories` 和物理结构允许跨 category 同名。

重点文件：

- `cmd/add.go`
- `cmd/source_cache.go`
- `cmd/skill_origin.go`
- `internal/registry/skill.go`
- `internal/registry/discovery.go`
- `internal/registry/frontmatter.go`
- `internal/registry/lint.go`
- `internal/fsutil/fsutil.go`

### `cmd/install.go` / `internal/installer`

- 单参数默认总被当作 Source；Registry Install 必须 `--from-registry`。
- Direct Install 写 Origin，但 ref metadata 没有 ref kind。
- `ensureSkillsInRegistry` 对跨来源同名直接覆盖并 warning。
- Profile installer 对缺失 Skill/MCP 只 warning + continue。
- Profile 安装会边解析边写，不是原子事务。
- Registry Install 仍支持 category 消歧，与全局唯一身份冲突。

重点文件：

- `cmd/install.go`
- `internal/installer/installer.go`
- `internal/profile/profile.go`
- `cmd/profile.go`

### `cmd/update.go`

- 裸 `sm update` 当前只收集当前项目 + global 已安装来源。
- `--registry` 才更新整个 Registry。
- `--global`/`--project` 的意义与新契约不一致，部分路径甚至没有实际过滤效果。
- 非空 ref 一律按 pinned 处理，因此 `--ref main` 不前进。
- 无 `.git`/Origin 一律归为 orphan，无法区分 Snapshot。
- summary 有 Errors 也常返回 nil，自动化无法通过退出码发现部分失败。
- 完整 Git repo、origin-backed extracted Skill、Well-Known 更新路径分散，需要统一状态和错误模型。

### `cmd/list.go`

- 裸命令当前列 Installed Skills。
- 需要新增 `--installed`，翻转默认并维护 text/JSON 两种输出。
- `--registry` 改为兼容 alias。

### `cmd/rm.go` / `cmd/doctor.go` / DB

- `rm` 混合 uninstall 与 Registry delete。
- 引用扫描只有当前项目 + global，无法保护其他项目。
- DB 已有 projects/installations，可用于枚举已知项目，但需要补充可靠的引用检查、不可访问项目报告和强制清理结果。
- doctor 尚不检查全局重复名称、非法 metadata、Orphan/Snapshot 分类。

### 文档

- `README.md`、`README.zh-CN.md`、help text、`PROJECT_INDEX.md` 仍混有 Direct-Install-first / Installed-list-first 叙述。
- `docs/audits/npx-skills-1.5.20.md` 把“宽松 frontmatter”列为既有 intentional divergence；新决策已明确反转，需要更新审计记录，不能静默留下矛盾。
- 所有被修改 Go 文件需同步文件头 Input/Output/Pos 注释；架构或目录职责变化时同步 `FOLDER_INDEX.md` 与 `PROJECT_INDEX.md`。

## 推荐的实现边界

不要继续让“目录是否含 `.git`”承担 provenance 类型判断。建议给每个 Registry Skill 一个版本化 metadata 记录，至少包含：

```text
schema version
skill name
category / agent target
source kind: git | local-snapshot | well-known
source
requested ref
resolved ref kind: default-branch | branch | tag | commit
resolved commit
relative skill path
```

可以演进现有 `.sm-origin.json`，也可以引入更通用的 per-Skill manifest；无论采用哪一种，都必须：

- 向后兼容读取现有 `.sm-origin.json`；
- 明确区分 Snapshot 与 metadata 丢失；
- 写入采用临时文件 + rename；
- 不把 metadata 文件泄漏给 Agent（当前 Link Install 指向整个 Registry 目录，若 Agent 会读取隐藏文件，需要在兼容性测试中验证）；
- backup/export/import 覆盖新 metadata；
- 不再让完整单 Skill Git repo 与抽取 Skill 拥有两套不同生命周期。

建议集中提供 Registry 层原语，避免 cmd 各自重新扫描：

```text
ValidateCandidate
ResolveUniqueSkill
Register / Refresh / ForceReplace
ListConflicts
ListUpdateTargets
ListReferences
```

具体命名可调整，但身份、provenance、冲突和引用判断应由 Registry/业务层统一负责，不能继续散落在 add/install/update/rm。

## 建议实施顺序

1. **先写失败测试**：覆盖全局唯一、严格 frontmatter、本地单文件、本地集合、Git extracted Origin、ref kind。
2. **统一 Registry metadata 与解析原语**：完成旧 metadata 兼容读取和 Snapshot 标记。
3. **改 Register**：默认 global、`--all`、`--force`、Git/本地一致发现、写前验证。
4. **改 Registry Install 分流**：裸名称本地安装，明确 Source 语法 Direct Install，弃用旧 flag。
5. **翻转 List 默认**：新增 `--installed`，保留 text/JSON 和旧 flag 兼容。
6. **重构 Update**：Registry 默认、ref kind、Source transaction、状态分类、非零错误。
7. **Profile 原子化**：先 validate create/update，再实现 install preflight + rollback。
8. **Remove/Uninstall 分离**：DB 枚举所有已知项目、引用保护、`--force` 清理。
9. **Doctor 和迁移诊断**：重复名称、Orphan、坏 metadata、不可访问项目。
10. **文档与索引**：中英文 README、help、PROJECT/FOLDER_INDEX、兼容审计、升级说明。

每一步保持测试可运行。不要把所有行为翻转塞进一个无法审查的大改动。

## 必须覆盖的验收场景

### Register

1. `sm add ./one-skill` 无 category 成功进入 `registry/skills/global/<name>`。
2. `sm add ./SKILL.md` 成功物化标准目录；任意其它文件名失败且 Registry 无变化。
3. 本地集合在非 TTY 无 selector 失败；`--skill a` 和 `--all` 成功。
4. Git 单仓、tree URL、多 Skill 选择都产生等价 Origin。
5. 同 Source 重注册刷新；不同 Source 同名失败；`--force` 替换。
6. 缺 name/description、非法 name、超长 description 都在写入前失败。
7. 跨 category 同名注册失败。

### Install / Profile

1. `sm install known-name` 不联网并创建指向 Registry 的项目链接。
2. `sm install unknown-name` 不联网，返回明确 Register 提示。
3. `sm install owner/repo` 仍走 Direct Install。
4. Profile create/update 遇到未知成员不改旧文件。
5. Profile Install 任一成员缺失时零副作用。
6. 模拟第 N 个链接或 MCP 写入失败，前 N-1 个改动全部回滚。

### List / Update

1. 裸 list 只显示 Registry；`--installed` 按 scope/agent 正确显示。
2. 裸 update 包含未安装但已注册的 tracking Git Skill。
3. named/project/global 范围与并集准确。
4. branch 前进；tag/commit 不前进；同名 branch/tag 要求 qualified ref。
5. Snapshot 正常跳过；Orphan 非零失败。
6. 一个 Source 失败不阻止另一个更新，但最终退出码非零。
7. 同一 Source 下多个 Skill 更新验证失败时全部保持旧版本。

### Remove / migration

1. 另一个已知项目仍链接时 `sm rm` 拒绝并列出路径。
2. `sm uninstall` 不删除 Registry。
3. `sm rm --force` 清理所有可访问链接与 lock entries 后删除原件。
4. 不可访问历史项目被明确报告。
5. doctor 报告旧 Registry 重复名称；不会自动删除任一副本。

## 回归保护

以下既有能力不在本次删除范围：

- Well-Known Skills v1/v2 与 digest 验证；
- `skills-lock.json` restore；
- `--offline` 与 source cache；
- `--copy` 与 `sm update --in-place`；
- Eve subagents；
- MCP transport/import/export/backup；
- 全部 Agent catalog 与特殊 category targeting；
- 更新后 lint/score/rollback；
- Web dashboard（若 Registry 输出模型变化，需要同步 API 测试）。

## 最终验证

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go build ./...
git diff --check
```

再使用临时 HOME/Registry 做 CLI 手测，不能污染真实 `~/.sm`：

```bash
sm --registry <tmp>/registry --data <tmp>/data --profiles <tmp>/profiles ...
```

手测至少覆盖 Register、Registry Install、Profile Install、Registry Update、List、Uninstall、Remove 和重复名称迁移诊断。

## 完成定义

只有同时满足以下条件才能宣称完成：

- 上述命令契约已经由代码和回归测试证明；
- 旧 Registry 数据不会被静默删除或错误选择；
- 任一共享原件的高影响覆盖/删除都需要显式 force；
- Profile 安装和 Source 更新不会留下部分状态；
- 中英文文档、help、领域术语、ADR 与实际行为一致；
- 对 `npx skills` 的差异化描述准确，不把双方共有能力包装成独有能力。
