// Package txcontract 提供延迟安装事务的最小一致性契约，供 curation 的测试
// 假事务、installer 的真实 InstallTransaction 与 cmd 的 planTx 适配器跑同一
// 组终态断言。
//
// 边界（ADR 0018 渐进提取）：本包仅为测试支持，只依赖标准库与
// internal/project；禁止导入 curation、installer 或 cmd，也不被生产代码导入。
// 因此它定义自己的 Entity，而不是复用 curation.AddedEntity。
package txcontract

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
)

// Entity 是本次事务实际改变的一个实体。
type Entity struct {
	Agent   string
	Skill   string
	Path    string
	Symlink bool
}

// Tx 是与 curation.AdditionTx 同形的最小事务方法集。
type Tx interface {
	Added() []Entity
	StageProjectConfig(*project.Config) error
	Committed() bool
	Commit() error
	Rollback() error
}

// Scenario 是 fixture 必须能构造的故障时点。
type Scenario int

const (
	// Happy：事务可正常提交。
	Happy Scenario = iota
	// ConfigWriteFails：内容写盘失败，必须内部补偿。
	ConfigWriteFails
	// CleanupFails：内容已写盘、仅备份清理失败。
	CleanupFails
	// ExistingEntity：目标已是正确的既有实体（幂等，无改变）。
	ExistingEntity
)

func (s Scenario) String() string {
	switch s {
	case Happy:
		return "Happy"
	case ConfigWriteFails:
		return "ConfigWriteFails"
	case CleanupFails:
		return "CleanupFails"
	case ExistingEntity:
		return "ExistingEntity"
	}
	return fmt.Sprintf("Scenario(%d)", int(s))
}

// Disk 提供真实文件系统观察。fixture 必须实现真实观察，不得用内存标志代替
// 可观察的磁盘状态；唯一例外是 BackupPreserved，它只表示诊断路径是否仍存在。
type Disk interface {
	// EntityPresent 报告某实体路径当前是否存在（跟随 Lstat 语义）。
	EntityPresent(path string) bool
	// ConfigBytes 返回项目配置当前字节；文件不可读时返回 nil。
	ConfigBytes() []byte
	// ConfigBaseline 返回 fixture 打开时（任何事务写入前）的配置字节。
	ConfigBaseline() []byte
	// BackupPreserved 报告清理失败后是否仍保留备份路径（其他场景返回 false）。
	BackupPreserved() bool
	// Preexisting 返回事务开始前已存在的实体路径（其他场景返回 nil）。
	Preexisting() []string
}

// ErrorKind 是跨实现可判定的错误类别，避免契约比较错误字符串。
type ErrorKind int

const (
	ErrNone ErrorKind = iota
	ErrAlreadyCommitted
	ErrAlreadyRolledBack
	ErrContent
	ErrCleanup
	ErrOther
)

func (k ErrorKind) String() string {
	switch k {
	case ErrNone:
		return "none"
	case ErrAlreadyCommitted:
		return "already-committed"
	case ErrAlreadyRolledBack:
		return "already-rolled-back"
	case ErrContent:
		return "content-failure"
	case ErrCleanup:
		return "cleanup-incomplete"
	}
	return fmt.Sprintf("ErrorKind(%d)", int(k))
}

// Classifier 由 fixture 的 Tx 实现：把各实现自己的错误映射到统一类别。
// 契约不要求统一错误类型，只要求类别可判定。
type Classifier interface {
	ClassifyCommitError(error) ErrorKind
	ClassifyRollbackError(error) ErrorKind
}

