# 架构改进验收记录

状态：本轮实现与独立验收已完成。结论为 **PASS_WITH_WARNINGS**，不等于全量测试通过。`go test ./...` 仍有8个继承基线失败，新增失败为0；`go vet ./...` 与8个相关内部包的race测试通过。源码在验收期间保持冻结，167个文件前后指纹一致。用户原有改动保留，未提交、未推送。

本文件前面的分阶段记录是执行历史；以本段和末尾“最终独立验收”作为当前结论。未执行远端CI，不声称Linux/macOS远端作业通过。

## 基线快照

- 路径：`/tmp/skillsmanager-architecture.40Z0ed/`。
- 文件：baseline.diff、baseline-index.diff、files.tar.gz、paths.zlist、status.txt、head.txt。
- 初始变更：cmd/lint_test.go、cmd/list_test.go、cmd/update.go、cmd/update_test.go、cmd/util_test.go、internal/picker/picker_test.go、web/handler.go、.pi/。禁止reset/stash，重叠文件最小增量。
- 当前本机：darwin/arm64，CGO_ENABLED=1。
- `go test ./...`：exit 1，日志 baseline-test.log。`go vet ./...`：exit 0，日志 baseline-vet.log。两者均在源码冻结、开发actor只读期间执行。

## 八个既有测试失败

下表行号指向基线快照。最终8项与基线逐项同名、同断言消息。Detected Agents与固定Claude fixture不一致已有代码、日志与PATH隔离fixture支持；这不等于8项均已完整诊断。未确认的原因仍明确保留，尤其aivo子命令具体失败原因未知。未为变绿修改无关代码或测试。

| 测试 | 基线错误 | 最小归因证据 | 最终对比 |
| --- | --- | --- | --- |
| cmd/TestInstallBareNameFromRegistry | install_registry_test.go:66：期望 `.claude/skills/deployable`，路径不存在 | 测试固定Claude路径；install.go:502使用DetectInstalled；基线输出实际安装 `.agents/skills/deployable`。推断选择Agent与fixture预期不符。 | 仍失败；与基线同名、同断言消息 |
| cmd/TestPlanCommandApplyRemovesOwnedLinkInstall | plan_test.go:155：owned foo未移除 | fixture仅创建Claude并记录managed.claude；基线plan.go:80按DetectInstalled扫描。本机检测集合不含Claude；PATH隔离fixture可确定性扫描该实体。 | 仍失败；与基线同名、同断言消息 |
| cmd/TestPlanCommandApplyBootstrapWithSkillTarget | plan_test.go:202/205：Claude链接不存在，managed为codex/opencode/zed | fixture固定Claude；plan.go:378按DetectInstalled选目标；日志已显示实际host集合。 | 仍失败；与基线同名、同断言消息 |
| cmd/TestPlanCommandApplyBootstrapWithProfileTarget | plan_test.go:290/293：Claude链接不存在，managed为codex/opencode/zed | 同一目标检测路径，日志给出managed集合。未修改测试假设。 | 仍失败；与基线同名、同断言消息 |
| cmd/TestPlanCommandCheckPassesWhenBaselineSatisfied | plan_test.go:386：`curation plan not satisfied` | fixture固定Claude项目路径；runPlanCommand按Detected Agents建计划。curation审计的PATH隔离fixture显示同一Baseline满足，日志见cmd-path-isolation.log；本轮未改默认范围。 | 仍失败；与基线同名、同断言消息 |
| cmd/TestWriteProjectStatusShowsProjectInstallAndOrphan | status_test.go:72：status不含orphan | fixture在tool.Claude.ProjectSkillDir建链接；status.go:179使用DetectInstalled；输出未含该实体。 | 仍失败；与基线同名、同断言消息 |
| cmd/TestWriteProjectStatusReportsBrokenSymlink | status_test.go:102：status不含broken issue | 同上fixture固定Claude；输出不含该链接。具体检测链尚未隔离验证。 | 仍失败；与基线同名、同断言消息 |
| internal/aivo/TestGetStats | aivo_test.go:52：GetStats returned nil，5.01秒 | 测试仅在Detect().Installed为真时调用真实aivo；aivo.go:24有5秒CommandContext超时，GetStats命令失败或JSON解析失败均返回nil。未采集子命令具体失败原因，不声称已确认为超时。 | 仍失败；与基线同名、同断言消息 |

