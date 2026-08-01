// Package registry 的 origin.go 实现 Skill Origin：每个 Registry Skill 的
// 版本化 provenance 元数据，取代"目录是否含 .git"判断身份/可更新性。
//
// 解决的旧问题：
//   - 旧 .sm-origin.json 只有 source/ref/rel_path/commit，无 ref kind；
//     非空 ref 一律按 pinned，因此 --ref main 不前进。
//   - 无 .git/origin 的本地快照一律归 orphan，无法与元数据损坏区分。
//   - 完整单 Skill Git repo 与抽取 Skill 走两套不同生命周期。
//
// 本文件提供统一的 Registry 层原语（见 ADR 0007-0017）：
//   - SkillOrigin：版本化 provenance 记录；
//   - ValidateSkillName / ValidateDescription：写入前身份与质量门槛；
//   - ResolveUniqueSkill / FindConflicts：全局唯一身份解析与冲突检测；
//   - ListAllOriginals：全表分类枚举（tracking/pinned/snapshot/orphan）。
//
// 向后兼容：读取旧 .sm-origin.json（无 source_kind/ref_kind）时推断：
//   - 无 source_kind 且 source 非空 → git；
//   - 无 ref_kind 且 ref 为空 → default-branch；非空 → pinned（tag/commit
//     未知，更新路径不前进，保留旧行为）。
//
// Input: encoding/json, fmt, os, path/filepath, sort, strings, time
// Output: const OriginFile, const MetadataSchemaVersion, type SourceKind, type RefKind, type UpdateClass, type SkillOrigin, type OriginRead, type RegistryOriginal, type NameConflictError, type NameNotFoundError, type SkillResolution, func ValidateSkillName, func ValidateDescription, func (Registry) WriteOrigin, func (Registry) ReadOrigin, func (Registry) ResolveUniqueSkill, func (Registry) FindConflicts, func (Registry) ListAllOriginals
// Pos: 数据层-skill来源元数据与身份解析原语
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// OriginFile 是写入每个 Registry Skill 目录的 provenance 文件名。
// 以 . 开头，避免被 agent 当作技能内容读取。导出供 cmd 包共享，使
// add/install/update/rm 使用同一常量（取代 cmd/skill_origin.go 的旧值）。
const OriginFile = ".sm-origin.json"

// MetadataSchemaVersion 是 SkillOrigin JSON 的当前 schema 版本。
// 0（或缺失）= 旧 schema（source/ref/rel_path/commit），读取时推断字段。
const MetadataSchemaVersion = 1

// SourceKind 描述 Registry Skill 的来源类型。
type SourceKind string

const (
	// SourceGit：来自 Git 仓库（单仓、tree URL、多仓抽取）。可刷新。
	SourceGit SourceKind = "git"
	// SourceLocalSnapshot：本地目录或单个 SKILL.md 的独立快照。不随源变化，
	// 仅重新 sm add 才刷新。
	SourceLocalSnapshot SourceKind = "local-snapshot"
	// SourceWellKnown：Well-Known Source 端点（skills.sh 等）。可刷新。
	SourceWellKnown SourceKind = "well-known"
)

// RefKind 描述请求 ref 的解析类型，控制 update 行为（ADR 0014）。
type RefKind string

const (
	// RefDefaultBranch：未指定 ref，跟踪远端默认分支，参与更新。
	RefDefaultBranch RefKind = "default-branch"
	// RefBranch：显式分支，跟踪该分支，参与更新。
	RefBranch RefKind = "branch"
	// RefTag：tag，pinned，不自动前进。
	RefTag RefKind = "tag"
	// RefCommit：commit，pinned，不自动前进。
	RefCommit RefKind = "commit"
	// RefUnknown：旧 metadata 无法判断（用于兼容读取）。
	RefUnknown RefKind = ""
)

// UpdateClass 是 Registry Skill 在 update 语境下的分类。
type UpdateClass string

const (
	// ClassTracking：跟踪 Git（default-branch 或显式 branch），尝试更新。
	ClassTracking UpdateClass = "tracking"
	// ClassPinned：pinned tag/commit，健康跳过。
	ClassPinned UpdateClass = "pinned"
	// ClassSnapshot：本地快照，健康跳过。
	ClassSnapshot UpdateClass = "snapshot"
	// ClassOrphan：元数据损坏（应有 provenance 却无），计为错误。
	ClassOrphan UpdateClass = "orphan"
)

