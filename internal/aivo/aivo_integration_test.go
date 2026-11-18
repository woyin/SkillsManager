// internal/aivo/aivo_integration_test.go
package aivo

import (
	"os"
	"testing"
)

// TestIntegrationRealAivo 是 opt-in 集成测试：仅在 SM_AIVO_INTEGRATION=1 时
// 调用真实 aivo（可能触网、读取本机凭证）。默认不执行。
// 只输出计数，不输出 key 名称、ID 或 base_url 等个人数据。
func TestIntegrationRealAivo(t *testing.T) {
	if os.Getenv("SM_AIVO_INTEGRATION") != "1" {
		t.Skip("set SM_AIVO_INTEGRATION=1 to run against the real aivo binary")
	}

	info := Detect()
	if !info.Installed {
		t.Skip("aivo not installed")
	}
	t.Logf("aivo detected: version=%q", info.Version)

	keys := ListKeys()
	t.Logf("keys=%d active=%v", len(keys), ActiveKeyFromKeys(keys) != nil)

	stats := GetStats()
	if stats == nil {
		t.Log("stats unavailable")
		return
	}
	t.Logf("stats: sessions=%d models=%d tokens=%d", stats.Sessions, stats.Models, stats.TotalTokens)
}
