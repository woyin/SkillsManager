package cmd

// plan_addition_contract_test.go 让 cmd 的真实 planTx 适配器（包装
// installer.InstallTransaction）跑 txcontract 的固定终态契约。
//
// 本文件不读也不写 cmd 全局（RegistryDir/ProfilesDir/planDir…），不调用
// 共享测试 helper，也不触发宿主 Agent 检测：registry、profiles、目标目录
// 全部来自 t.TempDir() 与显式 tool.Tool。

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/installer"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
	"github.com/woyin/skills-manager/internal/txcontract"
)

// contractCmdSkill 是 cmd 契约 fixture 使用的技能名。
const contractCmdSkill = "contract-skill"

// cmdContractTools 是固定的项目 scope 目标，避免依赖宿主已装 Agent。
func cmdContractTools() []tool.Tool {
	return []tool.Tool{{Name: "alpha", ProjectSkillDir: filepath.Join(".agents", "skills")}}
}

// cmdContractEnv 汇总一个临时项目环境。
type cmdContractEnv struct {
	registryDir string
	profilesDir string
	projectDir  string
	entity      string
	configPath  string
}

func newCmdContractEnv(t *testing.T) *cmdContractEnv {
	t.Helper()
	base := t.TempDir()
	env := &cmdContractEnv{
		registryDir: filepath.Join(base, "registry"),
		profilesDir: filepath.Join(base, "profiles"),
		projectDir:  filepath.Join(base, "project"),
	}
	skillDir := filepath.Join(env.registryDir, "skills", "global", contractCmdSkill)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("prepare registry skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# contract\n"), 0644); err != nil {
		t.Fatalf("prepare SKILL.md: %v", err)
	}
	if err := os.MkdirAll(env.profilesDir, 0755); err != nil {
		t.Fatalf("prepare profiles dir: %v", err)
	}
	if err := os.MkdirAll(env.projectDir, 0755); err != nil {
		t.Fatalf("prepare project dir: %v", err)
	}
	env.entity = filepath.Join(env.projectDir, ".agents", "skills", contractCmdSkill)
	env.configPath = filepath.Join(env.projectDir, ".sm.json")
	return env
}

// newInstaller 返回只作用于该临时项目的安装器。
func (e *cmdContractEnv) newInstaller(t *testing.T) *installer.Installer {
	t.Helper()
	inst, err := installer.New(e.registryDir, e.profilesDir, cmdContractTools())
	if err != nil {
		t.Fatalf("installer.New: %v", err)
	}
	inst.SetScope(e.projectDir, false)
	return inst
}

// planTxAdapter 把 planTx 适配为 txcontract.Tx。
type planTxAdapter struct{ tx *planTx }

func (a *planTxAdapter) Added() []txcontract.Entity {
	added := a.tx.Added()
	out := make([]txcontract.Entity, 0, len(added))
	for _, e := range added {
		out = append(out, txcontract.Entity{Agent: e.Agent, Skill: e.Skill, Path: e.Path, Symlink: e.Symlink})
	}
	return out
}

func (a *planTxAdapter) StageProjectConfig(cfg *project.Config) error {
	return a.tx.StageProjectConfig(cfg)
}

func (a *planTxAdapter) Committed() bool { return a.tx.Committed() }

func (a *planTxAdapter) Commit() error { return a.tx.Commit() }

func (a *planTxAdapter) Rollback() error { return a.tx.Rollback() }

func (a *planTxAdapter) ClassifyCommitError(err error) txcontract.ErrorKind {
	switch {
	case err == nil:
		return txcontract.ErrNone
	case errors.Is(err, installer.ErrTransactionRolledBack):
		return txcontract.ErrAlreadyRolledBack
	}
	var cleanup *installer.CommitCleanupError
	if errors.As(err, &cleanup) {
		return txcontract.ErrCleanup
	}
	return txcontract.ErrContent
}

func (a *planTxAdapter) ClassifyRollbackError(err error) txcontract.ErrorKind {
	switch {
	case err == nil:
		return txcontract.ErrNone
	case errors.Is(err, installer.ErrTransactionCommitted):
		return txcontract.ErrAlreadyCommitted
	}
	return txcontract.ErrOther
}

// cmdContractDisk 用真实文件系统观察实体、配置与备份目录。
type cmdContractDisk struct {
	config   string
	baseline []byte
	preexist []string
	backups  []string
}

