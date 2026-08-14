// Package registry 的 register.go 实现 Register 原语：统一的"注册一个 Skill
// 到 Registry"路径，供 `sm add`（Register）与 `sm install`（Direct Install
// 的注册阶段）共享。
//
// 与旧 AddSkillWithOptions 的关键差异（ADR 0007-0017）：
//   - 默认 category 为 global（不再要求显式 category/special）；
//   - 写入前用 ValidateCandidate 校验 name/description（不再先写后 lint）；
//   - 目标目录以 frontmatter name 规范化（不再用来源目录名补造 name）；
//   - 同名同 Source = 刷新；同名不同 Source = 失败，除非 force；
//   - 本地单 SKILL.md 文件物化为标准目录；
//   - 任何 Git 形态都写 Origin（含抽取 Skill）；
//   - 本地目录/文件 = Snapshot（写 source_kind=local-snapshot）。
//
// Input: fmt, io, os, path/filepath, time
// Output: type RegisterRequest, type RegisterOutcome, type RegisteredSkill, type CrossSourceError, func (Registry) Register, const OutcomeCreated, const OutcomeRefreshed, const OutcomeReplaced
// Pos: 数据层-技能注册原语
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// RegisteredSkill 是一次注册的结果：入库技能的标识与路径。
type RegisteredSkill struct {
	Name     string
	Category string
	Path     string // 绝对路径
	Outcome  RegisterOutcome
}

// RegisterOutcome 描述单个技能的注册结果类型。
type RegisterOutcome string

const (
	OutcomeCreated   RegisterOutcome = "created"
	OutcomeRefreshed RegisterOutcome = "refreshed"
	OutcomeReplaced  RegisterOutcome = "replaced"
)

// CrossSourceError 在同名不同 Source 且未 --force 时返回。
type CrossSourceError struct {
	Name        string
	ExistingSrc string
	Requested   string
	Path        string
}

func (e *CrossSourceError) Error() string {
	return fmt.Sprintf("skill %q already registered from a different source (%q); use --force to replace (all Link Installs will be affected)", e.Name, e.ExistingSrc)
}

// canonicalCategory 返回目标 category：空 → global；否则校验。
func canonicalCategory(category string) (string, error) {
	if category == "" {
		return Global, nil
	}
	if err := validateCategory(category); err != nil {
		return "", err
	}
	return category, nil
}

// Register 把一个已发现的技能目录注册到 Registry。
//
// 这是 Register / Direct Install 共享的最底层原语。它：
//  1. 校验 candidate（name + description）—— 失败则不写入；
//  2. 解析目标 category（默认 global）；
//  3. 检查同名：同 Source → 刷新；不同 Source → 失败（除非 force）；
//  4. 物化到 <category>/<name>，写 Origin（temp file + rename）。
//
// srcDir 是已落地的技能目录（本地路径或克隆内的子目录）。origin 描述
// provenance；本地来源用 source_kind=local-snapshot。force 控制跨来源替换。
//
// 返回入库技能的标识。多个技能时调用方逐个调用。
func (r *Registry) Register(srcDir, category string, origin SkillOrigin, force bool) (*RegisteredSkill, error) {
	// 单文件来源（SKILL.md）需先物化为临时目录再验证/拷贝。
	staged, cleanup, err := stageSourceDir(srcDir)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	srcDir = staged

	// 1. 写入前验证 candidate。
	name, _, verr := ValidateCandidate(srcDir)
	if verr != nil {
		return nil, fmt.Errorf("validating skill at %s: %w", srcDir, verr)
	}

	// 2. 目标 category。
	cat, cerr := canonicalCategory(category)
	if cerr != nil {
		return nil, cerr
	}

	// 3. 检查同名（全局唯一身份）：刷新/替换，或跨 category 拒绝。
	existing, _ := r.FindSkillCategories(name)
	if len(existing) > 0 {
		return r.registerExisting(srcDir, name, cat, existing, origin, force)
	}

	// 4. 新建：物化到 <category>/<name>。
	return r.createNewSkill(srcDir, name, cat, origin)
}

// stageSourceDir 把单文件 SKILL.md 来源物化为临时目录；目录来源原样返回。
// 返回可能为 nil 的清理函数（仅在创建了临时目录时非空）。
func stageSourceDir(srcDir string) (string, func(), error) {
	srcInfo, statErr := os.Lstat(srcDir)
	if statErr != nil {
		return "", nil, fmt.Errorf("stat source %s: %w", srcDir, statErr)
	}
	if srcInfo.IsDir() || filepath.Base(srcDir) != "SKILL.md" {
		return srcDir, nil, nil
	}
	tmp, err := os.MkdirTemp("", "sm-regfile-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp dir for single-file source: %w", err)
	}
	if err := copyOneFile(srcDir, filepath.Join(tmp, "SKILL.md")); err != nil {
		os.RemoveAll(tmp)
		return "", nil, fmt.Errorf("staging single-file source: %w", err)
	}
	return tmp, func() { os.RemoveAll(tmp) }, nil
}

