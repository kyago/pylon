// Package config handles parsing and validation of pylon configuration files.
// Spec Reference: Section 16 "config.yml Schema"
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kyago/pylon/internal/fsutil"
)

// Config represents the full pylon workspace configuration.
// Spec Reference: Section 16 "Full Schema"
type Config struct {
	Version   string                    `yaml:"version"`
	Runtime   RuntimeConfig             `yaml:"runtime"`
	Providers map[string]ProviderConfig `yaml:"providers,omitempty"`
	Routing   RoutingConfig             `yaml:"routing,omitempty"`
	Git       GitConfig                 `yaml:"git"`
	Projects  map[string]ProjectConfig  `yaml:"projects"`
	Wiki      WikiConfig                `yaml:"wiki"`
	Memory    MemoryConfig              `yaml:"memory"`
	Workflow  WorkflowConfig            `yaml:"workflow"`
	Skills    SkillsConfig              `yaml:"skills"`
}

// SkillsConfig defines agent skill management settings.
type SkillsConfig struct {
	Enabled               bool `yaml:"enabled"`
	PreloadToAgents       bool `yaml:"preload_to_agents"`
	ProgressiveDisclosure bool `yaml:"progressive_disclosure"`
}

// RuntimeConfig defines agent runtime settings.
// Spec Reference: Section 16 "runtime"
type RuntimeConfig struct {
	Provider              string            `yaml:"provider,omitempty"`
	Backend               string            `yaml:"backend,omitempty"`
	ExecutionMode         string            `yaml:"execution_mode,omitempty"`
	MaxConcurrent         int               `yaml:"max_concurrent"`
	MaxPipelines          int               `yaml:"max_pipelines"`
	TaskTimeout           string            `yaml:"task_timeout"`
	MaxAttempts           int               `yaml:"max_attempts"`
	MaxTurns              int               `yaml:"max_turns"`
	PermissionMode        string            `yaml:"permission_mode"`
	AutoApproveTaskReview bool              `yaml:"auto_approve_task_review"`
	Env                   map[string]string `yaml:"env"`
	WorkerLimits          map[string]int    `yaml:"worker_limits"` // model → max concurrent
}

type ProviderConfig struct {
	Enabled string `yaml:"enabled,omitempty"`
	Command string `yaml:"command,omitempty"`
}

type RoutingConfig struct {
	Fallback                   string `yaml:"fallback,omitempty"`
	RequireResumeForBackground bool   `yaml:"require_resume_for_background,omitempty"`
}

func (r RuntimeConfig) EffectiveProvider() (string, bool) {
	if providerName := strings.TrimSpace(r.Provider); providerName != "" {
		return providerName, false
	}
	if backendName := strings.TrimSpace(r.Backend); backendName != "" {
		return backendName, true
	}
	return "auto", false
}

// GitConfig defines git integration settings.
// Spec Reference: Section 16 "git"
type GitConfig struct {
	BranchPrefix string         `yaml:"branch_prefix"`
	DefaultBase  string         `yaml:"default_base"`
	AutoPush     bool           `yaml:"auto_push"`
	Worktree     WorktreeConfig `yaml:"worktree"`
	PR           PRConfig       `yaml:"pr"`
}

// WorktreeConfig defines git worktree isolation settings.
// Spec Reference: Section 8 "Git Worktree Isolation"
type WorktreeConfig struct {
	Enabled     bool `yaml:"enabled"`
	AutoCleanup bool `yaml:"auto_cleanup"`
}

// PRConfig defines pull request settings.
// Spec Reference: Section 16 "git.pr"
type PRConfig struct {
	AutoPR    bool     `yaml:"auto_pr"`
	Reviewers []string `yaml:"reviewers"`
	Draft     bool     `yaml:"draft"`
	Template  *string  `yaml:"template"`
}

// ProjectConfig defines per-project settings.
// Spec Reference: Section 16 "projects"
type ProjectConfig struct {
	Stack  string   `yaml:"stack"`
	Agents []string `yaml:"agents,omitempty"`
}

