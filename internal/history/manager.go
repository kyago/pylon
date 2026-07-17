// Package history manages the Fossil-backed, curated workspace history.
package history

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

type Phase string

const (
	PhasePlanned   Phase = "planned"
	PhaseExecuted  Phase = "executed"
	PhaseCompleted Phase = "completed"
	PhaseCancelled Phase = "cancelled"
	PhaseFailed    Phase = "failed"
)

var validPhases = map[Phase]bool{
	PhasePlanned: true, PhaseExecuted: true, PhaseCompleted: true,
	PhaseCancelled: true, PhaseFailed: true,
}

// isTerminalPhase reports whether a phase records the final state of a
// pipeline, after which the runtime directory may be safely removed.
func isTerminalPhase(phase Phase) bool {
	return phase == PhaseCompleted || phase == PhaseCancelled || phase == PhaseFailed
}

type CommandRunner interface {
	Run(dir string, args ...string) (string, error)
}

type fossilRunner struct{}

func (fossilRunner) Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("fossil", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("fossil %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

type Manager struct {
	Root   string
	Config config.HistoryConfig
	Store  *store.Store
	Runner CommandRunner
	Now    func() time.Time
}

type Manifest struct {
	SchemaVersion    int               `json:"schema_version"`
	PipelineID       string            `json:"pipeline_id"`
	Phase            Phase             `json:"phase"`
	RecordedAt       time.Time         `json:"recorded_at"`
	Status           string            `json:"status"`
	AffectedProjects []string          `json:"affected_projects"`
	Artifacts        map[string]string `json:"artifacts"`
}

type CheckpointResult struct {
	PipelineID  string `json:"pipeline_id"`
	Phase       Phase  `json:"phase"`
	Checkin     string `json:"checkin"`
	Duplicate   bool   `json:"duplicate"`
	PendingSync bool   `json:"pending_sync"`
}

type checkpointState struct {
	Digest      string `json:"digest"`
	Checkin     string `json:"checkin"`
	PendingSync bool   `json:"pending_sync"`
}

type historyState struct {
	// Checkpoints is an external deduplication cache. Fossil remains the durable
	// history; deleting state.json only causes an otherwise identical checkpoint
	// to be committed again.
	Checkpoints map[string]map[Phase]checkpointState `json:"checkpoints"`
}

func NewManager(root string, cfg config.HistoryConfig, s *store.Store, runner CommandRunner) *Manager {
	if runner == nil {
		runner = fossilRunner{}
	}
	return &Manager{Root: root, Config: cfg, Store: s, Runner: runner, Now: time.Now}
}

func VerifyFossil() (string, error) {
	path, err := exec.LookPath("fossil")
	if err != nil {
		return "", fmt.Errorf("fossil 실행 파일을 찾을 수 없습니다: %w", err)
	}
	out, err := exec.Command(path, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fossil version 확인 실패: %w", err)
	}
	fields := strings.Fields(string(out))
	for i, field := range fields {
		if field == "version" && i+1 < len(fields) {
			return strings.TrimRight(fields[i+1], ",;"), nil
		}
	}
	return "", fmt.Errorf("fossil version 출력을 해석할 수 없습니다: %s", strings.TrimSpace(string(out)))
}

func (m *Manager) historyDir() string {
	return filepath.Join(m.Root, ".pylon", "history")
}

func (m *Manager) repositoryPath() string {
	return filepath.Join(m.historyDir(), "pylon-history.fossil")
}

func (m *Manager) checkoutDir() string {
	return filepath.Join(m.historyDir(), "checkout")
}

func (m *Manager) Initialize() error {
	if err := os.MkdirAll(m.historyDir(), 0700); err != nil {
		return fmt.Errorf("history directory 생성 실패: %w", err)
	}
	if _, err := os.Stat(m.repositoryPath()); os.IsNotExist(err) {
		if _, err := m.Runner.Run(m.Root, "init", m.repositoryPath()); err != nil {
			return fmt.Errorf("Fossil 저장소 초기화 실패: %w", err)
		}
	} else if err != nil {
		return err
	}

	if !m.checkoutInitialized() {
		if err := os.MkdirAll(m.checkoutDir(), 0700); err != nil {
			return err
		}
		args := []string{"open", m.repositoryPath(), "--workdir", m.checkoutDir(), "--nosync", "--force"}
		if _, err := m.Runner.Run(m.Root, args...); err != nil {
			return fmt.Errorf("Fossil checkout 초기화 실패: %w", err)
		}
	}

	statePath := filepath.Join(m.historyDir(), "state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return writeJSONAtomic(statePath, historyState{Checkpoints: make(map[string]map[Phase]checkpointState)})
	}
	return nil
}

func (m *Manager) IsInitialized() bool {
	_, repoErr := os.Stat(m.repositoryPath())
	return repoErr == nil && m.checkoutInitialized()
}

func (m *Manager) checkoutInitialized() bool {
	for _, name := range []string{".fslckout", "_FOSSIL_", ".fos"} {
		if _, err := os.Stat(filepath.Join(m.checkoutDir(), name)); err == nil {
			return true
		}
	}
	return false
}

func (m *Manager) Log(pipelineID string, limit int) (string, error) {
	if !m.IsInitialized() {
		return "", fmt.Errorf("history 저장소가 초기화되지 않았습니다 — 'pylon history init'을 실행하세요")
	}
	if limit <= 0 {
		limit = 20
	}
	fossilLimit := limit
	if pipelineID != "" {
		fossilLimit = 0
	}
	out, err := m.Runner.Run(m.Root, "timeline", "--oneline", "-n", fmt.Sprintf("%d", fossilLimit), "-t", "ci", "-R", m.repositoryPath())
	if err != nil {
		return strings.TrimSpace(out), err
	}
	var timelineLines []string
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "+++") {
			timelineLines = append(timelineLines, line)
		}
	}
	if pipelineID == "" {
		return strings.TrimSpace(strings.Join(timelineLines, "\n")), nil
	}
	marker := fmt.Sprintf("파이프라인 %s:", pipelineID)
	var filtered []string
	for _, line := range timelineLines {
		if strings.Contains(line, marker) {
			filtered = append(filtered, line)
			if len(filtered) == limit {
				break
			}
		}
	}
	return strings.TrimSpace(strings.Join(filtered, "\n")), nil
}

