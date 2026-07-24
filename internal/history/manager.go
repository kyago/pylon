// Package history manages the file-based, curated workspace history.
// 체크포인트 1건 = .pylon/history/pipelines/<pipeline-id>/<phase>/ 디렉토리.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/layout"
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

// Manager records curated pipeline history as directory snapshots.
type Manager struct {
	Root string
	Now  func() time.Time
}

func NewManager(root string) *Manager {
	return &Manager{Root: root, Now: time.Now}
}

func (m *Manager) historyDir() string   { return layout.HistoryDir(m.Root) }
func (m *Manager) pipelinesDir() string { return filepath.Join(m.historyDir(), "pipelines") }

func (m *Manager) phaseDir(pipelineID string, phase Phase) string {
	return filepath.Join(m.pipelinesDir(), pipelineID, string(phase))
}

type Manifest struct {
	SchemaVersion    int               `json:"schema_version"` // 파일 기반 포맷은 2
	PipelineID       string            `json:"pipeline_id"`
	Phase            Phase             `json:"phase"`
	RecordedAt       time.Time         `json:"recorded_at"`
	Status           string            `json:"status"`
	Digest           string            `json:"digest"`
	AffectedProjects []string          `json:"affected_projects"`
	Artifacts        map[string]string `json:"artifacts"`
}

type CheckpointResult struct {
	PipelineID string `json:"pipeline_id"`
	Phase      Phase  `json:"phase"`
	Ref        string `json:"ref"`
	Digest     string `json:"digest"`
	Duplicate  bool   `json:"duplicate"`
}

// Checkpoint captures a curated snapshot of a pipeline's runtime directory.
func (m *Manager) Checkpoint(pipelineID string, phase Phase) (CheckpointResult, error) {
	result := CheckpointResult{PipelineID: pipelineID, Phase: phase}
	if err := validatePipelineID(pipelineID); err != nil {
		return result, err
	}
	if !validPhases[phase] {
		return result, fmt.Errorf("지원하지 않는 history phase: %s", phase)
	}
	if err := os.MkdirAll(m.pipelinesDir(), 0755); err != nil {
		return result, err
	}

	unlock, err := fsutil.AcquireLock(filepath.Join(m.historyDir(), ".checkpoint.lock"), 10*time.Second)
	if err != nil {
		return result, err
	}
	defer unlock()

	sourceDir := filepath.Join(layout.RuntimeDir(m.Root), pipelineID)
	if info, err := os.Stat(sourceDir); err != nil || !info.IsDir() {
		return result, fmt.Errorf("파이프라인 디렉토리를 찾을 수 없습니다: %s", sourceDir)
	}

	tempDir, err := os.MkdirTemp(m.pipelinesDir(), "."+pipelineID+"-")
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
	result.Ref = pipelineID + "/" + string(phase)
	result.Digest = digest

	target := m.phaseDir(pipelineID, phase)
	prev, err := readManifest(target)
	if err != nil {
		return result, err
	}
	if prev != nil && prev.Digest == digest {
		result.Duplicate = true
		return result, nil
	}

	manifest := Manifest{
		SchemaVersion: 2, PipelineID: pipelineID, Phase: phase,
		RecordedAt: m.Now().UTC(), Status: status, Digest: digest,
		AffectedProjects: projects, Artifacts: artifacts,
	}
	if err := fsutil.WriteJSONAtomic(filepath.Join(tempDir, "manifest.json"), manifest); err != nil {
		return result, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return result, err
	}
	// 기존 스냅샷은 삭제 대신 옆으로 옮긴 뒤 교체한다 — 삭제와 rename 사이에
	// 프로세스가 죽으면 해당 phase의 유일한 사본이 사라지기 때문이다.
	// aside는 pipelines/ 바로 아래(depth 1)라 Log의 <id>/<phase> glob에 잡히지 않는다.
	aside := filepath.Join(m.pipelinesDir(), "."+pipelineID+"-"+string(phase)+".old")
	if err := os.RemoveAll(aside); err != nil {
		return result, err
	}
	restore := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, aside); err != nil {
			return result, err
		}
		restore = true
	}
	if err := os.Rename(tempDir, target); err != nil {
		if restore {
			_ = os.Rename(aside, target) // 교체 실패 시 기존 스냅샷을 되돌린다
		}
		return result, err
	}
	_ = os.RemoveAll(aside)
	return result, nil
}

func readManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest 파싱 실패 (%s): %w", dir, err)
	}
	return &m, nil
}

// curatedMemoryCategories are the only memory categories preserved in history
// snapshots — raw 변경 이력 같은 오염 카테고리는 제외한다.
var curatedMemoryCategories = []string{"learning", "decision", "architecture", "pattern"}

