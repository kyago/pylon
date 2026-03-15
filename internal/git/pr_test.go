package git

import "testing"

func TestPRCreateConfig_Defaults(t *testing.T) {
	cfg := PRCreateConfig{}

	if cfg.Title != "" {
		t.Errorf("default Title = %q, want empty", cfg.Title)
	}
	if cfg.Body != "" {
		t.Errorf("default Body = %q, want empty", cfg.Body)
	}
	if cfg.Branch != "" {
		t.Errorf("default Branch = %q, want empty", cfg.Branch)
	}
	if cfg.Base != "" {
		t.Errorf("default Base = %q, want empty", cfg.Base)
	}
	if cfg.Draft != false {
		t.Errorf("default Draft = %v, want false", cfg.Draft)
	}
	if len(cfg.Reviewers) != 0 {
		t.Errorf("default Reviewers = %v, want empty", cfg.Reviewers)
	}
}

func TestPRCreateConfig_Fields(t *testing.T) {
	cfg := PRCreateConfig{
		Title:     "feat: add login",
		Body:      "Implements login flow",
		Branch:    "feature/login",
		Base:      "main",
		Reviewers: []string{"alice", "bob"},
		Draft:     true,
	}

	if cfg.Title != "feat: add login" {
		t.Errorf("Title = %q, want %q", cfg.Title, "feat: add login")
	}
	if cfg.Body != "Implements login flow" {
		t.Errorf("Body = %q, want %q", cfg.Body, "Implements login flow")
	}
	if cfg.Branch != "feature/login" {
		t.Errorf("Branch = %q, want %q", cfg.Branch, "feature/login")
	}
	if cfg.Base != "main" {
		t.Errorf("Base = %q, want %q", cfg.Base, "main")
	}
	if !cfg.Draft {
		t.Error("Draft = false, want true")
	}
	if len(cfg.Reviewers) != 2 {
		t.Errorf("Reviewers len = %d, want 2", len(cfg.Reviewers))
	}
}

func TestPushBranch_InvalidDir(t *testing.T) {
	// git repo가 아닌 디렉토리에서 push 시도 → 에러 발생 확인
	err := PushBranch(t.TempDir(), "some-branch")
	if err == nil {
		t.Error("PushBranch() expected error for non-git directory, got nil")
	}
}

func TestCreatePR_InvalidDir(t *testing.T) {
	// gh CLI가 설치되어 있지 않거나 인증이 없는 환경에서 에러 반환 확인
	cfg := PRCreateConfig{
		Title: "test PR",
		Base:  "main",
	}
	_, err := CreatePR(t.TempDir(), cfg)
	if err == nil {
		t.Error("CreatePR() expected error for non-git directory, got nil")
	}
}
