package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/kyago/pylon/internal/agent"
	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/git"
	"github.com/kyago/pylon/internal/memory"
	"github.com/kyago/pylon/internal/protocol"
	"github.com/kyago/pylon/internal/store"
)

// LoopConfig holds all dependencies for the orchestration loop.
type LoopConfig struct {
	Config      *config.Config
	Store       *store.Store
	WorkDir     string // workspace root
	PipelineID  string
	Requirement string
	Branch      string
	MemManager  *memory.Manager
	Runner      *agent.Runner
	WorktreeMgr *git.WorktreeManager
	Agents      map[string]*config.AgentConfig
	Projects    []config.ProjectInfo
}

// Loop drives the pipeline stage-by-stage execution.
type Loop struct {
	cfg      LoopConfig
	orch     *Orchestrator
	watcher  *OutboxWatcher
	inboxDir string
	mu       sync.Mutex // protects Pipeline.Agents writes and saveState calls
}

// NewLoop creates a new orchestration loop.
func NewLoop(cfg LoopConfig) *Loop {
	orch := NewOrchestrator(cfg.Config, cfg.Store, cfg.WorkDir)
	runtimeDir := filepath.Join(cfg.WorkDir, ".pylon", "runtime")

	return &Loop{
		cfg:      cfg,
		orch:     orch,
		watcher:  NewOutboxWatcher(filepath.Join(runtimeDir, "outbox"), 2*time.Second),
		inboxDir: filepath.Join(runtimeDir, "inbox"),
	}
}

// Run executes the pipeline from its current stage until completion or failure.
// If the pipeline is already at a PO interactive stage, it returns ErrInteractiveRequired.
func (l *Loop) Run(ctx context.Context) error {
	// Try to recover existing state (for --continue scenarios).
	// Skip recovery if a pipeline was pre-set via SetPipeline() to avoid clobbering it.
	if l.orch.Pipeline == nil {
		if err := l.orch.Recover(); err != nil {
			return fmt.Errorf("recovery failed: %w", err)
		}

		// If the recovered pipeline ID doesn't match, start fresh.
		if l.orch.Pipeline != nil && l.cfg.PipelineID != "" &&
			l.orch.Pipeline.ID != l.cfg.PipelineID {
			fmt.Fprintf(os.Stderr, "⚠ recovered pipeline %s does not match expected %s, starting fresh\n",
				l.orch.Pipeline.ID, l.cfg.PipelineID)
			l.orch.Pipeline = nil
		}
	}

	// If no pipeline recovered, start a new one
	if l.orch.Pipeline == nil {
		if err := l.orch.StartPipeline(l.cfg.PipelineID); err != nil {
			return fmt.Errorf("failed to start pipeline: %w", err)
		}
		l.orch.Pipeline.TaskSpec = l.cfg.Requirement
		l.orch.Pipeline.MaxAttempts = l.cfg.Config.Runtime.MaxAttempts
	}

	// Ensure agents map is initialized (may be nil after JSON recovery)
	if l.orch.Pipeline.Agents == nil {
		l.orch.Pipeline.Agents = make(map[string]AgentStatus)
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		if l.orch.Pipeline.IsTerminal() {
			return nil
		}

		var err error
		switch l.orch.Pipeline.CurrentStage {
		case StageInit:
			err = l.transitionTo(StagePOConversation)
		case StagePOConversation:
			err = l.runPOConversation(ctx)
		case StageArchitectAnalysis:
			err = l.runHeadlessAgent(ctx, "architect", StageArchitectAnalysis, StagePMTaskBreakdown)
		case StagePMTaskBreakdown:
			err = l.runHeadlessAgent(ctx, "pm", StagePMTaskBreakdown, StageAgentExecuting)
		case StageAgentExecuting:
			err = l.runAgentExecution(ctx)
		case StageVerification:
			err = l.runVerification(ctx)
		case StagePRCreation:
			err = l.runPRCreation(ctx)
		case StagePOValidation:
			err = l.runPOValidation(ctx)
		case StageWikiUpdate:
			err = l.runWikiUpdate(ctx)
		default:
			return fmt.Errorf("unknown stage: %s", l.orch.Pipeline.CurrentStage)
		}

		if err != nil {
			// ErrInteractiveRequired is a signal, not a failure
			if errors.Is(err, ErrInteractiveRequired) {
				return err
			}
			// Attempt to transition to failed state
			failErr := l.transitionTo(StageFailed)
			if failErr != nil {
				return fmt.Errorf("stage %s error: %w (also failed to transition: %v)",
					l.orch.Pipeline.CurrentStage, err, failErr)
			}
			return fmt.Errorf("pipeline failed at %s: %w", l.orch.Pipeline.CurrentStage, err)
		}
	}
}

