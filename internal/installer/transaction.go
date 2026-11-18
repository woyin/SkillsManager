package installer

// transaction.go owns the deferred install transaction: BeginInstall and
// BeginRegistryInstall stage skills (and MCP definitions in memory), while
// Commit performs the single ordered write pass (MCP → project config →
// cleanup) and Rollback compensates only what this transaction changed.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/woyin/skills-manager/internal/fsutil"
	"github.com/woyin/skills-manager/internal/project"
)

// ErrTransactionCommitted 表示事务已提交：内容不可再撤销。
var ErrTransactionCommitted = errors.New("install transaction already committed")

// ErrTransactionRolledBack 表示事务已回滚：不能再提交。
var ErrTransactionRolledBack = errors.New("install transaction already rolled back")

// CommitCleanupError 表示内容已提交但清理未完成。Paths 列出仍保留的备份
// 目录，调用方必须原样报告，且不得据此恢复已提交内容（例如已移除的链接）。
type CommitCleanupError struct {
	Paths []string
	Err   error
}

func (e *CommitCleanupError) Error() string {
	if e == nil {
		return "install content committed; cleanup failed"
	}
	if len(e.Paths) == 0 {
		return fmt.Sprintf("install content committed; cleaning up placement backups failed: %v", e.Err)
	}
	return fmt.Sprintf("install content committed; cleaning up placement backups failed: %v; backups preserved at: %v", e.Err, e.Paths)
}

// Unwrap 支持 errors.Is/As 追到底层清理错误。
func (e *CommitCleanupError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// PlacementChange 描述事务实际改变的一个落地实体（幂等既有实体不在其中）。
// Agent 是 tool.Tool.Name；共享目录按代理展开为多行，Destination 相同。
type PlacementChange struct {
	Agent       string
	Skill       string
	Source      string
	Destination string
	ActualMode  PlacementMode
	Fallback    bool
}

type transactionState uint8

const (
	txOpen transactionState = iota
	txCommitted
	txRolledBack
)

// placementRecord 保留每条落地记录及其代理名，供 ChangedPlacements 使用。
type placementRecord struct {
	Agents      []string
	Skill       string
	Source      string
	Destination string
	Result      *PlacementResult
}

// fileSnapshot 保存项目文件实际写入目标（symlink 已解析到 referent）在写入前的
// 状态，供补偿恢复。它是“写哪里、备份哪里”的唯一来源：
//   - 目标是悬空 symlink → existed=false，补偿只删除本次创建的 referent；
//   - 目标原本存在 → 记录 referent 字节与 mode，补偿写回 referent。
//
// 两种情形都不会删除或替换 symlink 本身。
type fileSnapshot struct {
	path    string // 实际写入目标（referent）
	display string // 调用方传入的原始路径，用于错误文案
	data    []byte
	mode    os.FileMode
	existed bool
}

func snapshotFile(path string) (*fileSnapshot, error) {
	target, err := fsutil.ResolveWriteTarget(path)
	if err != nil {
		return nil, fmt.Errorf("inspecting %s: %w", path, err)
	}
	snapshot := &fileSnapshot{path: target.Path, display: path, mode: target.Mode, existed: target.Exists}
	if !target.Exists {
		return snapshot, nil
	}
	// 写前必须能读到原内容，否则无法承诺可恢复：这时明确失败而非部分写入。
	data, readErr := os.ReadFile(target.Path)
	if readErr != nil {
		return nil, fmt.Errorf("reading %s: %w", target.Path, readErr)
	}
	snapshot.data = data
	return snapshot, nil
}

func (s *fileSnapshot) restore() error {
	if s == nil {
		return nil
	}
	if !s.existed {
		// 只删除本次创建的 referent，原 symlink（悬空或指向它）保持不变。
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", s.path, err)
		}
		return nil
	}
	if err := fsutil.WriteFileAtomic(s.path, s.data, s.mode); err != nil {
		return fmt.Errorf("restoring %s: %w", s.path, err)
	}
	return nil
}

// InstallTransaction 是一次延迟安装事务。Begin* 之后调用方可以完成自己的
// 状态变更，再调用 Commit 或 Rollback；两者都是终态操作。
type InstallTransaction struct {
	inst       *Installer
	projectDir string
	result     *InstallResult
	placements []*PlacementResult
	records    []placementRecord

	mcpPath     string
	mcpMerged   *mcpConfig
	mcpSnapshot *fileSnapshot
	mcpWritten  bool

	configPath     string
	config         *project.Config
	configData     []byte
	configSnapshot *fileSnapshot
	configWritten  bool

	state       transactionState
	commitErr   error
	rollbackErr error
}

