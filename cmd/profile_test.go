package cmd

import (
	"testing"
)

func TestFormatList(t *testing.T) {
	tests := []struct {
		name   string
		items  []string
		expect string
	}{
		{"empty", nil, "(none)"},
		{"empty slice", []string{}, "(none)"},
		{"single", []string{"a"}, "a"},
		{"multiple", []string{"a", "b", "c"}, "a, b, c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatList(tt.items)
			if result != tt.expect {
				t.Errorf("formatList(%v) = %q, want %q", tt.items, result, tt.expect)
			}
		})
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{"single", "a", []string{"a"}},
		{"multiple", "a,b,c", []string{"a", "b", "c"}},
		{"spaces", " a , b , c ", []string{"a", "b", "c"}},
		{"empty parts", "a,,b,", []string{"a", "b"}},
		{"just spaces", "  ,  ,  ", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input)
			if len(result) != len(tt.expect) {
				t.Fatalf("splitAndTrim(%q) returned %d items, want %d: %v", tt.input, len(result), len(tt.expect), result)
			}
			for i, v := range result {
				if v != tt.expect[i] {
					t.Errorf("splitAndTrim(%q)[%d] = %q, want %q", tt.input, i, v, tt.expect[i])
				}
			}
		})
	}
}