// ErrInteractiveRequired signals that an interactive PO session is needed.
var ErrInteractiveRequired = errors.New("interactive PO session required")

// runPOConversation handles the PO interactive conversation stage.
// Since PO uses ExecInteractive (syscall.Exec), the loop returns ErrInteractiveRequired
// so that the CLI can handle the handoff.
func (l *Loop) runPOConversation(ctx context.Context) error {
	return ErrInteractiveRequired
}

// runHeadlessAgent executes a single headless agent and transitions to the next stage.
func (l *Loop) runHeadlessAgent(ctx context.Context, agentName string, currentStage, nextStage Stage) error {
	if err := l.executeAgent(ctx, agentName); err != nil {
		return err
	}
	return l.transitionTo(nextStage)
}

// executeAgent handles the common agent execution pattern without stage transition.
func (l *Loop) executeAgent(ctx context.Context, agentName string) error {
	agentCfg := l.findAgent(agentName)
	if agentCfg == nil {
		return fmt.Errorf("agent config not found: %s", agentName)
	}

	agentCfg.ResolveDefaults(l.cfg.Config)

	taskID := fmt.Sprintf("%s-%s", l.cfg.PipelineID, agentName)

	// Build handoff context from blackboard
	handoffCtx := l.buildHandoffContext(taskID)

	// Write task to inbox
	if err := l.writeTaskToInbox(agentName, taskID, handoffCtx); err != nil {
		return fmt.Errorf("failed to write task for %s: %w", agentName, err)
	}

	// Build outbox path for this agent
	outboxDir := filepath.Join(l.watcher.OutboxDir, agentName)
	outboxPath := filepath.Join(outboxDir, taskID+".result.json")

	// Build CLAUDE.md with concrete outbox path
	claudeMD, err := l.buildClaudeMD(agentCfg, taskID, outboxPath, outboxDir)
	if err != nil {
		return fmt.Errorf("failed to build CLAUDE.md for %s: %w", agentName, err)
	}

	// Prepare work directory
	projectDir := l.cfg.WorkDir
	if len(l.cfg.Projects) > 0 {
		projectDir = l.cfg.Projects[0].Path
	}

	workDir, cleanup, err := agent.PrepareWorkDir(
		l.cfg.WorktreeMgr, agentCfg.Isolation, l.cfg.Branch, projectDir, agentName,
	)
	if err != nil {
		return fmt.Errorf("failed to prepare workdir for %s: %w", agentName, err)
	}
	defer cleanup()

	// Build task prompt with concrete inbox and outbox paths
	taskPrompt := agent.BuildTaskPrompt(agentCfg.Role, agentName, taskID, l.inboxDir, l.watcher.OutboxDir)

	// Track agent status
	l.mu.Lock()
	l.orch.Pipeline.Agents[agentName] = AgentStatus{
		TaskID:  taskID,
		AgentID: agentName,
		Status:  AgentStatusRunning,
	}
	l.saveState()
	l.mu.Unlock()

	// Launch agent with context propagation
	result, err := l.cfg.Runner.Start(agent.RunConfig{
		Ctx:        ctx,
		Agent:      agentCfg,
		Global:     l.cfg.Config,
		TaskPrompt: taskPrompt,
		WorkDir:    workDir,
		ClaudeMD:   claudeMD,
	})
	if err != nil {
		l.mu.Lock()
		l.orch.Pipeline.Agents[agentName] = AgentStatus{TaskID: taskID, AgentID: agentName, Status: AgentStatusFailed}
		l.saveState()
		l.mu.Unlock()
		return fmt.Errorf("agent %s execution failed: %w", agentName, err)
	}

	if result.ExitCode != 0 {
		l.mu.Lock()
		l.orch.Pipeline.Agents[agentName] = AgentStatus{TaskID: taskID, AgentID: agentName, Status: AgentStatusFailed}
		l.saveState()
		l.mu.Unlock()
		return fmt.Errorf("agent %s exited with code %d: %s", agentName, result.ExitCode, result.Stderr)
	}

	// Check for outbox result (non-blocking single poll, agent already finished)
	foundOutbox := false
	watchResults, pollErr := l.watcher.PollOnce()
	if pollErr != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to poll outbox: %v\n", pollErr)
	}
	for _, wr := range watchResults {
		if wr.AgentName == agentName {
			l.mu.Lock()
			l.processAgentResult(wr)
			l.mu.Unlock()
			foundOutbox = true
			if err := markProcessed(filepath.Dir(wr.FilePath), filepath.Base(wr.FilePath)); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ failed to mark processed %s: %v\n", wr.FilePath, err)
			}
		}
	}

	// Fallback: if agent didn't write outbox result, create one from stream-json output
	if !foundOutbox && result.Stdout != "" {
		l.createOutboxFromStream(agentName, taskID, result.Stdout)
	}

	l.mu.Lock()
	l.orch.Pipeline.Agents[agentName] = AgentStatus{TaskID: taskID, AgentID: agentName, Status: AgentStatusCompleted}
	l.saveState()
	l.mu.Unlock()

	return nil
}