## Swarm 与执行证据

- run key：`arch-1117-t6j`；topic：`team.arch-1117-t6j`。
- installer actor：`905f512871d042aeab64a5390ece8d23`。
- curation actor：`15f2e83041b1470eb682da7d83b73071`。
- modules actor：`5037a0b3f45b4f9ebef96bac9202b60a`。
- reviewer actor：`f0184190ca814d29866bcc64b8f3a353`。
- runner=pi，transport=herdr-core；persistent actor，mesh CAS任务。Herdr `hc ls` 已观察到父终端t6j创建的node worker终端；不是普通shell并行冒充swarm。

## 已核实的只读审计证据

- installer：installer-audit.log 与 installer-interface-design.log。核实MCP未回滚、category失败被吞、rollback/commit错误丢弃。额外确认无效MCP JSON可能被覆盖；Placement恢复失败仍清理唯一backup的路径须修复。
- curation：仓库外副本 `audit/` 中新增只读复现fixture，curation-red.log与cmd-red.log分别2个指定失败：新增在ownership失败后残留；新增回调失败残留；ghost安装假成功；foo+ghost部分成功假成功。源码未写。curation-audit-report.md记录根因。
- modules：audit-modules-repro.log记录4个CLI二进制复现，R1同fixture CLI因EACCES失败而Web报告healthy。R2/R3/R4为新发现的in-place/install/rm部分失败退出契约，先记录，避免未经许可改变退出码。
- 审计已明确R5为当前Detected Agents范围与已有项目Agent目录不一致。是否扩大默认范围属于后续行为决策，本轮保留现有契约，不为基线变绿修改无关测试/产品逻辑。

## 方案预检

- 首次precheck：NEEDS_REVISION。只读reviewer完成任务；completed仅表示检查结束，不代表通过。
- 3项blocking：冲突的两份冻结协议；单次合并写与旧“MCP第二次写”场景矛盾；无效JSON/category失败/CommitCleanupError缺机器验收。
- v3修订：唯一权威为仓库plan的接口协议v2，旧frozen/v6稿废止；统一InstallTransaction+StageProjectConfig，Commit写MCP→最终配置→cleanup；改用后续合并/单次写失败fixture；补齐三个P0测试及symlink/只读原子写fixture。
- 同时补齐B2/B3精确迁移函数与新增fixture、A2项目包依赖、回调替换决策、测试屏障宣布者、C2独占最终全量。v3 SHA256：cfebe8727b9c93d39d0a0fa00ada16adc2dfb000a956b92b5335cf629bca100d。
- v3复检仍有两处文内矛盾：A1顺序简写误导、legacy回调条件句。v4已逐行修正。precheck-v4最终PASS_WITH_WARNINGS，blocking=0，hash `26ce22c1997f5202f788be633deffc46dfc21095a428c15b38272567f2240bbc`。唯一剩余warning为新包索引要求未统一写正文，实施指令已明确health/lifecycle均交付FOLDER_INDEX。阶段I A1+B1已CAS解锁；A2等待A1全部完成。

## 阶段I执行记录（尚待最终验收）