func (d *cmdContractDisk) EntityPresent(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func (d *cmdContractDisk) ConfigBytes() []byte {
	data, err := os.ReadFile(d.config)
	if err != nil {
		return nil
	}
	return data
}

func (d *cmdContractDisk) ConfigBaseline() []byte { return d.baseline }

func (d *cmdContractDisk) BackupPreserved() bool {
	for _, dir := range d.backups {
		if _, err := os.Lstat(dir); err == nil {
			return true
		}
	}
	return false
}

func (d *cmdContractDisk) Preexisting() []string { return d.preexist }

// cmdRequirePermissionSemantics 是 cmd 包内的权限语义探针：root 或文件系统
// 不强制只读时明确跳过依赖权限的场景（不使用 os.Geteuid，保持 Windows 可编译）。
func cmdRequirePermissionSemantics(t *testing.T) {
	t.Helper()
	probe := filepath.Join(t.TempDir(), "probe")
	if err := os.WriteFile(probe, []byte("x"), 0444); err != nil {
		t.Fatalf("prepare probe: %v", err)
	}
	if err := os.WriteFile(probe, []byte("y"), 0644); err == nil {
		t.Skip("filesystem does not enforce read-only file semantics")
	}
}

// planTxContractOpen 按场景构造真实 planTx，并返回真实磁盘观察。
func planTxContractOpen(t *testing.T, scenario txcontract.Scenario) (txcontract.Tx, txcontract.Disk) {
	t.Helper()
	env := newCmdContractEnv(t)
	inst := env.newInstaller(t)
	baseline := []byte("{}")
	disk := &cmdContractDisk{config: env.configPath, baseline: baseline}

	switch scenario {
	case txcontract.ExistingEntity:
		// 既有正确 Link Install：事务不改变文件系统，Added 必须为空。
		if err := os.MkdirAll(filepath.Dir(env.entity), 0755); err != nil {
			t.Fatalf("prepare target dir: %v", err)
		}
		source := filepath.Join(env.registryDir, "skills", "global", contractCmdSkill)
		if err := os.Symlink(source, env.entity); err != nil {
			t.Fatalf("prepare existing link: %v", err)
		}
		disk.preexist = []string{env.entity}
		tx, err := inst.BeginRegistryInstall([]string{contractCmdSkill}, "")
		if err != nil {
			t.Fatalf("BeginRegistryInstall: %v", err)
		}
		return &planTxAdapter{tx: &planTx{tx: tx}}, disk

	case txcontract.CleanupFails:
		cmdRequirePermissionSemantics(t)
		inst.SetCopyMode(true)
		if err := os.MkdirAll(env.entity, 0755); err != nil {
			t.Fatalf("prepare existing entity: %v", err)
		}
		if err := os.WriteFile(filepath.Join(env.entity, "old.txt"), []byte("old"), 0644); err != nil {
			t.Fatalf("prepare old content: %v", err)
		}
		tx, err := inst.BeginRegistryInstall([]string{contractCmdSkill}, "")
		if err != nil {
			t.Fatalf("BeginRegistryInstall: %v", err)
		}
		backups, globErr := filepath.Glob(filepath.Join(filepath.Dir(env.entity), ".sm-placement-backup-*"))
		if globErr != nil || len(backups) == 0 {
			t.Fatalf("expected placement backups under %s: %v (err=%v)", filepath.Dir(env.entity), backups, globErr)
		}
		for _, dir := range backups {
			if chmodErr := os.Chmod(dir, 0555); chmodErr != nil {
				t.Fatalf("chmod backup dir: %v", chmodErr)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0755) })
		}
		disk.backups = backups
		return &planTxAdapter{tx: &planTx{tx: tx}}, disk

	default:
		if err := os.WriteFile(env.configPath, baseline, 0644); err != nil {
			t.Fatalf("prepare .sm.json: %v", err)
		}
		tx, err := inst.BeginRegistryInstall([]string{contractCmdSkill}, "")
		if err != nil {
			t.Fatalf("BeginRegistryInstall: %v", err)
		}
		cfg := &project.Config{Skills: []string{contractCmdSkill}}
		if stageErr := tx.StageProjectConfig(cfg); stageErr != nil {
			t.Fatalf("StageProjectConfig: %v", stageErr)
		}
		if scenario == txcontract.ConfigWriteFails {
			// Stage 已快照；改成只读后写盘确定性失败（fsutil 按权限位拒绝，
			// 与 root/平台无关）。
			if chmodErr := os.Chmod(env.configPath, 0444); chmodErr != nil {
				t.Fatalf("chmod .sm.json: %v", chmodErr)
			}
			t.Cleanup(func() { _ = os.Chmod(env.configPath, 0644) })
		}
		return &planTxAdapter{tx: &planTx{tx: tx}}, disk
	}
}

// TestPlanTxAdditionContract 固定 cmd 适配器的终态契约（真实 installer 事务）。
func TestPlanTxAdditionContract(t *testing.T) {
	txcontract.Run(t, planTxContractOpen)
}
