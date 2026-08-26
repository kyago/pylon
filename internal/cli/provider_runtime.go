package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/provider"
	providerclaude "github.com/kyago/pylon/internal/provider/claude"
	providercodex "github.com/kyago/pylon/internal/provider/codex"
)

const claudeInstallURL = "https://docs.anthropic.com/en/docs/claude-code"
const codexInstallURL = "https://developers.openai.com/codex/cli"

type providerWorkspacePreparer func(string, *config.Config, []config.ProjectInfo) error
type providerPermissionSelector func(string) (string, error)
type providerDoctorSync func(io.Reader, bool)

type providerCatalogEntry struct {
	adapter          provider.InteractiveAdapter
	prepareWorkspace providerWorkspacePreparer
	selectPermission providerPermissionSelector
	syncDoctor       providerDoctorSync
	installURL       string
}

type providerCatalog struct {
	registry *provider.Registry
	entries  map[string]providerCatalogEntry
}

func newProviderCatalog(cfg *config.Config) (*providerCatalog, error) {
	if cfg == nil {
		return nil, fmt.Errorf("provider config is nil")
	}
	catalog := &providerCatalog{
		registry: provider.NewRegistry(),
		entries:  make(map[string]providerCatalogEntry),
	}

	providerConfig := cfg.Providers[providerclaude.Name]
	enabled, err := providerEnabled(providerConfig.Enabled)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", providerclaude.Name, err)
	}
	if enabled {
		command := providerConfig.Command
		if strings.TrimSpace(command) == "" {
			command = "claude"
		}
		if err := catalog.register(providerCatalogEntry{
			adapter:          providerclaude.New(command),
			prepareWorkspace: generateClaudeDir,
			selectPermission: selectPermissionMode,
			syncDoctor:       syncClaudeProviderResourcesIfWorkspace,
			installURL:       claudeInstallURL,
		}); err != nil {
			return nil, err
		}
	}

	codexConfig := cfg.Providers[providercodex.Name]
	codexEnabled, err := providerEnabled(codexConfig.Enabled)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", providercodex.Name, err)
	}
	if codexEnabled {
		command := codexConfig.Command
		if strings.TrimSpace(command) == "" {
			command = "codex"
		}
		if err := catalog.register(providerCatalogEntry{
			adapter:          providercodex.New(command),
			prepareWorkspace: prepareCodexWorkspace,
			selectPermission: selectCodexSandboxMode,
			syncDoctor:       syncCodexProviderResourcesIfWorkspace,
			installURL:       codexInstallURL,
		}); err != nil {
			return nil, err
		}
	}

	return catalog, nil
}

// prepareCodexWorkspace materializes what a codex session needs: codex reads
// the workspace-root AGENTS.md natively, so no .claude/ 생성이 필요 없다.
// pylon 소유 리소스 갱신은 launch 계약(모든 launch에서 refresh)에 따라
// claude 경로(generateClaudeDir)와 동일하게 수행한다.
func prepareCodexWorkspace(root string, _ *config.Config, projects []config.ProjectInfo) error {
	if _, overwritten := syncPylonResources(layout.PylonDir(root)); len(overwritten) > 0 {
		fmt.Fprintf(os.Stderr, "⚠ 내장 버전으로 되돌린 pylon 소유 파일 %d개: %s\n",
			len(overwritten), strings.Join(overwritten, ", "))
	}
	bootstrapped, backedUp, err := ensureRootAgentFiles(root, projects)
	if err != nil {
		return err
	}
	for _, name := range backedUp {
		fmt.Fprintf(os.Stderr, "ℹ 기존 %s를 %s%s로 백업했습니다.\n", name, name, rootFileBackupSuffix)
	}
	if bootstrapped {
		fmt.Fprintln(os.Stderr, "ℹ AGENTS.md를 부트스트랩했습니다 — 세션이 첫 턴에 이 워크스페이스에 맞게 재작성합니다.")
	}
	// 파이프라인 커맨드를 codex repo 스킬(.agents/skills/)로 노출한다 — claude의
	// .claude/commands/ 대응물. launch/doctor가 같은 생성기를 공유한다.
	if err := generateCodexSkills(root); err != nil {
		return fmt.Errorf("codex 스킬 생성 실패: %w", err)
	}
	return nil
}

