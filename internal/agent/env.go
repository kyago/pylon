// Package agent implements the agent execution engine for pylon.
// Spec Reference: Section 5, 8
package agent

// ResolveEnv merges global and agent-specific environment variables.
// Agent env takes precedence over global env.
// Priority: agent frontmatter env > config.yml runtime.env > system default
func ResolveEnv(globalEnv, agentEnv map[string]string) map[string]string {
	merged := make(map[string]string)

	// Start with global env
	for k, v := range globalEnv {
		merged[k] = v
	}

	// Agent env overrides
	for k, v := range agentEnv {
		merged[k] = v
	}

	return merged
}
