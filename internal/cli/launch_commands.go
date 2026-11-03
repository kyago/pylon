package cli

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kyago/pylon/internal/layout"
)

//go:embed commands/*.md
var embeddedCommands embed.FS

const managedClaudeCommandsManifest = ".pylon-managed.json"

// buildDesiredClaudeCommands computes the full set of .claude/commands/ files
// that should exist, keyed by path relative to the commands dir (e.g.
// "pl/cleanup.md"). It is read-only and shared by `pylon launch` and `pylon doctor`
// so both agree on the desired state.
func buildDesiredClaudeCommands(root string) map[string]string {
	desired := make(map[string]string)

	// Slash commands → pl/: the built-ins come straight from the embeds
	// (.pylon/builtin/commands/ is a copy of them), then user-added commands from
	// .pylon/commands/. A built-in name always wins — see syncBuiltinResources.
	embedded, _ := embeddedCommands.ReadDir("commands")
	for _, entry := range embedded {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := embeddedCommands.ReadFile("commands/" + entry.Name())
		if err != nil {
			continue
		}
		desired[filepath.Join("pl", strings.TrimPrefix(entry.Name(), "pl-"))] = string(content)
	}
	userCmdsDir := layout.ResourceDir(root, "commands")
	entries, _ := os.ReadDir(userCmdsDir)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		dest := filepath.Join("pl", strings.TrimPrefix(entry.Name(), "pl-"))
		if _, builtin := desired[dest]; builtin {
			continue
		}
		content, err := os.ReadFile(filepath.Join(userCmdsDir, entry.Name()))
		if err != nil {
			continue
		}
		desired[dest] = string(content)
	}

	return desired
}

// applyClaudeCommands writes the desired command files into commandsDir and removes
// previously managed files no longer desired. It intentionally does NOT remove other
// on-disk files so any user-added commands are preserved.
func applyClaudeCommands(commandsDir string, desired map[string]string) error {
	previouslyManaged := readManagedClaudeCommandSet(commandsDir)

	for rel := range previouslyManaged {
		if _, ok := desired[rel]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(commandsDir, rel)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("이전 커맨드 파일 제거 실패 (%s): %w", rel, err)
		}
	}
	for rel, content := range desired {
		cmdPath := filepath.Join(commandsDir, rel)
		if err := os.MkdirAll(filepath.Dir(cmdPath), 0755); err != nil {
			return fmt.Errorf("커맨드 디렉토리 생성 실패: %w", err)
		}
		if err := os.WriteFile(cmdPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("커맨드 %s 생성 실패: %w", rel, err)
		}
	}
	if err := writeManagedClaudeCommandSet(commandsDir, desired); err != nil {
		return err
	}
	return nil
}

func readManagedClaudeCommandSet(commandsDir string) map[string]bool {
	managed := make(map[string]bool)
	content, err := os.ReadFile(filepath.Join(commandsDir, managedClaudeCommandsManifest))
	if err != nil {
		return managed
	}
	var files []string
	if err := json.Unmarshal(content, &files); err != nil {
		return managed
	}
	for _, rel := range files {
		if isSafeCommandRel(rel) {
			managed[rel] = true
		}
	}
	return managed
}

func writeManagedClaudeCommandSet(commandsDir string, desired map[string]string) error {
	files := make([]string, 0, len(desired))
	for rel := range desired {
		files = append(files, rel)
	}
	sort.Strings(files)
	content, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return fmt.Errorf("커맨드 매니페스트 생성 실패: %w", err)
	}
	content = append(content, '\n')
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return fmt.Errorf("커맨드 디렉토리 생성 실패: %w", err)
	}
	if err := os.WriteFile(filepath.Join(commandsDir, managedClaudeCommandsManifest), content, 0644); err != nil {
		return fmt.Errorf("커맨드 매니페스트 쓰기 실패: %w", err)
	}
	return nil
}

func isSafeCommandRel(rel string) bool {
	if rel == "" || filepath.IsAbs(rel) {
		return false
	}
	clean := filepath.Clean(rel)
	return clean == rel && clean != "." && !strings.HasPrefix(clean, ".."+string(os.PathSeparator)) && clean != ".."
}
