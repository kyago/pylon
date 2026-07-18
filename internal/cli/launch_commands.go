package cli

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kyago/pylon/internal/layout"
)

//go:embed commands/*.md
var embeddedCommands embed.FS

// buildSlashCommands generates .claude/commands/pl/ markdown files.
func buildSlashCommands(root string) map[string]string {
	commands := map[string]string{
		"pl/project-list": `# /pl:project-list — 프로젝트 목록 및 인덱싱 정보 조회

워크스페이스에 등록된 프로젝트들의 목록과 인덱싱 정보를 조회하여 사용자에게 제공합니다.

## 절차

1. 프로젝트 목록을 동기화하고 조회합니다 (DB에 최신 상태를 반영하기 위해 sync를 먼저 실행):
   ` + "```" + `bash
   pylon sync-projects
   ` + "```" + `

2. 각 프로젝트의 인덱싱 상태를 판단합니다. 다음 두 지표를 확인하여 판단합니다:

   a. 컨텍스트 파일 존재 여부 (인덱싱의 주요 지표):
      - ` + "`" + `<프로젝트명>/.pylon/context.md` + "`" + ` 파일이 존재하면 인덱싱 완료로 판단

   b. 프로젝트 메모리 조회 (카테고리 필터 없이 전체 조회):
      ` + "```" + `bash
      pylon mem list --project <프로젝트명>
      ` + "```" + `
      결과가 없으면 워크스페이스명으로 재시도합니다 (멀티 프로젝트 워크스페이스에서는 워크스페이스명으로 저장됨):
      ` + "```" + `bash
      pylon mem list --project <워크스페이스명>
      ` + "```" + `

3. 각 프로젝트의 추가 정보를 확인합니다:
   - ` + "`" + `<프로젝트명>/.pylon/context.md` + "`" + ` — 프로젝트 컨텍스트 (존재 여부 및 요약)
   - ` + "`" + `<프로젝트명>/.pylon/agents/` + "`" + ` — 프로젝트 전용 에이전트 정의

4. 수집한 정보를 다음 형식으로 정리하여 사용자에게 보여줍니다:

   **각 프로젝트별 표시 항목:**
   - 프로젝트명
   - 경로
   - 기술 스택 (tech stack)
   - 인덱싱 상태 (context.md 존재 여부 + 메모리 엔트리 수)
   - 컨텍스트 파일 존재 여부
   - 등록된 에이전트 수

## 인덱싱 상태 판단 기준

- **인덱싱 완료**: ` + "`" + `<프로젝트명>/.pylon/context.md` + "`" + ` 파일이 존재하거나, 해당 프로젝트의 메모리 엔트리가 1개 이상 있는 경우
- **미인덱싱**: 위 조건을 모두 만족하지 않는 경우

## 주의사항

- 인덱싱이 되지 않은 프로젝트는 "미인덱싱" 상태로 표시하고 ` + "`" + `/pl:index` + "`" + ` 실행을 안내
- 프로젝트가 없는 경우 ` + "`" + `pylon add-project <git-url>` + "`" + `로 추가하도록 안내
- 출력은 테이블 또는 구조화된 목록 형태로 보기 좋게 정리
- 메모리 조회 시 ` + "`" + `--category` + "`" + ` 필터를 사용하지 않습니다 (실제 데이터는 change, learning 등 다양한 카테고리로 저장됨)
`,

		"pl/index": `# /pl:index — 프로젝트 코드베이스 인덱싱

프로젝트 코드베이스를 분석하여 도메인 위키와 프로젝트 컨텍스트를 갱신합니다.

## 절차

1. 대상 프로젝트를 사용자에게 확인합니다
2. 각 프로젝트 디렉토리의 구조, 주요 파일, 의존성을 분석합니다
3. ` + "`" + `.pylon/domain/` + "`" + ` 하위에 도메인 지식 문서를 생성/갱신합니다:
   - overview.md — 프로젝트 구조 개요
   - practices.md — 작업 관행
   - glossary.md — 용어 사전
4. 각 프로젝트의 ` + "`" + `{프로젝트}/.pylon/context.md` + "`" + `를 실제 코드에 맞게 갱신합니다
5. 발견한 기술 스택 정보를 메모리에 저장합니다:
   ` + "```" + `bash
   pylon mem store --project <name> --key "tech-stack" --content "..." --category codebase
   ` + "```" + `

## 주의사항

- 기존 내용이 있으면 보존하되, 변경된 부분만 업데이트
- 추측이 아닌 코드에서 확인된 사실만 기록
- 위키 문서는 마크다운 형식으로 작성
`,
	}
	return commands
}

// legacyCommandFiles lists pre-namespace top-level command files that should be
// removed from .claude/commands/ to prevent stale slash commands.
var legacyCommandFiles = []string{"index", "status", "verify", "add-project", "cancel", "review"}

const managedClaudeCommandsManifest = ".pylon-managed.json"

