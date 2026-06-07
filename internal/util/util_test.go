package util

import "testing"

func TestSanitizeHostname(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase letters and digits pass through", "abc123", "abc123"},
		{"uppercase is lowercased", "ABC", "abc"},
		{"dots become dashes", "postgres.prod", "postgres-prod"},
		{"non-alphanumeric collapses to single dash", "a@@@b", "a-b"},
		{"leading and trailing separators trimmed", "--abc--", "abc"},
		{"run of separators collapses", "a___---b", "a-b"},
		{"all separators sanitize to empty", "@@@", ""},
		{"empty input stays empty", "", ""},
		{"spaces become dashes then collapse", "my  project", "my-project"},
		{"mixed", "My_Project.01!", "my-project-01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeHostname(tt.input); got != tt.want {
				t.Errorf("SanitizeHostname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