- A1已提交任务结果，实际代码为BeginInstall/BeginRegistryInstall、InstallTransaction.Result/ChangedSkills/ChangedPlacements/StageProjectConfig/Commit/Rollback；未提供旧草案Added/DesiredConfig/Committed。CommitCleanupError通过errors.As识别。
- A1初次阶段证据：A1-red1.log包含5个真实失败；A1-green-final.log记录installer/project/lockfile/fsutil四包通过，A1-green-targeted.log记录定向通过。它们不是最终验收。
- 主协调者复审发现新增fixture有绝对ProjectSkillDir拼接路径风险，原路径断言可能为空；已要求全部通过TargetDirectories解析实际路径，并补“Commit末次配置写失败”的真实断言。修正后必须在仓库外基线副本重新证明受影响red，再补green。该验收门禁尚未关闭。
- B1已落地health.Scan(dirs,registry,knownProjects)，CLI/Web保持global技能目录+DB项目记录范围。主协调者发现旧bool Verify会吞Stat EACCES/ELOOP；已改为Lstat/Readlink/Stat并对非不存在错误返回error。新增TestScanReportsInaccessibleTargetAsError和TestScanReportsSymlinkLoopAsError。
- B1阶段日志：audit-modules-b1-redgreen.log。health/Web定向通过，cmd TestCheck在A1生产代码停写的窗口运行。最终完整fixture分类一致性、IO与健康状态区分仍需独立review。
- review-A1任务已交给独立只读reviewer，不运行测试。生产写入待修复/验收结果再解锁后续。

## 阶段I审查修复与 B1 验收

- B1任务completed。同一fixture集成测试为cmd/TestCheckCLIAndWebShareIssueSetOnSameFixture（CLI输出解析集合 vs Web JSON集合，reflect.DeepEqual）及TestCheckCLIAndWebAgreeOnIOError（CLI error/HTTP500）。定向7项通过，日志audit-modules-b1-redgreen.log；health/Web先前仍适用证据已复用，不重复全量。
- A1 fixture修正后，在仓库外原实现副本定向运行兼容旧入口，A1-red1-clean.log再次出现5个真实断言失败。当前修正green见A1-green-fixture.log；实际路径由TargetDirectories生成。不能将缺符号编译失败作为红灯证据。
- 独立review-A1：NEEDS_REVISION，1blocking（新建文件Chmod覆盖umask，0644替代原0600）及6warnings（两处rollback丢错、恢复顺序、ErrPermission、既有MCP0600恢复、root权限fixture）。fix-A1已建CAS任务，A2/B2加入依赖，修复前不实施。
- 已知兼容限制：事务需要读旧字节；只写不可读文件不能安全补偿，应明确失败。无fsync/目录同步。legacy损坏.sm.json仍按原入口覆盖。普通Registry partial warning/continue保持；cleanup失败现在透明报告，category Profile失败改整体拒绝符合ADR0012。

## 阶段I必要修复的当前证据

- fix-A1已completed。A1-green-4pkgs.log与A1-green-newcases.log在修复后的快照通过。独立reviewer增量复核确认原1blocking和6warnings已关闭，代码层面0blocking。umask、ErrPermission、多错误汇总、逆序恢复、既有MCP0600恢复、权限探针均有代码/fixture支持。
- 随后新增dangling文件链接fixture为有意红灯，旧四包green不能代表含新fixture的当前树。fix-A1-symlink-red.log记录5个真实失败（单链接/链接链创建referent、MCP/config经悬空链接写入、末次失败恢复原链接）。补偿需删新referent而非原symlink。修复只限fsutil与installer快照，不建通用框架。
- B1补充missing_project后的同fixture集成已通过，`go test ./cmd -run TestCheckCLIAndWeb -count=1 -v`：2/2 PASS。CLI读取同一临时DB并拼接两类扫描结果，Web传同DB；三类集合逐项相同。目录GlobalSkillDir覆盖当前catalog与SkillDir等值，相关getter由结构核对证明（两入口均直接调用），不为测试另加注入接口。
- C1完成：.github/workflows/ci.yml对PR/branch push/manual生效，ubuntu全量test+vet+定向race，macos-14运行代表性文件系统包。release.yml未修改。远端未运行；既有失败是否在远端出现取决于其软件检测环境，不声称CI已过。
- D1完成设计取舍：保留SetScope/SetCopyMode兼容，代价是调用前需设置状态；复用installer.TargetDirectories与tool.GetGlobalSkillDir，不加builder/flag/config。当前没有实证支持更大接口迁移。

