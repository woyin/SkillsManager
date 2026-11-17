# SkillsManager 架构改进方案

状态：v4方案已通过预检，已完成分批实施与独立验收。最终全量保留8个继承失败，无新增失败；详情见architecture-improvement-acceptance.md。唯一权威协议为本文件“接口协议v2”，旧installer-interface-frozen.md、installer-interface-frozen-v6.md、installer-protocol-response.md已废止。本文是实施设计；完成状态与实际证据见 architecture-improvement-acceptance.md。未运行的测试不得记为通过。

## 授权、基线与执行约束

- 本轮实现并验收，不提交、不推送、不安装或升级依赖，不修改全局配置。保持现有 CLI 参数、退出码和 JSON schema；普通 Registry 批量安装继续 warning/continue。
- 用户原有差异为 cmd/lint_test.go、cmd/list_test.go、cmd/update.go、cmd/update_test.go、cmd/util_test.go、internal/picker/picker_test.go、web/handler.go 和 .pi/。基线位于仓库外 `/tmp/skillsmanager-architecture.40Z0ed/`：baseline.diff、baseline-index.diff、files.tar.gz、paths.zlist、status.txt、head.txt。所有重叠编辑从当前内容增量进行。禁止 reset/stash。
- 依据 CONTEXT.md 与 ADR 0007、0012、0013、0014、0017、0018、0020、0023、0028。Profile/Curation 全成功；Registry Update 按 Source 隔离；tag/commit pinned，branch tracking；只移除 owned 项目 Link Install。
- swarm run `arch-1117-t6j`，topic `team.arch-1117-t6j`，transport `herdr-core`，runner `pi`。三个开发 persistent actors 与独立只读 reviewer。协调者拥有本文和验收文件。hc 使用 `/Users/breestealth/.local/bin/hc send t6c`，不改变全局 PATH。
- 每项先读 dependencies，全部 completed 才能 CAS claim ready；ifVersion 为刚读到的版本，claim失败停止。完成更新也使用观察版本。任务保存在 `runs/arch-1117-t6j/tasks/<ID>`。先只读核实，再预检，再实现。开发最多3个并行角色。共同包/测试读取也是依赖；执行包测试时其他角色不得修改该包。
- 全量测试必须在冻结源码快照上串行执行。基线 `go test ./...` exit 1，`go vet ./...` exit 0。8个既有失败逐项记录在验收文件；不得为变绿而修改无关测试/逻辑。定向fixture必须固定 Agent 集合，不依赖本机已安装软件。

## 状态权威与事务边界

| 状态 | 权威与失败语义 |
| --- | --- |
| Registry 原件、Origin、实际 Installed Skill | 内容和实体的实际状态；不得用索引声称实体存在。Origin 控制 tracking/pinned；本地 Snapshot 不猜来源。 |
| `.sm.json` | 项目意图、显式 Baseline 和 ownership；只有本次实际成功且符合 owned 规则的 Link Install 可新增 ownership；已有手工链接/Copy 不因存在而被接管。 |
| `skills-lock.json` | 可复现来源；写入失败必须报告，不把失败保存说成来源已持久化。无全 Registry 事务，保留 Source 隔离。 |
| SQLite 项目索引与历史 | 可不完整的发现索引和审计记录；不能赋予删除授权。索引不可访问应提示或返回明确错误，不能假称完整无引用。 |

本轮采用进程内补偿事务：预检 → 保存原文件/保留 Placement backup → 应用实体/MCP/项目配置 → 完成后清理 backup。失败逆序补偿，汇总原错误与 rollback 错误。Commit cleanup 失败表示内容已提交但清理未完成，必须明确报告，不能宣称已全回滚。无跨文件原子保证，无掉电持久保证，无跨进程锁或 journal。文件保存采用同目录临时文件、关闭后 rename；保留已有权限及必要 symlink/只读错误语义。不得以 rename 意外放宽原写入权限或替换原 symlink。

接口协议 v2（以下均拟新增；现有方法保持兼容）：

