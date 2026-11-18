package curation

// 本文件只放测试用的假安装事务（fakeTx / cleanupTx）与构造 helper。
// 行为断言按职责分布在 apply_transaction_test.go（补偿/提交/清理）、
// apply_result_test.go（结果报告）、apply_ownership_test.go（ownership 登记）、
// apply_bootstrap_test.go（bootstrap 应用语义）。

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

// ErrFakeTransactionCommitted / ErrFakeTransactionRolledBack 是 fakeTx 的终态
// 错误，与 installer 的 ErrTransactionCommitted / ErrTransactionRolledBack 对应。
// 它们只存在于测试中，用来让 fake 与真实事务遵守同一契约。
var (
	ErrFakeTransactionCommitted  = errors.New("fake install transaction already committed")
	ErrFakeTransactionRolledBack = errors.New("fake install transaction already rolled back")
)

// fakeTx 是 AdditionTx 的测试实现：只补偿自己创建的文件。
type fakeTx struct {
	added       []AddedEntity
	cfg         *project.Config
	staged      *project.Config
	created     []string
	committed   bool
	closed      bool
	commitErr   error
	stageErr    error
	rollbackErr error
	onRollback  func()
	saveDir     string
}

func (f *fakeTx) Added() []AddedEntity           { return f.added }
func (f *fakeTx) DesiredConfig() *project.Config { return f.cfg }
func (f *fakeTx) Committed() bool                { return f.committed }
func (f *fakeTx) StageProjectConfig(c *project.Config) error {
	if f.stageErr != nil {
		return f.stageErr
	}
	f.staged = c
	return nil
}
func (f *fakeTx) Commit() error {
	// 终态语义与 installer 一致：已回滚的句柄不得提交，已提交的句柄重复
	// Commit 返回首次结果，不重复写盘。
	if f.closed {
		return ErrFakeTransactionRolledBack
	}
	if f.committed {
		return f.commitErr
	}
	if f.commitErr != nil {
		return f.commitErr
	}
	if f.staged != nil && f.saveDir != "" {
		if err := project.NewManager(f.saveDir).Save(f.staged); err != nil {
			// 内容写盘失败＝未提交：内部补偿本次已落地实体，之后 Rollback
			// 成为 no-op（与 installer 的 compensate 行为一致）。
			return errors.Join(err, f.compensate())
		}
	}
	f.committed = true
	return nil
}

// compensate 撤销本次落地实体并进入已回滚终态。
func (f *fakeTx) compensate() error {
	if f.closed {
		return nil
	}
	f.closed = true
	var errs []error
	for i := len(f.created) - 1; i >= 0; i-- {
		if err := os.Remove(f.created[i]); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("compensating %s: %w", f.created[i], err))
		}
	}
	return errors.Join(errs...)
}
func (f *fakeTx) Rollback() error {
	// 内容已提交时拒绝回滚，且不得删除已提交实体（installer 返回
	// ErrTransactionCommitted）。已回滚时返回首次结果（幂等）。
	if f.committed {
		return ErrFakeTransactionCommitted
	}
	if f.closed {
		return f.rollbackErr
	}
	f.closed = true
	if f.onRollback != nil {
		f.onRollback()
	}
	for i := len(f.created) - 1; i >= 0; i-- {
		_ = os.Remove(f.created[i])
	}
	return f.rollbackErr
}

// newFakeTx 在 env 项目目录里为每个技能建立 Claude 项目级链接，并登记为已新增实体。
func newFakeTx(env *testEnv, skills ...string) *fakeTx {
	linkHost := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir)
	tx := &fakeTx{cfg: &project.Config{}, saveDir: env.projectDir}
	for _, s := range skills {
		_ = os.MkdirAll(linkHost, 0755)
		path := filepath.Join(linkHost, s)
		if err := os.Symlink(filepath.Join(env.registryDir, "skills", "global", s), path); err != nil {
			panic(err)
		}
		tx.created = append(tx.created, path)
		tx.added = append(tx.added, AddedEntity{Agent: tool.Claude.Name, Skill: s, Path: path, Symlink: true})
	}
	return tx
}

// cleanupTx 模拟“内容已提交但清理未完成”的句柄：Commit 置 committed 并返回错误。
type cleanupTx struct {
	fakeTx
	paths []string
}

// Commit 模拟“内容已写盘、仅备份清理失败”：先落盘暂存配置，再返回清理错误。
// 终态语义与 installer 一致：已回滚后拒绝提交；重复提交返回首次结果。
func (c *cleanupTx) Commit() error {
	if c.closed {
		return ErrFakeTransactionRolledBack
	}
	if c.committed {
		return c.commitErr
	}
	if c.staged != nil && c.saveDir != "" {
		if err := project.NewManager(c.saveDir).Save(c.staged); err != nil {
			return err
		}
	}
	c.committed = true
	c.commitErr = &fakeCleanupError{paths: c.paths}
	return c.commitErr
}

type fakeCleanupError struct{ paths []string }

func (e *fakeCleanupError) Error() string { return "cleanup incomplete: " + e.paths[0] }