## 阶段II放行

- fix-A1-symlink任务completed且生产冻结。主协调者亲读实现与A1-symlink-newcases.log，7项PASS；A1-symlink-green.log为fsutil/installer两包通过。实际快照记录referent路径、存在性、mode与原字节；补偿只操作referent，原链接保持。新API仅WriteFileAtomic和事务快照两个调用者使用。
- 限制：解析链有8次迭代上限；referent父目录缺失/目录目标/循环均明确失败，无自动mkdir；0200只写文件必须可读原快照才允许事务。无掉电/跨进程保证。
- A2/B2所有依赖逐项核实后CAS解锁：A2负责curation/plan生命周期，B2负责updater Origin业务；文件所有权不重叠。cmd包测试统一等两方停写。旧未变包证据复用，最终全量将覆盖所有当前fixture。

## 阶段II当前验证

- A2首次内部27例通过；主审发现ApplyRemovals=false清managed、MCP-only bootstrap漏Begin、共享目录managed键不同步三类分支，owner以a2-review-red.log复现后修正，a2-green3.log/a2-recheck-curation.log记录32例通过（具体计数以最终日志为准）。
- review-A2只读结论PASS_WITH_WARNINGS、0blocking，确认补偿错误汇总、已有实体不接管、共享目录移除与managed同步、cleanup已提交分支、单点配置提交。剩余必要修正：回滚后Removed结果准确；无操作时不重写配置；only-removal配置失败恢复fixture；MCP-only bootstrap cmd真实入口fixture；Baseline保持Profile+额外skills原契约。
- A2 CLI冻结窗口：a2-cmd-validation.log，14例中10通过，4失败均为原基线PlanCommand四项；新TestPlanApplyFailsWhenRegistrySkillMissing、TestPlanApplyStrictAdditionsZeroSideEffect、TestPlanApplyBootstrapRecordsOwnership、TestPlanApplyOwnedRemovalLeavesNoDanglingOwnership全部通过。
- B2已真实迁移Origin分组、cache刷新、Source级原件重写、lock刷新到internal/updater；cmd保留范围选择与输出。成功写入才提示Copy过期，LockResult.Updated改为成功持久化数，保存失败为0。尚待cmd定向窗口及独立review-B2。不得把模块green当完整接线已验收。

## 阶段II审查修正收尾

- review-B2：PASS_WITH_WARNINGS，0blocking。确认实际迁移、Source隔离、RefKind不丢、Copy in-place三函数与用户基线逐字节相同、用户删除的三个legacy函数未复活。B2 cmd回归日志audit-modules-b2-cmd.log为PASS，未调整旧断言。
- fix-A2以fixa2-red.log复现结果Removed/no-op/Baseline兼容缺口后修复；curation最终37例PASS。补齐only-removal保存失败恢复链接和cleanup已提交时实际Added/Removed结果；Baseline仍为Profile+额外skills，不固化Profile展开成员。fixa2-cmd.log记录真实cmd/TestPlanApplyBootstrapProfileMCPOnly通过。源码冻结，等待CAS终态归档。
- fix-B2已修锁部分成功报告（warning与Refreshed N并存）和真实Save失败fixture：预先有效锁可Load，父目录只读阻止Save，断言Err含writing且Updated0；另测一项rehash失败但good条目已持久化。audit-modules-fix-b2.log模块通过，cmd只验证新锁相关断言，未重复全量。

## 阶段III交付与冻结前审查

