// internal/aivo/aivo_test.go
package aivo

import (
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

// stubAivo 替换执行 seam：lookPath 成功、runAivo 返回给定输出与错误。
// wantName/wantArgs 非空时逐次校验命令契约；返回 runAivo 的调用计数指针。
func stubAivo(t *testing.T, out []byte, runErr error, wantName string, wantArgs []string) *int {
	t.Helper()
	oldLook, oldRun := lookPath, runAivo
	t.Cleanup(func() { lookPath, runAivo = oldLook, oldRun })

	lookPath = func(string) (string, error) { return "/stub/aivo", nil }
	calls := 0
	runAivo = func(timeout time.Duration, name string, args ...string) ([]byte, error) {
		calls++
		if wantName != "" && name != wantName {
			t.Errorf("command name = %q, want %q", name, wantName)
		}
		if wantArgs != nil && !reflect.DeepEqual(args, wantArgs) {
			t.Errorf("command args = %#v, want %#v", args, wantArgs)
		}
		if timeout != cmdTimeout {
			t.Errorf("command timeout = %v, want %v", timeout, cmdTimeout)
		}
		return out, runErr
	}
	return &calls
}

func TestParseKeysValidPayload(t *testing.T) {
	payload := []byte(`[
  {"id":"id-1","name":"first","base_url":"https://one.invalid","active":false,
   "ping":{"ok":true,"message":"reachable"}},
  {"id":"id-2","name":"second","base_url":"https://two.invalid","active":true}
]`)

	keys, err := parseKeys(payload)
	if err != nil {
		t.Fatalf("parseKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("len(keys) = %d, want 2", len(keys))
	}

	first := keys[0]
	if first.ID != "id-1" || first.Name != "first" || first.BaseURL != "https://one.invalid" || first.Active {
		t.Errorf("first key = %#v, want id-1/first/https://one.invalid/inactive", first)
	}
	if first.PingOK == nil {
		t.Fatal("first PingOK = nil, want non-nil when ping is present")
	}
	if !*first.PingOK || first.PingMsg != "reachable" {
		t.Errorf("first ping = ok:%t msg:%q, want ok:true msg:reachable", *first.PingOK, first.PingMsg)
	}

	second := keys[1]
	if second.ID != "id-2" || second.Name != "second" || !second.Active {
		t.Errorf("second key = %#v, want id-2/second/active", second)
	}
	if second.PingOK != nil {
		t.Errorf("second PingOK = %t, want nil when ping is absent", *second.PingOK)
	}
}

func TestParseKeysInvalidJSONReportsError(t *testing.T) {
	keys, err := parseKeys([]byte(`{"not":"an array"}`))
	if err == nil {
		t.Fatal("parseKeys err = nil, want unmarshal error")
	}
	if keys != nil {
		t.Fatalf("keys = %#v, want nil", keys)
	}
}

func TestParseStatsTotalsPayload(t *testing.T) {
	payload := []byte(`{"totals":{"tokens":1234,"cached_tokens":56,"sessions":7,"models":3}}`)

	stats, err := parseStats(payload)
	if err != nil {
		t.Fatalf("parseStats: %v", err)
	}
	if stats.TotalTokens != 1234 || stats.TotalCached != 56 || stats.Sessions != 7 || stats.Models != 3 {
		t.Fatalf("stats = %#v, want tokens:1234 cached:56 sessions:7 models:3", stats)
	}
	if stats.Raw != string(payload) {
		t.Errorf("stats.Raw = %q, want raw payload", stats.Raw)
	}
}

func TestParseStatsInvalidJSONReportsError(t *testing.T) {
	stats, err := parseStats([]byte("not json"))
	if err == nil {
		t.Fatal("parseStats err = nil, want unmarshal error")
	}
	if stats != nil {
		t.Fatalf("stats = %#v, want nil", stats)
	}
}

func TestParseVersionStripsPrefix(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "prefixed", in: "aivo 1.2.3\n", want: "1.2.3"},
		{name: "bare", in: "1.2.3\n", want: "1.2.3"},
		{name: "empty", in: "   \n", want: ""},
		{name: "prefix only", in: "aivo", want: "aivo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseVersion(tc.in); got != tc.want {
				t.Errorf("parseVersion(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDetectReportsNotInstalledWhenLookPathFails(t *testing.T) {
	oldLook, oldRun := lookPath, runAivo
	t.Cleanup(func() { lookPath, runAivo = oldLook, oldRun })

	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	runCalls := 0
	runAivo = func(_ time.Duration, _ string, _ ...string) ([]byte, error) {
		runCalls++
		return nil, errors.New("must not run")
	}

	info := Detect()
	if info.Installed {
		t.Fatalf("Installed = true, want false")
	}
	if info.Path != "" || info.Version != "" {
		t.Fatalf("Path/Version = %q/%q, want empty", info.Path, info.Version)
	}
	if runCalls != 0 {
		t.Fatalf("runAivo calls = %d, want 0 when aivo is missing", runCalls)
	}
}

func TestDetectReportsVersionFromOutput(t *testing.T) {
	calls := stubAivo(t, []byte("aivo 1.2.3\n"), nil, "aivo", []string{"-v"})

	info := Detect()
	if !info.Installed {
		t.Fatal("Installed = false, want true")
	}
	if info.Path != "/stub/aivo" {
		t.Errorf("Path = %q, want /stub/aivo", info.Path)
	}
	if info.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", info.Version)
	}
	if *calls != 1 {
		t.Errorf("runAivo calls = %d, want 1", *calls)
	}
}

func TestListKeysPassesExactArgs(t *testing.T) {
	calls := stubAivo(t, []byte("[]"), nil, "aivo", []string{"keys", "--ping", "--json"})

	keys := ListKeys()
	if keys == nil {
		t.Fatal("ListKeys = nil, want empty slice on valid payload")
	}
	if len(keys) != 0 {
		t.Fatalf("len(keys) = %d, want 0", len(keys))
	}
	if *calls != 1 {
		t.Errorf("runAivo calls = %d, want 1", *calls)
	}
}

func TestGetStatsPassesExactArgs(t *testing.T) {
	calls := stubAivo(t,
		[]byte(`{"totals":{"tokens":5,"cached_tokens":0,"sessions":1,"models":1}}`),
		nil, "aivo", []string{"stats", "--json"})

	stats := GetStats()
	if stats == nil {
		t.Fatal("GetStats = nil, want stats")
	}
	if stats.TotalTokens != 5 || stats.Sessions != 1 {
		t.Errorf("stats = %#v, want tokens:5 sessions:1", stats)
	}
	if *calls != 1 {
		t.Errorf("runAivo calls = %d, want 1", *calls)
	}
}

func TestListKeysNonZeroExitReturnsNil(t *testing.T) {
	stubAivo(t, []byte("boom"), errors.New("exit status 1"), "aivo", []string{"keys", "--ping", "--json"})

	if keys := ListKeys(); keys != nil {
		t.Fatalf("ListKeys = %#v, want nil on command failure", keys)
	}
}

func TestGetStatsNonZeroExitReturnsNil(t *testing.T) {
	stubAivo(t, []byte("boom"), errors.New("exit status 1"), "aivo", []string{"stats", "--json"})

	if stats := GetStats(); stats != nil {
		t.Fatalf("GetStats = %#v, want nil on command failure", stats)
	}
}

func TestRunCmdTimeoutReturnsError(t *testing.T) {
	t.Setenv("AIVO_HELPER_MODE", "slow")

	start := time.Now()
	out, err := runCmd(150*time.Millisecond, os.Args[0], "-test.run=TestHelperAivoProcess")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("runCmd err = nil (output %q), want timeout error", out)
	}
	// helper 睡 2s；若超时没杀掉子进程，耗时会接近 2s。
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("runCmd took %v, want < 1.5s so the timeout kills the child", elapsed)
	}
}

func TestActiveKeyFromKeys(t *testing.T) {
	cases := []struct {
		name string
		keys []Key
		want string
	}{
		{name: "first active wins", keys: []Key{{Name: "a"}, {Name: "b", Active: true}, {Name: "c", Active: true}}, want: "b"},
		{name: "no active", keys: []Key{{Name: "a"}}, want: ""},
		{name: "empty", keys: nil, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ActiveKeyFromKeys(tc.keys)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("ActiveKeyFromKeys = %#v, want nil", got)
				}
				return
			}
			if got == nil || got.Name != tc.want {
				t.Fatalf("ActiveKeyFromKeys = %#v, want %q", got, tc.want)
			}
		})
	}
}

// TestHelperAivoProcess 是超时测试的子进程入口：仅在 AIVO_HELPER_MODE=slow 时睡眠，
// 其余情况立即跳过，避免影响正常测试运行。
func TestHelperAivoProcess(t *testing.T) {
	if os.Getenv("AIVO_HELPER_MODE") != "slow" {
		t.Skip("helper process only")
	}
	time.Sleep(2 * time.Second)
}
