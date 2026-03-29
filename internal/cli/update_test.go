package cli

import (
	"runtime"
	"testing"
)

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// CalVer valid
		{"2026.3.1", true},
		{"2026.12.15", true},
		{"2025.1.1", true},
		{"2030.6.100", true},
		{"latest", true},
		{"Latest", true},
		{"LATEST", true},

		// Invalid
		{"abc", false},
		{"", false},
		{"v1.2.3", false},
		{"2026", false},
		{"2026.3", false},
		{"hello-world", false},
		{"2026.3.1.4", false},
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

func TestResolveUpdateTarget(t *testing.T) {
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
		{"latest uppercase", []string{"LATEST"}, "latest", false},
		{"latest mixed case", []string{"Latest"}, "latest", false},
		{"calver version", []string{"2026.3.1"}, "2026.3.1", false},
		{"calver high seq", []string{"2026.12.100"}, "2026.12.100", false},
		{"invalid version", []string{"abc"}, "", true},
		{"semver rejected", []string{"v1.2.3"}, "", true},
		{"partial version", []string{"2026.3"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveUpdateTarget(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveUpdateTarget(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("resolveUpdateTarget(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestBuildAssetName(t *testing.T) {
	name := buildAssetName("2026.3.1")

	// Should contain version, OS, and arch
	wantPrefix := "pylon_2026.3.1_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		want := wantPrefix + ".zip"
		if name != want {
			t.Errorf("buildAssetName(\"2026.3.1\") = %q, want %q", name, want)
		}
	} else {
		want := wantPrefix + ".tar.gz"
		if name != want {
			t.Errorf("buildAssetName(\"2026.3.1\") = %q, want %q", name, want)
		}
	}
}