// runAgentExecution runs implementation agents (potentially in parallel).
func (l *Loop) runAgentExecution(ctx context.Context) error {
	devAgents := l.findDevAgents()
	if len(devAgents) == 0 {
		return fmt.Errorf("no dev agents configured (expected agents with name: backend-dev, frontend-dev, or fullstack)")
	}

	// Single agent: no need for goroutine overhead
	if len(devAgents) == 1 {
		if err := l.executeAgent(ctx, devAgents[0]); err != nil {
			return err
		}
		return l.transitionTo(StageVerification)
	}

	// Multiple agents: run in parallel with concurrency limit
	maxConcurrent := l.cfg.Config.Runtime.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrent)

	for _, name := range devAgents {
		agentName := name // capture loop variable
		g.Go(func() error {
			return l.executeAgent(gctx, agentName)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	return l.transitionTo(StageVerification)
}

// runVerification executes build/test/lint commands.
func (l *Loop) runVerification(ctx context.Context) error {
	projectDir := l.cfg.WorkDir
	if len(l.cfg.Projects) > 0 {
		projectDir = l.cfg.Projects[0].Path
	}

	// Try to load verify.yml
	verifyPath := filepath.Join(projectDir, ".pylon", "verify.yml")
	vc, err := config.LoadVerifyConfig(verifyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("⚠ verify.yml not found, skipping verification\n")
			return l.transitionTo(StagePRCreation)
		}
		return fmt.Errorf("failed to load verify config: %w", err)
	}

	// Convert to VerifyCommands
	steps := vc.OrderedSteps()
	if len(steps) == 0 {
		return l.transitionTo(StagePRCreation)
	}

	var commands []VerifyCommand
	for _, s := range steps {
		commands = append(commands, VerifyCommand{
			Name:    s.Name,
			Command: s.Command,
			Timeout: s.Timeout,
		})
	}

	results, err := RunVerification(projectDir, commands)
	if err != nil {
		return fmt.Errorf("verification execution error: %w", err)
	}

	if AllPassed(results) {
		fmt.Println("✓ All verifications passed")
		return l.transitionTo(StagePRCreation)
	}

	// Verification failed
	fmt.Printf("✗ Verification failed:\n%s\n", FailedSummary(results))

	// Attempts tracking and max check are handled inside Pipeline.Transition()
	// when transitioning from StageVerification → StageAgentExecuting.

	// Write fix request to inbox for dev agents
	if err := l.writeFixRequest(results); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to write fix request: %v\n", err)
	}

	// Retry: go back to agent execution (Transition increments Attempts and enforces MaxAttempts)
	err = l.transitionTo(StageAgentExecuting)
	if err != nil {
		return fmt.Errorf("verification failed after %d attempts (%w):\n%s",
			l.orch.Pipeline.Attempts, err, FailedSummary(results))
	}
	return nil
}