// Run 按固定终态表执行全部子测试。fixture 通过闭包提供，避免为测试导出
// 适配器类型；Open 只允许在权限语义不可用时对 CleanupFails 调用 t.Skip。
func Run(t *testing.T, open func(t *testing.T, s Scenario) (Tx, Disk)) {
	t.Helper()

	t.Run("CommitTwice", func(t *testing.T) {
		tx, disk := open(t, Happy)
		added := tx.Added()
		if err := tx.Commit(); err != nil {
			t.Fatalf("first Commit = %v, want nil", err)
		}
		if !tx.Committed() {
			t.Fatal("Committed() = false after successful Commit")
		}
		second := tx.Commit()
		if !sameError(nil, second) {
			t.Fatalf("second Commit = %v, want the first result (nil)", second)
		}
		if !tx.Committed() {
			t.Fatal("Committed() = false after repeated Commit")
		}
		for _, e := range added {
			if !disk.EntityPresent(e.Path) {
				t.Errorf("committed entity %s must stay on disk", e.Path)
			}
		}
	})

	t.Run("RollbackAfterCommit", func(t *testing.T) {
		tx, disk := open(t, Happy)
		added := tx.Added()
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit = %v, want nil", err)
		}
		err := tx.Rollback()
		if err == nil {
			t.Fatal("Rollback after Commit must fail")
		}
		if kind := classifyRollback(t, tx, err); kind != ErrAlreadyCommitted {
			t.Fatalf("Rollback error kind = %v, want %v (err=%v)", kind, ErrAlreadyCommitted, err)
		}
		if !tx.Committed() {
			t.Error("Committed() = false after rejected Rollback")
		}
		for _, e := range added {
			if !disk.EntityPresent(e.Path) {
				t.Errorf("committed entity %s must not be removed by Rollback", e.Path)
			}
		}
	})

	t.Run("RollbackTwice", func(t *testing.T) {
		tx, disk := open(t, Happy)
		added := tx.Added()
		if err := tx.Rollback(); err != nil {
			t.Fatalf("first Rollback = %v, want nil", err)
		}
		if tx.Committed() {
			t.Fatal("Committed() = true after Rollback")
		}
		for _, e := range added {
			if disk.EntityPresent(e.Path) {
				t.Errorf("rolled-back entity %s must be gone", e.Path)
			}
		}
		second := tx.Rollback()
		if !sameError(nil, second) {
			t.Fatalf("second Rollback = %v, want the first result (nil)", second)
		}
		for _, e := range added {
			if disk.EntityPresent(e.Path) {
				t.Errorf("repeated Rollback re-created %s", e.Path)
			}
		}
	})

	t.Run("CommitAfterRollback", func(t *testing.T) {
		tx, disk := open(t, Happy)
		added := tx.Added()
		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback = %v, want nil", err)
		}
		err := tx.Commit()
		if err == nil {
			t.Fatal("Commit after Rollback must fail")
		}
		if kind := classifyCommit(t, tx, err); kind != ErrAlreadyRolledBack {
			t.Fatalf("Commit error kind = %v, want %v (err=%v)", kind, ErrAlreadyRolledBack, err)
		}
		if tx.Committed() {
			t.Error("Committed() = true after rejected Commit")
		}
		for _, e := range added {
			if disk.EntityPresent(e.Path) {
				t.Errorf("Commit after Rollback wrote %s", e.Path)
			}
		}
	})

	t.Run("ConfigWriteFailureCompensates", func(t *testing.T) {
		tx, disk := open(t, ConfigWriteFails)
		added := tx.Added()
		err := tx.Commit()
		if err == nil {
			t.Fatal("Commit must fail when the config write fails")
		}
		if kind := classifyCommit(t, tx, err); kind != ErrContent {
			t.Fatalf("Commit error kind = %v, want %v (err=%v)", kind, ErrContent, err)
		}
		if tx.Committed() {
			t.Error("Committed() = true after compensated content failure")
		}
		for _, e := range added {
			if disk.EntityPresent(e.Path) {
				t.Errorf("entity %s must be compensated away", e.Path)
			}
		}
		if got, want := disk.ConfigBytes(), disk.ConfigBaseline(); !bytes.Equal(got, want) {
			t.Errorf("project config bytes changed on failure: got %q, want %q", got, want)
		}
	})

	t.Run("CleanupCommittedKeepsContent", func(t *testing.T) {
		tx, disk := open(t, CleanupFails)
		added := tx.Added()
		err := tx.Commit()
		if err == nil {
			t.Fatal("Commit must report incomplete cleanup")
		}
		if kind := classifyCommit(t, tx, err); kind != ErrCleanup {
			t.Fatalf("Commit error kind = %v, want %v (err=%v)", kind, ErrCleanup, err)
		}
		if !tx.Committed() {
			t.Error("Committed() = false although content was committed")
		}
		for _, e := range added {
			if !disk.EntityPresent(e.Path) {
				t.Errorf("committed entity %s must stay after cleanup failure", e.Path)
			}
		}
		if !disk.BackupPreserved() {
			t.Error("cleanup failure must preserve the backup path for diagnosis")
		}
		if rerr := tx.Rollback(); rerr == nil {
			t.Error("Rollback after cleanup failure must be rejected")
		} else if kind := classifyRollback(t, tx, rerr); kind != ErrAlreadyCommitted {
			t.Errorf("Rollback error kind = %v, want %v (err=%v)", kind, ErrAlreadyCommitted, rerr)
		}
		for _, e := range added {
			if !disk.EntityPresent(e.Path) {
				t.Errorf("Rollback removed committed entity %s", e.Path)
			}
		}
	})

	t.Run("ExistingEntityNotAdded", func(t *testing.T) {
		tx, disk := open(t, ExistingEntity)
		preexisting := disk.Preexisting()
		if len(preexisting) == 0 {
			t.Fatal("ExistingEntity fixture must report the pre-existing paths")
		}
		for _, e := range tx.Added() {
			for _, p := range preexisting {
				if e.Path == p {
					t.Errorf("idempotent existing entity %s must not appear in Added()", p)
				}
			}
		}
		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback = %v, want nil", err)
		}
		for _, p := range preexisting {
			if !disk.EntityPresent(p) {
				t.Errorf("Rollback removed pre-existing entity %s", p)
			}
		}
	})
}

// sameError 判定两次调用返回了同一结果：都是 nil，或互相 Is。
func sameError(a, b error) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return errors.Is(a, b) && errors.Is(b, a)
}

func classifiers(t *testing.T, tx Tx) Classifier {
	t.Helper()
	c, ok := tx.(Classifier)
	if !ok {
		t.Fatalf("fixture Tx %T must implement Classifier", tx)
	}
	return c
}

func classifyCommit(t *testing.T, tx Tx, err error) ErrorKind {
	t.Helper()
	return classifiers(t, tx).ClassifyCommitError(err)
}

func classifyRollback(t *testing.T, tx Tx, err error) ErrorKind {
	t.Helper()
	return classifiers(t, tx).ClassifyRollbackError(err)
}
