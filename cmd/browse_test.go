// cmd/browse_test.go
package cmd

import (
	"os"
	"testing"
)

func TestBrowseCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "browse" {
			found = true
			break
		}
	}
	if !found {
		t.Error("browse command not registered")
	}
}

func TestParseSkillsFromHTML(t *testing.T) {
	html := `
		<a href="/vercel-labs/skills/find-skills">find-skills</a>
		<a href="/anthropics/skills/frontend-design">frontend-design</a>
		<a href="/vercel-labs/agent-skills/web-design-guidelines">web-design</a>
		<a href="/docs/api">API docs</a>
		<a href="/topic/react">React</a>
		<a href="/_next/static/chunks/abc.js">chunk</a>
		<a href="/about">About</a>
	`

	skills, err := parseSkillsFromHTML(html)
	if err != nil {
		t.Fatalf("parseSkillsFromHTML failed: %v", err)
	}

	if len(skills) < 3 {
		t.Errorf("expected at least 3 skills, got %d", len(skills))
	}

	// Check that non-skill paths are filtered
	for _, s := range skills {
		if s.Source == "docs" || s.Source == "topic" || s.Source == "_next" || s.Source == "about" {
			t.Errorf("non-skill path should be filtered: %s", s.Source)
		}
	}

	// Check specific skills
	found := map[string]bool{}
	for _, s := range skills {
		found[s.Source+"/"+s.Name] = true
	}

	expected := []string{
		"vercel-labs/skills/find-skills",
		"anthropics/skills/frontend-design",
		"vercel-labs/agent-skills/web-design-guidelines",
	}
	for _, e := range expected {
		if !found[e] {
			t.Errorf("expected skill %q not found in parsed results", e)
		}
	}
}

func TestParseSkillsFromHTMLDeduplication(t *testing.T) {
	html := `
		<a href="/vercel-labs/skills/find-skills">link1</a>
		<a href="/vercel-labs/skills/find-skills">link2</a>
		<a href="/vercel-labs/skills/find-skills">link3</a>
	`

	skills, err := parseSkillsFromHTML(html)
	if err != nil {
		t.Fatalf("parseSkillsFromHTML failed: %v", err)
	}

	count := 0
	for _, s := range skills {
		if s.Name == "find-skills" && s.Source == "vercel-labs/skills" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 entry for vercel-labs/skills/find-skills, got %d", count)
	}
}

func TestFormatInstalls(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0K"},
		{1500, "1.5K"},
		{534500, "534.5K"},
		{2000000, "2.0M"},
		{1234567, "1.2M"},
	}

	for _, tt := range tests {
		got := formatInstalls(tt.input)
		if got != tt.want {
			t.Errorf("formatInstalls(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetSkillsToken(t *testing.T) {
	// Save and restore env vars
	origSKILLS, hadSKILLS := os.LookupEnv("SKILLS_SH_TOKEN")
	origVERCEL, hadVERCEL := os.LookupEnv("VERCEL_OIDC_TOKEN")
	defer func() {
		if hadSKILLS {
			os.Setenv("SKILLS_SH_TOKEN", origSKILLS)
		} else {
			os.Unsetenv("SKILLS_SH_TOKEN")
		}
		if hadVERCEL {
			os.Setenv("VERCEL_OIDC_TOKEN", origVERCEL)
		} else {
			os.Unsetenv("VERCEL_OIDC_TOKEN")
		}
	}()

	// Test SKILLS_SH_TOKEN takes priority
	os.Setenv("SKILLS_SH_TOKEN", "skills-token")
	os.Setenv("VERCEL_OIDC_TOKEN", "vercel-token")
	if got := getSkillsToken(); got != "skills-token" {
		t.Errorf("expected SKILLS_SH_TOKEN to take priority, got %q", got)
	}

	// Test fallback to VERCEL_OIDC_TOKEN
	os.Unsetenv("SKILLS_SH_TOKEN")
	if got := getSkillsToken(); got != "vercel-token" {
		t.Errorf("expected VERCEL_OIDC_TOKEN fallback, got %q", got)
	}

	// Test empty when neither is set
	os.Unsetenv("VERCEL_OIDC_TOKEN")
	if got := getSkillsToken(); got != "" {
		t.Errorf("expected empty token, got %q", got)
	}
}

func TestParseSkillsFromHTMLEmptyInput(t *testing.T) {
	skills, err := parseSkillsFromHTML("")
	if err != nil {
		t.Fatalf("parseSkillsFromHTML failed on empty input: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills from empty HTML, got %d", len(skills))
	}
}

func TestParseSkillsFromHTMLComplexPaths(t *testing.T) {
	html := `
		<a href="/microsoft/azure-skills/microsoft-foundry">Microsoft Foundry</a>
		<a href="/agentspace-so/runcomfy-agent-skills/video-edit">Video Edit</a>
		<a href="/julius-brussee/caveman/caveman">Caveman</a>
		<a href="/larksuite/cli/lark-doc">Lark Doc</a>
		<a href="/search?q=test">search</a>
		<a href="/official">official</a>
	`

	skills, err := parseSkillsFromHTML(html)
	if err != nil {
		t.Fatalf("parseSkillsFromHTML failed: %v", err)
	}

	// Should have 4 skills (excluding search and official)
	if len(skills) != 4 {
		names := []string{}
		for _, s := range skills {
			names = append(names, s.Source+"/"+s.Name)
		}
		t.Errorf("expected 4 skills, got %d: %v", len(skills), names)
	}
}
