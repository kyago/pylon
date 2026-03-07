package protocol

import (
	"fmt"
	"strings"

	"github.com/yongjunkang/pylon/internal/store"
)

// BuildHandoffContext creates a narrative context for the next agent.
// It synthesizes the previous agent's result, blackboard entries, and relevant memories.
// Spec Reference: Section 8 "Handoff Protocol" (Narrative Casting)
func BuildHandoffContext(
	prevResult *MessageEnvelope,
	blackboard []store.BlackboardEntry,
	memories []store.MemorySearchResult,
) *MsgContext {
	ctx := &MsgContext{}

	// Extract context from previous result
	if prevResult != nil {
		if prevResult.Context != nil {
			ctx.TaskID = prevResult.Context.TaskID
			ctx.PipelineID = prevResult.Context.PipelineID
			ctx.ProjectID = prevResult.Context.ProjectID
		}

		// Build narrative summary from previous result
		var summaryParts []string
		summaryParts = append(summaryParts, fmt.Sprintf("이전 에이전트(%s) 결과:", prevResult.From))

		if prevResult.Subject != "" {
			summaryParts = append(summaryParts, prevResult.Subject)
		}

		// Extract decisions from body if it's a result
		if bodyMap, ok := prevResult.Body.(map[string]any); ok {
			if summary, ok := bodyMap["summary"].(string); ok {
				summaryParts = append(summaryParts, summary)
			}
		}

		ctx.Summary = strings.Join(summaryParts, " ")
	}

	// Add blackboard decisions
	var decisions []string
	for _, bb := range blackboard {
		if bb.Category == "decision" {
			decisions = append(decisions, fmt.Sprintf("%s: %s (신뢰도: %.0f%%)", bb.Key, bb.Value, bb.Confidence*100))
		}
	}
	ctx.Decisions = decisions

	// Add memory references
	var refs []string
	for _, m := range memories {
		refs = append(refs, fmt.Sprintf("[%s] %s", m.Category, m.Key))
	}
	if len(refs) > 0 {
		ctx.Summary += "\n관련 프로젝트 메모리: " + strings.Join(refs, ", ")
	}

	return ctx
}