- `Installer.BeginRegistryInstall(names []string, category string) (*InstallTransaction, error)`：全量解析预检后落地skills；失败自动补偿本次changed，成功返回deferred句柄。
- `Installer.BeginInstall(projectDir, profileName string, extraSkills, extraMCP []string) (*InstallTransaction, error)`：Profile完整预检；skills落地；MCP仅内存合并，不写项目文件。
- `InstallTransaction.Result() *InstallResult` 保持Applied实体结果；`ChangedSkills() []string`与`ChangedPlacements() []PlacementChange` 只含本次changed。PlacementChange保存Skill/Source/Destination/ActualMode/Fallback，值拷贝，不暴露可变Placement句柄。
- `StageProjectConfig(*project.Config) error` 暂存最终配置，保存写前快照。`Commit() error` 固定顺序MCP一次写→最终.sm.json一次写→释放Placement备份；前两步失败补偿本次实体与已写文件。Curation在Commit前完成owned移除和managed计算，提交错误时恢复移除；若为CommitCleanupError，内容已提交，不能恢复移除造成不一致。
- `Rollback() error` 在Commit前逆序恢复，幂等；Commit成功或仅cleanup失败后不再撤销已提交内容。`CommitCleanupError` 明确区分“内容已提交，备份清理失败”。errors.Join汇总主错误与恢复失败。恢复旧实体失败必须保留原备份并给出路径；不能像当前placementState.rollback那样Rename失败仍RemoveAll备份。
- 普通`InstallFromRegistry`保持warning/continue与nil error的部分失败契约；如增加`InstallResult.Failures`仅内部诊断且`json:"-"`，不新增CLI schema。Curation使用BeginRegistryInstall；Profile Install使用BeginInstall包装自动Stage/Commit。普通entry的cleanup失败须透明呈现，不能错误声称回滚。
- MCP旧文件无效JSON直接失败，保留原字节，不用空map覆盖。Category成员失败必须向Profile上报而不是warning吞错。
- 原子保存：同目录唯一临时文件→保持mode→写/关闭→rename；现有只读文件、symlink目标和目录错误要保留必要语义并测试。无fsync/跨文件原子保证。

共享源码读取纪律：阶段I并行A1与B1只允许各自独立package的fixture验证。Web/cmd测试依赖Installer，等A1及该阶段写入全部停止后运行。阶段II A2/B2可分文件实现，cmd包集成检查统一等两方停止。最终全量由独立验收一次执行，开发actor不得各自重跑全量。R5（Agent二进制不在PATH导致已有目录不可见）是独立范围问题，本轮不改变Detected Agents默认范围；基线4个Plan失败仍逐项比较。

## 任务与批次

以下测试名是拟新增验收用例，实施者可采用等价命名，但须在验收文件映射最终实际名字。所有命令从仓库根执行，预期 exit 0；红灯阶段预期指定断言失败。每批只回滚自己的增量，不恢复整份用户快照覆盖后续改动。

### A1 / P0：安装生命周期与文件写入

- 证据/根因：`internal/installer/installer.go` 的 Install 在 skills 后逐项 installMCP 再 project.Save；rollbackPlacements/commitPlacements 忽略错误。原 `.mcp.json` 未恢复。project.Manager.Save、lockfile.Manager.Save 直接 os.WriteFile。
- 目标/不变契约：Profile 完整预检；失败恢复本次 skills、MCP 与项目文件；既有链接保持。回滚失败透明。普通批量 Registry 保持 warning/continue；新增 strict 入口供 Curation，确保部分失败不能成功。
- 范围/非目标：Installer、project、lockfile、在现有fsutil包中拟新增的原子写辅助；无跨进程锁、journal、SQLite迁移，不全局重写错误框架。
- 接口/顺序：预检全部引用，捕获原文件存在/字节/权限。Begin阶段复用Placement落地skills并在内存合并MCP；StageProjectConfig暂存最终配置；Commit阶段一次写MCP→一次写.sm.json→清理备份。失败逆序Rollback并汇总多错误；cleanup失败是已提交，不能宣称回滚。权限fixture注明root/平台限制；以非目录目标、故障注入等确定性fixture补足，不能全部跳过。
- 所有权：installer actor 独占 `internal/installer/**`、`internal/project/**`、`internal/lockfile/**`、`internal/fsutil/**`（复用原包，原子写文件为拟新增）及对应 FOLDER_INDEX。其他角色不得修改。
- 依赖：audit-installer、precheck。为 A2 发布稳定接口；接口完成前 A2 只写自身回归fixture、不调用未确定方法。
- 失败恢复：只回滚 changed placements，幂等既有安装不进入补偿列表。恢复原文件或删除本次创建文件；恢复失败保留备份并报告路径。
- 测试：TestInstallRestoresMCPOnConfigFailure、TestInstallRestoresMCPOnLaterMCPMergeFailure、TestInstallKeepsExistingLinkOnFailure、TestRegistryStrictPartialFailure、TestRollbackReportsFailure、TestAtomicWritePreservesModeAndFailure、TestInstallFailsOnUnparsableMCPAndKeepsBytes（旧JSON无效原字节保留）、TestProfileCategoryMemberFailureAborts（category成员失败全量拒绝）、TestCommitCleanupFailurePreservesCommittedContent（errors.As识别CommitCleanupError且已提交内容不撤销）。单次MCP写失败使用不可写目标fixture，不能只断言类型。原子写另测TestAtomicWritePreservesSymlinkTarget及TestAtomicWriteRejectsReadOnlyTarget：写入symlink指向的实际文件并保留链接，不以rename替换symlink；只读目标保留错误。`go test ./internal/installer ./internal/project ./internal/lockfile ./internal/fsutil`；定向 `go test ./internal/installer -run 'Test(Install|ProfileCategory|RegistryStrict|Rollback|CommitCleanup)'`，全部预期exit0；红灯必须为真实断言失败，非缺符号编译失败。
- 回滚策略：恢复本任务 diff，保留用户基线；不在失败时清除可用于人工恢复的 backup。

