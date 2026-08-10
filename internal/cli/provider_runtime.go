package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/provider"
	providerclaude "github.com/kyago/pylon/internal/provider/claude"
)

const claudeInstallURL = "https://docs.anthropic.com/en/docs/claude-code"

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

	return catalog, nil
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