// exportMemory copies the affected projects' curated markdown memory into the snapshot.
func (m *Manager) exportMemory(destDir string, projects []string) error {
	seen := make(map[string]bool)
	for _, project := range projects {
		projectID := filepath.Base(project)
		if project == "." {
			projectID = filepath.Base(m.Root)
		}
		// 서로 다른 repo가 같은 basename을 가지면 projectID가 겹친다
		// (예: services/api, vendor/api) — 중복 복사로 실패하지 않도록 한 번만 복사한다.
		if seen[projectID] {
			continue
		}
		seen[projectID] = true
		src := layout.ProjectMemoryDir(m.Root, projectID)
		if info, err := os.Stat(src); err != nil || !info.IsDir() {
			continue
		}
		for _, category := range curatedMemoryCategories {
			categorySrc := filepath.Join(src, category)
			if info, err := os.Stat(categorySrc); err != nil || !info.IsDir() {
				continue
			}
			if err := fsutil.CopyDir(categorySrc, filepath.Join(destDir, "memory", projectID, category)); err != nil {
				return err
			}
		}
	}
	return nil
}

type LogEntry struct {
	Ref        string    `json:"ref"`
	PipelineID string    `json:"pipeline_id"`
	Phase      Phase     `json:"phase"`
	RecordedAt time.Time `json:"recorded_at"`
	Status     string    `json:"status"`
}

// Log returns checkpoints sorted by recording time, newest first.
func (m *Manager) Log(pipelineID string, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	pattern := filepath.Join(m.pipelinesDir(), "*", "*", "manifest.json")
	if pipelineID != "" {
		if err := validatePipelineID(pipelineID); err != nil {
			return nil, err
		}
		pattern = filepath.Join(m.pipelinesDir(), pipelineID, "*", "manifest.json")
	}
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var entries []LogEntry
	for _, path := range paths {
		manifest, err := readManifest(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		if manifest == nil {
			continue
		}
		entries = append(entries, LogEntry{
			Ref:        manifest.PipelineID + "/" + string(manifest.Phase),
			PipelineID: manifest.PipelineID,
			Phase:      manifest.Phase,
			RecordedAt: manifest.RecordedAt,
			Status:     manifest.Status,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].RecordedAt.After(entries[j].RecordedAt) })
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

// resolveRef parses "<pipeline-id>/<phase>" into an existing snapshot directory.
func (m *Manager) resolveRef(ref string) (string, error) {
	id, phase, ok := strings.Cut(ref, "/")
	if !ok || id == "" || phase == "" {
		return "", fmt.Errorf("ref 형식은 <pipeline-id>/<phase> 입니다: %q", ref)
	}
	if err := validatePipelineID(id); err != nil {
		return "", err
	}
	if !validPhases[Phase(phase)] {
		return "", fmt.Errorf("지원하지 않는 phase: %s", phase)
	}
	dir := m.phaseDir(id, Phase(phase))
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("체크포인트를 찾을 수 없습니다: %s", ref)
	}
	return dir, nil
}

// Show returns the manifest and sorted artifact list for a checkpoint ref.
func (m *Manager) Show(ref string) (*Manifest, []string, error) {
	dir, err := m.resolveRef(ref)
	if err != nil {
		return nil, nil, err
	}
	manifest, err := readManifest(dir)
	if err != nil {
		return nil, nil, err
	}
	if manifest == nil {
		return nil, nil, fmt.Errorf("manifest.json이 없습니다: %s", ref)
	}
	files := make([]string, 0, len(manifest.Artifacts))
	for name := range manifest.Artifacts {
		files = append(files, name)
	}
	sort.Strings(files)
	return manifest, files, nil
}

// Diff runs POSIX diff -ru between two checkpoint snapshots.
func (m *Manager) Diff(from, to string) (string, error) {
	fromDir, err := m.resolveRef(from)
	if err != nil {
		return "", err
	}
	toDir, err := m.resolveRef(to)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("diff", "-ru", fromDir, toDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return string(out), nil // 종료 코드 1 = 차이 존재 (정상)
		}
		return string(out), fmt.Errorf("diff 실행 실패: %w", err)
	}
	return string(out), nil
}

// Export copies a checkpoint snapshot to a new directory.
func (m *Manager) Export(ref, output string) error {
	if output == "" {
		return fmt.Errorf("output 경로가 필요합니다")
	}
	dir, err := m.resolveRef(ref)
	if err != nil {
		return err
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	return fsutil.CopyDir(dir, absOutput)
}