func (m *Manager) Show(checkin string) (string, error) {
	if checkin == "" {
		return "", fmt.Errorf("checkin이 필요합니다")
	}
	if !m.IsInitialized() {
		return "", fmt.Errorf("history 저장소가 초기화되지 않았습니다")
	}
	out, err := m.Runner.Run(m.Root, "info", checkin, "-R", m.repositoryPath())
	return strings.TrimSpace(out), err
}

func (m *Manager) Diff(from, to string) (string, error) {
	if from == "" || to == "" {
		return "", fmt.Errorf("from과 to checkin이 필요합니다")
	}
	if !m.IsInitialized() {
		return "", fmt.Errorf("history 저장소가 초기화되지 않았습니다")
	}
	out, err := m.Runner.Run(m.Root, "diff", "--from", from, "--to", to, "-R", m.repositoryPath())
	return strings.TrimSpace(out), err
}

func (m *Manager) Sync() error {
	if m.Config.Remote == "" {
		return fmt.Errorf("history.remote가 설정되지 않았습니다")
	}
	if !m.IsInitialized() {
		return fmt.Errorf("history 저장소가 초기화되지 않았습니다")
	}
	if _, err := m.Runner.Run(m.Root, "sync", m.Config.Remote, "--once", "-R", m.repositoryPath()); err != nil {
		return fmt.Errorf("Fossil 동기화 실패: %w", err)
	}
	state, err := m.loadState()
	if err != nil {
		return err
	}
	for pipelineID, phases := range state.Checkpoints {
		for phase, checkpoint := range phases {
			checkpoint.PendingSync = false
			phases[phase] = checkpoint
		}
		state.Checkpoints[pipelineID] = phases
	}
	return m.saveState(state)
}

