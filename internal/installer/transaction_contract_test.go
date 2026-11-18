package installer

// transaction_contract_test.go 让真实 InstallTransaction 跑 txcontract 的固定
// 终态契约（与 curation 假事务、cmd 的 planTx 适配器同一组断言）。

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/txcontract"
)

// contractSkillName 是契约 fixture 使用的 registry 技能名。
const contractSkillName = "cf-skill"

// installTxAdapter 把 *InstallTransaction 适配为 txcontract.Tx。
type installTxAdapter struct{ tx *InstallTransaction }

func (a *installTxAdapter) Added() []txcontract.Entity {
	changes := a.tx.ChangedPlacements()
	out := make([]txcontract.Entity, 0, len(changes))
	for _, c := range changes {
		out = append(out, txcontract.Entity{
			Agent:   c.Agent,
			Skill:   c.Skill,
			Path:    c.Destination,
			Symlink: c.ActualMode == SymlinkMode,
		})
	}
	return out
}

func (a *installTxAdapter) StageProjectConfig(cfg *project.Config) error {
	return a.tx.StageProjectConfig(cfg)
}

func (a *installTxAdapter) Committed() bool { return a.tx.state == txCommitted }

func (a *installTxAdapter) Commit() error { return a.tx.Commit() }

func (a *installTxAdapter) Rollback() error { return a.tx.Rollback() }

func (a *installTxAdapter) ClassifyCommitError(err error) txcontract.ErrorKind {
	switch {
	case err == nil:
		return txcontract.ErrNone
	case errors.Is(err, ErrTransactionRolledBack):
		return txcontract.ErrAlreadyRolledBack
	}
	var cleanup *CommitCleanupError
	if errors.As(err, &cleanup) {
		return txcontract.ErrCleanup
	}
	return txcontract.ErrContent
}

func (a *installTxAdapter) ClassifyRollbackError(err error) txcontract.ErrorKind {
	switch {
	case err == nil:
		return txcontract.ErrNone
	case errors.Is(err, ErrTransactionCommitted):
		return txcontract.ErrAlreadyCommitted
	}
	return txcontract.ErrOther
}

// installContractDisk 用真实文件系统观察实体、配置与备份目录。
type installContractDisk struct {
	config   string
	baseline []byte
	preexist []string
	backups  []string
}

