# Script Interface Contracts: Multi-Repo Pipeline

**Branch**: `002-multi-repo-pipeline` | **Date**: 2026-04-25

## GIT_ROOT 결정 우선순위 (모든 스크립트 공통)

```
1. --git-root <path>     스크립트 인자 (REPO_ROOT 기준 realpath로 변환)
2. config.yml git.repo   기존 config 설정
3. git rev-parse --show-toplevel  자동 탐지 (find_git_root 대체)
```

---

## `init-pipeline.sh`

### 루트 모드 (Step 1)

```bash
init-pipeline.sh "<requirement>"
```

**입력**:
- `$1`: 요구사항 문자열 (필수)

**동작**:
- PIPELINE_DIR 생성 (`$RUNTIME_DIR/{PIPELINE_ID}/`)
- `requirement.md` 작성
- `status.json` 초기화 (`stage: "init"`, `sub_pipelines: []`)
- **브랜치 생성 없음** (Step 1에서는 repo가 확정되지 않음)

**sub_pipelines 업데이트 책임**:
- 이 스크립트는 `sub_pipelines: []` 만 초기화
- PM이 Step 5에서 `affected_repos` 기반으로 항목을 채움 (스크립트 외부)

**출력 (JSON)**:
```json
{
  "pipeline_id": "20260425-feat-login",
  "pipeline_dir": "/path/to/.pylon/runtime/20260425-feat-login"
}
```

### 서브파이프라인 모드 (repo-Agent)

```bash
init-pipeline.sh "<requirement>" --git-root <repo-rel-path> --pipeline-dir <root-pipeline-dir>
```

**입력**:
- `$1`: 요구사항 문자열 (필수, 루트와 동일)
- `--git-root`: REPO_ROOT 기준 repo 상대경로 (예: `services/service-a`)
- `--pipeline-dir`: 루트 파이프라인 디렉토리 절대경로

**동작**:
- `<root-pipeline-dir>/<repo-basename>/` 서브파이프라인 dir 생성
- 지정된 `GIT_ROOT` repo에 브랜치 생성 (기존 있으면 checkout)
- 서브파이프라인 `status.json` 초기화

**출력 (JSON)**:
```json
{
  "pipeline_id": "20260425-feat-login",
  "branch": "task-feat-login",
  "pipeline_dir": "/path/to/.pylon/runtime/20260425-feat-login/service-a"
}
```

**에러 케이스**:
- `--git-root` 경로가 유효한 git repo가 아닌 경우: non-zero exit + 에러 메시지
- `--pipeline-dir` 없이 `--git-root`만 제공: non-zero exit

---

## `run-verification.sh`

```bash
run-verification.sh "<pipeline-dir>" [--git-root <repo-rel-path>]
```

**입력**:
- `$1`: 파이프라인 디렉토리 (필수)
- `--git-root`: 검증 대상 repo (생략 시 REPO_ROOT에서 실행)

**동작**:
- `--git-root` 있으면 해당 repo에서 검증 실행
- `--git-root` repo에 `go.mod` 없으면 검증 스킵 (성공 반환)
- `--git-root` 없으면 기존 동작 (REPO_ROOT에서 Go 검증)

**출력 (JSON, pipeline-dir/verification.json에도 저장)**:
```json
{
  "ok": true,
  "checks": [
    {"name": "build", "ok": true, "output": ""},
    {"name": "vet",   "ok": true, "output": ""},
    {"name": "test",  "ok": true, "output": ""}
  ],
  "timestamp": "2026-04-25T10:30:00Z"
}
```

**스킵 시 출력**:
```json
{
  "ok": true,
  "checks": [],
  "skipped": true,
  "timestamp": "2026-04-25T10:30:00Z"
}
```

---

## `create-pr.sh`

```bash
create-pr.sh "<pipeline-dir>" [--git-root <repo-rel-path>] --branch <branch> [--title <title>] [--body <body>] [--draft]
```

**입력**:
- `$1`: 파이프라인 디렉토리 (필수)
- `--git-root`: PR 생성 대상 repo (생략 시 기존 GIT_ROOT)
- `--branch`: 대상 브랜치 (필수)
- `--title`: PR 제목 (생략 시 requirement.md 첫 줄)
- `--body`: PR 본문 (생략 시 requirement.md + requirement-analysis.md)
- `--draft`: draft PR 생성

**동작**:
- `--git-root` 있으면 해당 repo에서 `gh pr create` 실행
- 기존 동작은 유지 (GIT_ROOT 기준)

**출력 (JSON, pipeline-dir/pr.json에도 저장)**:
```json
{
  "url": "https://github.com/org/service-a/pull/42",
  "number": "42",
  "title": "feat: 로그인 기능 구현"
}
```

---

## `common.sh` 변경사항

**제거**: `find_git_root()` 함수

**대체**:
```bash
# 기존
GIT_ROOT="$(find_git_root)"

# 변경
GIT_ROOT="$(git -C "$REPO_ROOT" rev-parse --show-toplevel 2>/dev/null || echo "$REPO_ROOT")"
```

**추가 없음**: `--git-root` 파싱은 각 스크립트에서 사전 처리 (common.sh 범위 밖)