// WikiConfig defines domain knowledge update settings.
// Spec Reference: Section 16 "wiki"
type WikiConfig struct {
	AutoUpdate bool     `yaml:"auto_update"`
	UpdateOn   []string `yaml:"update_on"`
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

// WorkflowConfig defines workflow template settings.
type WorkflowConfig struct {
	DefaultWorkflow string `yaml:"default_workflow"`
	TemplateDir     string `yaml:"template_dir"`
}

// ParseTaskTimeout parses the TaskTimeout string into a time.Duration.
// Returns 30 minutes as fallback if parsing fails or the value is non-positive.
func (r RuntimeConfig) ParseTaskTimeout() time.Duration {
	d, err := time.ParseDuration(r.TaskTimeout)
	if err != nil || d <= 0 {
		return 30 * time.Minute
	}
	return d
}

// MigrateRuntimeBackend rewrites deprecated runtime.backend to runtime.provider
// in config.yml. provider가 이미 있으면 backend 키만 제거한다 — EffectiveProvider가
// provider를 우선하므로 의미 변화가 없다. yaml.Node 단위로 키만 바꿔 주석과
// 문서 순서를 보존한다. Returns whether the file was rewritten.
func MigrateRuntimeBackend(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // config 부재는 뒤의 동기화 단계가 이미 보고한다
		}
		return false, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return false, err
	}
	// 멀티 문서 파일은 첫 문서만 재인코딩하면 나머지가 유실되므로 건드리지 않는다.
	if err := decoder.Decode(&yaml.Node{}); !errors.Is(err, io.EOF) {
		return false, nil
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return false, nil
	}
	root := doc.Content[0]
	var runtime *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "runtime" {
			runtime = root.Content[i+1]
			break
		}
	}
	if runtime == nil || runtime.Kind != yaml.MappingNode {
		return false, nil
	}
	backendIdx := -1
	emptyProviderIdx := -1
	hasProvider := false
	for i := 0; i+1 < len(runtime.Content); i += 2 {
		switch runtime.Content[i].Value {
		case "backend":
			backendIdx = i
		case "provider":
			// 빈 provider는 EffectiveProvider가 backend로 fallback하므로
			// "provider 있음"으로 취급하면 backend 삭제가 의미를 바꾼다.
			if value := runtime.Content[i+1]; value.Kind == yaml.ScalarNode && strings.TrimSpace(value.Value) != "" {
				hasProvider = true
			} else {
				emptyProviderIdx = i
			}
		}
	}
	if backendIdx < 0 {
		return false, nil
	}
	if hasProvider {
		runtime.Content = append(runtime.Content[:backendIdx], runtime.Content[backendIdx+2:]...)
	} else {
		if emptyProviderIdx >= 0 {
			runtime.Content = append(runtime.Content[:emptyProviderIdx], runtime.Content[emptyProviderIdx+2:]...)
			if emptyProviderIdx < backendIdx {
				backendIdx -= 2
			}
		}
		runtime.Content[backendIdx].Value = "provider"
	}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&doc); err != nil {
		return false, err
	}
	if err := encoder.Close(); err != nil {
		return false, err
	}
	if err := fsutil.WriteFileAtomic(path, buf.Bytes(), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// SetRuntimeProvider pins runtime.provider to the given name in config.yml,
// preserving comments and document order via yaml.Node surgery (같은 이유로
// MigrateRuntimeBackend와 동일한 방식). runtime 섹션이나 provider 키가 없으면
// 만들어 넣는다. 멀티 문서 파일은 유실 위험이 있어 건드리지 않는다.
func SetRuntimeProvider(path, providerName string) error {
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		return errors.New("provider name is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return err
	}
	if err := decoder.Decode(&yaml.Node{}); !errors.Is(err, io.EOF) {
		return errors.New("multi-document config.yml is not supported")
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return errors.New("config.yml root must be a mapping")
	}
	root := doc.Content[0]

	scalar := func(value string) *yaml.Node {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	}
	var runtime *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "runtime" {
			runtime = root.Content[i+1]
			break
		}
	}
	if runtime == nil {
		runtime = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, scalar("runtime"), runtime)
	}
	if runtime.Kind != yaml.MappingNode {
		return errors.New("runtime section must be a mapping")
	}
	set := false
	for i := 0; i+1 < len(runtime.Content); i += 2 {
		if runtime.Content[i].Value == "provider" {
			runtime.Content[i+1] = scalar(providerName)
			set = true
			break
		}
	}
	if !set {
		runtime.Content = append([]*yaml.Node{scalar("provider"), scalar(providerName)}, runtime.Content...)
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&doc); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, buf.Bytes(), 0644)
}

