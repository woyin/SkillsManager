# 测试可靠性第二轮实施计划

状态：**已执行完成。** T1–T7实现并通过固定快照门禁（`go test ./...`、`go vet ./...`、8包race、cmd shuffle count5、Windows全测试编译均exit 0，源码指纹前后一致）。独立review报DEFECT_FOUND 3项，已修复并复验。T4 Windows包列表空行warning已按过滤空行实现。远端CI未触发。权威运行协议仅此文件；run `reliability-MMhxiR`。

## 已确认输入与约束

- 仓库 `/Users/breestealth/Development/SkillsManager`，HEAD `f15816a050f31f7be9c15b3eae00eb44549dcc0a`。不 commit/push、安装依赖、改全局配置、stash/reset。
- 用户基线含 `cmd/lint_test.go`、`cmd/list_test.go`、`cmd/update.go`、`cmd/update_test.go`、`cmd/util_test.go`、`internal/picker/picker_test.go`、`web/handler.go` 与 `.pi/`。外部完整输入快照、diff、索引hash、tracked hash 在 `/tmp/skillsmanager-reliability.MMhxiR/`。重叠文件仅最小增量；保留 update 原删除。
- 当前冻结基线：`go test ./...` exit 1，`go vet ./...` exit 0。精确日志 `baseline-test.log` / `baseline-vet.log`。失败：`TestInstallBareNameFromRegistry`、`TestPlanCommandApplyRemovesOwnedLinkInstall`、`TestPlanCommandApplyBootstrapWithSkillTarget`、`TestPlanCommandApplyBootstrapWithProfileTarget`、`TestPlanCommandCheckPassesWhenBaselineSatisfied`、`TestWriteProjectStatusShowsProjectInstallAndOrphan`、`TestWriteProjectStatusReportsBrokenSymlink`，以及 `internal/aivo.TestGetStats`（5.01s 后 nil）。宿主检测目标与 fixture Claude 不同有日志证据；不得以改变产品检测默认值修测试。
- 同一源码冻结基线补测八包race：`go test -race ./internal/installer ./internal/curation ./internal/updater ./internal/health ./internal/lifecycle ./internal/project ./internal/lockfile ./internal/fsutil -count=1` exit 0，证据 `baseline-race.log`。这是当前基线，不继承远端标签。
- Windows 当前编译：`GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -exec=/usr/bin/true ./...` exit 1，`internal/fsutil/fsutil_atomic_test.go:202:10: undefined: syscall.Umask`。`-exec=true` 仅证明编译，不运行 Windows 测试。
- 继承 ADR0017 删除边界、ADR0018 渐进提取、ADR0023 仅拥有的 Link Install 可删除、ADR0014 tracking/pinned 行为与 CONTEXT 中 Detected Agents 契约。本轮不修订产品契约。
- 上轮 `/tmp/skillsmanager-architecture.40Z0ed` 仅作背景；旧状态标签不作当前通过证据。

## 调度、所有权与冻结协议

使用 Fabric persistent `agents.create`，固定 `runner: pi`、`transport: herdr-core`。至多三个开发actor和一个独立只读review actor。不用普通shell并行冒充swarm。mesh topic `team.reliability-MMhxiR`；任务键 `runs/reliability-MMhxiR/tasks/<ID>`。

每任务保留 `title/status/owner/dependencies/progress/result`。任务初始 `ready` 或 `blocked`。领取必须先读依赖全部为 completed，再以观察到的 `ifVersion` CAS ready→in_progress，owner为actor id；失败立即停止。后续状态也使用最新version CAS。只有全部依赖完成才解除后继blocked。开发完成为 `implemented` 子状态记录在result；任务 `completed` 表示本任务要求的冻结定向验证也通过。协调者可领取验证任务并将门禁证据写回；不把代码完成等同验收。