// syncCodexProviderResourcesIfWorkspace reconciles the generated codex skills
// on `pylon doctor`, mirroring the claude provider's doctor sync.
func syncCodexProviderResourcesIfWorkspace(io.Reader, bool) {
	root, err := resolveRoot()
	if err != nil {
		return
	}
	if err := generateCodexSkills(root); err != nil {
		fmt.Printf("⚠ codex 워크플로우 스킬 동기화 실패: %v\n", err)
		return
	}
	fmt.Println("✓ codex 워크플로우 스킬 최신 상태 (.agents/skills/)")
}

// selectCodexSandboxMode presents an interactive selector for the codex
// --sandbox mode (공식 값: read-only / workspace-write / danger-full-access).
func selectCodexSandboxMode(defaultMode string) (string, error) {
	if defaultMode != "read-only" && defaultMode != "danger-full-access" {
		defaultMode = "workspace-write"
	}

	modes := []huh.Option[string]{
		huh.NewOption("workspace-write — 워크스페이스 내 쓰기 허용", "workspace-write"),
		huh.NewOption("read-only — 읽기 전용", "read-only"),
		huh.NewOption("danger-full-access — 샌드박스 없음", "danger-full-access"),
	}
	for i, m := range modes {
		if m.Value == defaultMode {
			modes[i] = modes[i].Selected(true)
			break
		}
	}

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Codex Sandbox 선택").
				Description("codex --sandbox 모드를 설정합니다").
				Options(modes...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return "", fmt.Errorf("선택 취소됨: %w", err)
	}
	return selected, nil
}

func providerEnabled(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto", "true", "enabled":
		return true, nil
	case "false", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("invalid enabled value %q", value)
	}
}

func (catalog *providerCatalog) register(entry providerCatalogEntry) error {
	if entry.adapter == nil {
		return fmt.Errorf("interactive provider adapter is nil")
	}
	if err := catalog.registry.Register(entry.adapter); err != nil {
		return err
	}
	catalog.entries[entry.adapter.Name()] = entry
	return nil
}

func (catalog *providerCatalog) selectInteractive(ctx context.Context, cfg *config.Config) (providerCatalogEntry, provider.Selection, error) {
	if catalog == nil || catalog.registry == nil {
		return providerCatalogEntry{}, provider.Selection{}, fmt.Errorf("provider catalog is not configured")
	}
	if cfg == nil {
		return providerCatalogEntry{}, provider.Selection{}, fmt.Errorf("provider config is nil")
	}
	providerName, _ := cfg.Runtime.EffectiveProvider()
	selection, err := provider.NewRouter(catalog.registry).Select(ctx, provider.SelectionRequest{
		WorkspaceProvider: providerName,
	})
	if err != nil {
		return providerCatalogEntry{}, provider.Selection{}, err
	}
	entry, ok := catalog.entries[selection.Adapter.Name()]
	if !ok {
		return providerCatalogEntry{}, provider.Selection{}, fmt.Errorf("provider %s does not support interactive launch", selection.Adapter.Name())
	}
	return entry, selection, nil
}

func (catalog *providerCatalog) installURL(providerName string) string {
	if entry, ok := catalog.entries[providerName]; ok {
		return entry.installURL
	}
	if providerName == "" || providerName == "auto" {
		for _, adapter := range catalog.registry.Adapters() {
			if entry, ok := catalog.entries[adapter.Name()]; ok && entry.installURL != "" {
				return entry.installURL
			}
		}
	}
	return "configure a supported provider in .pylon/config.yml"
}
