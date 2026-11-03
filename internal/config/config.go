// Package config handles parsing and validation of pylon configuration files.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the workspace configuration. Keys left over from older releases
// are ignored.
type Config struct {
	Version string       `yaml:"version"`
	Memory  MemoryConfig `yaml:"memory"`
}

// MemoryConfig defines agent memory management settings.
// Spec Reference: Section 16 "memory"
type MemoryConfig struct {
	ProactiveInjection bool `yaml:"proactive_injection"`
	ProactiveMaxTokens int  `yaml:"proactive_max_tokens"`
	// RetentionDays: 카테고리별 보존 일수. 미지정 카테고리·0 이하 = 영구 보존.
	// nil(설정 파일에 키 없음)이면 기본값 {"learning": 30}이 적용되고,
	// 명시적 빈 맵({})은 전체 영구 보존 opt-out이다.
	RetentionDays retentionPolicy `yaml:"retention_days"`
}

// retentionPolicy is a category→days map that tolerates a legacy scalar
// retention_days value (older configs shipped `retention_days: 0`). A
// non-mapping node is treated as "no policy" so those configs still load,
// preserving the pre-existing behavior where the field was ignored.
type retentionPolicy map[string]int

func (r *retentionPolicy) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		*r = retentionPolicy{} // 레거시 스칼라 → 정책 없음(기존 무시 동작 보존)
		return nil
	}
	m := map[string]int{}
	if err := value.Decode(&m); err != nil {
		return err
	}
	*r = m
	return nil
}

// LoadConfig reads and parses a config.yml file, applying defaults for missing fields.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return ParseConfig(data)
}

// ParseConfig parses config.yml content from bytes, applying defaults for missing fields.
func ParseConfig(data []byte) (*Config, error) {
	// Pre-initialize bool fields that should default to true.
	// go-yaml v3 only overwrites fields present in the YAML,
	// so pre-initialized values are preserved when not explicitly set.
	cfg := &Config{
		Memory: MemoryConfig{
			ProactiveInjection: true,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate required fields
	if cfg.Version == "" {
		return nil, fmt.Errorf("config validation error: 'version' is required")
	}

	// Apply defaults for non-bool fields (Spec Section 16 defaults)
	applyDefaults(cfg)

	return cfg, nil
}

// applyDefaults fills in default values for any fields not specified in the config.
// Spec Reference: Section 16 default values
func applyDefaults(cfg *Config) {
	// Memory defaults
	if cfg.Memory.ProactiveMaxTokens == 0 {
		cfg.Memory.ProactiveMaxTokens = 2000
	}
	if cfg.Memory.RetentionDays == nil {
		cfg.Memory.RetentionDays = retentionPolicy{"learning": 30}
	}
	// ProactiveInjection defaults to true,
	// handled by pre-initialization in ParseConfig.
}
