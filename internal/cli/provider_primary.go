package cli

import (
	"context"
	"fmt"
	"slices"

	"github.com/charmbracelet/huh"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

var providerDisplayNames = map[string]string{
	"claude-code": "Claude Code",
	"codex":       "Codex",
}

func providerDisplayName(name string) string {
	if display, ok := providerDisplayNames[name]; ok {
		return display
	}
	return name
}

// detectInstalledProviders probes every registered adapter and returns the
// names of those whose CLI is actually installed, in registry order.
func detectInstalledProviders(catalog *providerCatalog) []string {
	var installed []string
	for _, adapter := range catalog.registry.Adapters() {
		if _, err := adapter.Probe(context.Background()); err == nil {
			installed = append(installed, adapter.Name())
		}
	}
	return installed
}

// ensurePrimaryProvider resolves runtime.provider to a concrete installed
// provider and pins it in config.yml:
//   - 이미 설정된 provider가 설치되어 있으면 그대로 둔다.
//   - 설치된 provider가 하나뿐이면 그것을 primary로 기본 fallback한다.
//   - 둘 이상이면 인터랙티브 선택 후 전용(pinned)으로 기록한다.
//     비인터랙티브 모드에서는 선택을 강요하지 않고 기존 값(auto 라우팅)을 유지한다.
//
// Returns the (possibly reloaded) config and whether the pin was written.
func ensurePrimaryProvider(root string, cfg *config.Config, interactive bool) (*config.Config, bool) {
	catalog, err := newProviderCatalog(cfg)
	if err != nil {
		return cfg, false
	}
	installed := detectInstalledProviders(catalog)
	current, _ := cfg.Runtime.EffectiveProvider()
	pinned := current != "" && current != "auto"

	if pinned && slices.Contains(installed, current) {
		return cfg, false
	}
	if pinned && len(installed) > 0 {
		fmt.Printf("⚠ 설정된 provider %s가 설치되어 있지 않습니다 — 대체 provider를 결정합니다\n", current)
	}

	var pick string
	switch len(installed) {
	case 0:
		return cfg, false // provider check가 설치 안내를 담당한다
	case 1:
		pick = installed[0]
		fmt.Printf("✓ %s를 primary provider로 설정합니다 (유일하게 설치된 provider)\n", providerDisplayName(pick))
	default:
		if !interactive {
			return cfg, false
		}
		pick, err = selectPrimaryProvider(installed, current)
		if err != nil {
			fmt.Printf("⚠ provider 선택 취소됨 — 기존 설정(%s)을 유지합니다\n", current)
			return cfg, false
		}
		fmt.Printf("✓ %s를 전용 provider로 설정합니다\n", providerDisplayName(pick))
	}

	cfgPath := layout.ConfigPath(root)
	if err := config.SetRuntimeProvider(cfgPath, pick); err != nil {
		fmt.Printf("⚠ runtime.provider 기록 실패: %v\n", err)
		return cfg, false
	}
	reloaded, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Printf("⚠ 설정 재로드 실패: %v\n", err)
		return cfg, true
	}
	return reloaded, true
}

// selectPrimaryProvider prompts the user to choose among installed providers.
func selectPrimaryProvider(installed []string, current string) (string, error) {
	options := make([]huh.Option[string], 0, len(installed))
	for _, name := range installed {
		option := huh.NewOption(fmt.Sprintf("%s (%s)", providerDisplayName(name), name), name)
		if name == current {
			option = option.Selected(true)
		}
		options = append(options, option)
	}

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Primary Provider 선택").
				Description("설치된 provider가 여러 개입니다 — 선택한 provider를 전용으로 사용합니다").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return "", err
	}
	return selected, nil
}
