// Package registry 的 skill.go 实现技能的增删改查：
//   - 从 GitHub URL 克隆或从本地路径拷贝进入注册表；
//   - 按名称 / 分类 / 特殊目录移除；
//   - 列举（简略名列表或带详情的结构）；
//   - 路径解析。
//
// 关键不变量：
//   - 注册表中每个技能 = 注册表根目录下的 skills/<category>/<name>/；
//   - 安装到代理目录的内容是对注册表原始文件的符号链接；
//   - dest 不允许已存在（用于让调用方识别"已安装"）。
package registry

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ── 技能添加 ──

// AddSkill 把一个技能加入注册表。
//   - special 非空时，覆盖 category；
//   - GitHub URL 触发克隆，本地路径触发拷贝；
//   - skillNames 非空时，仅从克隆结果中抽取指定技能。
//
// AddSkill 是 AddSkillWithOptions 的便捷封装（采用默认选项）。
func (r *Registry) AddSkill(source, category, special string) error {
	_, err := r.AddSkillWithOptions(source, category, special, nil, false)
	return err
}

// AddSkillWithOptions 是 AddSkill 的扩展版本。
//   - skillNames：非空时仅从来源中抽取这些技能；
//   - copyMode：为 true 时拷贝文件而非保留 git 克隆。
//
// 返回实际入库的技能相对路径（形如 "global/my-skill"），供调用方做后续
// 处理（如 frontmatter lint）。无技能入库时切片为空。
func (r *Registry) AddSkillWithOptions(source, category, special string, skillNames []string, copyMode bool) ([]string, error) {
	name := skillNameForSource(source)
	if name == "" {
		return nil, fmt.Errorf("cannot determine skill name from source: %s", source)
	}

	// 决定目标分类目录：special 优先，其次显式 category。
	var destCategory string
	if special != "" {
		destCategory = special
	} else if category != "" {
		destCategory = category
	} else {
		return nil, fmt.Errorf("must specify category or --global/--codex/--claude")
	}

	if IsGitURL(source) {
		// 解析 tree url：可能是 owner/repo/tree/<branch>/<path>。
		repoURL, branch, subPath, isTree := ParseTreeURL(source)
		if !isTree {
			repoURL = NormalizeGitURL(source)
		}

		// 指定了子路径或具体技能名时，走"克隆到临时目录再抽取"流程。
		if subPath != "" || len(skillNames) > 0 {
			return r.cloneAndExtract(repoURL, branch, subPath, destCategory, skillNames, copyMode)
		}

		name, err := r.cloneAndAdd(repoURL, branch, destCategory, name, copyMode)
		if err != nil {
			return nil, err
		}
		return []string{name}, nil
	}
	// 本地路径：直接拷贝。
	name = skillNameForLocalDir(source, name)
	dest := filepath.Join(r.skillsDir(), destCategory, name)
	if err := r.copyDir(source, dest); err != nil {
		return nil, err
	}
	return []string{filepath.Join(destCategory, name)}, nil
}

func skillNameForSource(source string) string {
	return usableSkillName("", SkillNameFromPath(source))
}

func skillNameForLocalDir(source, fallback string) string {
	fm := parseSkillFrontmatter(filepath.Join(source, "SKILL.md"))
	return usableSkillName(fm.Name, fallback)
}

func usableSkillName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) || filepath.Base(name) != name {
		return strings.TrimSpace(fallback)
	}
	return name
}