// findCategoryMatch 返回 existing 中 category 等于 cat 的第一条匹配。
func findCategoryMatch(existing []SkillMatch, cat string) *SkillMatch {
	for i := range existing {
		if existing[i].Category == cat {
			return &existing[i]
		}
	}
	return nil
}

// registerExisting 处理同名技能：同 category 走刷新/替换，
// 跨 category 则全局唯一拒绝（force 不绕开此不变量）。
func (r *Registry) registerExisting(srcDir, name, cat string, existing []SkillMatch, origin SkillOrigin, force bool) (*RegisteredSkill, error) {
	sameCat := findCategoryMatch(existing, cat)
	if sameCat == nil {
		cats := make([]string, 0, len(existing))
		for _, m := range existing {
			cats = append(cats, m.Category)
		}
		return nil, &NameConflictError{Name: name, Categories: cats}
	}
	return r.refreshSkill(srcDir, name, cat, sameCat, origin, force)
}

// refreshSkill 刷新同 category 同名技能：同 Source 或 force 时覆盖并回写
// Origin；不同 Source 且非 force 时返回 CrossSourceError。
func (r *Registry) refreshSkill(srcDir, name, cat string, sameCat *SkillMatch, origin SkillOrigin, force bool) (*RegisteredSkill, error) {
	read := r.ReadOrigin(sameCat.Path)
	if origin.Source != "" && read.Valid && read.Origin.Source != "" &&
		read.Origin.Source != origin.Source && !force {
		return nil, &CrossSourceError{
			Name:        name,
			ExistingSrc: read.Origin.Source,
			Requested:   origin.Source,
			Path:        sameCat.Path,
		}
	}
	outcome := OutcomeRefreshed
	if force && read.Valid && read.Origin.Source != "" && read.Origin.Source != origin.Source {
		outcome = OutcomeReplaced
	}
	if err := replaceDir(srcDir, sameCat.Path); err != nil {
		return nil, fmt.Errorf("refreshing skill %q: %w", name, err)
	}
	if err := r.WriteOrigin(sameCat.Path, origin); err != nil {
		return nil, fmt.Errorf("writing origin for %q: %w", name, err)
	}
	return &RegisteredSkill{Name: name, Category: cat, Path: sameCat.Path, Outcome: outcome}, nil
}

// createNewSkill 物化新技能到 <category>/<name> 并写 Origin。
func (r *Registry) createNewSkill(srcDir, name, cat string, origin SkillOrigin) (*RegisteredSkill, error) {
	dest := filepath.Join(r.skillsDir(), cat, name)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return nil, fmt.Errorf("creating category dir: %w", err)
	}
	if err := copyTreeForRegister(srcDir, dest); err != nil {
		return nil, fmt.Errorf("materializing skill %q: %w", name, err)
	}
	if err := r.WriteOrigin(dest, origin); err != nil {
		os.RemoveAll(dest)
		return nil, fmt.Errorf("writing origin for %q: %w", name, err)
	}
	return &RegisteredSkill{Name: name, Category: cat, Path: dest, Outcome: OutcomeCreated}, nil
}

// replaceDir 用 src 内容覆盖 dest（保留 dest 路径，先写旁路再替换）。
func replaceDir(src, dest string) error {
	parent := filepath.Dir(dest)
	tmp, err := os.MkdirTemp(parent, ".sm-reg-*")
	if err != nil {
		return err
	}
	if err := os.Remove(tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := copyTreeForRegister(src, tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	_ = os.Remove(filepath.Join(tmp, OriginFile))
	backup := dest + ".bak-sm"
	_ = os.RemoveAll(backup)
	hadDest := false
	if _, err := os.Lstat(dest); err == nil {
		hadDest = true
		if err := os.Rename(dest, backup); err != nil {
			os.RemoveAll(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, dest); err != nil {
		if hadDest {
			_ = os.Rename(backup, dest)
		}
		os.RemoveAll(tmp)
		return err
	}
	os.RemoveAll(backup)
	return nil
}

// copyTreeForRegister 把 src 树拷贝到 dest（dest 必须不存在）。
// 复用 fsutil.CopyDir（已跳过 .git/node_modules 等）。单文件来源物化为目录。
func copyTreeForRegister(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		// 单文件来源：必须是 SKILL.md，物化为 <dest>/SKILL.md。
		if filepath.Base(src) != "SKILL.md" {
			return fmt.Errorf("single-file source must be SKILL.md, got %q", filepath.Base(src))
		}
		if err := os.MkdirAll(dest, 0755); err != nil {
			return err
		}
		return copyOneFile(src, filepath.Join(dest, "SKILL.md"))
	}
	// 目录来源：fsutil.CopyDir 已跳过 .git/node_modules/dist/build/__pycache__。
	return copyDirExternal(src, dest)
}

// copyOneFile 拷贝单个文件并保留权限。
func copyOneFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