| actor | 独占文件范围 | 不得写入 |
| --- | --- | --- |
| A `7ed94861c23b453ea78b0d1df2666d72` | T1 `internal/aivo/**`；T5 `cmd/update.go`、新增 `cmd/update_discovery_test.go`、`internal/updater/discovery*.go`、`internal/updater/origin_metadata.go`、`cmd/skill_origin.go` 的legacy Origin读取兼容适配；T4 `.github/workflows/ci.yml`、`internal/fsutil/fsutil_test.go` 的POSIX权限断言、`internal/fsutil/fsutil_atomic*_test.go`、`internal/sourcecache/sourcecache_test.go`、`internal/registry/registry_test.go` 的跨平台fixture | cmd共享helper、update已有测试、release.yml |
| B `3c5297040857483db1defeb1b5558b67` | T2 cmd共享测试helper与现有cmd测试（除A/C新增测试）；窄检测seam及 `cmd/install.go`、`cmd/plan.go`、`cmd/status.go`、`cmd/list.go` 的最小接入；T6 `cmd/rm.go`及rm测试 | `cmd/update.go`、curation/installer、A/C新增测试 |
| C `6cb2445cc939414790f0852003dbc73e` | T3/T7 `internal/curation/**`、`internal/installer/transaction.go`、`internal/installer/transaction*_test.go`、若必要的小型 `internal/txcontract/**`、新增 `cmd/plan_addition_contract_test.go` | cmd共享helper及其他cmd生产代码 |
| 协调者 | 本计划、`docs/planning/test-reliability-acceptance.md`、外部日志/快照/变异副本 | actor已领取的源码 |
| reviewer `47030c2c2615425690a21f2c6571f915` | 只读所有输入、输出证据 | 所有项目文件 |

开发A/B/C可以同时写各自文件。任何测试、vet、跨编译、源码hash均要求所有开发者显式待命。协调者串行执行测试，测试日志独占外部路径。不允许开发者重复全量、不直接加 t.Parallel。T3 的新增cmd测试不得依赖尚未固定的B helper；自己设置明确Agent或用固定协议后再接入。T4编译等待所有相关源码冻结。Curation测试重组在T3覆盖固定后执行。

## T1：aivo确定性（P1，A）

根因证据：`internal/aivo/aivo_test.go` 的 Detect/ListKeys/GetStats/GetActiveKey 直接调用真实 aivo；当前 `TestGetStats` 已失败。`aivo.go` 中 `ListKeys` 调用 `keys --ping --json`，含外部凭证/网络行为。

目标契约：默认测试全用固定payload和窄执行seam或跨平台Go helper，不调用宿主aivo、不读取真实keys；解析结果字段逐项断言。保持公开 Detect/ListKeys/GetStats/GetActiveKey 与 nil兼容。非法JSON、非零退出、超时具有可断言结果。真实检测若保留，仅明确opt-in integration，默认不执行。

非目标：不加公共配置，不为测试引入通用执行框架，不改变5秒生产超时，不悄悄吞新错误。

接口固定：私有 `parseKeys([]byte) ([]Key,error)`、`parseStats([]byte) (*Stats,error)`、`parseVersion(string) string`；`lookPath=exec.LookPath`、`runAivo=runCmd` 为最小测试seam，cleanup恢复。生产公开函数仍调用现有runCmd。错误仍在原兼容层转换nil；若内部增加error-returning方法必须有真实调用收益，否则不加。

失败处理：执行或解析失败按既有nil契约；runCmd自身超时/非零退出不得测试为成功。owner `internal/aivo/**`；依赖 PRECHECK。

验收：新增解析/参数/失败/超时测试（最终名字在实施结果登记），`go test ./internal/aivo -count=1` exit 0。无真实aivo默认路径和个人数据日志。回退：只撤销本轮aivo差异，不碰用户基线。

## T2：CLI环境与Cobra状态隔离（P1，B）

根因证据：`cmd/plan_tx_test.go:13` withFakeBinary写 `#!/bin/sh` 并保留宿主PATH；baseline 7个cmd失败显示Claude fixture与检测结果不一致。`resetPlanFlags`、命令 Out/Err/Args、pflag Changed 状态需按真实读写点验证泄漏。

