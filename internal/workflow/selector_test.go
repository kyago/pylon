package workflow

import (
	"testing"
)

func TestSuggestWorkflow(t *testing.T) {
	tests := []struct {
		name           string
		requirement    string
		wantWorkflow   string
		wantHasKeyword bool
	}{
		{
			name:           "bugfix from fix keyword",
			requirement:    "fix: 로그인 에러",
			wantWorkflow:   "bugfix",
			wantHasKeyword: true,
		},
		{
			name:           "bugfix from bug keyword",
			requirement:    "bug: 회원가입 실패",
			wantWorkflow:   "bugfix",
			wantHasKeyword: true,
		},
		{
			name:           "hotfix from hotfix keyword",
			requirement:    "hotfix: 긴급 보안 패치",
			wantWorkflow:   "hotfix",
			wantHasKeyword: true,
		},
		{
			name:           "hotfix from urgent korean",
			requirement:    "긴급 배포 필요",
			wantWorkflow:   "hotfix",
			wantHasKeyword: true,
		},
		{
			name:           "docs from docs keyword",
			requirement:    "docs: API 문서 작성",
			wantWorkflow:   "docs",
			wantHasKeyword: true,
		},
		{
			name:           "docs from korean",
			requirement:    "문서 업데이트 필요",
			wantWorkflow:   "docs",
			wantHasKeyword: true,
		},
		{
			name:           "refactor from korean",
			requirement:    "리팩토링: 인증 모듈 개선",
			wantWorkflow:   "refactor",
			wantHasKeyword: true,
		},
		{
			name:           "review from korean",
			requirement:    "코드 리뷰 요청",
			wantWorkflow:   "review",
			wantHasKeyword: true,
		},
		{
			name:           "explore from korean",
			requirement:    "탐색: 로깅 구조 파악",
			wantWorkflow:   "explore",
			wantHasKeyword: true,
		},
		{
			name:           "explore from investigate",
			requirement:    "investigate memory leak in auth service",
			wantWorkflow:   "explore",
			wantHasKeyword: true,
		},
		{
			name:           "feature default when no match",
			requirement:    "사용자 프로필 페이지 추가",
			wantWorkflow:   "feature",
			wantHasKeyword: false,
		},
		{
			name:           "feature default for generic",
			requirement:    "implement new payment gateway",
			wantWorkflow:   "feature",
			wantHasKeyword: false,
		},
		{
			name:           "hotfix takes priority over bugfix",
			requirement:    "hotfix: fix critical auth bug",
			wantWorkflow:   "hotfix",
			wantHasKeyword: true,
		},
		{
			name:           "case insensitive matching",
			requirement:    "FIX: Login Error",
			wantWorkflow:   "bugfix",
			wantHasKeyword: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWorkflow, gotKeywords := SuggestWorkflow(tt.requirement)
			if gotWorkflow != tt.wantWorkflow {
				t.Errorf("SuggestWorkflow(%q) workflow = %q, want %q", tt.requirement, gotWorkflow, tt.wantWorkflow)
			}
			hasKeywords := len(gotKeywords) > 0
			if hasKeywords != tt.wantHasKeyword {
				t.Errorf("SuggestWorkflow(%q) hasKeywords = %v, want %v (keywords: %v)", tt.requirement, hasKeywords, tt.wantHasKeyword, gotKeywords)
			}
		})
	}
}