### A2 / P0：Curation 全生命周期与 ownership

- 证据/根因：`internal/curation/apply.go` 的ApplyOptions.InstallAdditions回调只返回error，新增无补偿；移除恢复忽略错误；`cmd/plan.go` applyPlan 后另 recordAddedOwnership，bootstrap Install 后另保存 ownership。
- 目标/不变契约：新增、owned 移除、配置、ownership 在同一进程内提交范围内。任何后续失败撤销本次新增，恢复删除与原配置。绝不删除/接管已有手工安装；Copy 不 owned。未实际安装不得报告已成功新增。
- 范围/非目标：curation Apply、cmd plan/bootstrap glue 与相关fixture；保留 plan JSON字段、确认门槛、explicit-target规则；不扩大移除权限。
- 接口/顺序：预检owned移除对象；调用A1 Begin入口并保留新增句柄；执行移除；合并managed/baseline并StageProjectConfig；由Commit统一写MCP→最终配置→cleanup。新增ownership仅来自changed Link Install的真实路径/Agent。必须替换ApplyOptions.InstallAdditions的error-only签名为可补偿事务回调；不保留无补偿入口。
- 所有权：curation actor 独占 `internal/curation/**`、`cmd/plan.go`、`cmd/plan*_test.go`。installer actor负责A1接口；不共享写入。
- 依赖：audit-curation、precheck、A1全部完成（含internal/project原子写）；由主协调者宣布阶段解锁后实施。ApplyOptions.InstallAdditions替换为可补偿事务回调，原error-only签名不保留，迁移仓库内部调用与测试；不影响CLI参数/JSON。A1全部完成后才跑集成测试。
- 失败恢复：逆序恢复 removals、配置与新增；汇总rollback失败，不输出虚假的 all changes rolled back。目标缺失和同名冲突在副作用前拒绝。cmd测试不得t.Parallel；用t.Cleanup恢复planDir/planApply/planJSON/planProfile/planSkills/planOut和其他全局状态；race只针对internal包，cmd全局fixture不纳入。本轮既有4个Plan失败不要求转绿，新隔离fixture必须通过。
- 测试：TestApplyAdditionRolledBackOnRemovalFailure、TestApplyAdditionRolledBackOnOwnershipFailure、TestBootstrapOwnershipFailureRollsBack、TestApplyDoesNotOwnExistingLink、TestApplyRollbackFailureIsVisible。`go test ./internal/curation`；冻结cmd后 `go test ./cmd -run 'Test(Plan|Apply|Bootstrap)'`，既有4个plan失败逐项与基线比较，新增隔离fixture须全部通过。
- 回滚策略：仅撤销本任务新增与自身diff，原用户内容不变。

### B1 / P1：统一只读健康扫描