目标契约：测试明确检测Agent集合；隔离HOME、必要PATH与全局状态，但git可运行。公共默认Detected Agents行为不变。Cobra flags Value/Changed、Out/Err/Args和包globals由cleanup恢复；before/after fixture证明前序测试改变后后序观察被正确复原。

非目标：不更换Cobra、不引入生产通用依赖容器、不用清空PATH使git消失、不t.Parallel。

接口固定：B独占 `cmd/agent_detect.go` 的 `detectAgents func() []tool.Tool`，默认仍调用 `tool.DetectInstalled(tool.AllTools())`，仅最小替换install两处、plan两处、status/list各一处；原fallback留在原调用点。`cmd/testenv_test.go` 的isolateAgents/isolateCmdGlobals/snapshotFlags/isolateEnv在赋值前快照，cleanup恢复。pflag切片用SliceValue.GetSlice/Replace，普通Value用Set，均恢复Changed；Cobra Args无公开getter时允许仅测试内无unsafe的窄反射读取字符串，Out/Err/In须保留原nil继承状态。HOME恢复后刷新home缓存。`cmd/fakeproc_test.go` 用TestMain+专用env且basename匹配早分派，复制当前Go测试二进制为git/aivo等，生产argv不加-test.run。B负责 `cmd/plan_tx_test.go`、`cmd/update_test.go` 三处及 `cmd/update_bench_test.go` 的全部被执行sh fake迁移；A不写这些文件。状态测试可用isolatePath显式依赖清单只保留git等必需程序，使宿主aivo不可见；Windows复制二进制，POSIX可用链接。只用于需要隔离的测试，不全包清PATH；保留真实git依赖。A/C不得改helper。

已确认的顺序泄漏是 `setupPlanCommand` 不恢复ProfilesDir，以及resetPlanFlags写零值；尚未证明的pflag Changed后果只作合同覆盖，不写成已复现缺陷。F2只新增 `cmd/cli_state_isolation_test.go`，不新增未定义helper或生产seam。固定red测试为 `TestCLIStateIsolationRestoresProfilesDir`（真实setupPlanCommand子测试后，后序观察原ProfilesDir）、`TestResetPlanFlagsRestoresPreviousValues`（原reset前后比对）；fixture自身cleanup不污染别的用例。不得用内联镜像实现代替真实前后观察。

失败处理：未知Agent或fake模式立即测试失败；cleanup恢复调用前值，避免固定写默认值。原产品无Agent fallback不变。依赖 PRECHECK；T2分fixture-red与实现-green两个冻结窗口。

测试/预期：先冻结 `go test ./cmd -run 'TestCLIStateIsolationRestoresProfilesDir|TestResetPlanFlagsRestoresPreviousValues' -count=1`（before阶段预期因ProfilesDir/reset快照断言失败，须实测）；实现后同命令exit 0；`go test ./cmd -count=1` exit 0；最终固定源码快照 `go test ./cmd -shuffle=20261117 -count=5` exit 0。命名若因现有冲突调整，先更新计划精确名字。回退：仅反向本轮helper/seam与调用替换，保留用户cmd测试基线。

## T3：事务一致契约（P1，C）

根因证据：`internal/curation/transaction.go` AdditionTx由curation fakeTx/cleanupTx与 `cmd/plan.go` planTx两类实现消费；当前测试各自维护终态语义，尚无共享conformance。此处是证据缺口，不先断言生产事务有错。`internal/installer/transaction.go` Result直接返回tx.result内部指针（可复现可变别名）。

目标契约：小shared contract让fake与真实cmd adapter/installer运行同一组约束：重复Commit/Rollback，已有实体不owned，config失败补偿，已提交cleanup失败，返回状态与磁盘一致。保持AdditionTx原接口、CLI schema与installer终态错误。Result可用最小copy隔离外部修改，明确它是尝试结果，不能误当已提交磁盘事实。

非目标：不增加重复Committed/Outcome生产接口，不导出大测试框架，不因测试造循环依赖。

