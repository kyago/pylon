package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/kyago/pylon/internal/layout"
)

// Settings holds the keys pylon reads from the workspace .pylon/config.yml. Every
// other key in the file is ignored.
type Settings struct {
	// AutoReview adds the "review your own PR" section to the root AGENTS.md block.
	AutoReview bool `yaml:"auto_review"`
}

// LoadSettings reads .pylon/config.yml. A missing file yields the defaults; a file
// that cannot be parsed yields the defaults and an error for the caller to report.
func LoadSettings(root string) (Settings, error) {
	var s Settings
	path := filepath.Join(layout.PylonDir(root), "config.yml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}