// cloneAndAdd 把仓库克隆到临时目录，读取根 SKILL.md 命名后加入注册表。
// 返回入库技能的相对路径（"category/name"）。
func (r *Registry) cloneAndAdd(repoURL, branch, category, fallback string, copyMode bool) (string, error) {
	tmpDir, err := os.MkdirTemp("", "sm-clone-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneDest := filepath.Join(tmpDir, "repo")
	if err := CloneRepoWithBranch(repoURL, branch, cloneDest); err != nil {
		return "", fmt.Errorf("cloning %s: %w", repoURL, err)
	}

	name := skillNameForLocalDir(cloneDest, fallback)
	dest := filepath.Join(r.skillsDir(), category, name)
	relDest := filepath.Join(category, name)
	if copyMode {
		os.RemoveAll(filepath.Join(cloneDest, ".git"))
		return relDest, r.copyDir(cloneDest, dest)
	}
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("destination already exists: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return "", err
	}
	if err := os.Rename(cloneDest, dest); err != nil {
		return relDest, r.copyDir(cloneDest, dest)
	}
	return relDest, nil
}

// cloneAndExtract 把仓库克隆到临时目录，然后拷贝指定的子路径或技能。
// 返回入库技能的相对路径（"category/name"）。
func (r *Registry) cloneAndExtract(repoURL, branch, subPath, category string, skillNames []string, copyMode bool) ([]string, error) {
	tmpDir, err := os.MkdirTemp("", "sm-clone-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneDest := filepath.Join(tmpDir, "repo")
	if err := CloneRepoWithBranch(repoURL, branch, cloneDest); err != nil {
		return nil, fmt.Errorf("cloning %s: %w", repoURL, err)
	}

	// 指定了子路径：直接拷贝该路径。
	if subPath != "" {
		src := filepath.Join(cloneDest, subPath)
		if _, err := os.Stat(src); err != nil {
			return nil, fmt.Errorf("path %q not found in repository", subPath)
		}
		name := skillNameForLocalDir(src, filepath.Base(src))
		dest := filepath.Join(r.skillsDir(), category, name)
		if err := r.copyDir(src, dest); err != nil {
			return nil, err
		}
		return []string{filepath.Join(category, name)}, nil
	}

	var added []string

	// 指定了技能名：先发现再选择性拷贝。
	if len(skillNames) > 0 {
		discovered, err := DiscoverSkills(cloneDest)
		if err != nil {
			return nil, fmt.Errorf("discovering skills: %w", err)
		}

		// 以技能名为键建立索引，避免内层线性查找。
		discoveredMap := make(map[string]DiscoveredSkill, len(discovered))
		for _, s := range discovered {
			discoveredMap[s.Name] = s
		}

		for _, name := range skillNames {
			// "*" 表示拷贝全部已发现的技能。
			if name == "*" {
				for _, s := range discovered {
					skillDest := filepath.Join(r.skillsDir(), category, s.Name)
					if err := r.copyDir(s.Path, skillDest); err != nil {
						fmt.Fprintf(os.Stderr, "warning: skipping skill %q: %v\n", s.Name, err)
						continue
					}
					added = append(added, filepath.Join(category, s.Name))
				}
				return added, nil
			}
			s, ok := discoveredMap[name]
			if !ok {
				return nil, fmt.Errorf("skill %q not found in repository", name)
			}
			skillDest := filepath.Join(r.skillsDir(), category, s.Name)
			if err := r.copyDir(s.Path, skillDest); err != nil {
				return nil, fmt.Errorf("copying skill %q: %w", name, err)
			}
			added = append(added, filepath.Join(category, s.Name))
		}
		return added, nil
	}

	return added, nil
}

// cloneAndCopy 克隆仓库并拷贝结果（去掉 .git 目录）。
func (r *Registry) cloneAndCopy(repoURL, branch, dest string) error {
	tmpDir, err := os.MkdirTemp("", "sm-clone-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneDest := filepath.Join(tmpDir, "repo")
	if err := CloneRepoWithBranch(repoURL, branch, cloneDest); err != nil {
		return fmt.Errorf("cloning %s: %w", repoURL, err)
	}

	// 拷贝前先删除 .git 目录，避免把版本控制元数据带进注册表。
	os.RemoveAll(filepath.Join(cloneDest, ".git"))
	return r.copyDir(cloneDest, dest)
}

// ── 技能移除 ──

// RemoveSkill 从注册表中移除一个技能。
// special / category 任一非空时限定搜索范围；两者都为空时全表搜索。
func (r *Registry) RemoveSkill(name, category, special string) error {
	var dir string
	if special != "" {
		dir = filepath.Join(r.skillsDir(), special, name)
	} else if category != "" {
		dir = filepath.Join(r.skillsDir(), category, name)
	} else {
		// 全分类搜索
		found, err := r.FindSkillDir(name)
		if err != nil {
			return err
		}
		if found == "" {
			return fmt.Errorf("skill %q not found in registry", name)
		}
		dir = found
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("skill not found: %s", dir)
	}
	return os.RemoveAll(dir)
}

// FindSkillDir 在所有分类目录下搜索名为 name 的技能，返回其路径；
// 未找到返回 ""。导出给 cmd/update 等包共享同一实现。
func (r *Registry) FindSkillDir(name string) (string, error) {
	path, _, err := r.FindSkillWithCategory(name)
	return path, err
}

// FindSkillWithCategory 与 FindSkillDir 类似，但同时返回技能所在的
// 分类目录（安装器需要它来决定目标工具）。两者在未找到时都返回 ""。
func (r *Registry) FindSkillWithCategory(name string) (path, category string, err error) {
	skillsDir := r.skillsDir()
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return "", "", err
	}
	for _, cat := range entries {
		if !cat.IsDir() {
			continue
		}
		candidate := filepath.Join(skillsDir, cat.Name(), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, cat.Name(), nil
		}
	}
	return "", "", nil
}

// ── 技能列举 ──

// ListSkills 返回按分类分组的全部技能名。
// 返回的 map 永不为 nil（注册表不存在时返回空 map）。
func (r *Registry) ListSkills() (map[string][]string, error) {
	result := make(map[string][]string)
	skillsDir := r.skillsDir()

	categories, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}

	for _, cat := range categories {
		if !cat.IsDir() {
			continue
		}
		skills, err := os.ReadDir(filepath.Join(skillsDir, cat.Name()))
		if err != nil {
			continue
		}
		for _, s := range skills {
			if s.IsDir() && s.Name() != ".gitkeep" {
				result[cat.Name()] = append(result[cat.Name()], s.Name())
			}
		}
	}
	return result, nil
}

// ListSkillDetails 返回带详情（路径、源 URL、最后更新时间）的技能列表。
//
// 性能：遍历时直接复用 os.ReadDir 返回的 DirEntry.Info()，避免对每个
// 技能再发一次 os.Stat 系统调用。
func (r *Registry) ListSkillDetails() (map[string][]ItemDetail, error) {
	result := make(map[string][]ItemDetail)
	skillsDir := r.skillsDir()

	categories, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}

	for _, cat := range categories {
		if !cat.IsDir() {
			continue
		}
		categoryDir := filepath.Join(skillsDir, cat.Name())
		skills, err := os.ReadDir(categoryDir)
		if err != nil {
			continue
		}
		for _, skill := range skills {
			if !skill.IsDir() || skill.Name() == ".gitkeep" {
				continue
			}
			path := filepath.Join(categoryDir, skill.Name())
			// 复用 entry 自带的 Info，省去一次 os.Stat。
			info, _ := skill.Info()
			result[cat.Name()] = append(result[cat.Name()], itemDetailFromInfo(skill.Name(), cat.Name(), path, info))
		}
		sort.Slice(result[cat.Name()], func(i, j int) bool {
			return result[cat.Name()][i].Name < result[cat.Name()][j].Name
		})
	}

	return result, nil
}

