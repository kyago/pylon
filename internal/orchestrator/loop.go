package orchestrator

import (
	"context"
	"encoding/json"
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
	"github.com/kyago/pylon/internal/workflow"
)

// LoopConfig holds all dependencies for the orchestration loop.
type LoopConfig struct {
	Config       *config.Config
	Store        *store.Store
	WorkDir      string // workspace root
	PipelineID   string
	Requirement  string
	Branch       string
	WorkflowName string // workflow template name (e.g., "feature", "bugfix")
	Requires     []string // pipeline IDs this pipeline depends on (cross-pipeline dependency)
	MemManager   *memory.Manager
	Runner       agent.AgentRunner
	WorktreeMgr  *git.WorktreeManager
	Agents       map[string]*config.AgentConfig
	Projects     []config.ProjectInfo
}

// Loop drives the pipeline stage-by-stage execution.
type Loop struct {
	cfg        LoopConfig
	orch       *Orchestrator
	watcher    *OutboxWatcher
	inboxDir   string
	lastResult *protocol.MessageEnvelope // last completed agent result for handoff
	mu         sync.Mutex               // protects Pipeline.Agents writes, saveState calls, and lastResult
	wf         *workflow.WorkflowTemplate // active workflow template (nil = default feature)
}

// NewLoop creates a new orchestration loop.
func NewLoop(cfg LoopConfig) *Loop {
	orch := NewOrchestrator(cfg.Config, cfg.Store, cfg.WorkDir)
	orch.SetPipelineID(cfg.PipelineID)
	runtimeDir := filepath.Join(cfg.WorkDir, ".pylon", "runtime")

	return &Loop{
		cfg:      cfg,
		orch:     orch,
		watcher:  NewOutboxWatcher(filepath.Join(runtimeDir, "outbox"), 2*time.Second, cfg.Store),
		inboxDir: filepath.Join(runtimeDir, "inbox"),
	}
}

