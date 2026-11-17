// lock.go 在 Registry 原件回写后刷新项目 skills-lock.json 的哈希与 ref。
// 只更新已存在的条目；lockfile 读写错误通过 LockResult.Err 暴露，由 cmd
// 保持既有 warning 输出与退出码。
//
// Input: errors, fmt, github.com/woyin/skills-manager/internal/lockfile
// Output: type LockResult, func RefreshProjectLock
// Pos: 业务层-项目锁文件刷新
package updater

import (
	"errors"
	"fmt"

	"github.com/woyin/skills-manager/internal/lockfile"
)

// LockResult 汇总一次锁文件刷新。
// Updated 是成功持久化的条目数：Save 失败时归零，不把尝试次数当成已保存。
// Err 汇总读取、重算与保存错误；Err 非 nil 时不承诺所有条目都已更新。
type LockResult struct {
	Updated int
	Err     error
}

// RefreshProjectLock 重新计算 targets 中与 lock 条目同名技能的内容哈希，
// 并在 entry.Ref 为空时填入 origin.Ref。锁文件不存在时返回零值。
func RefreshProjectLock(projectDir string, targets []OriginTarget) LockResult {
	lm := lockfile.NewManager(projectDir)
	if !lm.Exists() {
		return LockResult{}
	}
	lock, err := lm.Load()
	if err != nil {
		return LockResult{Err: err}
	}

	updated := 0
	var errs []error
	for _, t := range targets {
		entry, ok := lock.Skills[t.Name]
		if !ok {
			continue // 非 project Direct Install，跳过
		}
		hash, err := lockfile.ComputeHash(t.SkillDir)
		if err != nil {
			errs = append(errs, fmt.Errorf("rehashing %s for lockfile: %w", t.Name, err))
			continue
		}
		entry.ComputedHash = hash
		if entry.Ref == "" {
			entry.Ref = t.Origin.Ref
		}
		updated++
	}
	if updated > 0 {
		if err := lm.Save(lock); err != nil {
			errs = append(errs, fmt.Errorf("writing skills-lock.json: %w", err))
			return LockResult{Updated: 0, Err: errors.Join(errs...)}
		}
	}
	return LockResult{Updated: updated, Err: errors.Join(errs...)}
}