// GetSkillPath 返回注册表中某个技能的绝对路径。
// special / category 非空时限定范围；都为空时全表搜索。
func (r *Registry) GetSkillPath(name, category, special string) (string, error) {
	if special != "" {
		path := filepath.Join(r.skillsDir(), special, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("skill %q not found in %s", name, special)
	}
	if category != "" {
		path := filepath.Join(r.skillsDir(), category, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("skill %q not found in %s", name, category)
	}
	// 全表搜索
	found, err := r.FindSkillDir(name)
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("skill %q not found in registry", name)
	}
	return found, nil
}

// ── 技能与 MCP 列举共享的辅助函数 ──

// itemDetailFromInfo 用已获得的 FileInfo 构造一条 ItemDetail。
// info 为 nil 时 LastUpdated 留空。
func itemDetailFromInfo(name, category, path string, info os.FileInfo) ItemDetail {
	lastUpdated := ""
	if info != nil {
		lastUpdated = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
	}
	return ItemDetail{
		Name:        name,
		Category:    category,
		Path:        path,
		SourceURL:   gitRemoteURL(path),
		LastUpdated: lastUpdated,
	}
}

// itemDetail 是 itemDetailFromInfo 的兼容封装：当调用方未持有 FileInfo
// 时使用（例如 MCP 列举）。内部仍会做一次 os.Stat。
func itemDetail(name, category, path string) ItemDetail {
	info, _ := os.Stat(path)
	return itemDetailFromInfo(name, category, path, info)
}

// gitRemoteURL 读取 path/.git/config 中的 [remote "origin"] url 字段。
//
// 采用字节级行扫描，避免 string(data) 整段拷贝与 strings.Split 的
// 字符串数组分配，从而降低 ListSkillDetails 在大注册表下的 GC 压力。
func gitRemoteURL(path string) string {
	configPath := filepath.Join(path, ".git", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	inOrigin := false
	// 逐行扫描：每行用 bytes.IndexByte 切出（仍是 data 的子切片）。
	for {
		nl := bytes.IndexByte(data, '\n')
		var line []byte
		if nl < 0 {
			line = data
			data = nil
		} else {
			line = data[:nl]
			data = data[nl+1:]
		}
		if len(line) == 0 && nl < 0 {
			break
		}

		trimmed := bytes.TrimSpace(line)

		// 段落头（[section "name"]）：判断是否进入 origin 远端。
		if bytes.HasPrefix(trimmed, []byte("[")) {
			inOrigin = bytes.Equal(trimmed, []byte(`[remote "origin"]`))
			goto next
		}

		// 在 origin 段内，匹配 url = <value>。
		if inOrigin && bytes.HasPrefix(trimmed, []byte("url")) {
			if idx := bytes.IndexByte(trimmed, '='); idx >= 0 {
				return strings.TrimSpace(string(trimmed[idx+1:]))
			}
		}

	next:
		if nl < 0 {
			break
		}
	}
	return ""
}