- 证据/根因：cmd/check.go 使用 tool.GetGlobalSkillDir，web/handler.go 使用 home + SkillDir，且吞掉IO/数据库错误；两边重复 broken/orphaned 分类。
- 目标/不变契约：同一fixture得到相同问题集合；CLI与Web的既有global skill扫描/数据库project记录范围保持；不新增project技能范围。发现问题与IO错误分开，Web IO错误返回现有错误响应而不是healthy。
- 范围/非目标：窄 `internal/health`（拟新增）读取模块；调用端仍负责CLI --fix/呈现与Web JSON；无扫描状态缓存。
- 接口/顺序：共享 Issue{Type,Path}；传明确目录与Registry根（目录解析使用 tool.GetGlobalSkillDir）；不存在目录可忽略，读目录/Readlink/Stat的其他错误返回error；broken优先于orphaned。项目记录不存在可分类，权限/DB错误单独返回。
- 所有权：modules actor 独占 `internal/health/**`、`cmd/check*.go`、`web/handler*.go`。web/handler.go已有用户1行改动必须保留。
- 依赖：audit-modules、precheck。不得与A2或B2同时跑cmd包测试；可在它们静止时定向验证，扫描模块/Web包独立验证。
- 失败恢复：只读本身无补偿；CLI --fix保留现有成功输出，删除失败不可假报Removed。
- 测试：TestScanSharedFixture、TestCheckUsesGlobalSkillDir、TestCheckIOErrorIsNotHealthy，覆盖broken、outside、inside、普通目录、共享目录去重、缺失目录与非目录IO失败。`go test ./internal/health ./web`；cmd冻结后 `go test ./cmd -run TestCheck`。
- 回滚策略：恢复调用点与新模块的本任务diff。

### B2 / P1：深化更新模块

- 证据/根因：cmd/update.go 混合source分组、ref策略、cache刷新、原件重写、lock和输出；已有 internal/updater.Apply 是源级文件系统事务。
- 目标/不变契约：把一条内聚业务路径移入 internal/updater，cmd留参数、交互和输出。必须保留scope、pinned/tag/commit、branch tracking、Source隔离、Copy in-place和lock行为；不得机械拆文件或新建通用框架。
- 范围/非目标：优先Registry Origin的分组与refresh/rewrite路径；可随后迁移Copy refresh；网络/CLI输出保留Adapter。普通旧git pull路径若无可靠fixture本轮不做大搬迁。
- 接口/顺序（拟新增，基于audit已核对原函数）：`OriginTarget{Name,SkillDir,Origin registry.SkillOrigin}`、`OriginGroup{Source,Ref,RefKind,Targets}`、`GroupOriginTargets`承接groupOriginSkills；`RefreshOriginGroup`承接refreshOriginGroup+refreshTrackingCache，返回Pinned/CommitBefore/CommitAfter/Written/Failures与group级error；`RewriteOriginTargets`承接rewriteOriginSkills+prepareRewriteTargets，复用updater.Apply并由registry.WriteOrigin保存metadata。`RefreshProjectLock`返回Updated/Err，cmd保持当前warning与退出码。IsDetached/IsDirty/HeadHash从cmd私有git助手下沉；不迁移并发pull与输出。无需新origin包，复用registry.ReadOrigin/WriteOrigin，cmd/skill_origin.go只保留必要适配。先锁旧schema保守pinned及Copy in-place行为，不能迁移时丢掉RefKind。
- 实际验收fixture计划：TestGroupOriginTargetsKeepsSourceRefIsolation、TestRefreshOriginGroupPinnedRewritesWithoutAdvancing、TestRefreshOriginGroupDetachedIsPinned、TestRefreshOriginGroupDirtyCacheReportsGroupError、TestRewriteOriginTargetsAtomicAcrossSource、TestRefreshOriginGroupPreservesRegistryOriginSchema、TestRefreshProjectLockUpdatesHashAndSkipsMissingEntries、TestRefreshProjectLockReportsSaveError。名字可等价映射；每项须真实业务断言，不能仅验证不存在的接口编译失败。
- 所有权：modules actor 独占 `internal/updater/**`、`cmd/update*.go`、`cmd/skill_origin.go`（仅迁移必要adapter时；不新增internal/origin包）。
- 依赖：audit-modules、precheck、B1完成；与A2不共享文件，但cmd包测试串行到冻结。
- 失败恢复：继续复用updater.Apply每源补偿；其他源继续；模块Err/Failures是部分失败判据，lock结果Err明确暴露，由cmd保持现有warning和退出码策略，不新增CLI JSON字段。
- 测试：先用已有Update/Origin/InPlace/Lock/Source/Pinned测试锁行为，再新增模块界面测试。`go test ./internal/updater`；cmd冻结后 `go test ./cmd -run 'Test(Update|Refresh|Rewrite|Collect|Origin|InPlace|Lock)'`。
- 回滚策略：按路径回退本次搬迁，保留基线用户删除内容。不能用git checkout整文件。