// runPRCreation creates a pull request.
// PR creation failure is non-fatal — the pipeline skips to wiki update
// so that tech-writer can still document what agents did.
func (l *Loop) runPRCreation(ctx context.Context) error {
	projectDir := l.cfg.WorkDir
	if len(l.cfg.Projects) > 0 {
		projectDir = l.cfg.Projects[0].Path
	}

	if l.cfg.Config.Git.AutoPush {
		if err := git.PushBranch(projectDir, l.cfg.Branch); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ push failed (skipping PR): %v\n", err)
			return l.transitionTo(StageWikiUpdate)
		}
	}

	prURL, err := git.CreatePR(projectDir, git.PRCreateConfig{
		Title:     fmt.Sprintf("[pylon] %s", truncateOutput(l.cfg.Requirement, 60)),
		Body:      l.buildPRBody(),
		Branch:    l.cfg.Branch,
		Base:      l.cfg.Config.Git.DefaultBase,
		Reviewers: l.cfg.Config.Git.PR.Reviewers,
		Draft:     l.cfg.Config.Git.PR.Draft,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ PR creation failed (continuing to wiki update): %v\n", err)
		if l.cfg.Config.Git.AutoPush {
			fmt.Fprintf(os.Stderr, "  ℹ 리모트 브랜치 '%s'는 push된 상태입니다. 수동으로 PR을 생성하거나 브랜치를 삭제하세요.\n", l.cfg.Branch)
		}
		return l.transitionTo(StageWikiUpdate)
	}

	fmt.Printf("✓ PR created: %s\n", prURL)
	return l.transitionTo(StagePOValidation)
}

// runPOValidation handles PO validation of the PR.
// Like PO conversation, this may require interactive mode.
func (l *Loop) runPOValidation(ctx context.Context) error {
	// For Phase 0: auto-approve and move to wiki update
	fmt.Println("✓ PO validation: auto-approved (Phase 0)")
	return l.transitionTo(StageWikiUpdate)
}

// runWikiUpdate handles wiki/domain knowledge updates.
func (l *Loop) runWikiUpdate(ctx context.Context) error {
	if !l.cfg.Config.Wiki.AutoUpdate {
		return l.transitionTo(StageCompleted)
	}

	// Try to run tech-writer agent if available
	twAgent := l.findAgent("tech-writer")
	if twAgent == nil {
		fmt.Println("✓ Wiki update: skipped (no tech-writer agent)")
		return l.transitionTo(StageCompleted)
	}

	// Run tech-writer as headless agent — failure is non-fatal
	twAgent.ResolveDefaults(l.cfg.Config)
	taskID := fmt.Sprintf("%s-tech-writer", l.cfg.PipelineID)

	taskPrompt := agent.BuildTaskPrompt(twAgent.Role, "tech-writer", taskID, l.inboxDir, l.watcher.OutboxDir)

	projectDir := l.cfg.WorkDir
	if len(l.cfg.Projects) > 0 {
		projectDir = l.cfg.Projects[0].Path
	}

	// NOTE: wiki update는 inbox/outbox 프로토콜 없이 fire-and-forget으로 실행.
	// 실패해도 파이프라인 진행에 영향 없음. 태스크 추적/worktree 격리가 필요하면 executeAgent로 전환.
	result, err := l.cfg.Runner.Start(agent.RunConfig{
		Ctx:        ctx,
		Agent:      twAgent,
		Global:     l.cfg.Config,
		TaskPrompt: taskPrompt,
		WorkDir:    projectDir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ wiki update failed: %v\n", err)
	} else if result.ExitCode != 0 {
		fmt.Fprintf(os.Stderr, "⚠ wiki update exited with code %d\n", result.ExitCode)
	} else {
		fmt.Println("✓ Wiki update completed")
	}

	return l.transitionTo(StageCompleted)
}

// --- Helper methods ---

func (l *Loop) transitionTo(stage Stage) error {
	return l.orch.TransitionTo(stage)
}

// saveState persists pipeline state with error logging.
func (l *Loop) saveState() {
	if err := l.orch.savePipelineState(); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to save pipeline state: %v\n", err)
	}
}

func (l *Loop) findAgent(name string) *config.AgentConfig {
	if a, ok := l.cfg.Agents[name]; ok {
		return a
	}
	return nil
}

