package git

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBranchName(t *testing.T) {
	today := time.Now().Format("20060102")

	tests := []struct {
		name     string
		prefix   string
		taskDesc string
		want     string
	}{
		{
			name:     "정상 입력",
			prefix:   "task",
			taskDesc: "user login",
			want:     "task/" + today + "-user-login",
		},
		{
			name:     "40자 초과 slug 절삭",
			prefix:   "task",
			taskDesc: strings.Repeat("a", 50),
			want:     "task/" + today + "-" + strings.Repeat("a", 40),
		},
		{
			name:     "빈 taskDesc는 task 폴백",
			prefix:   "feature",
			taskDesc: "",
			want:     "feature/" + today + "-task",
		},
		{
			name:     "한글 입력",
			prefix:   "task",
			taskDesc: "로그인 기능",
			want:     "task/" + today + "-로그인-기능",
		},
		{
			name:     "trailing dash 제거",
			prefix:   "task",
			taskDesc: strings.Repeat("abcde-", 8), // 48 chars → truncated to 40 → "abcde-abcde-abcde-abcde-abcde-abcde-abcd" → no trailing dash
			want:     "task/" + today + "-" + "abcde-abcde-abcde-abcde-abcde-abcde-abcd",
		},
		{
			name:     "trailing dash 제거 실제 발생 케이스",
			prefix:   "task",
			taskDesc: strings.Repeat("abcdefgh-", 5), // 45 chars → truncated to 40 → "abcdefgh-abcdefgh-abcdefgh-abcdefgh-abcd" → no trailing dash
			want:     "task/" + today + "-abcdefgh-abcdefgh-abcdefgh-abcdefgh-abcd",
		},
		{
			name:     "특수문자가 포함된 입력",
			prefix:   "fix",
			taskDesc: "fix bug #123!",
			want:     "fix/" + today + "-fix-bug-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BranchName(tt.prefix, tt.taskDesc)
			if got != tt.want {
				t.Errorf("BranchName(%q, %q) = %q, want %q", tt.prefix, tt.taskDesc, got, tt.want)
			}
		})
	}
}

func TestBranchName_SlugTruncationRemovesTrailingDash(t *testing.T) {
	today := time.Now().Format("20060102")

	// Craft input where truncation at 40 chars produces trailing dash.
	// "a" repeated 39 times + "-b" = 41 chars → slug truncated to 40 → "aaa...a-" → TrimRight → "aaa...a"
	input := strings.Repeat("a", 39) + "-b"
	got := BranchName("task", input)
	want := "task/" + today + "-" + strings.Repeat("a", 39)

	if got != want {
		t.Errorf("trailing dash not removed: got %q, want %q", got, want)
	}
}

func TestCreateBranch(t *testing.T) {
	dir := initTestRepo(t)

	err := CreateBranch(dir, "feature/test-branch")
	if err != nil {
		t.Fatalf("CreateBranch() unexpected error: %v", err)
	}

	// 생성된 브랜치가 현재 브랜치인지 확인
	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch() unexpected error: %v", err)
	}
	if branch != "feature/test-branch" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "feature/test-branch")
	}
}

func TestCreateBranch_DuplicateName(t *testing.T) {
	dir := initTestRepo(t)

	if err := CreateBranch(dir, "dup-branch"); err != nil {
		t.Fatalf("first CreateBranch() unexpected error: %v", err)
	}

	// 같은 이름의 브랜치를 다시 만들면 에러
	// 먼저 다른 브랜치로 이동 후 다시 같은 이름으로 생성 시도
	cmd := exec.Command("git", "checkout", "-b", "temp-branch")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout temp branch: %v\n%s", err, output)
	}

	err := CreateBranch(dir, "dup-branch")
	if err == nil {
		t.Error("CreateBranch() expected error for duplicate branch name, got nil")
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := initTestRepo(t)

	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch() unexpected error: %v", err)
	}

	// git init 직후 기본 브랜치는 main 또는 master
	if branch != "main" && branch != "master" {
		t.Errorf("CurrentBranch() = %q, want main or master", branch)
	}
}

func TestCurrentBranch_AfterSwitch(t *testing.T) {
	dir := initTestRepo(t)

	if err := CreateBranch(dir, "my-branch"); err != nil {
		t.Fatalf("CreateBranch() unexpected error: %v", err)
	}

	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch() unexpected error: %v", err)
	}
	if branch != "my-branch" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "my-branch")
	}
}

func TestCurrentBranch_InvalidDir(t *testing.T) {
	_, err := CurrentBranch(t.TempDir()) // git repo가 아닌 디렉토리
	if err == nil {
		t.Error("CurrentBranch() expected error for non-git directory, got nil")
	}
}
