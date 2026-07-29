package lockfile

import "testing"

// TestResolveAlias verifies known aliases are resolved.
func TestResolveAlias(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"coinbase/agentWallet", "coinbase/agentic-wallet-skills"},
		{"vercel-labs/vercel-skills", "vercel-labs/agent-skills"},
		{"owner/repo", "owner/repo"}, // not an alias, unchanged
		{"./local-path", "./local-path"},
	}

	for _, tt := range tests {
		got := ResolveAlias(tt.input)
		if got != tt.want {
			t.Errorf("ResolveAlias(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestClassifySource verifies source type and URL classification.
func TestClassifySource(t *testing.T) {
	tests := []struct {
		source   string
		wantType string
		wantURL  string
	}{
		{"owner/repo", "github", "https://github.com/owner/repo"},
		{"https://github.com/owner/repo", "git", "https://github.com/owner/repo"},
		{"git@github.com:owner/repo.git", "git", ""},
		{"./local-skill", "local", ""},
		{"/abs/path/skill", "local", ""},
	}

	for _, tt := range tests {
		meta := ClassifySource(tt.source)
		if meta.SourceType != tt.wantType {
			t.Errorf("ClassifySource(%q).SourceType = %q, want %q", tt.source, meta.SourceType, tt.wantType)
		}
		if tt.wantURL != "" && meta.SourceURL != tt.wantURL {
			t.Errorf("ClassifySource(%q).SourceURL = %q, want %q", tt.source, meta.SourceURL, tt.wantURL)
		}
	}
}

// TestClassifySourceTreeURL verifies tree URL classification keeps the base repo URL.
func TestClassifySourceTreeURL(t *testing.T) {
	meta := ClassifySource("owner/repo/tree/main/skills/my-skill")
	if meta.SourceType != "github" {
		t.Errorf("SourceType = %q, want github", meta.SourceType)
	}
	if meta.SourceURL != "https://github.com/owner/repo" {
		t.Errorf("SourceURL = %q, want https://github.com/owner/repo", meta.SourceURL)
	}
}