// Run executes the pipeline from its current stage until completion or failure.
// If the pipeline is already at a PO interactive stage, it returns ErrInteractiveRequired.
func (l *Loop) Run(ctx context.Context) error {
	// Try to recover existing state (for --continue scenarios).
	// Skip recovery if a pipeline was pre-set via SetPipeline() to avoid clobbering it.
	if l.orch.Pipeline == nil {
		unprocessed, err := l.orch.Recover()
		if err != nil {
			return fmt.Errorf("recovery failed: %w", err)
		}

		// Process any unprocessed outbox results from before crash
		for _, ur := range unprocessed {
			env, readErr := protocol.ReadResult(ur.FilePath)
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "⚠ failed to read unprocessed result %s: %v\n", ur.FilePath, readErr)
				continue
			}
			data := l.extractAgentResult(WatchResult{AgentName: ur.AgentName, Envelope: env})
			l.storeAgentResult(data)
			l.enqueueResultAck(ur.AgentName, ur.TaskID)
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

	// Load and apply workflow template
	if err := l.applyWorkflow(); err != nil {
		return fmt.Errorf("failed to apply workflow: %w", err)
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		if l.orch.Pipeline.IsTerminal() {
			return nil
		}

		// Check preconditions before each stage transition
		if err := l.checkPreconditions(ctx); err != nil {
			return err
		}

		var err error
		// Skip stages not in the current workflow (e.g., after recovery with different workflow)
		if l.wf != nil && !l.wf.HasStage(l.orch.Pipeline.CurrentStage) {
			next := l.nextWorkflowStageAfter(l.orch.Pipeline.CurrentStage)
			if next == StageFailed {
				return fmt.Errorf("stage %s not in workflow and no valid next stage", l.orch.Pipeline.CurrentStage)
			}
			if err := l.orch.ForceStage(next); err != nil {
				return err
			}
			continue
		}

		switch l.orch.Pipeline.CurrentStage {
		case StageInit:
			err = l.transitionTo(l.nextStageAfter(StageInit))
		case StagePOConversation:
			err = l.runPOConversation(ctx)
		case StageArchitectAnalysis:
			err = l.runHeadlessAgent(ctx, "architect", StageArchitectAnalysis, l.nextStageAfter(StageArchitectAnalysis))
		case StagePMTaskBreakdown:
			err = l.runPMTaskBreakdown(ctx)
		case StageTaskReview:
			err = l.runTaskReview(ctx)
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
			// Record to DLQ before transitioning to failed.
			// Skip if the stage handler already recorded (e.g., runVerification on max attempts).
			if !l.orch.Pipeline.IsTerminal() {
				l.recordToDLQ(err, "")
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
	return l.executeAgentWithSuffix(ctx, agentName, "", "")
}

// executeAgentWithSuffix runs an agent with an optional task ID suffix to disambiguate
// multiple tasks assigned to the same agent within a single wave.
// taskDescription overrides l.cfg.Requirement for inbox/CLAUDE.md when non-empty.
func (l *Loop) executeAgentWithSuffix(ctx context.Context, agentName, taskSuffix, taskDescription string) error {
	agentCfg := l.findAgent(agentName)
	if agentCfg == nil {
		return fmt.Errorf("agent config not found: %s", agentName)
	}

	agentCfg.ResolveDefaults(l.cfg.Config)

	taskID := fmt.Sprintf("%s-%s", l.cfg.PipelineID, agentName)
	if taskSuffix != "" {
		taskID = fmt.Sprintf("%s-%s-%s", l.cfg.PipelineID, agentName, taskSuffix)
	}

	// Use task-specific description when provided (wave execution), else fall back to global requirement
	description := l.cfg.Requirement
	if taskDescription != "" {
		description = taskDescription
	}

	// Build handoff context from blackboard
	handoffCtx := l.buildHandoffContext(taskID)

	// Write task to inbox
	if err := l.writeTaskToInbox(agentName, taskID, description, handoffCtx); err != nil {
		return fmt.Errorf("failed to write task for %s: %w", agentName, err)
	}

	// Build outbox path for this agent
	outboxDir := filepath.Join(l.watcher.OutboxDir, agentName)
	outboxPath := filepath.Join(outboxDir, taskID+".result.json")

	// Build CLAUDE.md with concrete outbox path
	claudeMD, err := l.buildClaudeMD(agentCfg, taskID, outboxPath, outboxDir, description)
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

	// Apply task timeout from config.
	// ParseTaskTimeout always returns >= 30m (fallback), so this guard is defensive.
	// To disable timeout entirely, this condition would need to be extended.
	agentCtx := ctx
	if timeout := l.cfg.Config.Runtime.ParseTaskTimeout(); timeout > 0 {
		var cancel context.CancelFunc
		agentCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Launch agent with context propagation (timeout-wrapped)
	result, err := l.cfg.Runner.Start(agent.RunConfig{
		Ctx:        agentCtx,
		Agent:      agentCfg,
		Global:     l.cfg.Config,
		TaskPrompt: taskPrompt,
		WorkDir:    workDir,
		ClaudeMD:   claudeMD,
		PipelineID: l.cfg.PipelineID,
		TaskID:     taskID,
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
	var lastEnvelope *protocol.MessageEnvelope
	for _, wr := range watchResults {
		if wr.AgentName == agentName {
			data := l.extractAgentResult(wr)
			l.storeAgentResult(data)
			lastEnvelope = wr.Envelope
			foundOutbox = true
			l.enqueueResultAck(agentName, taskID)
		}
	}

	// Fallback: if agent didn't write outbox result, create one from stream-json output
	if !foundOutbox && result.Stdout != "" {
		if env := l.createOutboxFromStream(agentName, taskID, result.Stdout); env != nil {
			lastEnvelope = env
		}
	}

	l.mu.Lock()
	l.orch.Pipeline.Agents[agentName] = AgentStatus{TaskID: taskID, AgentID: agentName, Status: AgentStatusCompleted}
	if lastEnvelope != nil {
		l.lastResult = lastEnvelope
	}
	l.saveState()
	l.mu.Unlock()

	return nil
}

// runPMTaskBreakdown executes the PM agent and transitions to task review.
func (l *Loop) runPMTaskBreakdown(ctx context.Context) error {
	if err := l.executeAgent(ctx, "pm"); err != nil {
		return err
	}

	// Extract task graph from PM outbox result (if available)
	graph := l.extractTaskGraph()
	if graph != nil {
		if err := graph.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ PM task graph validation failed, falling back to parallel execution: %v\n", err)
			graph = nil
		}
	}
	l.orch.Pipeline.TaskGraph = graph

	return l.transitionTo(l.nextStageAfter(StagePMTaskBreakdown))
}

// runTaskReview handles the task review gate between PM breakdown and agent execution.
// If AutoApproveTaskReview is true or no PO agent is configured, it auto-approves.
// Otherwise, it returns ErrInteractiveRequired for PO review.
func (l *Loop) runTaskReview(ctx context.Context) error {
	// Auto-approve if configured or PO agent not available
	if l.cfg.Config.Runtime.AutoApproveTaskReview || l.findAgent("po") == nil {
		fmt.Println("✓ Task review: auto-approved")
		return l.transitionTo(l.nextStageAfter(StageTaskReview))
	}

	return ErrInteractiveRequired
}

// extractTaskGraph attempts to parse a TaskGraph from the last PM outbox result.
// Returns nil if no task graph is found (legacy PM output).
func (l *Loop) extractTaskGraph() *TaskGraph {
	l.mu.Lock()
	lastResult := l.lastResult
	l.mu.Unlock()

	if lastResult == nil {
		return nil
	}

	bodyMap, ok := lastResult.Body.(map[string]any)
	if !ok {
		return nil
	}

	tasksRaw, ok := bodyMap["tasks"]
	if !ok {
		return nil
	}

	// Marshal/unmarshal through JSON for robust type conversion
	data, err := json.Marshal(tasksRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to marshal PM tasks for task graph: %v\n", err)
		return nil
	}

	var items []TaskItem
	if err := json.Unmarshal(data, &items); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to parse PM tasks as TaskGraph: %v\n", err)
		return nil
	}

	if len(items) == 0 {
		return nil
	}

	return &TaskGraph{Tasks: items}
}

// runAgentExecution runs implementation agents (potentially in parallel).
// If a TaskGraph is available, it uses wave-based execution respecting dependencies.
// Otherwise, it falls back to running all dev agents in parallel (legacy behavior).
func (l *Loop) runAgentExecution(ctx context.Context) error {
	devAgents := l.findDevAgents()
	if len(devAgents) == 0 {
		return fmt.Errorf("no dev agents configured (expected agents with type: dev)")
	}

	graph := l.orch.Pipeline.TaskGraph
	if graph != nil {
		// Assign agents to unassigned tasks
		graph.AssignAgents(devAgents)
		if err := l.runAgentsWave(ctx, graph, devAgents); err != nil {
			return err
		}
	} else {
		if err := l.runAgentsParallel(ctx, devAgents); err != nil {
			return err
		}
	}

	// Merge agent worktree branches back into task branch (single or multi)
	if l.cfg.WorktreeMgr != nil && l.cfg.WorktreeMgr.Enabled {
		if err := l.mergeAgentBranches(devAgents); err != nil {
			return fmt.Errorf("agent branch merge failed: %w", err)
		}
	}

	return l.transitionTo(l.nextStageAfter(StageAgentExecuting))
}

// runAgentsParallel runs all dev agents in parallel (legacy path).
func (l *Loop) runAgentsParallel(ctx context.Context, devAgents []string) error {
	if len(devAgents) == 1 {
		return l.executeAgent(ctx, devAgents[0])
	}

	maxConcurrent := l.cfg.Config.Runtime.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = min(len(devAgents), 5)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrent)

	for _, name := range devAgents {
		agentName := name
		g.Go(func() error {
			return l.executeAgent(gctx, agentName)
		})
	}

	return g.Wait()
}

// runAgentsWave executes tasks wave-by-wave respecting dependency order.
// Tasks within the same wave run in parallel.
func (l *Loop) runAgentsWave(ctx context.Context, graph *TaskGraph, devAgents []string) error {
	waves, err := graph.TopoSort()
	if err != nil {
		return fmt.Errorf("task graph toposort failed: %w", err)
	}

	maxConcurrent := l.cfg.Config.Runtime.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = min(len(devAgents), 5)
	}

	for i, wave := range waves {
		fmt.Printf("▶ Wave %d: %d tasks\n", i+1, len(wave))

		if len(wave) == 1 {
			agentName := wave[0].AgentName
			if agentName == "" {
				agentName = devAgents[0]
			}
			if err := l.executeAgentWithSuffix(ctx, agentName, wave[0].ID, wave[0].Description); err != nil {
				return err
			}
			continue
		}

		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(maxConcurrent)

		for _, task := range wave {
			agentName := task.AgentName
			if agentName == "" {
				agentName = devAgents[0]
			}
			taskID := task.ID
			taskDesc := task.Description
			g.Go(func() error {
				return l.executeAgentWithSuffix(gctx, agentName, taskID, taskDesc)
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}
	}

	return nil
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
			return l.transitionTo(l.nextStageAfter(StageVerification))
		}
		return fmt.Errorf("failed to load verify config: %w", err)
	}

	// Convert to VerifyCommands
	steps := vc.OrderedSteps()
	if len(steps) == 0 {
		return l.transitionTo(l.nextStageAfter(StageVerification))
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
		return l.transitionTo(l.nextStageAfter(StageVerification))
	}

	// Verification failed
	failSummary := FailedSummary(results)
	fmt.Printf("✗ Verification failed:\n%s\n", failSummary)

	// Classify failure to decide retry vs terminal
	class := ClassifyFailure(nil, failSummary)
	if class == FailureTerminal {
		fmt.Println("✗ Failure classified as terminal — skipping retry")
		l.recordToDLQ(fmt.Errorf("terminal verification failure"), failSummary)
		return l.transitionTo(StageFailed)
	}

	// Attempts tracking and max check are handled inside Pipeline.Transition()
	// when transitioning from StageVerification → StageAgentExecuting.

	// Write fix request to inbox for dev agents
	if err := l.writeFixRequest(results); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to write fix request: %v\n", err)
	}

	// Retry: go back to agent execution (Transition increments Attempts and enforces MaxAttempts)
	err = l.transitionTo(StageAgentExecuting)
	if err != nil {
		// Max attempts exceeded — record to DLQ
		l.recordToDLQ(fmt.Errorf("max retry attempts exceeded"), failSummary)
		return fmt.Errorf("verification failed after %d attempts (%w):\n%s",
			l.orch.Pipeline.Attempts, err, failSummary)
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
			return l.skipPRFailure()
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
		fmt.Fprintf(os.Stderr, "⚠ PR creation failed (continuing): %v\n", err)
		if l.cfg.Config.Git.AutoPush {
			fmt.Fprintf(os.Stderr, "  ℹ 리모트 브랜치 '%s'는 push된 상태입니다. 수동으로 PR을 생성하거나 브랜치를 삭제하세요.\n", l.cfg.Branch)
		}
		return l.skipPRFailure()
	}

	fmt.Printf("✓ PR created: %s\n", prURL)
	return l.transitionTo(l.nextStageAfter(StagePRCreation))
}

// runPOValidation handles PO validation of the PR.
// Like PO conversation, this may require interactive mode.
func (l *Loop) runPOValidation(ctx context.Context) error {
	// For Phase 0: auto-approve and move to next stage
	fmt.Println("✓ PO validation: auto-approved (Phase 0)")
	return l.transitionTo(l.nextStageAfter(StagePOValidation))
}

// runWikiUpdate handles wiki/domain knowledge updates.
func (l *Loop) runWikiUpdate(ctx context.Context) error {
	if !l.cfg.Config.Wiki.AutoUpdate {
		return l.transitionTo(l.nextStageAfter(StageWikiUpdate))
	}

	// Try to run tech-writer agent if available
	twAgent := l.findAgent("tech-writer")
	if twAgent == nil {
		fmt.Println("✓ Wiki update: skipped (no tech-writer agent)")
		return l.transitionTo(l.nextStageAfter(StageWikiUpdate))
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

	return l.transitionTo(l.nextStageAfter(StageWikiUpdate))
}

// --- Helper methods ---

// applyWorkflow loads the workflow template and applies its transitions to the pipeline.
// Uses LoopConfig.WorkflowName, falling back to Pipeline.WorkflowName (recovery),
// then Config.Workflow.DefaultWorkflow, and finally "feature" as the ultimate default.
func (l *Loop) applyWorkflow() error {
	name := l.cfg.WorkflowName
	if name == "" && l.orch.Pipeline != nil {
		name = l.orch.Pipeline.WorkflowName
	}
	if name == "" {
		name = l.cfg.Config.Workflow.DefaultWorkflow
	}
	if name == "" {
		name = "feature"
	}

	// Warn if workflow name from --continue doesn't match the CLI flag
	if l.cfg.WorkflowName != "" && l.orch.Pipeline.WorkflowName != "" &&
		l.cfg.WorkflowName != l.orch.Pipeline.WorkflowName {
		fmt.Fprintf(os.Stderr, "⚠ workflow mismatch: pipeline was started with %q, --workflow flag specifies %q (using %q)\n",
			l.orch.Pipeline.WorkflowName, l.cfg.WorkflowName, name)
	}

	templateDir := l.cfg.Config.Workflow.TemplateDir
	if templateDir != "" && !filepath.IsAbs(templateDir) {
		templateDir = filepath.Join(l.cfg.WorkDir, templateDir)
	}

	wf, err := workflow.LoadWorkflow(name, templateDir)
	if err != nil {
		return err
	}

	l.wf = wf
	l.orch.Pipeline.WorkflowName = name
	l.orch.Pipeline.SetTransitions(wf.BuildTransitions())
	l.saveState()

	if name != "feature" {
		fmt.Printf("📋 Workflow: %s (%s)\n", wf.Name, wf.Description)
	}
	return nil
}

// nextStageAfter returns the next stage in the workflow after the given stage.
// Falls back to hardcoded defaults when no workflow template is set.
func (l *Loop) nextStageAfter(current Stage) Stage {
	if l.wf != nil {
		return l.wf.NextStageAfter(current)
	}
	// Fallback: use hardcoded default sequence (feature workflow)
	defaultOrder := []Stage{
		StageInit, StagePOConversation, StageArchitectAnalysis,
		StagePMTaskBreakdown, StageTaskReview, StageAgentExecuting,
		StageVerification, StagePRCreation, StagePOValidation,
		StageWikiUpdate, StageCompleted,
	}
	for i, s := range defaultOrder {
		if s == current && i+1 < len(defaultOrder) {
			return defaultOrder[i+1]
		}
	}
	return StageFailed
}

// nextWorkflowStageAfter finds the next workflow-member stage after a stage that
// may NOT be in the workflow. It uses the global stage ordering to find the first
// workflow stage that follows the given stage. Used by the stage-skip logic.
func (l *Loop) nextWorkflowStageAfter(current Stage) Stage {
	if l.wf == nil {
		return StageFailed
	}
	globalOrder := []Stage{
		StageInit, StagePOConversation, StageArchitectAnalysis,
		StagePMTaskBreakdown, StageTaskReview, StageAgentExecuting,
		StageVerification, StagePRCreation, StagePOValidation,
		StageWikiUpdate, StageCompleted,
	}
	found := false
	for _, s := range globalOrder {
		if s == current {
			found = true
			continue
		}
		if found && l.wf.HasStage(s) {
			return s
		}
	}
	return StageFailed
}

// skipPRFailure handles PR creation failure by skipping to a safe next stage.
// If the next stage is POValidation, skips it too (nothing to validate without a PR).
func (l *Loop) skipPRFailure() error {
	next := l.nextStageAfter(StagePRCreation)
	// Skip po_validation when PR failed — nothing to validate
	if next == StagePOValidation {
		next = l.nextStageAfter(StagePOValidation)
	}
	return l.orch.ForceStage(next)
}

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

// findDevAgents returns agent names with type "dev".
// Agents are resolved via ResolveDefaults which infers type from name for backward compatibility.
func (l *Loop) findDevAgents() []string {
	var devs []string

	for name, agentCfg := range l.cfg.Agents {
		agentCfg.ResolveDefaults(l.cfg.Config)
		if agentCfg.Type == "dev" {
			devs = append(devs, name)
		}
	}

	// Also check project-assigned agents with type "dev"
	if len(l.cfg.Projects) > 0 {
		for _, p := range l.cfg.Projects {
			if pCfg, ok := l.cfg.Config.Projects[p.Name]; ok {
				for _, a := range pCfg.Agents {
					ac := l.findAgent(a)
					if ac == nil {
						continue
					}
					ac.ResolveDefaults(l.cfg.Config)
					if ac.Type == "dev" && !containsStr(devs, a) {
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
	l.mu.Lock()
	prevResult := l.lastResult
	l.mu.Unlock()
	var blackboard []store.BlackboardEntry
	var memories []store.MemorySearchResult

	if l.cfg.Store != nil && len(l.cfg.Projects) > 0 {
		projectName := l.cfg.Projects[0].Name
		blackboard, _ = l.cfg.Store.GetBlackboardByCategory(projectName, "decision")

		if l.cfg.MemManager != nil {
			memories, _ = l.cfg.Store.SearchMemory(projectName, l.cfg.Requirement, 5)
		}
	}

	ctx := protocol.BuildHandoffContext(prevResult, blackboard, memories)
	ctx.TaskID = taskID
	ctx.PipelineID = l.cfg.PipelineID
	if ctx.Summary == "" {
		ctx.Summary = l.cfg.Requirement
	}
	return ctx
}

// mergeAgentBranches merges each agent's worktree branch into the task branch.
// On failure, it resets to the pre-merge state to avoid partial merge artifacts.
func (l *Loop) mergeAgentBranches(agentNames []string) error {
	projectDir := l.cfg.WorkDir
	if len(l.cfg.Projects) > 0 {
		projectDir = l.cfg.Projects[0].Path
	}

	// Save restore point before merging
	restorePoint, err := l.cfg.WorktreeMgr.HeadSHA(projectDir)
	if err != nil {
		return fmt.Errorf("failed to get HEAD for restore point: %w", err)
	}

	for _, name := range agentNames {
		// Skip agents that don't use worktree isolation (no branch was created)
		if agentCfg := l.findAgent(name); agentCfg == nil || agentCfg.Isolation != "worktree" {
			continue
		}
		agentBranch := fmt.Sprintf("%s/%s", l.cfg.Branch, git.SanitizeBranchName(name))
		if err := l.cfg.WorktreeMgr.MergeBranch(projectDir, agentBranch); err != nil {
			// Rollback to pre-merge state
			if abortErr := l.cfg.WorktreeMgr.AbortMerge(projectDir); abortErr != nil {
				fmt.Fprintf(os.Stderr, "⚠ merge --abort failed: %v\n", abortErr)
			}
			if resetErr := l.cfg.WorktreeMgr.ResetHard(projectDir, restorePoint); resetErr != nil {
				fmt.Fprintf(os.Stderr, "⚠ git reset --hard failed: %v (repository may be in dirty state)\n", resetErr)
			}
			return err
		}
		// Best-effort cleanup
		_ = l.cfg.WorktreeMgr.DeleteBranch(projectDir, agentBranch)
	}
	return nil
}

func (l *Loop) writeTaskToInbox(agentName, taskID, description string, msgCtx *protocol.MsgContext) error {
	msg := protocol.NewMessage(protocol.MsgTaskAssign, "orchestrator", agentName)
	msg.Subject = fmt.Sprintf("Task: %s", description)
	msg.Context = msgCtx
	msg.Body = protocol.TaskAssignBody{
		TaskID:      taskID,
		Description: description,
		Branch:      l.cfg.Branch,
	}
	if err := protocol.WriteTask(l.inboxDir, agentName, msg); err != nil {
		return err
	}

	// Record task assignment in message_queue (log-and-continue)
	if l.cfg.Store != nil {
		body, _ := json.Marshal(map[string]string{"task_id": taskID})
		ctx, _ := json.Marshal(map[string]string{"task_id": taskID, "pipeline_id": l.cfg.PipelineID})
		if enqErr := l.cfg.Store.Enqueue(&store.QueuedMessage{
			Type:      "task_assign",
			FromAgent: "orchestrator",
			ToAgent:   agentName,
			Subject:   msg.Subject,
			Body:      string(body),
			Context:   string(ctx),
			Status:    "acked",
		}); enqErr != nil {
			fmt.Fprintf(os.Stderr, "⚠ failed to record task assignment for %s: %v\n", agentName, enqErr)
		}
	}

	return nil
}

func (l *Loop) buildClaudeMD(agentCfg *config.AgentConfig, taskID, outboxPath, outboxDir, description string) (string, error) {
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
		TaskContext:        fmt.Sprintf("태스크: %s\n역할: %s", description, agentCfg.Role),
		CompactionRules:    agent.DefaultCompactionRules(),
		ProjectMemory:      projectMemory,
	})
}

// agentResultData holds extracted data from an agent result for deferred I/O.
type agentResultData struct {
	agentName string
	taskID    string
	project   string
	learnings []string
	summary   string
}

// extractAgentResult extracts data from a WatchResult without performing I/O.
// Must be called with l.mu held if Pipeline state is accessed concurrently.
func (l *Loop) extractAgentResult(wr WatchResult) *agentResultData {
	if wr.Envelope == nil {
		return nil
	}

	data := &agentResultData{agentName: wr.AgentName}
	if len(l.cfg.Projects) > 0 {
		data.project = l.cfg.Projects[0].Name
	}
	if wr.Envelope.Context != nil {
		data.taskID = wr.Envelope.Context.TaskID
	}

	bodyMap, ok := wr.Envelope.Body.(map[string]any)
	if !ok {
		return data
	}

	// Extract learnings
	if learnings, ok := bodyMap["learnings"].([]any); ok {
		for _, item := range learnings {
			if s, ok := item.(string); ok {
				data.learnings = append(data.learnings, s)
			}
		}
	}

	// Extract summary
	if summary, ok := bodyMap["summary"].(string); ok {
		data.summary = summary
	}

	return data
}

// storeAgentResult performs I/O operations (memory + blackboard) outside of mutex.
func (l *Loop) storeAgentResult(data *agentResultData) {
	if data == nil || data.project == "" {
		return
	}

	if l.cfg.MemManager != nil && len(data.learnings) > 0 {
		l.cfg.MemManager.StoreLearnings(data.project, data.taskID, data.agentName, data.learnings)
	}

	if l.cfg.Store != nil && data.summary != "" {
		l.cfg.Store.PutBlackboard(&store.BlackboardEntry{
			ProjectID:  data.project,
			Category:   "agent_result",
			Key:        data.agentName,
			Value:      data.summary,
			Confidence: 0.9,
			Author:     data.agentName,
		})
	}
}

// createOutboxFromStream parses stream-json output and creates an outbox result file.
// This is the fallback when the agent doesn't write its own outbox result.
// Returns the created envelope so callers can use it as lastResult.
func (l *Loop) createOutboxFromStream(agentName, taskID, stdout string) *protocol.MessageEnvelope {
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
		return nil
	}

	// Record as processed in message_queue to prevent duplicate processing by future PollOnce() calls
	l.enqueueResultAck(agentName, taskID)

	// Process the result we just wrote
	agentDir := filepath.Join(l.watcher.OutboxDir, agentName)
	resultFile := taskID + ".result.json"
	wr := WatchResult{
		AgentName: agentName,
		FilePath:  filepath.Join(agentDir, resultFile),
		Envelope:  msg,
	}
	data := l.extractAgentResult(wr)
	l.storeAgentResult(data)

	return msg
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

// enqueueResultAck records a result as processed (acked) in the message_queue.
// On Store failure, falls back to writing a .done marker file to prevent duplicate processing.
func (l *Loop) enqueueResultAck(agentName, taskID string) {
	if l.cfg.Store == nil {
		l.writeDoneFallback(agentName, taskID)
		return
	}
	body, _ := json.Marshal(map[string]string{"task_id": taskID})
	ctx, _ := json.Marshal(map[string]string{"task_id": taskID, "pipeline_id": l.cfg.PipelineID})
	if err := l.cfg.Store.Enqueue(&store.QueuedMessage{
		Type:      "result",
		FromAgent: agentName,
		ToAgent:   "orchestrator",
		Subject:   fmt.Sprintf("Result: %s", taskID),
		Body:      string(body),
		Context:   string(ctx),
		Status:    "acked",
	}); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to record result ack for %s/%s: %v, writing .done fallback\n", agentName, taskID, err)
		l.writeDoneFallback(agentName, taskID)
	}
}

// writeDoneFallback creates a .done marker file as a fallback when Store is unavailable.
func (l *Loop) writeDoneFallback(agentName, taskID string) {
	agentDir := filepath.Join(l.watcher.OutboxDir, agentName)
	donePath := filepath.Join(agentDir, taskID+".result.json.done")
	if err := os.WriteFile(donePath, []byte{}, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to write .done fallback for %s/%s: %v\n", agentName, taskID, err)
	}
}

// recordToDLQ saves the current pipeline state to the dead letter queue.
// This is a best-effort operation — failures are logged but do not block the pipeline.
func (l *Loop) recordToDLQ(pipelineErr error, output string) {
	if l.cfg.Store == nil {
		return
	}

	snapshot, err := l.orch.Pipeline.Snapshot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ DLQ: failed to snapshot pipeline: %v\n", err)
		return
	}

	errMsg := ""
	if pipelineErr != nil {
		errMsg = pipelineErr.Error()
	}

	dlqErr := l.cfg.Store.InsertDLQ(&store.DLQEntry{
		PipelineID:        l.orch.Pipeline.ID,
		WorkflowName:      l.orch.Pipeline.WorkflowName,
		Stage:             string(l.orch.Pipeline.CurrentStage),
		ErrorMessage:      errMsg,
		ErrorOutput:       truncateOutput(output, 4096),
		OriginalStateJSON: string(snapshot),
	})
	if dlqErr != nil {
		fmt.Fprintf(os.Stderr, "⚠ DLQ: failed to record entry: %v\n", dlqErr)
	} else {
		fmt.Printf("📋 Pipeline %s recorded to DLQ for later retry\n", l.orch.Pipeline.ID)
	}
}

// checkPreconditions blocks the loop while the pipeline is paused.
// It polls the DB every second to detect resume. Returns nil when unpaused
// or an error if the context is cancelled.
func (l *Loop) checkPreconditions(ctx context.Context) error {
	for l.orch.Pipeline.IsPaused() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}

		// Re-read pipeline state from DB to detect external resume
		if l.cfg.Store != nil {
			rec, err := l.cfg.Store.GetPipeline(l.orch.Pipeline.ID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠ checkPreconditions: failed to read pipeline state: %v\n", err)
				continue
			}
			if rec != nil {
				refreshed, err := LoadPipeline([]byte(rec.StateJSON))
				if err != nil {
					fmt.Fprintf(os.Stderr, "⚠ checkPreconditions: failed to parse pipeline state: %v\n", err)
					continue
				}
				l.orch.Pipeline.Status = refreshed.Status
				l.orch.Pipeline.PausedAtStage = refreshed.PausedAtStage
			}
		}
	}
	return nil
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
