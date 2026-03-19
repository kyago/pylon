package workflow

import (
	"strings"
	"unicode"
)

// workflowKeywords maps workflow names to their trigger keywords.
// Multi-word keywords use exact substring matching.
// Single-word keywords use word-boundary matching to avoid false positives
// (e.g., "fix" should not match "prefix", "bug" should not match "debug").
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
			if containsWord(lower, kw) {
				matched = append(matched, kw)
			}
		}
		if len(matched) > 0 {
			return wf, matched
		}
	}

	return "feature", nil
}

// containsWord checks if text contains the keyword with word-boundary awareness.
// For multi-word keywords (containing spaces), exact substring match is used.
// For single-word keywords, checks that characters before and after the match
// are not ASCII letters (word boundary), preventing "fix" from matching "prefix".
// Korean characters are always treated as word boundaries.
func containsWord(text, keyword string) bool {
	if strings.Contains(keyword, " ") {
		// Multi-word keywords: exact substring match
		return strings.Contains(text, keyword)
	}

	idx := 0
	for {
		pos := strings.Index(text[idx:], keyword)
		if pos < 0 {
			return false
		}
		pos += idx

		// Check left boundary
		leftOK := pos == 0 || !isASCIILetter(rune(text[pos-1]))
		// Check right boundary
		end := pos + len(keyword)
		rightOK := end >= len(text) || !isASCIILetter(rune(text[end]))

		if leftOK && rightOK {
			return true
		}

		idx = pos + 1
		if idx >= len(text) {
			return false
		}
	}
}

// isASCIILetter returns true for ASCII letters only.
// Non-ASCII characters (Korean, etc.) are NOT considered letters for boundary
// purposes, so Korean text adjacent to keywords won't block matching.
func isASCIILetter(r rune) bool {
	return unicode.IsLetter(r) && r < 128
}
