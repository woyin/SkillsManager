# 测试可靠性第二轮验收

状态：**T1–T7已实现并通过本机固定快照门禁及远端 GitHub Actions 三平台验证；本轮完成。** 独立只读review给出 DEFECT_FOUND（1行为+2文档），三项已修复并复验；随后按用户授权推送临时分支 `ci/test-reliability`，在远端捕获并修复 `macos-14` 下线与 Windows `internal/backup` 反斜杠 tar 条目缺陷，最终 Linux / macOS / Windows 三个远端作业全绿通过。

run：`reliability-MMhxiR`。权威设计：[test-reliability-plan.md](test-reliability-plan.md)。本报告验证的是包含用户未提交改动的**工作区快照**，不是HEAD或已提交快照。HEAD保持 `f15816a050f31f7be9c15b3eae00eb44549dcc0a`；index hash与初始值相同；未commit/push、安装依赖、stash/reset。

## 最终结果（固定快照，2026-11-18）

| 门禁 | 退出码 | 证据 |
| --- | ---: | --- |
| `go test ./... -count=1` | 0（23包） | `final-gates/go-test-all.log` |
| `go vet ./...` | 0 | `final-gates/go-vet-all.log` |
| 8包 `-race` | 0 | `final-gates/race-8.log` |
| `go test ./cmd -shuffle=20261117 -count=5` | 0 | `final-gates/cmd-shuffle5.log` |
| Windows 全测试编译 | 0（仅编译） | `final-gates/windows-compile.log` |
| 源码指纹前后一致 | 0 | `final-gates/before.sha256` / `after.sha256` |

T4：POSIX-only 断言（`syscall.Umask`、权限位）移入 `internal/fsutil/fsutil_atomic_posix_test.go`（`!windows`），通用文件保留跨平台断言。CI 拆为 linux（full+vet+8包race）、macos（8包）、windows（vet + 逐包 `go test -c` 到 `RUNNER_TEMP` + 固定通用子集）三个 job；release 工作流未改。Windows job 使用真实编译，不把 `-exec` 当运行证据。

T5：`collectInstalledUpdateTargets` 路径提取到 `internal/updater/discovery.go`，接口 `DiscoverInstalledTargets(DiscoverOptions) (DiscoveredTargets, []Diagnostic)`；`PathInside`/`NearestGitRepo` 单源，cmd 保留兼容委托；legacy `.sm-origin.json` 与 registry schema 未合并。cmd/update.go 只留显式输入转换与输出。

T6：`knownProjectPaths()` 在 `removeRegistryOriginal` 生命周期只读一次并显式下传；历史项目报告只 Stat，不再 openDB；无原件分支仍单次读取；删除边界未放宽。

T7：curation 测试按行为重组为 transaction/result/ownership/bootstrap 四文件加 `txfake_test.go`，22 个测试全部保留、改名 8、删除 0。两处外部副本单点变异均被语义断言杀死：去掉新增回滚补偿 → `TestBootstrapOwnershipFailureRollsBack`；忽略 Link Install 所有权校验 → `TestApplyDoesNotOwnExistingLink`（日志 `mutation-t7/`）。

## 独立review与修复

reviewer（只读，未跑测试）结论 DEFECT_FOUND：

1. **行为缺陷（已修）**：`cmd/doctor_test.go` 的 `TestRunDoctor` 未隔离 PATH，`runDoctor→checkAivo→aivo.Detect/ListKeys` 会真实执行 `aivo keys --ping --json`（宿主已安装 aivo）。已加 `isolatePath(t)`（只保留 git），定向 `go test ./cmd -run TestRunDoctor` exit 0，并重跑全部门禁。
2. **审计文档测试名（已修）**：补齐 `TestApply` 前缀并更新 T7 改名后的引用。
3. **验收状态陈旧（已修）**：本节即更新后的状态。

非阻塞项处理：`internal/updater/FOLDER_INDEX.md` 依赖描述改为实际；`cmd/FOLDER_INDEX.md` 补登记 `agent_detect.go`；`internal/curation/FOLDER_INDEX.md` 补 `apply_tx_contract_test.go`。未处理（记录在案）：`cmd/rm.go` 的 `projectIndexReadHook` 为测试钩子全局；`updater` 的 `Diagnostic`/`DiscoveredTargets.Empty` 暂无生产消费者；registry/sourcecache 仍有 shell fake；`rm_test.go`/`update_test.go` 部分裸 `t.Setenv("HOME")` 未刷缓存（不泄漏）。以上均不影响门禁结论。

## 第一批结果

