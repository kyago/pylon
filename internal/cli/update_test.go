package cli

import "testing"

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"v1.2.3", true},
		{"v0.1.0", true},
		{"v10.20.30", true},
		{"1.2.3", true},
		{"0.1.0", true},
		{"latest", true},
		{"Latest", true},
		{"LATEST", true},
		{"v1.2.3-beta.1", true},
		{"v1.2.3+build.123", true},
		{"abc", false},
		{"", false},
		{"v1", false},
		{"v1.2", false},
		{"1.2.3.4.5", false},
		{"v.1.2.3", false},
		{"hello-world", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := validateVersion(tt.input)
			if got != tt.want {
				t.Errorf("validateVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.2.3", "v1.2.3"},
		{"v1.2.3", "v1.2.3"},
		{"0.1.0", "v0.1.0"},
		{"v0.1.0", "v0.1.0"},
		{"latest", "latest"},
		{"Latest", "latest"},
		{"LATEST", "latest"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeVersion(tt.input)
			if got != tt.want {
				t.Errorf("normalizeVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveInstallTarget(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"no args", nil, "latest", false},
		{"empty args", []string{}, "latest", false},
		{"empty string", []string{""}, "latest", false},
		{"whitespace only", []string{"  "}, "latest", false},
		{"latest keyword", []string{"latest"}, "latest", false},
		{"version with v", []string{"v0.3.0"}, "v0.3.0", false},
		{"version without v", []string{"0.3.0"}, "v0.3.0", false},
		{"pre-release", []string{"v1.0.0-rc.1"}, "v1.0.0-rc.1", false},
		{"invalid version", []string{"abc"}, "", true},
		{"partial version", []string{"v1.2"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveInstallTarget(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveInstallTarget(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("resolveInstallTarget(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