// SkillOrigin 是 Registry Skill 的 provenance 记录。写入时序列化为
// OriginFile（JSON）。SchemaVersion 控制向后兼容。
//
// 字段映射（新 → 旧兼容读取）：
//   - SourceKind 缺失 + Source 非空 → 推断 git；
//   - RefKind 缺失：Ref 空 → default-branch；非空 → pinned（RefTag/RefCommit
//     未知，但 IsPinned 返回 true，update 不前进）；
//   - SubPath ← 旧 rel_path。
type SkillOrigin struct {
	SchemaVersion int        `json:"schema_version,omitempty"`
	SourceKind    SourceKind `json:"source_kind"`
	Source        string     `json:"source"`
	Ref           string     `json:"ref,omitempty"`
	RefKind       RefKind    `json:"ref_kind,omitempty"`
	SubPath       string     `json:"sub_path,omitempty"`
	Commit        string     `json:"commit,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at,omitempty"`

	// 旧 schema 字段（仅兼容读取；新写入不再使用，但保留以使旧读者可读）。
	RelPath string `json:"rel_path,omitempty"`
}

// IsPinned 报告此 origin 是否 pinned（不自动前进）。
// default-branch 与显式 branch 返回 false；tag/commit 返回 true。
// 旧 schema（RefKind 空但 Ref 非空）也视为 pinned。
func (o SkillOrigin) IsPinned() bool {
	switch o.RefKind {
	case RefTag, RefCommit:
		return true
	case RefDefaultBranch, RefBranch:
		return false
	case RefUnknown:
		// 旧 schema：非空 ref 视为 pinned。
		return o.Ref != ""
	}
	return false
}

// IsLocalSnapshot 报告此 origin 是否来自本地快照。
func (o SkillOrigin) IsLocalSnapshot() bool {
	return o.SourceKind == SourceLocalSnapshot
}

// OriginRead 是 ReadOrigin 的结果：读取到的 origin（若存在）+ 分类 + 状态。
type OriginRead struct {
	Origin  SkillOrigin
	Class   UpdateClass
	HasFile bool // 是否存在 OriginFile
	Valid   bool // 文件存在且可解析且 Source 非空
}

// SkillResolution 是按全局唯一名称解析到 Registry 原件的结果。
type SkillResolution struct {
	Name     string
	Category string
	Path     string
}

// RegistryOriginal 描述一个 Registry 原件条目，含其 update 分类。
type RegistryOriginal struct {
	Name     string
	Category string
	Path     string
	Class    UpdateClass
	Origin   SkillOrigin
}

// NameConflictError 在 name 跨多 category 出现时返回（违反全局唯一不变量）。
type NameConflictError struct {
	Name       string
	Categories []string
	Paths      []string
}

func (e *NameConflictError) Error() string {
	return fmt.Sprintf("skill %q exists in multiple categories (%s); global uniqueness requires removing all but one", e.Name, strings.Join(e.Categories, ", "))
}

// NameNotFoundError 在 name 在 Registry 中不存在时返回。
type NameNotFoundError struct {
	Name string
}

func (e *NameNotFoundError) Error() string {
	return fmt.Sprintf("skill %q is not in the registry; run `sm add <source>` first", e.Name)
}

// skillNamePattern 的合法字符集：小写字母、数字、单连字符。
// 由 ValidateSkillName 进一步约束首尾与连续连字符。

// ValidateSkillName 校验 frontmatter name 是否满足 Registry 身份要求：
// 1–64 个小写字母、数字或单连字符；不能以连字符开头/结尾；不能含连续连字符。
// category 不参与身份（ADR 0010）。
func ValidateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("skill name is empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("skill name %q exceeds 64 characters (%d)", name, len(name))
	}
	// 字符集：仅 [a-z0-9-]
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-':
		default:
			return fmt.Errorf("skill name %q contains invalid character %q (allowed: lowercase letters, digits, hyphen)", name, string(rune(c)))
		}
	}
	// 首尾连字符
	if name[0] == '-' {
		return fmt.Errorf("skill name %q must not start with a hyphen", name)
	}
	if name[len(name)-1] == '-' {
		return fmt.Errorf("skill name %q must not end with a hyphen", name)
	}
	// 连续连字符
	if strings.Contains(name, "--") {
		return fmt.Errorf("skill name %q must not contain consecutive hyphens", name)
	}
	return nil
}