// buildDesiredClaudeCommands computes the full set of .claude/commands/ files
// that should exist, keyed by path relative to the commands dir (e.g.
// "pl/index.md"). It is read-only and shared by `pylon launch` and `pylon doctor`
// so both agree on the desired state.
func buildDesiredClaudeCommands(root string) map[string]string {
	desired := make(map[string]string)

	// Dynamic slash commands generated from Go code
	for name, content := range buildSlashCommands(root) {
		desired[filepath.FromSlash(name)+".md"] = content
	}

	// Pipeline slash commands → pl/: prefer .pylon/commands/ (user customization),
	// fall back to embedded defaults for workspaces without .pylon/commands/.
	pylonCmdsDir := layout.CommandsDir(root)
	if entries, err := os.ReadDir(pylonCmdsDir); err == nil && len(entries) > 0 {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(pylonCmdsDir, entry.Name()))
			if err != nil {
				continue
			}
			destName := strings.TrimPrefix(entry.Name(), "pl-")
			desired[filepath.Join("pl", destName)] = string(content)
		}
	} else {
		embedded, _ := embeddedCommands.ReadDir("commands")
		for _, entry := range embedded {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			content, err := embeddedCommands.ReadFile("commands/" + entry.Name())
			if err != nil {
				continue
			}
			destName := strings.TrimPrefix(entry.Name(), "pl-")
			desired[filepath.Join("pl", destName)] = string(content)
		}
	}

	return desired
}

// applyClaudeCommands writes the desired command files into commandsDir and
// removes legacy (pre-namespace) top-level command files. It intentionally does
// NOT remove other on-disk files so any user-added commands are preserved.
func applyClaudeCommands(commandsDir string, desired map[string]string) error {
	previouslyManaged := readManagedClaudeCommandSet(commandsDir)

	for _, name := range legacyCommandFiles {
		legacy := filepath.Join(commandsDir, name+".md")
		if err := os.Remove(legacy); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("레거시 커맨드 파일 제거 실패 (%s): %w", legacy, err)
		}
	}
	for rel := range previouslyManaged {
		if _, ok := desired[rel]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(commandsDir, rel)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("이전 커맨드 파일 제거 실패 (%s): %w", rel, err)
		}
	}
	for rel, content := range desired {
		cmdPath := filepath.Join(commandsDir, rel)
		if err := os.MkdirAll(filepath.Dir(cmdPath), 0755); err != nil {
			return fmt.Errorf("커맨드 디렉토리 생성 실패: %w", err)
		}
		if err := os.WriteFile(cmdPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("커맨드 %s 생성 실패: %w", rel, err)
		}
	}
	if err := writeManagedClaudeCommandSet(commandsDir, desired); err != nil {
		return err
	}
	return nil
}

func readManagedClaudeCommandSet(commandsDir string) map[string]bool {
	managed := make(map[string]bool)
	content, err := os.ReadFile(filepath.Join(commandsDir, managedClaudeCommandsManifest))
	if err != nil {
		return managed
	}
	var files []string
	if err := json.Unmarshal(content, &files); err != nil {
		return managed
	}
	for _, rel := range files {
		if isSafeCommandRel(rel) {
			managed[rel] = true
		}
	}
	return managed
}

func writeManagedClaudeCommandSet(commandsDir string, desired map[string]string) error {
	files := make([]string, 0, len(desired))
	for rel := range desired {
		files = append(files, rel)
	}
	sort.Strings(files)
	content, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return fmt.Errorf("커맨드 매니페스트 생성 실패: %w", err)
	}
	content = append(content, '\n')
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return fmt.Errorf("커맨드 디렉토리 생성 실패: %w", err)
	}
	if err := os.WriteFile(filepath.Join(commandsDir, managedClaudeCommandsManifest), content, 0644); err != nil {
		return fmt.Errorf("커맨드 매니페스트 쓰기 실패: %w", err)
	}
	return nil
}

func isSafeCommandRel(rel string) bool {
	if rel == "" || filepath.IsAbs(rel) {
		return false
	}
	clean := filepath.Clean(rel)
	return clean == rel && clean != "." && !strings.HasPrefix(clean, ".."+string(os.PathSeparator)) && clean != ".."
}

// bootstrapPylonCommands writes embedded default commands into .pylon/commands/
// when the directory is empty, giving users a starting point to customize.
func bootstrapPylonCommands(pylonCmdsDir string) error {
	if entries, err := os.ReadDir(pylonCmdsDir); err == nil && len(entries) > 0 {
		return nil // already populated — preserve user customizations
	}
	if err := os.MkdirAll(pylonCmdsDir, 0755); err != nil {
		return fmt.Errorf(".pylon/commands/ 디렉토리 생성 실패: %w", err)
	}
	embedded, err := embeddedCommands.ReadDir("commands")
	if err != nil {
		return nil
	}
	for _, entry := range embedded {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := embeddedCommands.ReadFile("commands/" + entry.Name())
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(pylonCmdsDir, entry.Name()), content, 0644)
	}
	return nil
}
