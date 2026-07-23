package history

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
