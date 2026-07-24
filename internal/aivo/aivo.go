// Package aivo 封装与可选外部工具 aivo 的交互。
//
// aivo 提供 API key 管理与模型路由；它非 sm 必需，但若存在则被用于
// `sm doctor` / `sm status` / Web 仪表盘的状态展示。
//
// 所有调用都带 5 秒超时（cmdTimeout），避免 aivo 卡住时拖累 sm。
//
// Input: context, encoding/json, os/exec, strings, time
// Output: type Info, type Key, type Stats, func Detect, func ListKeys, func GetStats, func ActiveKeyFromKeys, func GetActiveKey
// Pos: 业务层-aivo集成
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package aivo

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// cmdTimeout 是所有 aivo 子进程调用的统一超时上限。
const cmdTimeout = 5 * time.Second

// Info 描述检测到的 aivo 状态。
type Info struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
}

// Key 表示一个 aivo API key（含 ping 状态）。
type Key struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Active  bool   `json:"active"`
	PingOK  *bool  `json:"ping_ok,omitempty"`
	PingMsg string `json:"ping_msg,omitempty"`
}

// Stats 描述 aivo 的使用统计。
type Stats struct {
	TotalTokens int64  `json:"total_tokens"`
	TotalCached int64  `json:"total_cached_tokens"`
	Sessions    int    `json:"sessions"`
	Models      int    `json:"models"`
	Raw         string `json:"-"`
}

// runCmd 在超时控制下运行命令并返回其合并输出。
func runCmd(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// Detect 检测 aivo 是否安装，返回其路径与版本。
func Detect() Info {
	path, err := exec.LookPath("aivo")
	if err != nil {
		return Info{Installed: false}
	}

	version := ""
	out, err := runCmd(cmdTimeout, "aivo", "-v")
	if err == nil {
		version = strings.TrimSpace(string(out))
		version = strings.TrimPrefix(version, "aivo ")
	}

	return Info{
		Installed: true,
		Path:      path,
		Version:   version,
	}
}

// ListKeys 列出全部已配置的 aivo API key，含 ping 状态。
// 任何错误（aivo 缺失、JSON 非法等）都返回 nil。
func ListKeys() []Key {
	out, err := runCmd(cmdTimeout, "aivo", "keys", "--ping", "--json")
	if err != nil {
		return nil
	}

	// 用匿名结构解耦 aivo 的外层格式，再映射到稳定的 Key。
	var raw []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		BaseURL string `json:"base_url"`
		Active  bool   `json:"active"`
		Ping    *struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		} `json:"ping"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil
	}

	keys := make([]Key, 0, len(raw))
	for _, r := range raw {
		k := Key{
			ID:      r.ID,
			Name:    r.Name,
			BaseURL: r.BaseURL,
			Active:  r.Active,
		}
		if r.Ping != nil {
			ok := r.Ping.OK
			k.PingOK = &ok
			k.PingMsg = r.Ping.Message
		}
		keys = append(keys, k)
	}
	return keys
}

// GetStats 从 `aivo stats --json` 获取使用统计；失败返回 nil。
func GetStats() *Stats {
	out, err := runCmd(cmdTimeout, "aivo", "stats", "--json")
	if err != nil {
		return nil
	}

	var raw struct {
		Totals struct {
			Tokens   int64 `json:"tokens"`
			Cached   int64 `json:"cached_tokens"`
			Sessions int   `json:"sessions"`
			Models   int   `json:"models"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil
	}

	return &Stats{
		TotalTokens: raw.Totals.Tokens,
		TotalCached: raw.Totals.Cached,
		Sessions:    raw.Totals.Sessions,
		Models:      raw.Totals.Models,
		Raw:         string(out),
	}
}

// ActiveKeyFromKeys 从已加载的 key 切片中返回当前激活项；无激活则返回 nil。
// 优先使用此函数，避免再次调用 ListKeys。
func ActiveKeyFromKeys(keys []Key) *Key {
	for i := range keys {
		if keys[i].Active {
			return &keys[i]
		}
	}
	return nil
}

// GetActiveKey 返回当前激活的 key；无则 nil。
// 若已持有 keys 切片，请改用 ActiveKeyFromKeys。
func GetActiveKey() *Key {
	return ActiveKeyFromKeys(ListKeys())
}