func (m *Manager) Export(checkin, output string) error {
	if checkin == "" || output == "" {
		return fmt.Errorf("checkin과 output 경로가 필요합니다")
	}
	if !m.IsInitialized() {
		return fmt.Errorf("history 저장소가 초기화되지 않았습니다")
	}
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("export 대상이 이미 존재합니다: %s", output)
	} else if !os.IsNotExist(err) {
		return err
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absOutput), 0755); err != nil {
		return err
	}
	archive, err := os.CreateTemp(m.historyDir(), ".export-*.tar.gz")
	if err != nil {
		return err
	}
	archivePath := archive.Name()
	if err := archive.Close(); err != nil {
		return err
	}
	defer os.Remove(archivePath)
	if _, err := m.Runner.Run(m.Root, "tarball", checkin, archivePath, "--name", "pylon-history", "-R", m.repositoryPath()); err != nil {
		return fmt.Errorf("history export 실패: %w", err)
	}
	if err := extractTarball(archivePath, absOutput); err != nil {
		_ = os.RemoveAll(absOutput)
		return fmt.Errorf("history export 압축 해제 실패: %w", err)
	}
	return nil
}

func extractTarball(archivePath, output string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(header.Name)
		if filepath.ToSlash(clean) == "pylon-history" && header.Typeflag == tar.TypeDir {
			continue
		}
		parts := strings.Split(filepath.ToSlash(clean), "/")
		if len(parts) < 2 || parts[0] != "pylon-history" {
			return fmt.Errorf("예상하지 못한 archive 경로: %s", header.Name)
		}
		rel := filepath.FromSlash(strings.Join(parts[1:], "/"))
		target := filepath.Join(output, rel)
		if !strings.HasPrefix(target, output+string(filepath.Separator)) {
			return fmt.Errorf("안전하지 않은 archive 경로: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			mode := os.FileMode(header.Mode) & 0777
			if mode == 0 {
				mode = 0644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, reader)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("지원하지 않는 archive 항목: %s", header.Name)
		}
	}
}

