package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// DashboardInfo tracks a running dashboard instance for a workspace.
type DashboardInfo struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Workspace string `json:"workspace"`
}

func dashInfoPath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".pylon", "runtime", "dashboard.json")
}

// readDashboardInfo reads the dashboard info file. Returns nil if not found.
func readDashboardInfo(workspaceRoot string) *DashboardInfo {
	data, err := os.ReadFile(dashInfoPath(workspaceRoot))
	if err != nil {
		return nil
	}
	var info DashboardInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil
	}
	return &info
}

// isDashboardAlive checks if the dashboard PID is still running.
func isDashboardAlive(info *DashboardInfo) bool {
	if info == nil || info.PID <= 0 {
		return false
	}
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return false
	}
	// Signal 0 tests process existence without actually sending a signal
	return proc.Signal(syscall.Signal(0)) == nil
}

// writeDashboardInfo writes dashboard info to the workspace runtime directory.
func writeDashboardInfo(workspaceRoot string, port int) error {
	info := DashboardInfo{
		PID:       os.Getpid(),
		Port:      port,
		Workspace: workspaceRoot,
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(dashInfoPath(workspaceRoot))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(dashInfoPath(workspaceRoot), data, 0644)
}

// removeDashboardInfo removes the dashboard info file.
func removeDashboardInfo(workspaceRoot string) {
	_ = os.Remove(dashInfoPath(workspaceRoot))
}

// checkExistingDashboard checks if a dashboard is already running for this workspace.
// Returns the port if alive, 0 if not.
func checkExistingDashboard(workspaceRoot string) int {
	info := readDashboardInfo(workspaceRoot)
	if info == nil {
		return 0
	}
	if isDashboardAlive(info) {
		return info.Port
	}
	// Stale info file — remove it
	fmt.Fprintf(os.Stderr, "⚠ 이전 대시보드 프로세스(PID %d)가 종료되어 정보 파일을 정리합니다\n", info.PID)
	removeDashboardInfo(workspaceRoot)
	return 0
}