| 验证 | 当前退出码 | 证据（外部目录内） |
| --- | ---: | --- |
| `go test ./internal/aivo -count=1 -v` | 0 | `batch1-aivo-fixed.log` |
| `go test ./internal/curation ./internal/installer -run 'Contract\|Result' -count=1 -v` | 0 | `batch1-contract.log` |
| `go test ./internal/curation ./internal/installer -count=1` | 0 | `batch1-transaction-packages.log` |
| cmd真实adapter及隔离helper定向合同（含原red两项） | 0 | `batch1-helper-contract-green.log` |
| `go test ./cmd -count=1`（最后源码快照） | 0 | `batch1-cmd-home-fixed.log` |
| `go test ./cmd -shuffle=20261117 -count=5`（同一快照） | 0 | `batch1-cmd-shuffle-fixed.log` |
| `go vet ./internal/aivo ./internal/curation ./internal/installer ./cmd` | 0 | `batch1-vet.log`；随后cmd恢复helper修改又以 `batch1-cmd-vet-fixed.log` 验证0，其他三包输入未变 |

最后一轮前后源码指纹相同：`batch1-final-before.sha256` 与 `batch1-final-after.sha256`。aivo与事务包的先前通过证据按未变文件继承；没有把整个仓库标为已通过。

T1：默认测试不调用真实aivo。固定payload覆盖解析、精确参数、非零退出、非法JSON、超时与nil兼容。真实集成 `TestIntegrationRealAivo` 默认skip，仅 `SM_AIVO_INTEGRATION=1` 才运行；本次未启用。`TestHelperAivoProcess` 正常枚举时skip，但timeout测试以专用env启动它。公开函数签名、5秒生产超时、nil返回语义保持。

T2：6个cmd检测调用通过单一私有detectAgents seam，产品默认和原fallback不变。测试显式指定Agent；Go测试二进制helper替代cmd shell fake，记录argv与真实进程退出；status隔离宿主aivo且保留git。cleanup恢复Cobra flags（含切片/Changed）、root及子命令Out/Err/In/Args、包globals和HOME缓存。

T3：`internal/txcontract` 仅依赖stdlib/project，fake、installer、cmd真实planTx运行同一7场景suite：重复Commit、已提交Rollback、重复Rollback、回滚后Commit、config失败补偿、cleanup已提交、既有实体不owned。三实现全部通过；Result用slices.Clone返回独立Skills/MCP/Failures切片，保留nil性与尝试结果语义。生产事务错误类别/接口和CLI schema未变。

## 真实失败、修复及限制

### 原始冻结基线

外部目录：`/tmp/skillsmanager-reliability.MMhxiR/`。