// ValidateDescription 校验 description：1–1024 字符（按 rune）。
// 注册的写入前门槛（ADR 0010）；其它 lint 规则可继续 warning。
func ValidateDescription(desc string) error {
	n := len([]rune(desc))
	if n == 0 {
		return fmt.Errorf("description is empty")
	}
	if n > 1024 {
		return fmt.Errorf("description exceeds 1024 characters (%d)", n)
	}
	return nil
}

// ValidateCandidate 在写入前验证一个 SKILL.md 候选：必须含 SKILL.md、
// name 合法、description 非空且合法。返回规范化的 name 与 description。
// 失败时不写入 Registry（ADR 0010 写入前拒绝）。
func ValidateCandidate(skillDir string) (name, description string, err error) {
	skillMD := filepath.Join(skillDir, "SKILL.md")
	data, err := os.ReadFile(skillMD)
	if err != nil {
		return "", "", fmt.Errorf("missing SKILL.md in %s: %w", skillDir, err)
	}
	fm := parseFrontmatterBytes(data)
	name = strings.TrimSpace(fm.Name)
	if err := ValidateSkillName(name); err != nil {
		return "", "", fmt.Errorf("invalid skill name: %w", err)
	}
	description = strings.TrimSpace(fm.Description)
	if err := ValidateDescription(description); err != nil {
		return "", "", fmt.Errorf("invalid description: %w", err)
	}
	return name, description, nil
}

// WriteOrigin 把 origin 以临时文件 + rename 写入 skillDir/.sm-origin.json。
// SchemaVersion 与 UpdatedAt 自动填充。
func (r *Registry) WriteOrigin(skillDir string, origin SkillOrigin) error {
	if origin.SchemaVersion == 0 {
		origin.SchemaVersion = MetadataSchemaVersion
	}
	origin.UpdatedAt = time.Now().UTC()
	// 保持 rel_path 与 sub_path 一致（旧读者兼容）。
	if origin.SubPath != "" && origin.RelPath == "" {
		origin.RelPath = origin.SubPath
	}
	data, err := json.MarshalIndent(origin, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling origin: %w", err)
	}
	path := filepath.Join(skillDir, OriginFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing origin temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming origin file: %w", err)
	}
	return nil
}

// ReadOrigin 读取 skillDir 的 provenance 并分类。
// 不存在 OriginFile 时返回 ClassOrphan + HasFile=false。
// 存在但不可解析时返回 ClassOrphan + HasFile=true + Valid=false。
// 兼容旧 schema：缺 source_kind → git；缺 ref_kind 按推断规则。
func (r *Registry) ReadOrigin(skillDir string) OriginRead {
	path := filepath.Join(skillDir, OriginFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return OriginRead{Class: ClassOrphan, HasFile: false, Valid: false}
	}
	var o SkillOrigin
	if err := json.Unmarshal(data, &o); err != nil {
		return OriginRead{Class: ClassOrphan, HasFile: true, Valid: false}
	}
	// 兼容旧 schema：rel_path → sub_path。
	if o.SubPath == "" && o.RelPath != "" {
		o.SubPath = o.RelPath
	}
	if o.SubPath == "" {
		o.SubPath = "."
	}
	// 推断 source_kind。
	if o.SourceKind == "" {
		if o.Source != "" {
			o.SourceKind = SourceGit
		} else {
			// 既无 source 又无 source_kind：损坏。
			return OriginRead{Origin: o, Class: ClassOrphan, HasFile: true, Valid: false}
		}
	}
	// 推断 ref_kind（仅 git 类）。
	if o.RefKind == RefUnknown && o.SourceKind == SourceGit {
		if o.Ref == "" {
			o.RefKind = RefDefaultBranch
		} else {
			// 旧 schema 非空 ref：无法判断是 branch/tag/commit。
			// 保守视为 pinned（不前进），保留旧行为；RefKind 留空使 IsPinned() 正确。
		}
	}
	valid := o.Source != ""
	return OriginRead{Origin: o, Class: classifyOrigin(o), HasFile: true, Valid: valid}
}