### B3 / P1：安装与删除生命周期的有界收拢

- 证据/根因：cmd/install.go、cmd/rm.go仍承载非交互的实体定位、引用扫描与lock清理。Placement 已提供安装生命周期，ADR0017规定删除边界。
- 目标/不变契约：本轮分别抽取一个可独立验证的业务单元：安装目标/实体落地复用Placement；删除引用/已知项目扫描集中内聚模块。CLI仍负责flags/确认/输出，Registry originals删除与uninstall语义不变。
- 范围/非目标：仅有现成行为fixture支持的生命周期路径，不把install/rm整文件转移，不顺便改历史dead code。
- 接口/顺序（拟新增，原函数已核对）：安装现有`installSkillsConcurrently`已复用Placement，无需重新实现。`internal/lifecycle.InstallJob`、`BuildJobs`承接buildInstallJobs+appendEveSubagentJobs，`DedupeJobs`承接dedupeJobsByDest；`ApplyJobs`承接Placement.PlaceMany+Commit，返回Installed/Failures，cmd保留逐项warning、成功计数、输出与退出码。删除`CollectReferences(name,registryPath,tools,knownProjects,projectDir,includeGlobal)`承接listRegistryReferences+collectProjectRefs+addPointingRefs，返回引用、不可访问路径和errors；`CleanupInstalls`/`RemoveLockEntry`承接cleanupInstallsByName+cleanupInProject+cleanupInDirs，返回Removed/Failures。模块不读cmd全局/DB；cmd获取knownProjects并执行ADR0017拒绝/force/Registry删除决策。扫描失败至少必须可观察，不能用空引用集声称完整；不新增退出码/JSON合同。
- 验收fixture计划：TestBuildJobsDedupesUniversalProjectDir、TestBuildJobsExpandsEveSubagents、TestApplyJobsReportsPerJobFailure、TestCollectReferencesFindsGlobalAndProjectLinks、TestCollectReferencesReportsUnreadableProject、TestCleanupInstallsReportsInaccessibleRemoval、TestRemoveLockEntryReportsCorruptLock。权限fixture注明root/平台限制；可用确定性的非目录/冲突fixture替代，不能跳过所有失败路径。
- 所有权：modules actor 独占 `cmd/install*.go`、`cmd/rm*.go`、拟新增 `internal/lifecycle/**`；不得改installer/placement；需要其接口时先协调依赖。
- 依赖：A1、A2、B2完成；本批单角色开发，源码冻结后定向测试。
- 失败恢复：保留已有Remove拒绝有引用原件、--force报告不可访问项目、Uninstall不删Registry；本轮不扩大事务承诺。
- 测试：`go test ./cmd -run 'Test(Install|Rm|Uninstall|Remove)'`，已知InstallBareName既有失败保留并比较；新增模块fixture只用临时目录且显式Agent。`go test ./internal/lifecycle`（仅当模块实际新增）。
- 回滚策略：按业务单元恢复本任务diff，不动原用户文件其余改动。

### C1 / P1：持续集成

- 证据/根因：只有 .github/workflows/release.yml tag/manual go test；未覆盖PR/push日常回归。
- 目标/不变契约：增加PR/push tests+vet，Linux和macOS实际运行代表性文件系统测试；race在支持平台定向运行；保留release go test保障，避免branch push与tag双重昂贵workflow。
- 所有权：modules actor 独占 `.github/workflows/**`。
- 依赖：precheck、B1完成。无需安装工具；静态读回核对trigger、矩阵、commands和release保障。
- 范围/顺序/失败恢复：新增workflow，push只branches，PR和manual；现有release不改或复用须不取消保障。不以GOOS交叉编译声称运行；CI当前未触发，远端结果待实际运行。回滚仅删除新workflow。
- 测试：C1只静态核对workflow，不自行跑全量。C2独占本地 `go test ./...`、`go vet ./...` 最终冻结各一次，以及定向 `go test -race ./internal/installer ./internal/curation ./internal/updater ./internal/health`。本机darwin/arm64 CGO_ENABLED=1，远端Linux/macOS不能提前记通过。