func (d *installContractDisk) EntityPresent(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func (d *installContractDisk) ConfigBytes() []byte {
	data, err := os.ReadFile(d.config)
	if err != nil {
		return nil
	}
	return data
}

func (d *installContractDisk) ConfigBaseline() []byte { return d.baseline }

func (d *installContractDisk) BackupPreserved() bool {
	for _, dir := range d.backups {
		if _, err := os.Lstat(dir); err == nil {
			return true
		}
	}
	return false
}

func (d *installContractDisk) Preexisting() []string { return d.preexist }

// installContractOpen 按场景构造真实事务，并返回真实磁盘观察。
// CleanupFails 依赖 POSIX 权限语义，权限不强制时跳过该子场景。
func installContractOpen(t *testing.T, scenario txcontract.Scenario) (txcontract.Tx, txcontract.Disk) {
	t.Helper()
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	configPath := filepath.Join(projectDir, ".sm.json")
	baseline := []byte("{}")
	if err := os.WriteFile(configPath, baseline, 0644); err != nil {
		t.Fatalf("prepare .sm.json: %v", err)
	}
	disk := &installContractDisk{config: configPath, baseline: baseline}

	switch scenario {
	case txcontract.ExistingEntity:
		// 目标已是正确的 Link Install：事务不改变文件系统，Added 为空。
		source := filepath.Join(registryDir, "skills", "cloudflare", contractSkillName)
		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("prepare target dir: %v", err)
			}
			link := filepath.Join(dir, contractSkillName)
			if err := os.Symlink(source, link); err != nil {
				t.Fatalf("prepare existing link: %v", err)
			}
			disk.preexist = append(disk.preexist, link)
		}
		tx, err := inst.BeginRegistryInstall([]string{contractSkillName}, "")
		if err != nil {
			t.Fatalf("BeginRegistryInstall: %v", err)
		}
		return &installTxAdapter{tx: tx}, disk

	case txcontract.CleanupFails:
		requirePermissionSemantics(t)
		inst.SetCopyMode(true)
		existing := filepath.Join(dirs[0], contractSkillName)
		if err := os.MkdirAll(existing, 0755); err != nil {
			t.Fatalf("prepare existing entity: %v", err)
		}
		if err := os.WriteFile(filepath.Join(existing, "old.txt"), []byte("old"), 0644); err != nil {
			t.Fatalf("prepare old content: %v", err)
		}
		tx, err := inst.BeginRegistryInstall([]string{contractSkillName}, "")
		if err != nil {
			t.Fatalf("BeginRegistryInstall: %v", err)
		}
		backups, globErr := filepath.Glob(filepath.Join(dirs[0], ".sm-placement-backup-*"))
		if globErr != nil || len(backups) == 0 {
			t.Fatalf("expected placement backups under %s: %v (err=%v)", dirs[0], backups, globErr)
		}
		for _, dir := range backups {
			if err := os.Chmod(dir, 0555); err != nil {
				t.Fatalf("chmod backup dir: %v", err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0755) })
		}
		disk.backups = backups
		return &installTxAdapter{tx: tx}, disk

	default:
		tx, err := inst.BeginRegistryInstall([]string{contractSkillName}, "")
		if err != nil {
			t.Fatalf("BeginRegistryInstall: %v", err)
		}
		cfg := &project.Config{Skills: []string{contractSkillName}}
		if stageErr := tx.StageProjectConfig(cfg); stageErr != nil {
			t.Fatalf("StageProjectConfig: %v", stageErr)
		}
		if scenario == txcontract.ConfigWriteFails {
			// Stage 已快照可读内容；改成只读后写盘确定性失败（fsutil 按权限位
			// 拒绝，与 root/平台无关）。
			if err := os.Chmod(configPath, 0444); err != nil {
				t.Fatalf("chmod .sm.json: %v", err)
			}
			t.Cleanup(func() { _ = os.Chmod(configPath, 0644) })
		}
		return &installTxAdapter{tx: tx}, disk
	}
}

// TestInstallTransactionAdditionContract 固定真实安装事务的终态契约。
func TestInstallTransactionAdditionContract(t *testing.T) {
	txcontract.Run(t, installContractOpen)
}

// TestResultReturnsIndependentCopy 固定 Result 返回只读副本：
// 修改返回值不得影响事务内部记录，nil 切片性保持不变。
func TestResultReturnsIndependentCopy(t *testing.T) {
	tx := &InstallTransaction{result: &InstallResult{}}
	empty := tx.Result()
	if empty.Skills != nil || empty.MCP != nil || empty.Failures != nil {
		t.Fatalf("nil slices must stay nil, got %+v", empty)
	}
	empty.Skills = append(empty.Skills, "mutated")
	if len(tx.result.Skills) != 0 {
		t.Errorf("mutating the returned result changed the transaction: %v", tx.result.Skills)
	}

	tx.result.Skills = []string{"cf-skill"}
	tx.result.MCP = []string{"test"}
	tx.result.Failures = []InstallFailure{{Kind: "resolve", Name: "nope", Err: os.ErrNotExist}}
	got := tx.Result()
	if len(got.Skills) != 1 || got.Skills[0] != "cf-skill" {
		t.Fatalf("Skills = %v, want [cf-skill]", got.Skills)
	}
	if len(got.Failures) != 1 || got.Failures[0].Name != "nope" {
		t.Fatalf("Failures = %+v, want one entry named nope", got.Failures)
	}
	got.Skills[0] = "mutated"
	got.MCP[0] = "mutated"
	got.Failures[0].Name = "mutated"
	if tx.result.Skills[0] != "cf-skill" || tx.result.MCP[0] != "test" || tx.result.Failures[0].Name != "nope" {
		t.Errorf("returned result still aliases transaction state: %+v", tx.result)
	}
}