// SyncConfigDefaults reads config.yml, detects missing fields, and adds them.
// Only writes to disk if there are actually missing fields.
// When only entire sections are missing, they are appended to preserve existing content.
// When sub-keys within existing sections are missing, the file is rewritten via map merge.
// Note: rewrite mode (sub-key merge) may alter YAML formatting/comments.
// Returns the updated Config and a list of field paths that were added.
func SyncConfigDefaults(path string) (*Config, []string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		return nil, nil, err
	}

	// Parse raw into nested map to detect existing keys
	var rawMap map[string]any
	if unmarshalErr := yaml.Unmarshal(raw, &rawMap); unmarshalErr != nil {
		rawMap = make(map[string]any)
	}

	// Build defaults as map for comparison
	defaultBytes, _ := yaml.Marshal(cfg)
	var defaultMap map[string]any
	_ = yaml.Unmarshal(defaultBytes, &defaultMap)
	if rawRuntime, ok := rawMap["runtime"].(map[string]any); ok {
		if _, hasBackend := rawRuntime["backend"]; hasBackend {
			if _, hasProvider := rawRuntime["provider"]; !hasProvider {
				if defaultRuntime, ok := defaultMap["runtime"].(map[string]any); ok {
					delete(defaultRuntime, "provider")
				}
			}
		}
	}

	// Sort keys for deterministic output
	sortedKeys := make([]string, 0, len(defaultMap))
	for k := range defaultMap {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	// Find missing fields
	var added []string
	var appendBuf []byte
	hasSubKeyMerge := false

	for _, key := range sortedKeys {
		defaultVal := defaultMap[key]
		rawVal, exists := rawMap[key]
		if !exists {
			// Entire section missing — add to rawMap (for rewrite) and appendBuf (for append)
			rawMap[key] = defaultVal
			section, _ := yaml.Marshal(map[string]any{key: defaultVal})
			appendBuf = append(appendBuf, '\n')
			appendBuf = append(appendBuf, section...)
			added = append(added, key)
			continue
		}

		// Section exists — recursively check sub-keys at all depths
		defaultSub, ok := defaultVal.(map[string]any)
		if !ok {
			continue
		}
		rawSub, _ := rawVal.(map[string]any)
		if rawSub == nil {
			rawSub = make(map[string]any)
		}

		subAdded := findMissingFields(rawSub, defaultSub, key)
		if len(subAdded) > 0 {
			rawMap[key] = rawSub
			added = append(added, subAdded...)
			hasSubKeyMerge = true
		}
	}

	if len(added) == 0 {
		return cfg, nil, nil
	}

	if hasSubKeyMerge {
		// Sub-key merge needed — rewrite full file (rawMap already contains everything)
		data, err := yaml.Marshal(rawMap)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, nil, fmt.Errorf("failed to write config: %w", err)
		}
	} else if len(appendBuf) > 0 {
		// Only new sections — append to preserve existing content
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open config for append: %w", err)
		}
		defer f.Close()
		if _, err := f.Write(appendBuf); err != nil {
			return nil, nil, fmt.Errorf("failed to append config: %w", err)
		}
	}

	return cfg, added, nil
}