// findDevAgents returns agent names with development-related roles.
func (l *Loop) findDevAgents() []string {
	var devs []string
	devRoles := map[string]bool{
		"backend-dev":  true,
		"frontend-dev": true,
		"fullstack":    true,
	}

	for name := range l.cfg.Agents {
		if devRoles[name] {
			devs = append(devs, name)
		}
	}

	// Also check project-assigned agents (only if they match devRoles)
	if len(l.cfg.Projects) > 0 {
		for _, p := range l.cfg.Projects {
			if pCfg, ok := l.cfg.Config.Projects[p.Name]; ok {
				for _, a := range pCfg.Agents {
					if devRoles[a] && l.findAgent(a) != nil && !containsStr(devs, a) {
						devs = append(devs, a)
					}
				}
			}
		}
	}

	sort.Strings(devs)
	return devs
}

func (l *Loop) buildHandoffContext(taskID string) *protocol.MsgContext {
	ctx := &protocol.MsgContext{
		TaskID:     taskID,
		PipelineID: l.cfg.PipelineID,
		Summary:    l.cfg.Requirement,
	}

	// Add blackboard decisions
	if l.cfg.Store != nil && len(l.cfg.Projects) > 0 {
		entries, err := l.cfg.Store.GetBlackboardByCategory(l.cfg.Projects[0].Name, "decision")
		if err == nil {
			for _, e := range entries {
				ctx.Decisions = append(ctx.Decisions, fmt.Sprintf("%s: %s", e.Key, e.Value))
			}
		}
	}

	return ctx
}

func (l *Loop) writeTaskToInbox(agentName, taskID string, msgCtx *protocol.MsgContext) error {
	msg := protocol.NewMessage(protocol.MsgTaskAssign, "orchestrator", agentName)
	msg.Subject = fmt.Sprintf("Task: %s", l.cfg.Requirement)
	msg.Context = msgCtx
	msg.Body = protocol.TaskAssignBody{
		TaskID:      taskID,
		Description: l.cfg.Requirement,
		Branch:      l.cfg.Branch,
	}
	return protocol.WriteTask(l.inboxDir, agentName, msg)
}

func (l *Loop) buildClaudeMD(agentCfg *config.AgentConfig, taskID, outboxPath, outboxDir string) (string, error) {
	builder := &agent.ClaudeMDBuilder{MaxLines: 200}

	var projectMemory string
	if l.cfg.MemManager != nil && len(l.cfg.Projects) > 0 {
		mem, err := l.cfg.MemManager.GetProactiveContext(l.cfg.Projects[0].Name, l.cfg.Requirement, 0)
		if err == nil {
			projectMemory = mem
		}
	}

	inboxPath := filepath.Join(l.inboxDir, agentCfg.Name, taskID+".task.json")

	return builder.Build(agent.BuildInput{
		CommunicationRules: agent.CommunicationRulesWithPaths(inboxPath, outboxPath, outboxDir),
		TaskContext:        fmt.Sprintf("태스크: %s\n역할: %s", l.cfg.Requirement, agentCfg.Role),
		CompactionRules:    agent.DefaultCompactionRules(),
		ProjectMemory:      projectMemory,
	})
}

func (l *Loop) processAgentResult(wr WatchResult) {
	if wr.Envelope == nil {
		return
	}

	// Extract learnings and store to memory
	if l.cfg.MemManager != nil && len(l.cfg.Projects) > 0 {
		if bodyMap, ok := wr.Envelope.Body.(map[string]any); ok {
			if learnings, ok := bodyMap["learnings"].([]any); ok {
				var strs []string
				for _, item := range learnings {
					if s, ok := item.(string); ok {
						strs = append(strs, s)
					}
				}
				if len(strs) > 0 {
					taskID := ""
					if wr.Envelope.Context != nil {
						taskID = wr.Envelope.Context.TaskID
					}
					l.cfg.MemManager.StoreLearnings(l.cfg.Projects[0].Name, taskID, wr.AgentName, strs)
				}
			}
		}
	}

	// Update blackboard with summary
	if l.cfg.Store != nil && len(l.cfg.Projects) > 0 {
		if bodyMap, ok := wr.Envelope.Body.(map[string]any); ok {
			if summary, ok := bodyMap["summary"].(string); ok {
				l.cfg.Store.PutBlackboard(&store.BlackboardEntry{
					ProjectID:  l.cfg.Projects[0].Name,
					Category:   "agent_result",
					Key:        wr.AgentName,
					Value:      summary,
					Confidence: 0.9,
					Author:     wr.AgentName,
				})
			}
		}
	}
}

