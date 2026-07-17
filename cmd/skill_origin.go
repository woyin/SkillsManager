// skill_origin.go 记录 Direct Install 写入 registry 的技能来源，
// 使 copy 入库（无 .git）的技能仍可通过 source cache 刷新。
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// replaceSkillDir 用 src 内容覆盖 dest（先写到旁路目录再原子替换），
// 保留/不依赖 dest 是否已存在。用于 registry 回写。
func replaceSkillDir(src, dest string) error {
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".sm-skill-*")
	if err != nil {
		return err
	}
	// CopyDir 要求 dest 不存在；tmp 已存在为空目录 → 删掉再 copy
	if err := os.Remove(tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := copySkillDir(src, tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	// 去掉可能随 copy 带入的 origin；调用方会重写
	_ = os.Remove(skillOriginPath(tmp))

	backup := dest + ".bak-sm"
	_ = os.RemoveAll(backup)
	if _, err := os.Lstat(dest); err == nil {
		if err := os.Rename(dest, backup); err != nil {
			os.RemoveAll(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Rename(backup, dest)
		os.RemoveAll(tmp)
		return err
	}
	os.RemoveAll(backup)
	return nil
}
