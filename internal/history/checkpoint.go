package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
		for _, artifact := range []struct {
			name    string
			dest    string
			allowed map[string]bool
			filter  func(string) bool
		}{
			{name: "criteria.json", dest: "criteria-summary.json", allowed: criteriaKeys},
			{name: "task-report.json", dest: "task-reports-summary.json", allowed: taskReportKeys},
			{name: "failure-record.json", dest: "failure-records-summary.json", allowed: failureKeys},
			{name: "evaluator-result.json", dest: "evaluator-summary.json", allowed: evaluatorKeys},
			{name: "state.json", dest: "attempt-state-summary.json", allowed: attemptStateKeys, filter: taskStatePath},
			{name: "provider.json", dest: "provider-summary.json", allowed: providerKeys, filter: attemptArtifactPath},
			{name: "result.json", dest: "attempt-result-summary.json", allowed: attemptResultKeys, filter: attemptArtifactPath},
		} {
			if err := summarizeRecursiveFiles(sourceDir, artifact.name, filepath.Join(destDir, artifact.dest), artifact.allowed, artifact.filter); err != nil {
				return nil, "", err
			}
		}
		if err := m.exportMemory(destDir, projects); err != nil {
			return nil, "", err
		}
	}
	return projects, status, nil
}

func summarizeRecursiveFiles(sourceDir, name, dest string, allowed map[string]bool, filter func(string) bool) error {
	type record struct {
		Path   string `json:"path"`
		Result any    `json:"result"`
	}
	records := make([]record, 0)
	err := filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "evaluator-input" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != name {
			return nil
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if filter != nil && !filter(relative) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("%s JSON 파싱 실패: %w", path, err)
		}
		records = append(records, record{Path: relative, Result: curateJSON(value, allowed)})
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })
	return fsutil.WriteJSONAtomic(dest, map[string]any{"records": records})
}

func taskStatePath(relative string) bool {
	return strings.HasPrefix(relative, "tasks/") && strings.HasSuffix(relative, "/state.json")
}

func attemptArtifactPath(relative string) bool {
	return strings.HasPrefix(relative, "tasks/") && strings.Contains(relative, "/attempts/")
}

func summarizeResultFiles(sourceDir, name, dest string, allowed map[string]bool) error {
	rootFile := filepath.Join(sourceDir, name)
	if _, err := os.Stat(rootFile); err == nil {
		return summarizeJSONFile(rootFile, dest, allowed)
	} else if !os.IsNotExist(err) {
		return err
	}
	var paths []string
	for _, pattern := range []string{
		filepath.Join(sourceDir, "*", name),
		filepath.Join(sourceDir, "repos", "*", name),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return err
		}
		paths = append(paths, matches...)
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
	"attempt": true, "provider": true, "capabilities": true, "evidence_refs": true,
	"hypotheses_rejected": true, "remaining_unknowns": true,
}
var verificationKeys = map[string]bool{
	// reason: 검증이 수행되지 않아 ok=false가 된 사유. 이게 빠지면 요약만 보고는
	// "검증 실패"와 "검증 미수행"을 구분할 수 없다.
	"ok": true, "skipped": true, "reason": true, "timestamp": true, "checks": true, "name": true,
	"kind": true, "command": true, "exit_code": true, "timed_out": true, "duration_ms": true,
	"output": true, "criteria_digest": true, "integrity_ok": true, "source_changed": true,
}
var prKeys = map[string]bool{"url": true, "number": true, "title": true, "repo": true, "prs": true}
var statusKeys = map[string]bool{
	"stage": true, "status": true, "branch": true, "started_at": true,
	"completed_at": true, "sub_pipelines": true, "repo": true, "pipeline_dir": true,
	"scope": true, "root_pipeline_id": true, "repo_id": true, "base_revision": true,
	"criteria": true, "evaluator_request": true, "failure_record": true, "cleanup": true,
	"path": true, "digest": true, "source_digest": true, "created_at": true, "recorded_at": true,
	"task_id": true, "attempt": true, "phase": true, "reason": true, "error": true,
}

var criteriaKeys = map[string]bool{
	"schema_version": true, "run_id": true, "repo_id": true, "base_revision": true,
	"acceptance_criteria": true, "id": true, "description": true, "verification": true,
	"name": true, "kind": true, "command": true, "source": true, "path": true,
	"mode": true, "digest": true, "created_at": true,
}

var taskReportKeys = map[string]bool{
	"schema_version": true, "run_id": true, "task_id": true, "repo_id": true,
	"attempt": true, "status": true, "summary": true, "provider": true, "name": true,
	"external_id": true, "capabilities": true, "required_capabilities": true,
	"changed_files": true, "evidence_refs": true, "verification": true,
	"criteria_digest": true, "deterministic_passed": true, "evaluator_status": true,
	"hypotheses_rejected": true, "hypothesis": true, "probe": true, "result": true,
	"remaining_unknowns": true, "started_at": true, "completed_at": true, "digest": true,
}

var failureKeys = map[string]bool{
	"schema_version": true, "run_id": true, "task_id": true, "repo_id": true,
	"attempt": true, "phase": true, "terminal_cause": true, "evidence_refs": true,
	"hypotheses_rejected": true, "hypothesis": true, "probe": true, "result": true,
	"remaining_unknowns": true, "abandoned_reason": true, "recorded_at": true, "digest": true,
}

var evaluatorKeys = map[string]bool{
	"schema_version": true, "request_digest": true, "status": true, "summary": true,
	"criteria": true, "id": true, "evidence": true, "risks": true,
	"evidence_refs": true, "evaluator": true, "recorded_at": true,
}

var attemptStateKeys = map[string]bool{
	"schema_version": true, "run_id": true, "task_id": true, "status": true,
	"revision": true, "attempt": true, "provider": true, "result": true,
	"verification": true, "message": true, "created_at": true, "updated_at": true,
	"name": true, "external_id": true, "resume_token": true, "state": true,
	"exit_code": true, "changed_files": true, "evidence": true,
	"deterministic_passed": true, "evaluator_passed": true, "evidence_refs": true, "recorded_at": true,
}

var providerKeys = map[string]bool{
	"provider": true, "external_id": true, "resume_token": true, "attempt": true,
}

var attemptResultKeys = map[string]bool{
	"state": true, "exit_code": true, "changed_files": true, "evidence": true,
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
