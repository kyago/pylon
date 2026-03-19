package workflow

import (
	"strings"
)

// workflowKeywords maps workflow names to their trigger keywords.
// Keywords are checked against the lowercased requirement text.
var workflowKeywords = map[string][]string{
	"hotfix": {
		"hotfix", "긴급", "urgent", "emergency", "critical fix",
		"보안 패치", "security patch", "production fix", "prod fix",
	},
	"bugfix": {
		"fix", "bug", "버그", "에러", "error", "오류", "수정",
		"broken", "crash", "깨진", "장애", "defect", "issue",
		"regression", "리그레션",
	},
	"docs": {
		"docs", "문서", "documentation", "readme", "api 문서",
		"주석", "comment", "wiki", "가이드", "guide", "tutorial",
	},
	"refactor": {
		"refactor", "리팩토링", "리팩터", "개선", "cleanup", "정리",
		"restructure", "reorganize", "simplify", "단순화",
		"tech debt", "기술 부채",
	},
	"review": {
		"review", "리뷰", "코드 리뷰", "code review", "검토",
		"audit", "감사", "inspect", "점검",
	},
	"explore": {
		"explore", "탐색", "조사", "investigate", "research",
		"분석", "analyze", "파악", "이해", "understand", "spike",
		"prototype", "프로토타입", "poc",
	},
}

// workflowPriority defines the matching priority order.
// Higher priority workflows are checked first to avoid false positives
// (e.g., "hotfix" should match before "bugfix" since "fix" is in both).
var workflowPriority = []string{
	"hotfix", "bugfix", "docs", "refactor", "review", "explore",
}

// SuggestWorkflow analyzes the requirement text and suggests the most
// appropriate workflow based on keyword matching.
// Returns the workflow name and the matched keywords.
// If no keywords match, returns "feature" with an empty keyword list.
func SuggestWorkflow(requirement string) (string, []string) {
	lower := strings.ToLower(requirement)

	for _, wf := range workflowPriority {
		keywords := workflowKeywords[wf]
		var matched []string
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				matched = append(matched, kw)
			}
		}
		if len(matched) > 0 {
			return wf, matched
		}
	}

	return "feature", nil
}
