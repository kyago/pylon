// Package history manages Fossil-backed, curated workspace history.
package history

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
			return fmt.Errorf("fossil 저장소 초기화 실패: %w", err)
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
			return fmt.Errorf("fossil checkout 초기화 실패: %w", err)
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
		return fmt.Errorf("fossil 동기화 실패: %w", err)
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
