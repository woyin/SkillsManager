package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFetchAPIBodyCacheHit 用 httptest server 计数，验证第二次同 endpoint
// 走缓存而非网络。
func TestFetchAPIBodyCacheHit(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
	}))
	defer srv.Close()

	oldBase := skillsShAPIBase
	skillsShAPIBase = srv.URL
	defer func() { skillsShAPIBase = oldBase }()

	DataDir = t.TempDir()

	endpoint := "/test?q=1"
	if _, err := fetchAPIBody(endpoint, "tok", false); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := fetchAPIBody(endpoint, "tok", false); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 network call (second served from cache), got %d", calls)
	}

	// refresh=true 强制再请求一次。
	if _, err := fetchAPIBody(endpoint, "tok", true); err != nil {
		t.Fatalf("refresh call: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls after refresh, got %d", calls)
	}
}

// TestFetchAPIBodyOfflineFallback 验证网络失败时降级用过期缓存。
func TestFetchAPIBodyOfflineFallback(t *testing.T) {
	// 先写一份过期缓存。
	DataDir = t.TempDir()
	endpoint := "/offline"
	key := cacheKey(endpoint)
	cachePath := filepath.Join(DataDir, "cache", "browse", key)
	cached := []byte(`[{"name":"cached"}]`)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, cached, 0644); err != nil {
		t.Fatal(err)
	}
	// 把 mtime 改到 TTL 之前，模拟过期。
	past := time.Now().Add(-30 * time.Minute)
	os.Chtimes(cachePath, past, past)

	// base 指向一个一定失败的地址。
	oldBase := skillsShAPIBase
	skillsShAPIBase = "http://127.0.0.1:0" // 端口 0 不可达
	defer func() { skillsShAPIBase = oldBase }()

	body, err := fetchAPIBody(endpoint, "tok", false)
	if err != nil {
		t.Fatalf("expected offline fallback to cached body, got error: %v", err)
	}
	if string(body) != string(cached) {
		t.Errorf("got %q, want cached %q", body, cached)
	}
}

// TestCacheKeyStable 验证同 endpoint 同 key、不同 endpoint 不同 key。
func TestCacheKeyStable(t *testing.T) {
	a := cacheKey("/foo?q=1")
	b := cacheKey("/foo?q=1")
	c := cacheKey("/foo?q=2")
	if a != b {
		t.Error("same endpoint should produce same key")
	}
	if a == c {
		t.Error("different endpoints should produce different keys")
	}
}

// TestBrowseCachesValidJSONEndToEnd: 缓存的 body 必须可被 fetchSkillsAPI 解析。
func TestFetchSkillsAPIParsesCachedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 裸数组形式 + 一个真实字段。
		body, _ := json.Marshal([]map[string]any{
			{"name": "x", "description": "d", "total_installs": 5},
		})
		w.Write(body)
	}))
	defer srv.Close()

	oldBase := skillsShAPIBase
	skillsShAPIBase = srv.URL
	defer func() { skillsShAPIBase = oldBase }()
	DataDir = t.TempDir()

	first, err := fetchSkillsAPI("/e", "tok")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// 关掉 server，强制第二次只能用缓存。用 io.Discard 占位避免 nil。
	srv.CloseClientConnections()
	second, err := fetchSkillsAPI("/e", "tok")
	if err != nil {
		t.Fatalf("second (cached): %v", err)
	}
	if len(first) != len(second) || (len(first) > 0 && first[0].Name != second[0].Name) {
		t.Errorf("cached parse mismatch: first=%v second=%v", first, second)
	}
	// 引用 io 防止 import 被裁。
	_ = io.Discard
}
