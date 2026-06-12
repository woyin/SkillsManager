// internal/aivo/aivo_test.go
package aivo

import (
	"testing"
)

func TestDetect(t *testing.T) {
	info := Detect()
	// We don't require aivo to be installed in CI,
	// but if it is, version should be non-empty.
	if info.Installed && info.Version == "" {
		t.Log("aivo is installed but version detection returned empty (non-fatal)")
	}
	if !info.Installed {
		t.Log("aivo not installed; skipping further checks")
		return
	}

	t.Logf("aivo detected: path=%s version=%s", info.Path, info.Version)
}

func TestListKeys(t *testing.T) {
	info := Detect()
	if !info.Installed {
		t.Skip("aivo not installed")
	}

	keys := ListKeys()
	t.Logf("found %d keys", len(keys))
	for _, k := range keys {
		ping := "n/a"
		if k.PingOK != nil {
			if *k.PingOK {
				ping = "ok"
			} else {
				ping = "fail: " + k.PingMsg
			}
		}
		t.Logf("  %s (%s) active=%v ping=%s", k.Name, k.ID, k.Active, ping)
	}
}

func TestGetStats(t *testing.T) {
	info := Detect()
	if !info.Installed {
		t.Skip("aivo not installed")
	}

	stats := GetStats()
	if stats == nil {
		t.Fatal("GetStats returned nil")
	}
	t.Logf("stats: tokens=%d cached=%d sessions=%d models=%d",
		stats.TotalTokens, stats.TotalCached, stats.Sessions, stats.Models)
}

func TestGetActiveKey(t *testing.T) {
	info := Detect()
	if !info.Installed {
		t.Skip("aivo not installed")
	}

	key := GetActiveKey()
	if key == nil {
		t.Log("no active key found")
		return
	}
	t.Logf("active key: %s (%s) base_url=%s", key.Name, key.ID, key.BaseURL)
}