- `go test ./...` exit 1：cmd 7项失败 + aivo `TestGetStats`失败，见 `baseline-test.log`。
- `go vet ./...` exit 0，见 `baseline-vet.log`。
- 八包 `go test -race ... -count=1` exit 0，见 `baseline-race.log`。**这是修改前基线，不是当前代码race验收。**
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -exec=/usr/bin/true ./...` exit 1：`internal/fsutil/fsutil_atomic_test.go:202:10: undefined: syscall.Umask`。这是编译失败；没有运行Windows二进制。

cmd基线失败：`TestInstallBareNameFromRegistry`、`TestPlanCommandApplyRemovesOwnedLinkInstall`、`TestPlanCommandApplyBootstrapWithSkillTarget`、`TestPlanCommandApplyBootstrapWithProfileTarget`、`TestPlanCommandCheckPassesWhenBaselineSatisfied`、`TestWriteProjectStatusShowsProjectInstallAndOrphan`、`TestWriteProjectStatusReportsBrokenSymlink`。最后cmd正序与shuffle5全包通过，这些原失败当前均未出现。

### 本轮新增失败（均已修复）

1. `batch1-aivo.log`：裸 `"aivo"` 版本用例want为空，与保留的旧实现不符。只把测试want改为 `"aivo"`；未改生产语义。`batch1-aivo-fixed.log`通过。
2. `batch1-cmd.log`：benchmark fake helper对已声明返回参数用 `:=`，编译失败。改为赋值 `=`；cmd重编译通过。
3. `batch1-helper-contract-red.log`：真实子测试证明HOME缓存未恢复、nil Args变空切片、子命令流未恢复。修现有helper与plan fixture后，同一合同green通过。
4. `batch1-cmd-shuffle.log`：前序Add等旧fixture恢复HOME但留下临时缓存。统一12个测试文件中既有 `t.Setenv(HOME)+home.ResetForTest` 为同一setTestHome cleanup。用户断言保留；最后shuffle5通过。没有通过归零原值或修改断言掩盖泄漏。

最初before/after证据保留：F2仅新增 `cmd/cli_state_isolation_test.go`，`t2-seg1-red.log` 中ProfilesDir泄漏和reset写零值均真实断言失败；F2文件字节未为过测改写。最终这两个测试转绿。

### 独立审查证据

计划初检NEEDS_REVISION；修blocking后有界复检PASS_WITH_WARNINGS（0 blocking，Windows包枚举过滤空行1项warning），见 `precheck-v1.md`、`precheck-final.md`。

独立T1审查发现上述裸aivo预期错误（DEFECT_FOUND）。actor最终directive JSON格式错误，**未当作审查通过**；协调者直接读取其正式mesh任务REVIEW-T1结果并保存 `review-t1-structured-evidence.json`。正式CAS已追加复现、最小修复及exit0证据。该修复尚未独立复审，最终review仍未执行。

## 用户基线保留与快照

初始外部材料：`baseline.diff`、`baseline-index.diff`、`baseline.sha256`、`baseline-index.sha256`、`baseline-status.txt`、`baseline-head.txt`。最初 `baseline-files.tgz` 实际只收录未跟踪`.pi/`（tar多份-T列表行为导致tracked未收录），没有据此声称完整tracked快照。

已用未改变的初始HEAD导出 `baseline-head-tracked.tar`，在**仓库外**应用保存的原始baseline.diff，重建 `baseline-tree/`；初始253文件sha256全部匹配，见 `reconstructed-baseline-hash-check.log`。因此可精确核对原有用户内容，工作区未被回退。

`cmd/update.go`、`cmd/lint_test.go`、`cmd/util_test.go`、`internal/picker/picker_test.go`、`web/handler.go` 与初始文件hash相同。重叠的 `cmd/list_test.go` 只把HOME设置迁入恢复helper，用户新增setup与断言保留；`cmd/update_test.go` 只迁移fake程序/HOME及由此失效的本轮helper，用户原删除未恢复。完整快照与当前差异留外部证据目录。

## 预算与收尾

上一轮被 `spent $11.981516 of $10.000000` 拒绝。用户随后批准本轮累计上限 USD 30，并要求临时项目级 `.pi/fabric.json` 覆盖。

实际处理：本项目 `.pi/fabric.json` 原状态为**不存在**（已外部记录）；本次仅新增该文件，`agents.budgetUsd` 最终为 18（已花 11.981516 + 18 ≈ 29.98 ≤ 30），未改 global、未清账本。运行中该值不热加载，需会话内真实 `/reload` 重建 AgentManager 账本才生效；`/reload` 后原 10 限额不再拒绝。收尾已删除该文件，恢复“不存在”原状。

角色：v1 四个错误 requires 定义早已 stopped；本批开发 A/B/C 与独立 reviewer 均已 stopped，定义与 mesh 历史保留，无重复并行 actor。

未声称覆盖率或性能提升。

## 远端 GitHub Actions 验证补充

用户随后指示在远端 GitHub Actions 上执行测试。由于仓库为公开仓库（Actions 时长无私有配额限制），已同步将 `macos` 作业扩展为全量测试 + `vet` + 8 包 `race`、在 `linux` 增加 `cmd` 乱序多轮验证、扩展 `windows` 通用测试包集合，并通过临时 Git 索引构建提交推送至远端 `ci/test-reliability` 分支（本地 `main`、`.git/index` 与工作区保持原状）：

- 首次远端运行 `44235233102`（提交 `2000579`）捕获 3 个真实问题：
  1. GitHub Actions 已下线 `macos-14` 镜像（`The configuration 'macos-14-arm64-us-default' is not supported`）→ 切换为 `macos-latest`。
  2. Windows `internal/backup` 的 `TestRestoreFromArchive` 失败：`addDirToTar` 原用 `filepath.Join` 在 Windows 上写入 `\` tar 头，`resolveTargetPath` 随后用 `/` 前缀匹配导致静默跳过所有技能与预设还原 → 改为 `filepath.ToSlash` 写标准 `/` 头，并在还原时兼容旧 `\` 条目且保持路径逃逸拦截，新增 `TestBackupWritesSlashHeadersAndRestoresBackslashEntries`。
  3. 排除工作区 `cmd/lint_test.go` 时，旧 `HEAD` 的 `TestLintStrictExitsNonZero` 在 `-shuffle=on -count=3` 下泄漏 `lintStrict` 标志（工作区 `cmd/lint_test.go` 已包含隔离恢复）；若后续将 `ci.yml` 推入 `main`，需一并纳入 `cmd/lint_test.go` 的标志隔离恢复。
- 修复后远端运行 `44236085592`（提交 `a635bb2`，https://github.com/woyin/SkillsManager/actions/runs/44236085592）全部 `success`：
  - `linux / full suite + vet + targeted race`（`ubuntu-latest`，含 `go test ./... -count=1`、8 包 `-race`、`cmd -shuffle=on -count=3`）：`success`（27s）。
  - `macos / full suite + vet + targeted race`（`macos-latest`，含 `go vet ./...`、`go test ./... -count=1`、8 包 `-race`）：`success`（1m19s）。
  - `windows / vet + compile all tests + portable subset`（`windows-latest`，含 `go vet ./...`、23 个测试包全部 `go test -c` 编译、11 个通用包 + `tool`/`updater` 通用测试运行）：`success`（2m0s）。
- 后续收口（用户指示“合并处理，不考虑Windows”）：已从 `.github/workflows/ci.yml` 移除 `windows` 作业，固定为 `linux`（`ubuntu-latest`）与 `macos`（`macos-latest`）双平台对等门禁（全量测试 + `vet` + 8 包 `race` + `cmd` 乱序 3 轮），将本轮改动与 `cmd/lint_test.go` 标志隔离合并提交至 `main` 并清理临时分支 `ci/test-reliability`。