// createOutboxFromStream parses stream-json output and creates an outbox result file.
// This is the fallback when the agent doesn't write its own outbox result.
func (l *Loop) createOutboxFromStream(agentName, taskID, stdout string) {
	sr := ParseStreamJSON(stdout)

	summary := sr.Summary
	if summary == "" {
		summary = fmt.Sprintf("에이전트 %s 작업 완료 (요약 미제공)", agentName)
	}

	// Build result body as map for processAgentResult compatibility
	body := map[string]any{
		"task_id":       taskID,
		"status":        "completed",
		"summary":       summary,
		"files_changed": sr.FilesChanged,
		"source":        "stream-json-fallback",
	}
	if len(sr.CommitCommands) > 0 {
		body["commit_commands"] = sr.CommitCommands
	}

	// Write to outbox via protocol
	msg := protocol.NewMessage(protocol.MsgResult, agentName, "orchestrator")
	msg.Context = &protocol.MsgContext{
		TaskID:     taskID,
		PipelineID: l.cfg.PipelineID,
	}
	msg.Body = body

	if err := protocol.WriteResult(l.watcher.OutboxDir, agentName, msg); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to write fallback outbox for %s: %v\n", agentName, err)
		return
	}

	// Process the result we just wrote
	wr := WatchResult{
		AgentName: agentName,
		Envelope:  msg,
	}
	l.mu.Lock()
	l.processAgentResult(wr)
	l.mu.Unlock()
}

func (l *Loop) writeFixRequest(results []VerifyResult) error {
	devAgents := l.findDevAgents()
	failSummary := FailedSummary(results)

	for _, agentName := range devAgents {
		taskID := fmt.Sprintf("%s-%s-fix-%d", l.cfg.PipelineID, agentName, l.orch.Pipeline.Attempts)

		msg := protocol.NewMessage(protocol.MsgTaskAssign, "orchestrator", agentName)
		msg.Subject = "검증 실패 수정 요청"
		msg.Context = &protocol.MsgContext{
			TaskID:     taskID,
			PipelineID: l.cfg.PipelineID,
			Summary:    fmt.Sprintf("검증 실패 수정: %s", failSummary),
		}
		msg.Body = protocol.TaskAssignBody{
			TaskID:      taskID,
			Description: fmt.Sprintf("다음 검증이 실패했습니다. 수정해주세요:\n%s", failSummary),
			Branch:      l.cfg.Branch,
		}
		if err := protocol.WriteTask(l.inboxDir, agentName, msg); err != nil {
			return fmt.Errorf("failed to write fix request for %s: %w", agentName, err)
		}
	}
	return nil
}

func (l *Loop) buildPRBody() string {
	body := fmt.Sprintf("## 요약\n\n%s\n\n", l.cfg.Requirement)
	body += fmt.Sprintf("## Pipeline\n\n- ID: `%s`\n- Branch: `%s`\n", l.cfg.PipelineID, l.cfg.Branch)

	// Add agent results summary (sorted for deterministic output)
	if len(l.orch.Pipeline.Agents) > 0 {
		body += "\n## 에이전트 결과\n\n"
		agentNames := make([]string, 0, len(l.orch.Pipeline.Agents))
		for name := range l.orch.Pipeline.Agents {
			agentNames = append(agentNames, name)
		}
		sort.Strings(agentNames)
		for _, name := range agentNames {
			status := l.orch.Pipeline.Agents[name]
			body += fmt.Sprintf("- %s: %s\n", name, status.Status)
		}
	}

	if l.orch.Pipeline.Attempts > 0 {
		body += fmt.Sprintf("\n## 검증\n\n- 검증 재시도 횟수: %d\n", l.orch.Pipeline.Attempts)
	}

	return body
}

// GetOrchestrator returns the underlying orchestrator for external access.
func (l *Loop) GetOrchestrator() *Orchestrator {
	return l.orch
}

// SetPipeline allows setting the pipeline directly (for resume scenarios).
func (l *Loop) SetPipeline(p *Pipeline) {
	l.orch.Pipeline = p
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
