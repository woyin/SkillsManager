// internal/aivo/aivo.go
package aivo

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

const cmdTimeout = 5 * time.Second

// Info holds detected aivo state.
type Info struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
}

// Key represents an aivo API key.
type Key struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Active  bool   `json:"active"`
	PingOK  *bool  `json:"ping_ok,omitempty"`
	PingMsg string `json:"ping_msg,omitempty"`
}

// Stats holds aivo usage statistics.
type Stats struct {
	TotalTokens int64  `json:"total_tokens"`
	TotalCached int64  `json:"total_cached_tokens"`
	Sessions    int    `json:"sessions"`
	Models      int    `json:"models"`
	Raw         string `json:"-"`
}

// runCmd runs a command with a timeout and returns its combined output.
func runCmd(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// Detect checks if aivo is installed and returns its path and version.
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

// ListKeys returns all configured aivo API keys with ping status.
func ListKeys() []Key {
	out, err := runCmd(cmdTimeout, "aivo", "keys", "--ping", "--json")
	if err != nil {
		return nil
	}

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

// GetStats retrieves usage statistics from aivo stats --json.
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

// ActiveKeyFromKeys returns the active key from an already-loaded slice,
// or nil if none is active. Use this to avoid a second ListKeys call.
func ActiveKeyFromKeys(keys []Key) *Key {
	for i := range keys {
		if keys[i].Active {
			return &keys[i]
		}
	}
	return nil
}

// GetActiveKey returns the currently active key, or nil if none.
// Prefer ActiveKeyFromKeys when you already have a keys slice.
func GetActiveKey() *Key {
	return ActiveKeyFromKeys(ListKeys())
}
