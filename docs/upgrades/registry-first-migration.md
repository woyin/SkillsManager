# Registry-first 迁移与升级说明

**日期**：2026-08-01
**适用版本**：自 Registry-first 架构（ADR 0007–0017）起，`sm` 的核心模型从
“按项目/全局 Agent 的 canonical copy + 软链接安装器”调整为“跨项目个人 Skill
Registry”：原件统一存放在 `~/.sm/registry`，项目和 Agent 默认通过软链接复用，
Profile 批量部署，一次 `sm update` 刷新所有可跟踪原件。

本文档只说明**数据兼容**与**命令契约变化**；用户无需重建 Registry。旧数据
不会被静默删除、改名或自动选择。

## 1. 旧 `.sm-origin.json` 向后兼容

旧格式（`source` / `ref` / `rel_path` / `commit`，无 `source_kind` /
`ref_kind`）仍可读取，读取时按以下规则推断（`internal/registry/origin.go`）：

- `source_kind` 缺失且 `source` 非空 → 推断为 `git`；
- `ref_kind` 缺失且 `ref` 为空 → 推断为 `default-branch`（参与更新）；
- `ref_kind` 缺失且 `ref` 非空 → 保守视为 pinned（可能是 tag/commit，
  更新路径不前进，保留旧行为）；
- `rel_path` → 映射为 `sub_path`；
- 既无 `source` 又无 `source_kind` → 视为损坏 metadata（`orphan`）。

新写入统一使用 schema v1（含 `source_kind` 与 `ref_kind`）。写入采用
临时文件 + rename，不会留下半截文件。

## 2. Ref kind 推断

请求 ref 在注册/安装时解析并记录 `ref_kind`（ADR 0014）：

| 请求 ref | 解析结果 | update 行为 |
| --- | --- | --- |
| （无） | `default-branch` | tracking，参与更新 |
| branch | `branch` | tracking，参与更新 |
| tag | `tag` | pinned，健康跳过 |
| commit | `commit` | pinned，健康跳过 |
| branch 与 tag 同名 | 拒绝，要求 `refs/heads/...` 或 `refs/tags/...` | — |

旧数据中非空 `ref` 一律视为 pinned；如需让旧条目参与更新，重新
`sm add <source> --ref <branch>`（同源刷新）即可记录明确的 ref kind。

## 3. 跨 category 同名：必须手工清理

Registry 身份是 frontmatter `name` 且全局唯一（ADR 0010）。旧 Registry 如果
已存在跨 category 同名：

- `sm doctor` 会列出所有冲突路径（`Registry conflicts` = fail）；
- 依赖唯一身份的操作（`sm install <name>`、Profile Install、`sm update`、
  `sm rm <name>`）会失败，直到只剩一个原件；
- `sm` **不会**自动删除、改名或选择任一副本——请用户自行决定保留哪个
  category 的原件并删除其余，然后这些操作自动恢复正常。

## 4. 命令契约变化

### `sm install <name>`：裸名称 = Registry Install（ADR 0016）

- `sm install foo` 现在从本地 Registry 按全局唯一名称安装；不存在则报错并
  提示 `sm add`，**绝不隐式联网**。
- 本地目录恰好与技能同名时，显式写来源：`sm install ./foo`。
- `--from-registry` 保留为弃用兼容 flag（会打印 deprecation warning）。
- `--category` 不再用于消解身份歧义（弃用 warning）；同名多副本必须先按
  第 3 节手工清理。

### `sm list`：默认显示 Registry（ADR 0015）

- 裸 `sm list` 现在显示 Registry 清单；`sm list --installed` 才列出
  Installed Skills（可配合 `--project` / `--global` / `--agent` / `--dir` /
  `--json`）。
- `--registry` 保留为弃用别名（打印 deprecation warning）。
- `--mcp` 仍为 Registry 视图。

### `sm update`：裸命令刷新整个 Registry（ADR 0008）

- `sm update` 现在刷新**整个 Registry** 中所有可更新原件；`--registry` 是
  弃用别名。
- `sm update foo bar` 更新指定 Registry Skills。
- `sm update --project [--dir PATH]` / `--global`：更新安装引用的 Registry
  Skills（可并集）。
- `sm update --in-place`：保留 Copy Install 就地刷新语义。
- tracking 尝试更新；pinned / snapshot 健康跳过；orphan 计为错误；任一
  Source 失败不影响其它 Source，但最终退出码非零。

### `sm rm <name>`：删除 Registry 原件（ADR 0017）

- `sm rm <name>` 现在删除 Registry 原件，任何已知项目/全局安装仍引用时
  默认拒绝并列出引用；`--force` 先清理所有已知 Link Installs 与 lock
  entries 再删除原件。
- 只删 Installed Skill（保留原件）用 `sm uninstall`。
- 旧脚本若依赖 `sm rm` 的“卸装”语义，请改用 `sm uninstall`。

### `sm add`：默认 global、先验证再写（ADR 0007/0010）

- 未指定 category 时默认写入 `global`（不再要求显式 category/特殊 flag）。
- 注册前校验 frontmatter `name`（1–64 小写/数字/单连字符）与 `description`
  （1–1024）；失败不写入，也不再从来源路径补造 name。
- 本地目录/文件作为 Snapshot 复制入库（不链接回原路径）；重新 `sm add`
  才刷新。
- 同名同源 = 刷新；同名不同源 = 失败，需 `--force`（会影响所有 Link
  Install）。

### Profile：保存前校验、安装原子化（ADR 0012）

- `sm profile create/update` 保存前校验所有引用存在且唯一，失败不改写旧
  Profile。
- `sm install --profile` 在任何写入前完整预检 Skills 与 MCP；写入阶段失败
  会回滚本次已产生的链接、配置与 DB 状态。

## 5. 迁移检查清单

1. 运行 `sm doctor`，确认 `Registry conflicts` 为 pass、无 orphan 报错。
2. 若有跨 category 同名：手工保留一个原件，删除其余。
3. 验证 `sm list`（Registry）与 `sm list --installed` 输出符合预期。
4. 验证 `sm update` 对 tracking 条目前进、pinned/snapshot 健康跳过。
5. 更新脚本/文档中已弃用的调用：
   - `sm install --from-registry <name>` → `sm install <name>`
   - `sm list --registry` → `sm list`
   - `sm update --registry` → `sm update`
   - `sm rm`（卸装语义）→ `sm uninstall`