接口固定：`internal/txcontract` 仅为测试支持，禁止导入curation、installer或cmd，只可依赖stdlib与project；定义本地Entity与最小5方法契约（Added/StageProjectConfig/Committed/Commit/Rollback）或等价fixture闭包。curation的fake adapter与cmd的真实planTx adapter各放在自己测试文件中，映射Entity，无循环依赖；installer包内fixture可调用同一suite。每个fixture提供真实磁盘观察和config/cleanup故障注入（外部filesystem可复现时优先，不新增生产故障框架）。C独占 `cmd/plan_addition_contract_test.go`，不得改plan.go或B helper。Result返回最小copy且复制Skills/MCP等可变切片，仍表示尝试安装结果，不新增Committed/Outcome接口。

终态预期固定：

| 操作 | 结果 | 磁盘与返回状态 |
| --- | --- | --- |
| Commit后重复Commit | 返回首次Commit结果（含cleanup错误） | 已提交内容不变，Committed=true |
| Commit后Rollback | 返回已提交错误（真实installer为ErrTransactionCommitted） | 已提交内容不删，Committed=true |
| Rollback后重复Rollback | 返回首次Rollback结果 | 不再次撤销，Committed=false |
| Rollback后Commit | 返回已回滚错误 | 无新写入，Committed=false |
| config内容写失败 | 返回内容错误并内部补偿 | 新实体移除，旧文件恢复，Committed=false |
| cleanup失败 | 返回可识别cleanup错误并保留路径诊断 | 新内容/删除结果保留，Committed=true |
| 既有幂等实体 | Added不包含它 | 不登记owned，回滚不得删除 |

已证实fakeTx.Rollback在Commit后仍删链接，与真实installer相反；C须按上述表最小修正fakeTx/cleanupTx并让同suite验证。其他未经复现的生产缺陷不预先断言。

失败处理：Commit内容失败=未提交且补偿；cleanup失败=已提交、保留新内容且报告备份；RollBack已提交返回原有ErrTransactionCommitted。owner如表；依赖 PRECHECK。C不改plan.go。

测试/预期：仅协调者在A/B/C全部明确待命窗口运行 `go test ./internal/curation ./internal/installer ./cmd -run 'Contract|Transaction|Apply|Bootstrap|Result' -count=1` exit 0；T7改名后以curation全包命令为准，不依赖旧FixA2正则。具体新增test名登记。回退：本轮contract与Result copy差异单独撤销，保留既有错误分支。

## T4：平台契约与CI（P1，A）

根因证据：Windows当前测试编译失败于Umask；sourcecache/registry/cmd fixtures有sh依赖。CI当前Linux full+vet但race仅4包；macOS代表包缺curation/lifecycle。旧远端Linux成功、macOS cancelled都不代表新输入已验收。

目标契约：只把真正POSIX语义的权限/umask测试移到带适当build tags的文件；跨平台行为测试继续编译。执行fake改为Go helper。CI保留release工作流，Linux full+vet，race统一8包installer/curation/updater/health/lifecycle/project/lockfile/fsutil；macOS覆盖这8个filesystem相关包；Windows编译所有测试并运行明确平台通用的测试，避免假称全量Windows运行通过。

非目标：不触发远端workflow、不改release、不装yaml工具，不在Windows套用POSIX权限语义。