func (m *Manager) Checkpoint(pipelineID string, phase Phase) (CheckpointResult, error) {
	result := CheckpointResult{PipelineID: pipelineID, Phase: phase}
	if err := validatePipelineID(pipelineID); err != nil {
		return result, err
	}
	if !validPhases[phase] {
		return result, fmt.Errorf("지원하지 않는 history phase: %s", phase)
	}
	if err := os.MkdirAll(m.historyDir(), 0700); err != nil {
		return result, err
	}

	unlock, err := acquireLock(filepath.Join(m.historyDir(), ".checkpoint.lock"), 10*time.Second)
	if err != nil {
		return result, err
	}
	defer unlock()

	if err := m.Initialize(); err != nil {
		return result, err
	}
	sourceDir := filepath.Join(m.Root, ".pylon", "runtime", pipelineID)
	if info, err := os.Stat(sourceDir); err != nil || !info.IsDir() {
		return result, fmt.Errorf("파이프라인 디렉토리를 찾을 수 없습니다: %s", sourceDir)
	}

	pipelinesDir := filepath.Join(m.checkoutDir(), "pipelines")
	if err := os.MkdirAll(pipelinesDir, 0755); err != nil {
		return result, err
	}
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
	if phases := state.Checkpoints[pipelineID]; phases != nil {
		if previous, ok := phases[phase]; ok && previous.Digest == digest {
			result.Checkin = previous.Checkin
			result.Duplicate = true
			result.PendingSync = previous.PendingSync
			return result, nil
		}
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

	if _, err := m.Runner.Run(m.checkoutDir(), "addremove"); err != nil {
		return result, fmt.Errorf("Fossil 변경 감지 실패: %w", err)
	}
	message := fmt.Sprintf("파이프라인 %s: %s", pipelineID, phaseLabel(phase))
	out, err := m.Runner.Run(m.checkoutDir(), "commit", "--nosync", "--no-prompt", "--no-warnings", "--no-verify-comment", "-m", message)
	if err != nil {
		return result, fmt.Errorf("Fossil 체크인 실패: %w", err)
	}
	result.Checkin = parseCheckin(out)
	if result.Checkin == "" {
		info, infoErr := m.Runner.Run(m.checkoutDir(), "info", "current")
		if infoErr != nil {
			return result, fmt.Errorf("Fossil 체크인 식별 실패: %w", infoErr)
		}
		result.Checkin = parseCheckin(info)
		if result.Checkin == "" {
			return result, fmt.Errorf("Fossil 체크인 식별자를 찾을 수 없습니다")
		}
	}

	if m.Config.Remote != "" {
		result.PendingSync = true
		if m.Config.SyncOnCheckpoint {
			if _, err := m.Runner.Run(m.Root, "sync", m.Config.Remote, "--once", "-R", m.repositoryPath()); err == nil {
				result.PendingSync = false
			}
		}
	}
	if state.Checkpoints[pipelineID] == nil {
		state.Checkpoints[pipelineID] = make(map[Phase]checkpointState)
	}
	state.Checkpoints[pipelineID][phase] = checkpointState{Digest: digest, Checkin: result.Checkin, PendingSync: result.PendingSync}
	if err := m.saveState(state); err != nil {
		return result, err
	}
	return result, nil
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

func digestTree(root string) (string, map[string]string, error) {
	artifacts := make(map[string]string)
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	sort.Strings(paths)
	total := sha256.New()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", nil, err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		sum := sha256.Sum256(data)
		artifacts[rel] = hex.EncodeToString(sum[:])
		io.WriteString(total, rel)
		total.Write([]byte{0})
		total.Write(data)
		total.Write([]byte{0})
	}
	return hex.EncodeToString(total.Sum(nil)), artifacts, nil
}

func (m *Manager) loadState() (historyState, error) {
	state := historyState{Checkpoints: make(map[string]map[Phase]checkpointState)}
	data, err := os.ReadFile(filepath.Join(m.historyDir(), "state.json"))
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	if state.Checkpoints == nil {
		state.Checkpoints = make(map[string]map[Phase]checkpointState)
	}
	return state, nil
}

func (m *Manager) saveState(state historyState) error {
	return writeJSONAtomic(filepath.Join(m.historyDir(), "state.json"), state)
}

func validatePipelineID(id string) error {
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return fmt.Errorf("유효하지 않은 pipeline ID: %q", id)
	}
	return nil
}

func phaseLabel(phase Phase) string {
	switch phase {
	case PhasePlanned:
		return "계획 완료"
	case PhaseExecuted:
		return "실행 완료"
	case PhaseCancelled:
		return "작업 취소"
	case PhaseFailed:
		return "작업 실패"
	default:
		return "작업 완료"
	}
}

func parseCheckin(output string) string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		for _, prefix := range []string{"New_Version:", "checkout:"} {
			if strings.HasPrefix(line, prefix) {
				fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
				if len(fields) > 0 {
					return fields[0]
				}
			}
		}
	}
	return ""
}

func copyIfExists(source, dest string) error {
	data, err := os.ReadFile(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0644)
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func acquireLock(path string, timeout time.Duration) (func(), error) {
	deadline := time.Now().Add(timeout)
	for {
		if err := os.Mkdir(path, 0700); err == nil {
			return func() { _ = os.Remove(path) }, nil
		} else if !os.IsExist(err) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("history checkpoint 잠금 시간 초과: %s (실행 중인 pylon 프로세스가 없다면 이 디렉토리를 제거한 뒤 다시 시도하세요)", path)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