// BeginInstall 完整预检后落地 skills，并把 MCP 合并保留在内存中。
// 任一失败返回 (nil, error) 且恢复本次已落地实体。不写 .mcp.json 与 .sm.json。
func (inst *Installer) BeginInstall(projectDir, profileName string, extraSkills, extraMCP []string) (*InstallTransaction, error) {
	if profileName == "" && len(extraSkills) == 0 && len(extraMCP) == 0 {
		return nil, fmt.Errorf("nothing to install: specify --profile, or add skills/mcp to .sm.json")
	}
	allSkills, allMCP, err := inst.gatherAndPreflight(projectDir, profileName, extraSkills, extraMCP)
	if err != nil {
		return nil, err
	}

	tx := &InstallTransaction{
		inst:       inst,
		projectDir: projectDir,
		result:     &InstallResult{},
		mcpPath:    filepath.Join(projectDir, ".mcp.json"),
	}
	if len(allMCP) > 0 {
		merged, _, _, loadErr := loadMCPConfig(tx.mcpPath)
		if loadErr != nil {
			return nil, loadErr
		}
		snapshot, snapErr := snapshotFile(tx.mcpPath)
		if snapErr != nil {
			return nil, fmt.Errorf("preparing %s: %w", tx.mcpPath, snapErr)
		}
		tx.mcpMerged = merged
		tx.mcpSnapshot = snapshot
	}

	for _, skillName := range allSkills {
		links, changed, records, skillErr := inst.installSkillWithPlacements(skillName)
		if skillErr != nil {
			rollbackErr := rollbackPlacements(tx.placements)
			return nil, errors.Join(
				fmt.Errorf("profile install aborted: skill %q failed: %w", skillName, skillErr),
				rollbackErr,
			)
		}
		tx.result.Skills = append(tx.result.Skills, links...)
		tx.placements = append(tx.placements, changed...)
		tx.records = append(tx.records, records...)
	}

	for _, mcpName := range allMCP {
		if mergeErr := inst.mergeRegistryMCP(tx.mcpMerged, mcpName, tx.mcpPath); mergeErr != nil {
			rollbackErr := rollbackPlacements(tx.placements)
			return nil, errors.Join(
				fmt.Errorf("profile install aborted: MCP %q failed: %w", mcpName, mergeErr),
				rollbackErr,
			)
		}
		tx.result.MCP = append(tx.result.MCP, mcpName)
	}
	return tx, nil
}

// BeginRegistryInstall 先对全部名字做解析预检（零写入），再逐个落地 skills。
// 任一失败返回 (nil, error) 且恢复本次已落地实体。
func (inst *Installer) BeginRegistryInstall(names []string, category string) (*InstallTransaction, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("registry install requires at least one skill name")
	}
	type resolvedSkill struct {
		name string
		path string
		cat  string
	}
	resolved := make([]resolvedSkill, 0, len(names))
	for _, name := range names {
		path, cat, err := inst.resolveRegistrySkill(name, category)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, resolvedSkill{name: name, path: path, cat: cat})
	}

	tx := &InstallTransaction{inst: inst, projectDir: inst.projectDir, result: &InstallResult{}}
	for _, target := range resolved {
		links, changed, records, placeErr := inst.createSymlinksWithPlacements(target.name, target.path, target.cat)
		if placeErr != nil {
			rollbackErr := rollbackPlacements(tx.placements)
			return nil, errors.Join(
				fmt.Errorf("registry install aborted: skill %q failed: %w", target.name, placeErr),
				rollbackErr,
			)
		}
		tx.result.Skills = append(tx.result.Skills, links...)
		tx.placements = append(tx.placements, changed...)
		tx.records = append(tx.records, records...)
	}
	return tx, nil
}

// Result 返回本次“尝试”的 Applied 实体结果（含幂等项，兼容既有语义）。
// 失败路径上的数据仅用于诊断：它不代表最终落盘状态，绝不能作为 ownership
// 记录的依据（ownership 必须取 Commit 前的 ChangedPlacements）。
//
// 返回的是只读副本：调用方修改它不会影响事务内部状态，事务后续写入也不会
// 改变已返回的结果。nil 切片性保持原样（原本为 nil 的切片仍为 nil）。
func (tx *InstallTransaction) Result() *InstallResult {
	if tx == nil || tx.result == nil {
		return &InstallResult{}
	}
	return tx.result.clone()
}

// clone 返回结果的独立副本，并复制 Skills/MCP/Failures 切片，使返回值与
// 事务内部记录不再共享底层数组。slices.Clone 保持 nil 与空非 nil 的区别。
func (r *InstallResult) clone() *InstallResult {
	if r == nil {
		return &InstallResult{}
	}
	return &InstallResult{
		Skills:   slices.Clone(r.Skills),
		MCP:      slices.Clone(r.MCP),
		Failures: slices.Clone(r.Failures),
	}
}

// ChangedPlacements 只返回本次真正改变文件系统的实体，供 ownership 记录。
// 它描述“尝试变更”，在 Rollback/失败后数据仍保留用于诊断；此类时刻不得
// 用它登记 ownership，只有 Commit 成功前的调用才具备该语义。
func (tx *InstallTransaction) ChangedPlacements() []PlacementChange {
	if tx == nil {
		return nil
	}
	changes := make([]PlacementChange, 0, len(tx.records))
	for _, record := range tx.records {
		if record.Result == nil || !record.Result.Changed {
			continue
		}
		for _, agent := range record.Agents {
			changes = append(changes, PlacementChange{
				Agent:       agent,
				Skill:       record.Skill,
				Source:      record.Source,
				Destination: record.Destination,
				ActualMode:  record.Result.ActualMode,
				Fallback:    record.Result.Fallback,
			})
		}
	}
	return changes
}

