package history

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func (m *Manager) Checkpoint(pipelineID string, phase Phase) (CheckpointResult, error) {
	result := CheckpointResult{PipelineID: pipelineID, Phase: phase}
	if err := validatePipelineID(pipelineID); err != nil {
		return result, err
	}
	if !validPhases[phase] {
		return result, fmt.Errorf("지원하지 않는 history phase: %s", phase)
	}
	sourceDir, pipelinesDir, unlock, err := m.beginCheckpoint(pipelineID)
	if err != nil {
		return result, err
	}
	defer unlock()
	tempDir, err := os.MkdirTemp(pipelinesDir, "."+pipelineID+"-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(tempDir)

	projects, status, err := m.stageCheckpoint(sourceDir, tempDir, phase)
	if err != nil {
		return result, err
	}
	digest, artifacts, err := digestTree(tempDir)
	if err != nil {
		return result, err
	}

	state, err := m.loadState()
	if err != nil {
		return result, err
	}
	if previous, ok := duplicateCheckpoint(state, pipelineID, phase, digest); ok {
		result.Checkin = previous.Checkin
		result.Duplicate = true
		result.PendingSync = previous.PendingSync
		return result, nil
	}

	manifest := Manifest{
		SchemaVersion: 1, PipelineID: pipelineID, Phase: phase,
		RecordedAt: m.Now().UTC(), Status: status,
		AffectedProjects: projects, Artifacts: artifacts,
	}
	if err := writeJSONAtomic(filepath.Join(tempDir, "manifest.json"), manifest); err != nil {
		return result, err
	}

	targetDir := filepath.Join(pipelinesDir, pipelineID)
	if err := os.RemoveAll(targetDir); err != nil {
		return result, err
	}
	if err := os.Rename(tempDir, targetDir); err != nil {
		return result, err
	}

	result.Checkin, err = m.commitCheckpoint(pipelineID, phase)
	if err != nil {
		return result, err
	}
	result.PendingSync = m.checkpointPendingSync()
	if state.Checkpoints[pipelineID] == nil {
		state.Checkpoints[pipelineID] = make(map[Phase]checkpointState)
	}
	state.Checkpoints[pipelineID][phase] = checkpointState{Digest: digest, Checkin: result.Checkin, PendingSync: result.PendingSync}
	if err := m.saveState(state); err != nil {
		return result, err
	}
	return result, nil
}

func (m *Manager) beginCheckpoint(pipelineID string) (string, string, func(), error) {
	if err := os.MkdirAll(m.historyDir(), 0o700); err != nil {
		return "", "", nil, err
	}
	unlock, err := acquireLock(filepath.Join(m.historyDir(), ".checkpoint.lock"), 10*time.Second)
	if err != nil {
		return "", "", nil, err
	}
	if err := m.Initialize(); err != nil {
		unlock()
		return "", "", nil, err
	}
	sourceDir := filepath.Join(m.Root, ".pylon", "runtime", pipelineID)
	if info, err := os.Stat(sourceDir); err != nil || !info.IsDir() {
		unlock()
		return "", "", nil, fmt.Errorf("파이프라인 디렉토리를 찾을 수 없습니다: %s", sourceDir)
	}
	pipelinesDir := filepath.Join(m.checkoutDir(), "pipelines")
	if err := os.MkdirAll(pipelinesDir, 0o755); err != nil {
		unlock()
		return "", "", nil, err
	}
	return sourceDir, pipelinesDir, unlock, nil
}

func duplicateCheckpoint(state historyState, pipelineID string, phase Phase, digest string) (checkpointState, bool) {
	previous, ok := state.Checkpoints[pipelineID][phase]
	return previous, ok && previous.Digest == digest
}

func (m *Manager) commitCheckpoint(pipelineID string, phase Phase) (string, error) {
	if _, err := m.Runner.Run(m.checkoutDir(), "addremove"); err != nil {
		return "", fmt.Errorf("fossil 변경 감지 실패: %w", err)
	}
	message := fmt.Sprintf("파이프라인 %s: %s", pipelineID, phaseLabel(phase))
	out, err := m.Runner.Run(m.checkoutDir(), "commit", "--nosync", "--no-prompt", "--no-warnings", "--no-verify-comment", "-m", message)
	if err != nil {
		return "", fmt.Errorf("fossil 체크인 실패: %w", err)
	}
	if checkin := parseCheckin(out); checkin != "" {
		return checkin, nil
	}
	info, err := m.Runner.Run(m.checkoutDir(), "info", "current")
	if err != nil {
		return "", fmt.Errorf("fossil 체크인 식별 실패: %w", err)
	}
	if checkin := parseCheckin(info); checkin != "" {
		return checkin, nil
	}
	return "", fmt.Errorf("fossil 체크인 식별자를 찾을 수 없습니다")
}

func (m *Manager) checkpointPendingSync() bool {
	if m.Config.Remote == "" {
		return false
	}
	if m.Config.SyncOnCheckpoint {
		if _, err := m.Runner.Run(m.Root, "sync", m.Config.Remote, "--once", "-R", m.repositoryPath()); err == nil {
			return false
		}
	}
	return true
}

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
	return writeJSONAtomic(dest, map[string]any{"records": records})
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
	return writeJSONAtomic(dest, curateJSON(value, allowed))
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

func (m *Manager) exportMemory(destDir string, projects []string) error {
	if m.Store == nil {
		return nil
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	for _, project := range projects {
		projectID := filepath.Base(project)
		if project == "." {
			projectID = filepath.Base(m.Root)
		}
		for _, category := range []string{"learning", "decision", "architecture", "pattern"} {
			entries, err := m.Store.GetMemoryByCategory(projectID, category)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				record := map[string]any{
					"id": entry.ID, "project": entry.ProjectID, "category": entry.Category,
					"key": entry.Key, "content": entry.Content, "author": entry.Author,
					"confidence": entry.Confidence, "created_at": entry.CreatedAt,
				}
				if err := encoder.Encode(record); err != nil {
					return err
				}
			}
		}
	}
	if buf.Len() == 0 {
		return nil
	}
	return os.WriteFile(filepath.Join(destDir, "memory.jsonl"), buf.Bytes(), 0600)
}