// findMissingFields recursively compares dst against defaults and merges
// missing fields into dst. Returns a list of dotted field paths that were added.
func findMissingFields(dst, defaults map[string]any, prefix string) []string {
	var added []string

	sortedKeys := make([]string, 0, len(defaults))
	for k := range defaults {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	for _, key := range sortedKeys {
		defaultVal := defaults[key]
		path := prefix + "." + key

		dstVal, exists := dst[key]
		if !exists {
			dst[key] = defaultVal
			added = append(added, path)
			continue
		}

		// If both are maps, recurse deeper
		defaultSub, defaultIsMap := defaultVal.(map[string]any)
		if !defaultIsMap {
			continue
		}
		dstSub, dstIsMap := dstVal.(map[string]any)
		if !dstIsMap {
			dstSub = make(map[string]any)
		}

		subAdded := findMissingFields(dstSub, defaultSub, path)
		if len(subAdded) > 0 {
			dst[key] = dstSub
			added = append(added, subAdded...)
		}
	}

	return added
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
		Git: GitConfig{
			AutoPush: true,
			Worktree: WorktreeConfig{
				Enabled:     true,
				AutoCleanup: true,
			},
		},
		Wiki: WikiConfig{
			AutoUpdate: true,
		},
		Memory: MemoryConfig{
			ProactiveInjection: true,
		},
		Skills: SkillsConfig{
			Enabled:               true,
			PreloadToAgents:       true,
			ProgressiveDisclosure: true,
		},
		Routing: RoutingConfig{
			RequireResumeForBackground: true,
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
	// Runtime defaults
	if cfg.Runtime.Provider == "" && cfg.Runtime.Backend == "" {
		cfg.Runtime.Provider = "auto"
	}
	if cfg.Runtime.ExecutionMode == "" {
		cfg.Runtime.ExecutionMode = "session-native"
	}
	if cfg.Runtime.MaxConcurrent == 0 {
		cfg.Runtime.MaxConcurrent = 5
	}
	if cfg.Runtime.TaskTimeout == "" {
		cfg.Runtime.TaskTimeout = "30m"
	}
	if cfg.Runtime.MaxAttempts == 0 {
		cfg.Runtime.MaxAttempts = 2
	}
	if cfg.Runtime.MaxTurns == 0 {
		cfg.Runtime.MaxTurns = 50
	}
	if cfg.Runtime.PermissionMode == "" {
		cfg.Runtime.PermissionMode = "acceptEdits"
	}
	if cfg.Runtime.Env == nil {
		cfg.Runtime.Env = map[string]string{
			"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "80",
			"CLAUDE_CODE_EFFORT_LEVEL":        "high",
		}
	}
	if cfg.Routing.Fallback == "" {
		cfg.Routing.Fallback = "serial"
	}

	// Git defaults
	if cfg.Git.BranchPrefix == "" {
		cfg.Git.BranchPrefix = "task"
	}
	if cfg.Git.DefaultBase == "" {
		cfg.Git.DefaultBase = "main"
	}
	// Bool fields (AutoPush, Worktree.Enabled, etc.) are handled by
	// pre-initialization in ParseConfig. Only set non-bool defaults here.

	// Wiki defaults: set update_on triggers if auto_update is true but no triggers specified
	if cfg.Wiki.AutoUpdate && len(cfg.Wiki.UpdateOn) == 0 {
		cfg.Wiki.UpdateOn = []string{"task_complete", "pr_merged"}
	}

	// Memory defaults
	if cfg.Memory.ProactiveMaxTokens == 0 {
		cfg.Memory.ProactiveMaxTokens = 2000
	}
	if cfg.Memory.RetentionDays == nil {
		cfg.Memory.RetentionDays = retentionPolicy{"learning": 30}
	}
	// ProactiveInjection defaults to true,
	// handled by pre-initialization in ParseConfig.

	// Workflow defaults
	if cfg.Workflow.DefaultWorkflow == "" {
		cfg.Workflow.DefaultWorkflow = "feature"
	}
}