- B3任务completed。实际新增internal/lifecycle/install.go（BuildJobs/DedupeJobs/ApplyJobs结构化结果）与removal.go（CollectReferences/CleanupInstalls/RemoveLockEntry显式入参）；cmd保留flags、确认、输出和ADR0017删除决策。真实搬迁而非按行拆文件。
- 迁移前/后相同命令 `go test ./cmd -run 'Test(Install|Rm|Uninstall|Remove)' -count=1`：37项中36通过，仅原TestInstallBareNameFromRegistry失败；无新增失败。模块 `go test ./internal/lifecycle -count=1 -v`：9/9通过。日志audit-modules-b3-baseline-cmd.log、audit-modules-b3-cmd.log、audit-modules-b3-log.md。
- B3共享索引已同步：curation新增事务/测试，updater，lifecycle，PROJECT_INDEX/cmd索引。A1内部索引未越权修改。
- 原状态权威方案要求“索引不可用不能假称完整”；最后fix-index-semantics仅为knownProjectPaths的DB打开/读取失败增加明确warning和一个损坏DBfixture，保持普通退出与删除决策。独立review-B3正在只读审查。
- 编译红灯说明：B2新模块接口曾先写测试再实现，日志里的undefined符号仅为测试先行记录，不算功能缺陷复现。Refactor行为依据初始全量及定向前/后命令；真实问题红灯证据另列A1/Curation/健康扫描日志，不混称。

## 最终冻结前独立审查

- review-B3：PASS_WITH_WARNINGS、0blocking，逐项对HEAD确认scope/Eve/subagents、去重顺序、PlaceMany并发数/Copy fallback/重叠拒绝、输出计数、ADR0017策略与退出码不变。模块无cmd全局依赖。
- fix-index-semantics已completed。cmd/rm.go在项目DB打开或读取失败时warning引用可能不完整；正常空项目不报错。`go test ./cmd -run TestKnownProjectPaths -count=1 -v`：3/3 PASS；受影响rm定向也通过。未改删除策略/SQLite架构。
- 主协调者已比较用户快照：cmd/lint_test.go、cmd/list_test.go、cmd/util_test.go、internal/picker/picker_test.go逐字节一致；cmd/update_test.go相对用户基线70增行、0删行；用户删除的legacy更新函数未复活；web/handler.go原注释改动保留。索引和HEAD未变，无提交推送。
- 最后fix-B3仅关闭审查指出的ProjectDir为空且IncludeGlobal时旧全局清理语义（direct fixture红→绿），并同步cmd/install.go失效文件头符号。重复DB调用/重复warning为既有结构，保留明确限制。完成后重建最终源指纹；此前167文件指纹是冻结候选，不能当最终验证快照。

## 最终独立验收

独立只读actor `f0184190ca814d29866bcc64b8f3a353` 执行C2。主协调者亲自读取任务终态、全量失败日志、race日志和指纹收据。所有开发actor在验收前均为idle、queued=0；验收期间未改源码、测试或依赖。

| 检查 | 实际结果 | 证据（均在外部快照目录） |
| --- | --- | --- |
| 验收前源码指纹 | exit 0，167/167 OK | final-source.sha256、final-source-before.log |
| `go test ./...` | exit 1；8个继承失败，0个新增失败；失败包仍为cmd/internal/aivo | final-test.log，与baseline-test.log逐项比较 |
| `go vet ./...` | exit 0，输出0字节 | final-vet.log |
| `go test -race ./internal/installer ./internal/curation ./internal/updater ./internal/health ./internal/lifecycle ./internal/project ./internal/lockfile ./internal/fsutil -count=1` | exit 0，8包全部ok | final-race.log |
| 验收后同一源码指纹 | exit 0，167/167 OK | final-source-after.log |

五步串行、各一次。全量测试按要求原样运行，部分未变包命中Go缓存；race使用`-count=1`实际重跑。检查结论只覆盖本机darwin/arm64和冻结源码；远端CI未触发。