// classifyOrigin 把已知 origin 映射到 update 分类。
func classifyOrigin(o SkillOrigin) UpdateClass {
	if o.SourceKind == SourceLocalSnapshot {
		return ClassSnapshot
	}
	// git / well-known：按 ref kind。
	if o.IsPinned() {
		return ClassPinned
	}
	return ClassTracking
}

// ResolveUniqueSkill 按全局唯一 name 解析 Registry 原件。
// 不存在 → *NameNotFoundError；跨多 category → *NameConflictError。
// 命中唯一原件时返回路径与 category。
func (r *Registry) ResolveUniqueSkill(name string) (*SkillResolution, error) {
	matches, err := r.FindSkillCategories(name)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, &NameNotFoundError{Name: name}
	}
	if len(matches) > 1 {
		cats := make([]string, 0, len(matches))
		paths := make([]string, 0, len(matches))
		for _, m := range matches {
			cats = append(cats, m.Category)
			paths = append(paths, m.Path)
		}
		sort.Strings(cats)
		return nil, &NameConflictError{Name: name, Categories: cats, Paths: paths}
	}
	return &SkillResolution{
		Name:     name,
		Category: matches[0].Category,
		Path:     matches[0].Path,
	}, nil
}

// FindConflicts 扫描整个 Registry，返回所有跨 category 同名的冲突。
// 不会删除、改名或选择任一副本（ADR 0010 迁移：仅报告）。
// 返回的切片按 name 排序，每个 conflict 的 Categories 也排序。
func (r *Registry) FindConflicts() ([]NameConflictError, error) {
	locations := make(map[string][]SkillMatch)

	skillsDir := r.skillsDir()
	categories, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, cat := range categories {
		if !cat.IsDir() {
			continue
		}
		catDir := filepath.Join(skillsDir, cat.Name())
		skills, err := os.ReadDir(catDir)
		if err != nil {
			continue
		}
		for _, s := range skills {
			if !s.IsDir() || s.Name() == ".gitkeep" {
				continue
			}
			name := s.Name()
			locations[name] = append(locations[name], SkillMatch{
				Path:     filepath.Join(catDir, name),
				Category: cat.Name(),
			})
		}
	}

	var conflicts []NameConflictError
	for name, locs := range locations {
		if len(locs) < 2 {
			continue
		}
		cats := make([]string, 0, len(locs))
		paths := make([]string, 0, len(locs))
		for _, l := range locs {
			cats = append(cats, l.Category)
			paths = append(paths, l.Path)
		}
		sort.Strings(cats)
		conflicts = append(conflicts, NameConflictError{
			Name:       name,
			Categories: cats,
			Paths:      paths,
		})
	}
	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i].Name < conflicts[j].Name
	})
	return conflicts, nil
}

// ListAllOriginals 遍历整个 Registry，对每个原件读取 origin 并分类。
// 用于 update 默认范围、doctor 诊断、list registry view。
func (r *Registry) ListAllOriginals() ([]RegistryOriginal, error) {
	skillsDir := r.skillsDir()
	categories, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var originals []RegistryOriginal
	for _, cat := range categories {
		if !cat.IsDir() {
			continue
		}
		catDir := filepath.Join(skillsDir, cat.Name())
		skills, err := os.ReadDir(catDir)
		if err != nil {
			continue
		}
		for _, s := range skills {
			if !s.IsDir() || s.Name() == ".gitkeep" {
				continue
			}
			dir := filepath.Join(catDir, s.Name())
			read := r.ReadOrigin(dir)
			originals = append(originals, RegistryOriginal{
				Name:     s.Name(),
				Category: cat.Name(),
				Path:     dir,
				Class:    read.Class,
				Origin:   read.Origin,
			})
		}
	}
	// 稳定输出：按 category 然后 name 排序。
	sort.Slice(originals, func(i, j int) bool {
		if originals[i].Category != originals[j].Category {
			return originals[i].Category < originals[j].Category
		}
		return originals[i].Name < originals[j].Name
	})
	return originals, nil
}
