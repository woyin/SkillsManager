package registry

import (
	"os"
	"path/filepath"
	"testing"
)

// writeMCPDef 在 dir 下写一个 name.json MCP 定义。
func writeMCPDef(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name+".json")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMCPServerTransports(t *testing.T) {
	dir := t.TempDir()
	def := writeMCPDef(t, dir, "mixed", `{
		"mcpServers": {
			"local": {"command": "npx", "args": ["-y", "foo"]},
			"web":   {"url": "https://api.example.com/mcp"},
			"legacy": {"type": "sse", "url": "https://old.example.com/sse"},
			"weird": {"foo": "bar"}
		}
	}`)

	got, err := MCPServerTransports(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byName := make(map[string]ServerTransport, len(got))
	for _, st := range got {
		byName[st.Server] = st
	}

	cases := map[string]struct {
		transport string
		detail    string
	}{
		"local":  {"stdio", "npx"},
		"web":    {"http", "https://api.example.com/mcp"},
		"legacy": {"sse", "https://old.example.com/sse"},
		"weird":  {"unknown", ""},
	}
	for name, want := range cases {
		st, ok := byName[name]
		if !ok {
			t.Errorf("missing server %q in result", name)
			continue
		}
		if st.Transport != want.transport || st.Detail != want.detail {
			t.Errorf("server %q = {Transport:%q, Detail:%q}, want {%q, %q}",
				name, st.Transport, st.Detail, want.transport, want.detail)
		}
	}

	// 排序确定性：结果按 server 名升序。
	for i := 1; i < len(got); i++ {
		if got[i-1].Server > got[i].Server {
			t.Errorf("result not sorted by server name: %q before %q", got[i-1].Server, got[i].Server)
		}
	}
}

func TestMCPServerTransportsErrors(t *testing.T) {
	dir := t.TempDir()

	// 缺失文件。
	if _, err := MCPServerTransports(filepath.Join(dir, "nope.json")); err == nil {
		t.Error("expected error for missing file")
	}

	// 缺 mcpServers。
	bad := writeMCPDef(t, dir, "noservers", `{"other": {}}`)
	if _, err := MCPServerTransports(bad); err == nil {
		t.Error("expected error for missing mcpServers")
	}

	// JSON 损坏。
	corrupt := writeMCPDef(t, dir, "corrupt", `{not json`)
	if _, err := MCPServerTransports(corrupt); err == nil {
		t.Error("expected error for corrupt JSON")
	}
}
