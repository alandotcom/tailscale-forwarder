package config

import "testing"

func TestParseServiceMapping(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    ServiceMapping
		wantErr bool
	}{
		{
			name:  "valid mapping",
			value: "postgres:5432:db.internal:5432",
			want:  ServiceMapping{Name: "postgres", SourcePort: 5432, TargetAddr: "db.internal", TargetPort: 5432},
		},
		{
			name:  "trims whitespace around name and addr",
			value: " redis :6379: cache.internal :6379",
			want:  ServiceMapping{Name: "redis", SourcePort: 6379, TargetAddr: "cache.internal", TargetPort: 6379},
		},
		{name: "too few fields", value: "postgres:5432:db.internal", wantErr: true},
		{name: "empty name", value: ":5432:db.internal:5432", wantErr: true},
		{name: "name with no usable hostname chars", value: "@@@:5432:db.internal:5432", wantErr: true},
		{name: "non-numeric source port", value: "postgres:abc:db.internal:5432", wantErr: true},
		{name: "source port zero", value: "postgres:0:db.internal:5432", wantErr: true},
		{name: "source port too high", value: "postgres:70000:db.internal:5432", wantErr: true},
		{name: "empty target addr", value: "postgres:5432::5432", wantErr: true},
		{name: "non-numeric target port", value: "postgres:5432:db.internal:xyz", wantErr: true},
		{name: "target port too high", value: "postgres:5432:db.internal:99999", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseServiceMapping(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseServiceMapping(%q) = %+v, want error", tt.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseServiceMapping(%q) unexpected error: %v", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("parseServiceMapping(%q) = %+v, want %+v", tt.value, got, tt.want)
			}
		})
	}
}

func TestParseServiceMappings(t *testing.T) {
	t.Run("parses only prefixed entries", func(t *testing.T) {
		environ := []string{
			"PATH=/usr/bin",
			"SERVICE_01=postgres:5432:db.internal:5432",
			"OTHER=ignored",
			"SERVICE_02=redis:6379:cache.internal:6379",
		}
		got, err := parseServiceMappings(environ, "SERVICE_")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d mappings, want 2: %+v", len(got), got)
		}
	})

	t.Run("malformed entry without = is skipped", func(t *testing.T) {
		got, err := parseServiceMappings([]string{"NOEQUALS"}, "SERVICE_")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d mappings, want 0", len(got))
		}
	})

	t.Run("duplicate source port is rejected", func(t *testing.T) {
		environ := []string{
			"SERVICE_01=postgres:5432:db.internal:5432",
			"SERVICE_02=other:5432:other.internal:5432",
		}
		if _, err := parseServiceMappings(environ, "SERVICE_"); err == nil {
			t.Fatal("expected duplicate source port error, got nil")
		}
	})

	t.Run("duplicate sanitized name is rejected", func(t *testing.T) {
		environ := []string{
			"SERVICE_01=My.Service:5432:db.internal:5432",
			"SERVICE_02=my-service:5433:db2.internal:5432",
		}
		if _, err := parseServiceMappings(environ, "SERVICE_"); err == nil {
			t.Fatal("expected duplicate name error after sanitization, got nil")
		}
	})

	t.Run("propagates a malformed mapping error", func(t *testing.T) {
		environ := []string{"SERVICE_01=bad:mapping"}
		if _, err := parseServiceMappings(environ, "SERVICE_"); err == nil {
			t.Fatal("expected parse error, got nil")
		}
	})

	t.Run("empty environ yields no mappings", func(t *testing.T) {
		got, err := parseServiceMappings(nil, "SERVICE_")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d mappings, want 0", len(got))
		}
	})
}
