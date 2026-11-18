// origin_metadata.go 集中 legacy .sm-origin.json 的读取。
// 该文件记录 copy 入库（无 .git）技能的来源，是 update 发现路径的输入。
// Registry schema（registry.SkillOrigin）是另一种既有 schema：本文件不合并、
// 不迁移它们，只把 legacy JSON 解析收在一处，供 cmd 兼容适配复用。
//
// Input: encoding/json, os, path/filepath, time
// Output: const LegacyOriginFile, type OriginMetadata, func LegacyOriginPath, func ReadOriginMetadata
// Pos: 业务层-已安装技能 legacy 来源元数据
package updater

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// LegacyOriginFile 是写入每个 registry 技能目录的 provenance 文件名。
// 以 . 开头，避免被 agent 当作技能内容；也不应出现在 SKILL.md 要求里。
const LegacyOriginFile = ".sm-origin.json"

// OriginMetadata 是 legacy schema：Source+Ref 用于定位 DataDir/sources 缓存，
// RelPath 是技能在克隆内的相对路径，UpdatedAt 记录写入时间。
type OriginMetadata struct {
	Source    string    `json:"source"`
	Ref       string    `json:"ref,omitempty"`
	RelPath   string    `json:"rel_path"`
	Commit    string    `json:"commit,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// LegacyOriginPath 返回 skillDir 下 legacy provenance 文件的路径。
func LegacyOriginPath(skillDir string) string {
	return filepath.Join(skillDir, LegacyOriginFile)
}

// ReadOriginMetadata 读取 legacy provenance；文件缺失、损坏或无 source 时
// 返回 ok=false（保持既有“无 origin 即孤儿”的判定）。
func ReadOriginMetadata(skillDir string) (OriginMetadata, bool) {
	var o OriginMetadata
	data, err := os.ReadFile(LegacyOriginPath(skillDir))
	if err != nil {
		return o, false
	}
	if err := json.Unmarshal(data, &o); err != nil || o.Source == "" {
		return o, false
	}
	return o, true
}