### 最终验收矩阵（测试均已运行，成功证据来自最终全量通过的相关包与race）

| 场景 | 实际用例/证据 | 结论 |
| --- | --- | --- |
| skills/MCP成功后配置末写失败，已有MCP字节与0600权限恢复 | TestInstallRestoresMCPOnConfigFailure、TestInstallRestoresExistingMCPBytesAndMode | 通过 |
| 后续MCP合并失败与单次MCP写失败 | TestInstallRestoresMCPOnLaterMCPMergeFailure、TestCommitMCPWriteFailureCompensates | 通过；单次合并替代旧逐次写场景 |
| 无效MCP JSON保留原字节、category成员失败整体拒绝 | TestInstallFailsOnUnparsableMCPAndKeepsBytes、TestProfileCategoryMemberFailureAborts | 通过 |
| 已有链接不被回滚、strict与普通partial合同区分 | TestInstallKeepsExistingLinkOnFailure、TestRegistryStrictPartialFailure、TestInstallFromRegistryPartialFailureKeepsCompat | 通过 |
| 回滚自身失败保留唯一备份、清理失败明确内容已提交 | TestRollbackReportsFailure、TestCommitCleanupFailurePreservesCommittedContent | 通过 |
| umask、权限谓词、链接目标/悬空链接补偿 | TestWriteFileAtomicNewFileHonorsUmask、TestWriteFileAtomicReadOnlyErrorIsPermission、TestCommitConfigFailureRestoresDanglingMCPLink | 通过 |
| Curation新增后移除/ownership失败补偿 | TestApplyAdditionRolledBackOnRemovalFailure、TestApplyAdditionRolledBackOnOwnershipFailure、TestBootstrapOwnershipFailureRollsBack | 通过；接口fixture执行真实临时FS操作 |
| 仅移除保存失败、无操作字节不变、已提交清理失败结果准确 | TestFixA2OnlyRemovalSaveFailureRestoresLink、TestFixA2NoopApplyLeavesConfigBytes、TestFixA2CleanupCommittedReportsAddedAndRemoved | 通过 |
| bootstrap仅MCP的真实CLI入口 | TestPlanApplyBootstrapProfileMCPOnly，fixa2-cmd.log | 通过 |
| CLI/Web相同HOME/Registry/DB的问题三分类与IO错误 | TestCheckCLIAndWebShareIssueSetOnSameFixture、TestCheckCLIAndWebAgreeOnIOError，audit-modules-b1-redgreen.log | 通过；最终全量cmd包虽有原7失败，这两项未失败 |
| scope/pinned/Source隔离/Copy in-place/lock | updater模块测试、audit-modules-b2-cmd.log、audit-modules-fix-b2-cmd.log | 定向通过；独立review确认in-place三函数未改、用户删除未复活 |
| 安装/删除生命周期 | lifecycle 10例、audit-modules-b3-cmd.log、audit-modules-fix-b3-green.log | 通过；CLI定向仅原InstallBareName失败 |
| 索引不可用故障提示 | TestKnownProjectPaths三例，audit-modules-b3-log.md | 通过；不改变删除决策/退出码 |
| PR/push CI与发布保障 | .github/workflows/ci.yml静态核对，release.yml无diff | 配置已落地；远端未运行 |

### 八个继承失败的最终归因边界

上方基线表已逐项更新最终状态。七个CLI失败与固定Claude fixture/本机Detected Agents集合不一致的代码路径相关：最终日志仍显示codex/opencode/zed，PATH隔离fixture通过。该默认范围属于既有契约，本轮未扩大。

`internal/aivo/TestGetStats` 仍是`GetStats returned nil`，约5.01秒。具体子命令失败原因**尚未诊断**。该测试只有Detect().Installed为真时才执行，因此独立报告中的“本机无真实aivo环境”推断未获证实，主协调者不采纳该归因。未为消除该既有失败修改无关逻辑或测试。

