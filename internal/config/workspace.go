package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectInfo holds basic information about a discovered project.
type ProjectInfo struct {
	Name string
	Path string
}

// FindWorkspaceRoot walks up from startDir looking for a .pylon/ directory
// that contains a config.yml file (indicating a workspace root, not a sub-project).
// Sub-project .pylon/ directories may exist without config.yml, so the search
// continues upward until a proper workspace root is found.
// Returns the workspace root path or an error if not found.
func FindWorkspaceRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	for {
		pylonDir := filepath.Join(dir, ".pylon")
		info, err := os.Stat(pylonDir)
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to stat %s: %w", pylonDir, err)
		}
		if err == nil && info.IsDir() {
			configPath := filepath.Join(pylonDir, "config.yml")
			configInfo, err := os.Stat(configPath)
			if err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("failed to stat %s: %w", configPath, err)
			}
			if err == nil {
				if configInfo.Mode().IsRegular() {
					return dir, nil
				}
				return "", fmt.Errorf("invalid workspace config: %s exists but is not a regular file", configPath)
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return "", fmt.Errorf("no .pylon/config.yml found (searched from %s to filesystem root)", startDir)
		}
		dir = parent
	}
}

// DiscoverProjects scans the workspace root for project directories.
// A project is identified as a subdirectory containing its own .pylon/ directory
// or being registered as a git submodule.
// Spec Reference: Section 4 "Workspace Structure"
func DiscoverProjects(root string) ([]ProjectInfo, error) {
	var projects []ProjectInfo

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("failed to read workspace root: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip hidden directories and common non-project directories
		if strings.HasPrefix(name, ".") {
			continue
		}

		projectDir := filepath.Join(root, name)

		// Check if directory has its own .pylon/ (project-level config)
		projectPylonDir := filepath.Join(projectDir, ".pylon")
		if _, err := os.Stat(projectPylonDir); err == nil {
			projects = append(projects, ProjectInfo{
				Name: name,
				Path: projectDir,
			})
		}
	}

	return projects, nil
}

// LoadAllAgents loads all agent configurations from the workspace.
// It reads both root-level agents (.pylon/agents/) and project-level agents.
func LoadAllAgents(root string) (map[string]*AgentConfig, error) {
	agents := make(map[string]*AgentConfig)

	// Load root agents from .pylon/agents/
	rootAgentsDir := filepath.Join(root, ".pylon", "agents")
	if err := loadAgentsFromDir(rootAgentsDir, agents); err != nil {
		return nil, fmt.Errorf("failed to load root agents: %w", err)
	}

	// Load project-level agents
	projects, err := DiscoverProjects(root)
	if err != nil {
		// Non-fatal: just skip project agents
		return agents, nil
	}

	for _, project := range projects {
		projectAgentsDir := filepath.Join(project.Path, ".pylon", "agents")
		if err := loadAgentsFromDir(projectAgentsDir, agents); err != nil {
			// Non-fatal: log warning and continue
			continue
		}
	}

	return agents, nil
}

// loadAgentsFromDir reads all .md files in a directory and parses them as agent configs.
func loadAgentsFromDir(dir string, agents map[string]*AgentConfig) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directory doesn't exist, that's fine
		}
		return fmt.Errorf("failed to read agents directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		agent, err := ParseAgentFile(path)
		if err != nil {
			// Non-fatal: skip invalid agent files
			continue
		}

		agents[agent.Name] = agent
	}

	return nil
}