// ChangedSkills 返回去重后的本次 changed 技能名（仅供参考/输出）。
func (tx *InstallTransaction) ChangedSkills() []string {
	if tx == nil {
		return nil
	}
	seen := make(map[string]bool)
	names := make([]string, 0, len(tx.records))
	for _, change := range tx.ChangedPlacements() {
		if seen[change.Skill] {
			continue
		}
		seen[change.Skill] = true
		names = append(names, change.Skill)
	}
	return names
}

// StageProjectConfig 暂存最终项目配置（含调用方合并的 curation 元数据）。
// 只记录写入前的字节快照，不解析旧文件；重复调用以最后一次为准。
// 未调用时 Commit 不写 .sm.json。
func (tx *InstallTransaction) StageProjectConfig(config *project.Config) error {
	if tx == nil {
		return fmt.Errorf("staging project config: nil transaction")
	}
	switch tx.state {
	case txCommitted:
		return ErrTransactionCommitted
	case txRolledBack:
		return ErrTransactionRolledBack
	}
	if config == nil {
		return fmt.Errorf("staging project config: nil config")
	}
	if tx.configPath == "" {
		tx.configPath = filepath.Join(tx.projectDir, ".sm.json")
	}
	if tx.configSnapshot == nil {
		snapshot, snapErr := snapshotFile(tx.configPath)
		if snapErr != nil {
			return fmt.Errorf("staging project config: %w", snapErr)
		}
		tx.configSnapshot = snapshot
	}
	data, marshalErr := json.MarshalIndent(config, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("encoding project config: %w", marshalErr)
	}
	tx.config = config
	tx.configData = data
	return nil
}

// Commit 按固定顺序写盘：.mcp.json → .sm.json → 释放 placement 备份。
// 前两步失败会补偿本次实体与已写文件；第三步失败表示内容已提交、仅清理未完成。
func (tx *InstallTransaction) Commit() error {
	if tx == nil {
		return fmt.Errorf("committing install transaction: nil transaction")
	}
	switch tx.state {
	case txCommitted:
		return tx.commitErr
	case txRolledBack:
		return ErrTransactionRolledBack
	}

	if tx.mcpMerged != nil {
		data, marshalErr := json.MarshalIndent(tx.mcpMerged, "", "  ")
		if marshalErr != nil {
			return tx.compensate(fmt.Errorf("encoding %s: %w", tx.mcpPath, marshalErr))
		}
		if writeErr := fsutil.WriteFileAtomic(tx.mcpPath, data, 0644); writeErr != nil {
			return tx.compensate(fmt.Errorf("writing %s: %w", tx.mcpPath, writeErr))
		}
		tx.mcpWritten = true
	}

	if tx.config != nil {
		if writeErr := fsutil.WriteFileAtomic(tx.configPath, tx.configData, 0644); writeErr != nil {
			return tx.compensate(fmt.Errorf("writing %s: %w", tx.configPath, writeErr))
		}
		tx.configWritten = true
	}

	paths, cleanupErr := commitPlacements(tx.placements)
	tx.state = txCommitted
	if cleanupErr != nil {
		tx.commitErr = &CommitCleanupError{Paths: paths, Err: cleanupErr}
		return tx.commitErr
	}
	return nil
}

// Rollback 逆序恢复本次 changed placements，并恢复已写入的项目文件。
// 幂等：已回滚时返回首次回滚结果；已提交时返回 ErrTransactionCommitted。
func (tx *InstallTransaction) Rollback() error {
	if tx == nil {
		return nil
	}
	switch tx.state {
	case txCommitted:
		return ErrTransactionCommitted
	case txRolledBack:
		return tx.rollbackErr
	}
	rollbackErr := tx.restoreAll()
	tx.state = txRolledBack
	tx.rollbackErr = rollbackErr
	return rollbackErr
}

// compensate 在 Commit 的写盘步骤失败后恢复本次实体与已写文件。
func (tx *InstallTransaction) compensate(cause error) error {
	rollbackErr := tx.restoreAll()
	tx.state = txRolledBack
	tx.rollbackErr = rollbackErr
	return errors.Join(cause, rollbackErr)
}

// restoreAll 按 Commit 写盘顺序（MCP → config → placements）的严格逆序补偿：
// 先恢复 .sm.json，再恢复 .mcp.json，最后撤销本次落地实体。
func (tx *InstallTransaction) restoreAll() error {
	var errs []error
	if tx.configWritten {
		if err := tx.configSnapshot.restore(); err != nil {
			errs = append(errs, fmt.Errorf("restoring %s: %w", tx.configPath, err))
		}
	}
	if tx.mcpWritten {
		if err := tx.mcpSnapshot.restore(); err != nil {
			errs = append(errs, fmt.Errorf("restoring %s: %w", tx.mcpPath, err))
		}
	}
	if err := rollbackPlacements(tx.placements); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
