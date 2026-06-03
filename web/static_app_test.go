package web

import (
	"strings"
	"testing"
)

func TestDashboardUsesCheckAPI(t *testing.T) {
	data, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("Read app.js failed: %v", err)
	}
	app := string(data)

	if !strings.Contains(app, "fetchJSON('/api/check')") {
		t.Fatalf("Dashboard should call /api/check")
	}
	if !strings.Contains(app, "Health") {
		t.Fatalf("Dashboard should render health status")
	}
	if !strings.Contains(app, "renderIssueList") {
		t.Fatalf("Dashboard should render check issues")
	}
}

func TestDashboardRendersRegistryMetadata(t *testing.T) {
	data, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("Read app.js failed: %v", err)
	}
	app := string(data)

	for _, expected := range []string{"skill_details", "mcp_details", "Source", "Last Updated"} {
		if !strings.Contains(app, expected) {
			t.Fatalf("Dashboard should render registry metadata %q", expected)
		}
	}
}

func TestDashboardRendersSortableFilterableHistory(t *testing.T) {
	data, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("Read app.js failed: %v", err)
	}
	app := string(data)

	for _, expected := range []string{"history-filter", "history-sort", "applyHistoryControls", "data-history-control"} {
		if !strings.Contains(app, expected) {
			t.Fatalf("Dashboard should support sortable/filterable history via %q", expected)
		}
	}
}