接口：仅test build tags、helper与CI矩阵/步骤。仅把Umask与权限位断言移入 `!windows` 测试文件；fsutil复制测试中的exclude-dir断言留通用文件，其余通用测试不整文件打tag。Windows CI固定三步：`go vet ./...`；PowerShell用 `go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...` 筛出有测试包后逐包 `go test -c -o <temp>/<ordinal>.test.exe <pkg>`（每次检查exit、输出不落仓库）；然后 `go test ./internal/aivo ./internal/wellknown ./internal/concurrency`，另跑 `go test ./internal/tool -run '^(TestAllTools|TestDefaultTools|TestToolByName|TestToolsByNames|TestDetectInstalledEmpty)$'` 与 `go test ./internal/updater -run 'TestNearestGitRepo|TestPathInside'`。tool名单须与已有test名核对；不把无匹配测试计为有效运行。本机Windows精确全测试编译命令固定为 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -exec=/usr/bin/true ./...`。本机可执行同一通用子集以证明选择器有测试；Windows运行本身仍remote未验证。

失败处理：缺少平台支持的语义明确跳过或tag，不用大范围skip隐藏普通失败。owner如表；依赖T1/T2/T3/T5/T6/T7源码阶段完成后统一编译，CI设置阶段可在写窗口进行。

测试/预期：受影响aivo/fsutil/sourcecache/registry/cmd定向tests exit 0；`GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -exec=/usr/bin/true ./...` exit 0。静态CI检查用已有可用解析器或结构断言，解析器不可用需报告限度。remote全标未跑。回退：反向本轮CI与fixture/tag变更，不动release。

## T5：updater目标发现与分类（P2，A）

根因证据：`cmd/update.go:751–998` collectInstalledUpdateTargets→collectDir/classify/resolveContentRoot/classifyOrigin与命令执行/交互同文件；已有用户删除69行必须保留。当前功能不先假设缺陷。

目标契约：只提取这条内聚发现路径到 `internal/updater`，返回结构化git/origin/WellKnown目标与孤儿/跳过诊断。项目/全局scope、tracking/pinned Origin、cache/nearestgit上界、copy查Registry原件、link受管路径、去重、孤儿与WellKnown消除孤儿规则不变。cmd仍掌管flag、scope选择、输出、交互、copy原位刷新与旧git pull执行。

非目标：不机械按行拆、不改退出码/文案、不改原位刷新、不搬所有cmd工具函数、不新增网络调用。

接口：输入显式RegistryDir/DataDir/ProjectDir/Names和scope（不让internal读取cmd globals），输出结构化DiscoveryResult。legacy `.sm-origin.json` 与registry.SkillOrigin是两种现有schema；发现返回保留UpdatedAt的OriginMetadata，cmd仍调用registrySchemaOrigin取得registry视图，不合并或迁移schema。接口固定为 `DiscoverInstalledTargets(DiscoverOptions) (DiscoveredTargets, []Diagnostic)`；DiscoverOptions嵌入updater.Options并显式包含ProjectDir/Names/IncludeProject/IncludeGlobal。输出含GitRepos/Origins/WellKnownSkills/Orphans；Diagnostic为Op/Path/Err。legacy Origin读取集中在updater，cmd保留含UpdatedAt的原schema及read兼容适配，不复制JSON解析。PathInside/NearestGitRepo在updater单源实现，cmd保留兼容委托供cache/status调用。updateSpecificSkills按名路径语义不同，本轮不合并。cmd包装保留旧测试/调用接口并作最薄转换；不留下同一分类算法的双份实现。

失败处理：保留现有扫描不可读目录/lockfile失败的跳过行为，结构化诊断可以记录原因但cmd默认输出保持原样。依赖T1后由A串行；不得与cmd定向验收并发。owner如表。

测试/预期：提取前后同一锁定fixture覆盖上述所有分类；`go test ./internal/updater ./cmd -run 'Discover|Collect|Update|NearestGit|PathInside' -count=1` exit 0。回退：仅本轮提取与wrapper差异反向，用户原删除仍保持。

## T6：删除流程读取索引一次（P2，B）

根因证据：`cmd/rm.go` listRegistryReferences与cleanupInstallsByName分别调用knownProjectPaths，force移除单流程重复读DB并可能重复warning。

目标契约：命令生命周期读取一次，将结果显式传给引用检查与强制清理；不跨命令缓存。索引不可用仍warning、保留删除策略及ADR0017边界。

非目标：不增加全局缓存，不修改DB schema或删除安全策略。

接口固定：仅在 `removeRegistryOriginal` 生命周期获取 `known := knownProjectPaths()` 一次；`listRegistryReferences(skillName,skillPath string,knownProjects []string)`、`cleanupInstallsByName(skillName string,knownProjects []string)`、`reportInaccessibleProjects(skillName string,knownProjects []string)`显式复用。knownProjectPaths签名和失败warning不变。不存在原件的清理分支仍只读一次；不可访问历史项目报告仅Stat这份路径，不再openDB。`removeAll`和`removeSkillLegacy`未走该路径，本轮不改。损坏DB fixture固定 `TestRmForceReadsProjectIndexOnce` 断言原来重复warning修为恰好一次；`TestRmForceReusesIndexForCleanupAndReport` 验证可访问项目清理与历史项目报告同份索引。

失败处理：读取失败按既有warning/当前project和global扫描策略；不静默忽略。owner如表；依赖T2 helper固定后B串行。

测试/预期：counting seam或损坏DB fixture断言一次warning和引用检查/删除共用结果；`go test ./cmd -run 'Rm|RemoveRegistry|KnownProject' -count=1` exit 0。回退：仅本轮显式参数及fixture，保留原删除边界。

## T7：行为导向测试组织与定向变异（P2，C）

根因证据：`internal/curation/apply_tx_test.go`、`apply_tx_more_test.go`、`fix_a2_test.go` 以实施批次命名，交叉分布planning/ownership/transaction/bootstrap断言。

目标契约：在T3固定后按行为职责重组并改FixA2名字；仅合并输入、故障时点、观察都相同的fixture。保留所有独立错误路径，不追求减少测试数。

非目标：不宣称覆盖率或性能提高，不安装mutation工具，不改当前源码再还原。

接口：仅测试文件与helper组织。依赖T3代码与定向验证；owner C的curation tests。协调者用固定源码外部副本做两次单点变异，独占副本，不与源码写并发读取。

失败处理：原覆盖断言不可丢失；变异存活需判断是否等价或缺覆盖，非等价缺口在本范围补强。实际实施错误不以重命名掩盖。

测试/预期：`go test ./internal/curation -count=1` exit 0；仓库外副本删除Commit失败补偿/忽略ownership校验两个非等价变异，各定向命令exit非零且因相应语义断言失败（不能是编译失败）。回退：反向本轮测试移动，保留T3契约覆盖。

## 有界执行批次与最终验收

1. D1/D2/D3只读调查，补入接口和证据；PRECHECK由独立reviewer只读执行。blocking修完复检受影响项一次；不反复全文改写计划。
2. B先写T2顺序泄漏fixture，其余actor待命。冻结执行red，记录精确失败。随后T1/T2/T3在A/B/C所有权范围并行实施。完成后全员待命，协调者串行定向验证（失败只修真实原因）。
3. A做T5、B做T6、C在T3通过后做T7；A随后T4，其他开发待命或各自已完成。再次全冻结串行受影响检查。
4. 独立reviewer审实际diff、基线保留、契约和测试。修具体缺陷后仅受影响定向验证；有新相关修改才重复测试。
5. 最终freeze：保存源码/fixture/CI指纹，明确工作区输入而非HEAD/index或待提交快照。依次各一次 `go test ./... -count=1`、`go vet ./...`、`go test -race ./internal/installer ./internal/curation ./internal/updater ./internal/health ./internal/lifecycle ./internal/project ./internal/lockfile ./internal/fsutil -count=1`、`go test ./cmd -shuffle=20261117 -count=5`、Windows全测试编译。所有最终门禁期望exit 0且无新增失败；若失败要区分初始基线与当前新增，不能笼统归因环境。每项记录命令/exit/log；验证前后同一源码指纹。文档更新不冒充源码变化。
6. 两个mutation与CI静态检查可以在此前同一freeze完成，最终证据标明其输入绑定。独立验收actor核对各结果和限度，不重复全套。
7. 更新验收文档开头最终状态并清理旧待验收段。保留baseline与diff/hash、所有actor终态及mesh历史。停止本轮自建actor，HEAD/index不变。监工t6c收到文档路径、测试退出、remote未跑与实际局限。
