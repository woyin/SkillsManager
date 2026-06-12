package cmd

import (
	"testing"
)

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		name   string
		input  int64
		expect string
	}{
		{"zero", 0, "0"},
		{"small", 42, "42"},
		{"thousands", 1500, "1.5K"},
		{"millions", 2500000, "2.5M"},
		{"billions", 3000000000, "3.0B"},
		{"exact thousand", 1000, "1.0K"},
		{"exact million", 1000000, "1.0M"},
		{"exact billion", 1000000000, "1.0B"},
		{"under thousand", 999, "999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTokenCount(tt.input)
			if result != tt.expect {
				t.Errorf("formatTokenCount(%d) = %q, want %q", tt.input, result, tt.expect)
			}
		})
	}
}
