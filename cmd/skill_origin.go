// skill_origin.go 记录 Direct Install 写入 registry 的技能来源，
// 使 copy 入库（无 .git）的技能仍可通过 source cache 刷新。
//
// Input: encoding/json, fmt, os, path/filepath, strings, time
// Output: const skillOriginFile, type skillOrigin, func writeSkillOrigin, func readSkillOrigin, func replaceSkillDir, func rollbackSkillDir, func skillRelForLint
// Pos: 数据层-skill来源元数据
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// skillOriginFile 是写入每个 registry 技能目录的 provenance 文件名。
// 以 . 开头，避免被 agent 当作技能内容；也不应出现在 SKILL.md 要求里。
const skillOriginFile = ".sm-origin.json"

// skillOrigin 描述技能从哪个远程源物化而来。
// Source+Ref 用于定位 DataDir/sources 缓存；RelPath 是技能在克隆内的相对路径。
type skillOrigin struct {
	Source    string    `json:"source"`
	Ref       string    `json:"ref,omitempty"`
	RelPath   string    `json:"rel_path"`
	Commit    string    `json:"commit,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

func skillOriginPath(skillDir string) string {
	return filepath.Join(skillDir, skillOriginFile)
}

func writeSkillOrigin(skillDir string, origin skillOrigin) error {
	origin.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(origin, "", "  ")
	if err != nil {
		return err
	}
	tmp := skillOriginPath(skillDir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, skillOriginPath(skillDir))
}

func readSkillOrigin(skillDir string) (skillOrigin, bool) {
	var o skillOrigin
	data, err := os.ReadFile(skillOriginPath(skillDir))
	if err != nil {
		return o, false
	}
	if err := json.Unmarshal(data, &o); err != nil || o.Source == "" {
		return o, false
	}
	return o, true
}

// replaceSkillDir 用 src 内容覆盖 dest（先写旁路再原子替换）。
// keepBackup=true 时保留 dest.bak-sm，由调用方决定何时删除或回滚。
// 返回 backup 路径（无旧 dest 时为空）。
func replaceSkillDir(src, dest string, keepBackup bool) (backup string, err error) {
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(parent, ".sm-skill-*")
	if err != nil {
		return "", err
	}
	// CopyDir 要求 dest 不存在；tmp 已存在为空目录 → 删掉再 copy
	if err := os.Remove(tmp); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	if err := copySkillDir(src, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	// 去掉可能随 copy 带入的 origin；调用方会重写
	_ = os.Remove(skillOriginPath(tmp))

	backup = dest + ".bak-sm"
	_ = os.RemoveAll(backup)
	hadDest := false
	if _, err := os.Lstat(dest); err == nil {
		hadDest = true
		if err := os.Rename(dest, backup); err != nil {
			os.RemoveAll(tmp)
			return "", err
		}
	} else {
		backup = ""
	}
	if err := os.Rename(tmp, dest); err != nil {
		if hadDest {
			_ = os.Rename(backup, dest)
		}
		os.RemoveAll(tmp)
		return "", err
	}
	if hadDest && !keepBackup {
		os.RemoveAll(backup)
		backup = ""
	}
	return backup, nil
}

// rollbackSkillDir 把 keepBackup 留下的 backup 还原为 dest。
func rollbackSkillDir(dest, backup string) error {
	if backup == "" {
		return fmt.Errorf("no backup to roll back for %s", dest)
	}
	_ = os.RemoveAll(dest)
	if err := os.Rename(backup, dest); err != nil {
		return err
	}
	return nil
}

// skillRelForLint 返回 registry 技能绝对路径对应的相对路径（skills/ 下），
// 供 LintSkill 使用；不在 registry 内时返回 ""。
func skillRelForLint(skillAbs string) string {
	skillsRoot := filepath.Join(RegistryDir, "skills")
	try := func(skill, root string) string {
		rel, err := filepath.Rel(root, skill)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
		return rel
	}
	if rel := try(skillAbs, skillsRoot); rel != "" {
		return rel
	}
	absSkill, err1 := filepath.EvalSymlinks(skillAbs)
	absRoot, err2 := filepath.EvalSymlinks(skillsRoot)
	if err1 != nil || err2 != nil {
		return ""
	}
	return try(absSkill, absRoot)
}
