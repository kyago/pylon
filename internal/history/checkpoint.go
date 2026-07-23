package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/kyago/pylon/internal/fsutil"
)

// stageCheckpoint copies and curates a pipeline's runtime artifacts into destDir,
// returning the affected projects and the pipeline status.
func (m *Manager) stageCheckpoint(sourceDir, destDir string, phase Phase) ([]string, string, error) {
	planned := []string{"requirement.md", "requirement-analysis.md", "architecture.md", "routing-decision.json", "tasks.json"}
	for _, name := range planned {
		if err := copyIfExists(filepath.Join(sourceDir, name), filepath.Join(destDir, name)); err != nil {
			return nil, "", err
		}
	}
	if phase != PhasePlanned {
		executionSource := filepath.Join(sourceDir, "execution-log.json")
		if _, err := os.Stat(executionSource); os.IsNotExist(err) {
			executionSource = filepath.Join(sourceDir, "status.json")
		}
		if err := summarizeJSONFile(executionSource, filepath.Join(destDir, "execution-summary.json"), executionKeys); err != nil {
			return nil, "", err
		}
	}
	projects, status, err := inspectPipeline(sourceDir)
	if err != nil {
		return nil, "", err
	}
	if status == "" {
		status = string(phase)
	}
	if isTerminalPhase(phase) {
		if err := summarizeResultFiles(sourceDir, "verification.json", filepath.Join(destDir, "verification-summary.json"), verificationKeys); err != nil {
			return nil, "", err
		}
		if err := summarizeResultFiles(sourceDir, "pr.json", filepath.Join(destDir, "pr-summary.json"), prKeys); err != nil {
			return nil, "", err
		}
		if err := summarizeJSONFile(filepath.Join(sourceDir, "status.json"), filepath.Join(destDir, "status-summary.json"), statusKeys); err != nil {
			return nil, "", err
		}
		if err := m.exportMemory(destDir, projects); err != nil {
			return nil, "", err
		}
	}
	return projects, status, nil
}

func summarizeResultFiles(sourceDir, name, dest string, allowed map[string]bool) error {
	rootFile := filepath.Join(sourceDir, name)
	if _, err := os.Stat(rootFile); err == nil {
		return summarizeJSONFile(rootFile, dest, allowed)
	} else if !os.IsNotExist(err) {
		return err
	}
	paths, err := filepath.Glob(filepath.Join(sourceDir, "*", name))
	if err != nil {
		return err
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil
	}
	records := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("%s JSON 파싱 실패: %w", path, err)
		}
		records = append(records, map[string]any{
			"pipeline": filepath.Base(filepath.Dir(path)),
			"result":   curateJSON(value, allowed),
		})
	}
	return fsutil.WriteJSONAtomic(dest, map[string]any{"records": records})
}

var executionKeys = map[string]bool{
	"tasks": true, "id": true, "title": true, "agent": true, "repo": true,
	"status": true, "branch": true, "changed_files": true, "error": true,
	"started_at": true, "completed_at": true, "sub_pipelines": true, "pipeline_dir": true,
}
var verificationKeys = map[string]bool{
	"ok": true, "skipped": true, "timestamp": true, "checks": true, "name": true,
}
var prKeys = map[string]bool{"url": true, "number": true, "title": true, "repo": true, "prs": true}
var statusKeys = map[string]bool{
	"stage": true, "status": true, "branch": true, "started_at": true,
	"completed_at": true, "sub_pipelines": true, "repo": true, "pipeline_dir": true,
}

func summarizeJSONFile(source, dest string, allowed map[string]bool) error {
	data, err := os.ReadFile(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%s JSON 파싱 실패: %w", source, err)
	}
	return fsutil.WriteJSONAtomic(dest, curateJSON(value, allowed))
}

func curateJSON(value any, allowed map[string]bool) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any)
		for key, child := range v {
			if allowed[key] {
				out[key] = curateJSON(child, allowed)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = curateJSON(v[i], allowed)
		}
		return out
	default:
		return value
	}
}

func inspectPipeline(sourceDir string) ([]string, string, error) {
	projects := make(map[string]bool)
	status := ""
	for _, name := range []string{"tasks.json", "status.json"} {
		data, err := os.ReadFile(filepath.Join(sourceDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, "", fmt.Errorf("%s JSON 파싱 실패: %w", name, err)
		}
		collectStringField(value, "repo", projects)
		if name == "status.json" {
			if object, ok := value.(map[string]any); ok {
				status, _ = object["status"].(string)
			}
		}
	}
	if len(projects) == 0 {
		projects["."] = true
	}
	list := make([]string, 0, len(projects))
	for project := range projects {
		list = append(list, project)
	}
	sort.Strings(list)
	return list, status, nil
}

func collectStringField(value any, field string, result map[string]bool) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if key == field {
				if text, ok := child.(string); ok && text != "" {
					result[text] = true
				}
			}
			collectStringField(child, field, result)
		}
	case []any:
		for _, child := range v {
			collectStringField(child, field, result)
		}
	}
}