### C2 / P1：状态权威落实与独立验收

- 所有权：协调者独占本文与acceptance；reviewer严格只读，不写代码/文档；review结果由协调者原样摘要归档。
- 依赖：A1/A2/B1/B2/B3/C1完成，所有开发actor静止，源码冻结。
- 接口/顺序：主协调者宣布每阶段冻结/解锁。检查所有actor终态和CAS结果→审查增量diff及用户基线→独立review→修复缺陷（单文件所有者）→受影响定向验证→最终冻结→由reviewer唯一串行全量test/vet及必要race，协调者收集复用。若review先于最后修复，reviewer重查修复部分。全量只在相关代码变化或新证据失效后重复。
- 失败恢复：已知基线8失败逐项对比，不用成功包数掩盖失败；外部阻塞列具体错误并继续独立任务。索引不完整不改SQLite架构。原文件权限/备份/rollback错误实际验证。
- 验收：八类要求全部映射实际测试名和命令；8个基线失败记录最终状态；`git diff --check`；基线未触及文件哈希一致，重叠文件逐hunk对比；无提交推送，.pi协调产物单独说明。
- 回滚策略：只逆应用本轮增量patch，不能覆盖用户基线；若失败保留外部日志与恢复备份位置。

### D1 / P2：可变配置与目录字段取舍

- 证据：Installer的SetScope/SetCopyMode增加调用顺序要求，tool.Tool有SkillDir/GlobalSkillDir/ProjectSkillDir但现有tool目录是单一catalog派生。
- 目标：本轮先复用installer.TargetDirectories/installer.ResolveTargetDirectories和tool.GetGlobalSkillDir。保持构造/Set方法兼容；只有实际消除跨调用约束时才加内部不可变request，不新增用户flag/配置。
- 所有权：installer actor仅在A1必要范围；modules actor仅B1调用点。tool定义本轮只读。
- 依赖：A1、B1。验收：复用具名TestTargetDirectoriesResolvesScopeAndSkipsUnsupportedProjectTargets、TestTargetDirectoriesUsesAgentSpecificGlobalDirectory、TestTargetDirectoriesDeduplicatesSharedProjectDirectory；`go test ./internal/tool ./internal/installer`；决策记录必须说明保留兼容Set方法的代价，不以类型删字段冒充改进。
- 非目标/失败恢复：不删旧字段，不迁移全部调用，不引入公共builder框架；回退本轮调用点即可。

## 验收矩阵

| 场景 | 任务 | 必须观察到的结果 |
| --- | --- | --- |
| skill成功，MCP或末次配置失败 | A1 | 本次实体撤销、已有实体不变、原MCP/配置字节和权限恢复 |
| 后续MCP合并失败或单次合并写失败 | A1 | 原MCP字节/权限不变或恢复，本次skills撤销；不再要求已取消的第二次写 |
| rollback或Commit cleanup失败 | A1/A2 | 主错误与恢复错误透明；CommitCleanupError明确内容已提交且不恢复移除；恢复失败保留原备份 |
| Registry部分失败 | A1/A2 | 普通兼容warning；strict错误且本次成功项补偿；结果准确 |
| 新增成功后移除/ownership失败 | A2 | 新增撤销、删除恢复、配置恢复，已有链接不被撤销/owned |
| CLI/Web fixture | B1 | 同目录问题集合一致、special global路径一致、IO不能报告healthy |
| scope/pinned/source/copy/lock | B2/B3 | 原行为不变，当前用户diff保留 |
| 日常CI与平台 | C1 | workflow有PR/branch push/test/vet与实际runner；远端未跑如实标记 |
| 最终冻结、独立review、基线 | C2 | 已完成：167源文件前后指纹一致，vet与8包race通过；全量8个继承失败、0新增失败，actor终态已收集 |
