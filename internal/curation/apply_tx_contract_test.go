package curation

// apply_tx_contract_test.go 让 curation 的测试假事务（fakeTx / cleanupTx）跑
// txcontract 的固定终态契约，与 installer 真实事务、cmd 的 planTx 适配器
// 使用同一组断言。

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
	"github.com/woyin/skills-manager/internal/txcontract"
)

// contractSkill 是契约 fixture 使用的技能名。
const contractSkill = "contract-skill"

// contractBackupName 是 cleanup 场景中真实创建的备份目录名。
const contractBackupName = ".sm-contract-backup"

// contractFakeAdapter 把 fakeTx 适配为 txcontract.Tx。
type contractFakeAdapter struct{ *fakeTx }

// Added 返回契约实体，映射 curation.AddedEntity 的同名字段。
func (a *contractFakeAdapter) Added() []txcontract.Entity {
	src := a.fakeTx.Added()
	out := make([]txcontract.Entity, 0, len(src))
	for _, e := range src {
		out = append(out, txcontract.Entity{Agent: e.Agent, Skill: e.Skill, Path: e.Path, Symlink: e.Symlink})
	}
	return out
}

// ClassifyCommitError 把假事务的提交错误映射为契约类别。
func (a *contractFakeAdapter) ClassifyCommitError(err error) txcontract.ErrorKind {
	switch {
	case err == nil:
		return txcontract.ErrNone
	case errors.Is(err, ErrFakeTransactionRolledBack):
		return txcontract.ErrAlreadyRolledBack
	}
	var cleanup *fakeCleanupError
	if errors.As(err, &cleanup) {
		return txcontract.ErrCleanup
	}
	return txcontract.ErrContent
}

// ClassifyRollbackError 把假事务的回滚错误映射为契约类别。
func (a *contractFakeAdapter) ClassifyRollbackError(err error) txcontract.ErrorKind {
	switch {
	case err == nil:
		return txcontract.ErrNone
	case errors.Is(err, ErrFakeTransactionCommitted):
		return txcontract.ErrAlreadyCommitted
	}
	return txcontract.ErrOther
}

// contractCleanupAdapter 把 cleanupTx（值嵌入 fakeTx）适配为 txcontract.Tx。
type contractCleanupAdapter struct{ *cleanupTx }

func (a *contractCleanupAdapter) Added() []txcontract.Entity {
	return (&contractFakeAdapter{fakeTx: &a.cleanupTx.fakeTx}).Added()
}

func (a *contractCleanupAdapter) ClassifyCommitError(err error) txcontract.ErrorKind {
	return (&contractFakeAdapter{fakeTx: &a.cleanupTx.fakeTx}).ClassifyCommitError(err)
}

func (a *contractCleanupAdapter) ClassifyRollbackError(err error) txcontract.ErrorKind {
	return (&contractFakeAdapter{fakeTx: &a.cleanupTx.fakeTx}).ClassifyRollbackError(err)
}

// contractDisk 用真实文件系统观察实体、配置与备份目录。
type contractDisk struct {
	config   string
	baseline []byte
	preexist []string
	backup   string
}

func (d *contractDisk) EntityPresent(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// ConfigBytes 返回配置当前字节；不可读时返回 nil。
func (d *contractDisk) ConfigBytes() []byte {
	data, err := os.ReadFile(d.config)
	if err != nil {
		return nil
	}
	return data
}

func (d *contractDisk) ConfigBaseline() []byte { return d.baseline }

func (d *contractDisk) BackupPreserved() bool {
	if d.backup == "" {
		return false
	}
	_, err := os.Lstat(d.backup)
	return err == nil
}

func (d *contractDisk) Preexisting() []string { return d.preexist }

// fakeContractOpen 按场景构造假事务，并返回真实磁盘观察。
// 每个场景使用独立临时项目目录，避免前一场景的状态残留。
func fakeContractOpen(t *testing.T, scenario txcontract.Scenario) (txcontract.Tx, txcontract.Disk) {
	t.Helper()
	env := newTestEnv(t)
	env.mkRegistrySkill(contractSkill)

	entity := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, contractSkill)
	if err := os.MkdirAll(filepath.Dir(entity), 0755); err != nil {
		t.Fatalf("prepare target dir: %v", err)
	}
	if err := os.Symlink(filepath.Join(env.registryDir, "skills", "global", contractSkill), entity); err != nil {
		t.Fatalf("prepare entity: %v", err)
	}

	configPath := filepath.Join(env.projectDir, ".sm.json")
	baseline := []byte("{\n  \"skills\": [\n    \"contract-skill\"\n  ]\n}")
	if err := os.WriteFile(configPath, baseline, 0644); err != nil {
		t.Fatalf("prepare config: %v", err)
	}
	disk := &contractDisk{config: configPath, baseline: baseline}

	added := []AddedEntity{{Agent: tool.Claude.Name, Skill: contractSkill, Path: entity, Symlink: true}}
	switch scenario {
	case txcontract.ExistingEntity:
		// 幂等既有实体：事务不改变文件系统，Added 必须为空。
		disk.preexist = []string{entity}
		return &contractFakeAdapter{fakeTx: &fakeTx{cfg: &project.Config{}, saveDir: env.projectDir}}, disk

	case txcontract.CleanupFails:
		backup := filepath.Join(env.projectDir, contractBackupName)
		if err := os.MkdirAll(backup, 0755); err != nil {
			t.Fatalf("prepare backup dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(backup, "kept.txt"), []byte("kept"), 0644); err != nil {
			t.Fatalf("prepare backup content: %v", err)
		}
		disk.backup = backup
		tx := &fakeTx{cfg: &project.Config{}, saveDir: env.projectDir, added: added, created: []string{entity}}
		return &contractCleanupAdapter{cleanupTx: &cleanupTx{fakeTx: *tx, paths: []string{backup}}}, disk

	default: // Happy、ConfigWriteFails
		tx := &fakeTx{cfg: &project.Config{}, saveDir: env.projectDir, added: added, created: []string{entity}}
		if err := tx.StageProjectConfig(&project.Config{Skills: []string{contractSkill}}); err != nil {
			t.Fatalf("StageProjectConfig: %v", err)
		}
		if scenario == txcontract.ConfigWriteFails {
			// Stage 已快照配置；改成只读后写盘确定性失败（fsutil 按权限位
			// 拒绝，与 root/平台无关），旧字节保持不变。
			if err := os.Chmod(configPath, 0444); err != nil {
				t.Fatalf("chmod config: %v", err)
			}
			t.Cleanup(func() { _ = os.Chmod(configPath, 0644) })
		}
		return &contractFakeAdapter{fakeTx: tx}, disk
	}
}

// TestAdditionTxContractFakeAdapter 固定 curation 假事务的终态契约。
func TestAdditionTxContractFakeAdapter(t *testing.T) {
	txcontract.Run(t, fakeContractOpen)
}
