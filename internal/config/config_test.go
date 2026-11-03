package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfig_IgnoresRemovedFields(t *testing.T) {
	yml := `
version: "1"
memory:
  session_archive: true
  retention_days:
    learning: 30
conversation:
  retention_days: 90
`
	cfg, err := ParseConfig([]byte(yml))
	if err != nil {
		t.Fatalf("removed fields must be ignored, got error: %v", err)
	}
	if cfg.Memory.ProactiveMaxTokens != 2000 {
		t.Fatalf("defaults must still apply: %v", cfg.Memory.ProactiveMaxTokens)
	}
}

func TestParseConfig_MissingVersion(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "config", "missing_version.yml"))
	if err != nil {
		t.Fatalf("failed to read test fixture: %v", err)
	}

	_, err = ParseConfig(data)
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
}

func TestParseConfig_InvalidYAML(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "config", "invalid_yaml.yml"))
	if err != nil {
		t.Fatalf("failed to read test fixture: %v", err)
	}

	_, err = ParseConfig(data)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestParseConfig_EmptyInput(t *testing.T) {
	_, err := ParseConfig([]byte(""))
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestParseConfig_VersionOnly(t *testing.T) {
	cfg, err := ParseConfig([]byte(`version: "0.1"`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	if cfg.Version != "0.1" {
		t.Errorf("version = %q", cfg.Version)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yml")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestMemoryRetentionDaysDefault(t *testing.T) {
	cfg, err := ParseConfig([]byte("version: \"0.1\"\n"))
	if err != nil {
		t.Fatalf("ParseConfig 실패: %v", err)
	}
	if got := cfg.Memory.RetentionDays["learning"]; got != 30 {
		t.Errorf("retention_days.learning 기본값 = %d, want 30", got)
	}
}

func TestMemoryRetentionDaysOverride(t *testing.T) {
	yml := "version: \"0.1\"\nmemory:\n  retention_days:\n    learning: 7\n    note: 14\n"
	cfg, err := ParseConfig([]byte(yml))
	if err != nil {
		t.Fatalf("ParseConfig 실패: %v", err)
	}
	if cfg.Memory.RetentionDays["learning"] != 7 || cfg.Memory.RetentionDays["note"] != 14 {
		t.Errorf("명시값이 적용되어야 한다: %v", cfg.Memory.RetentionDays)
	}
}

// 빈 맵을 명시하면 "모든 카테고리 영구 보존" opt-out이다 (기본값 미적용).
func TestMemoryRetentionDaysExplicitEmpty(t *testing.T) {
	yml := "version: \"0.1\"\nmemory:\n  retention_days: {}\n"
	cfg, err := ParseConfig([]byte(yml))
	if err != nil {
		t.Fatalf("ParseConfig 실패: %v", err)
	}
	if len(cfg.Memory.RetentionDays) != 0 {
		t.Errorf("빈 맵 명시는 보존 정책 해제여야 한다: %v", cfg.Memory.RetentionDays)
	}
}

func TestMemoryRetentionDaysLegacyScalar(t *testing.T) {
	cfg, err := ParseConfig([]byte("version: \"0.1\"\nmemory:\n  retention_days: 0\n"))
	if err != nil {
		t.Fatalf("레거시 스칼라 retention_days는 파싱 에러 없이 로드되어야 한다: %v", err)
	}
	if len(cfg.Memory.RetentionDays) != 0 {
		t.Errorf("레거시 스칼라는 정책 없음(빈)이어야 한다: %v", cfg.Memory.RetentionDays)
	}
}