### 独立审查与swarm终态

- A1及umask/链接增量审查通过；A2、B1、B2、B3为PASS_WITH_WARNINGS，实质blocking均为0；有证据的兼容缺口已修并定向验证。
- review-B3曾把报告误写顶层键。主协调者核实B3依赖后CAS领取正式任务并归档原独立报告，保留originalKey/originalVersion/reportBy；没有伪称重跑审查。正式依赖随后正确解锁。
- A1/A2/B1/B2/B3/C1/D1、各fix任务、C2均completed。completed表示任务执行结束；C2另存acceptanceVerdict=PASS_WITH_WARNINGS，未把状态标签当测试通过。
- 三个开发actor与一个只读reviewer的ID见前表；run=`arch-1117-t6j`，runner=pi、transport=herdr-core。实际使用persistent actors、依赖检查和mesh CAS；协调产物在.pi/，原始.pi快照仍在仓库外归档。四个角色收齐idle/queued=0后已全部stop，终态均为stopped，未删除任务与历史。

### 用户改动保留与版本控制

用户未触及的4个修改文件逐字节等于外部快照；cmd/update_test.go相对用户基线仅新增70行，旧删除仍在；cmd/update.go的3个用户删除函数未复活，Copy in-place函数与基线逐字节一致；web/handler.go原注释改动保留。索引diff与HEAD均未变，未提交、未推送。final.diff记录已跟踪文件变化，新增文件见final-new-files.txt及当前工作区；最终报告只修改本文和方案，不触及已验收源码。

### 明确限制与后续范围

1. 全量仍有8个继承失败。它们已逐项记录，不是全绿，也不统称为已诊断环境故障。
2. 同目录rename只提供单文件替换；无fsync、跨文件原子、掉电持久、跨进程锁或持久journal保证。
3. 文件链接解析为有界8次迭代；合法更长链也可能明确报错。referent父目录缺失/目录目标/循环不自动修复。事务必须能读取旧字节，0200只写文件会明确失败。
4. rollback恢复失败保留备份并报路径，可能需要人工恢复；CommitCleanupError表示内容已提交，仅清理失败，不会谎称全回滚。
5. Registry普通批量warning/continue合同保留；Curation/Profile用strict事务。旧Direct Install部分失败退出策略、in-place缺缓存退出策略、不可访问项目引用拒绝策略未扩大修改；删除索引与清理失败现在至少明确warning。
6. knownProjectPaths在单个rm流程可能被调用两次并重复warning；按有界范围保留，不新增缓存/框架。部分旧文件头生成器注释存在既有痕迹，不影响vet/构建。
7. 本轮未测远端CI平台、掉电故障或跨进程并发；不把发布交叉编译当平台运行测试。

已完成授权范围内的方案、拆分、实现、独立审查与验收。无需新的用户批准；本轮没有提交或推送。

## 提交快照验证补充

用户随后授权 commit & push。提交时仅暂存本轮架构改进，原有7个已修改文件中的既有差异和`.pi/`运行记录保留在工作区，不纳入提交。通过索引导出待提交快照到`/tmp/sm-commit-snapshot/`验证；未修改工作区源码。

- `go test ./...`：exit 1，仅`internal/aivo/TestGetStats`失败（`GetStats returned nil`）；cmd包通过。日志`/tmp/sm-commit-test.log`。
- `go vet ./...`：exit 0。日志`/tmp/sm-commit-vet.log`。
- 此快照不含原有用户差异且工作目录不同，因此与前述工作区验收输入不同。七个CLI失败未复现，不据此宣称其根因已修复。
- 前述167文件指纹与race结果只适用于前述工作区快照；本段是实际待提交快照的新增验证证据。
- 前文“未提交、未推送”描述上一轮验收时的状态；本节记录后续提交发布授权与验证，实际提交和推送结果以Git记录为准。
