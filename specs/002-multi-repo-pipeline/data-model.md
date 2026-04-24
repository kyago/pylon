# Data Model: Multi-Repo Pipeline Harness

**Branch**: `002-multi-repo-pipeline` | **Date**: 2026-04-25

## tasks.json — 정적 태스크 명세

PM이 Step 5에서 생성하는 불변 계획 스냅샷.

### 스키마 변경

| 필드 | 변경 | 설명 |
|------|------|------|
| `id` | 유지 | 태스크 고유 식별자 |
| `title` | 유지 | 태스크 제목 |
| `description` | 유지 | 상세 설명 |
| `agent` | 유지 | 담당 에이전트 유형 |
| `repo` | **추가** | REPO_ROOT 기준 상대경로. 단일 repo는 `"."` |
| `dependencies` | 유지 | 선행 태스크 ID 목록 (비즈니스 의존성만) |
| `status` | **제거** | 런타임 상태는 status.json으로 이전 |

### 예시

```json
{
  "tasks": [
    {
      "id": "T001",
      "title": "서비스 A API 엔드포인트 추가",
      "description": "POST /users 엔드포인트 구현",
      "agent": "backend-dev",
      "repo": "services/service-a",
      "dependencies": []
    },
    {
      "id": "T002",
      "title": "서비스 B 클라이언트 업데이트",
      "description": "service-a의 새 API 호출 클라이언트 추가",
      "agent": "backend-dev",
      "repo": "services/service-b",
      "dependencies": []
    }
  ]
}
```

### 단일 repo 예시 (`repo: "."`)

```json
{
  "tasks": [
    {
      "id": "T001",
      "title": "로그인 기능 구현",
      "description": "JWT 기반 로그인 엔드포인트",
      "agent": "backend-dev",
      "repo": ".",
      "dependencies": []
    }
  ]
}
```

---

## status.json — 런타임 상태

오케스트레이터(PM 또는 스크립트)가 관리하는 동적 상태 파일.

**sub_pipelines 업데이트 책임**:
- 루트 `init-pipeline.sh` (Step 1): `sub_pipelines: []` 빈 배열로 초기화
- PM (Step 5, repo-Agent 스폰 직전): `affected_repos` 목록을 읽어 `sub_pipelines` 항목을 채움
- repo-Agent 완료 후: PM이 해당 항목의 `status`를 `success` 또는 `failed`로 업데이트

### 루트 status.json 스키마

| 필드 | 타입 | 설명 |
|------|------|------|
| `stage` | string | 현재 단계 (init/analyzing/executing/pr/done) |
| `status` | string | 전체 상태 (running/success/failed) |
| `branch` | string | 파이프라인 브랜치명 |
| `started_at` | ISO8601 | 시작 시각 |
| `sub_pipelines` | array | 서브파이프라인 목록 (단일 repo: 1개 항목) |

### sub_pipelines 항목 스키마

| 필드 | 타입 | 설명 |
|------|------|------|
| `repo` | string | REPO_ROOT 기준 상대경로 |
| `branch` | string | 해당 repo의 브랜치명 |
| `pipeline_dir` | string | 서브파이프라인 디렉토리 경로 |
| `status` | string | running / success / failed |

### 예시 (다중 repo)

```json
{
  "stage": "executing",
  "status": "running",
  "branch": "task-feat-login",
  "started_at": "2026-04-25T10:00:00Z",
  "sub_pipelines": [
    {
      "repo": "services/service-a",
      "branch": "task-feat-login",
      "pipeline_dir": ".pylon/runtime/20260425-feat-login/service-a",
      "status": "success"
    },
    {
      "repo": "services/service-b",
      "branch": "task-feat-login",
      "pipeline_dir": ".pylon/runtime/20260425-feat-login/service-b",
      "status": "running"
    }
  ]
}
```

### 예시 (단일 repo)

```json
{
  "stage": "executing",
  "status": "running",
  "branch": "task-feat-login",
  "started_at": "2026-04-25T10:00:00Z",
  "sub_pipelines": [
    {
      "repo": ".",
      "branch": "task-feat-login",
      "pipeline_dir": ".pylon/runtime/20260425-feat-login",
      "status": "running"
    }
  ]
}
```

---

## PIPELINE_DIR 계층구조

```
.pylon/runtime/
└── {PIPELINE_ID}/                      ← Step 1 (루트 init-pipeline.sh)
    ├── requirement.md                  ← 사용자 요구사항 원문
    ├── requirement-analysis.md         ← Step 2 PO 분석
    ├── architecture.md                 ← Step 3 아키텍처 (affected_repos 포함)
    ├── tasks.json                      ← Step 5 PM 태스크 명세
    ├── status.json                     ← 루트 파이프라인 상태
    ├── {repo-basename-A}/              ← repo-Agent (init-pipeline.sh --git-root)
    │   ├── status.json                 ← 서브파이프라인 A 상태
    │   └── pr.json                     ← PR 생성 결과
    └── {repo-basename-B}/
        ├── status.json
        └── pr.json
```

---

## architecture.md `affected_repos` 섹션

아키텍트 에이전트가 출력해야 하는 필수 섹션:

```markdown
## Affected Repositories

- `services/service-a`: REST API 변경으로 엔드포인트 추가 필요
- `services/service-b`: service-a 클라이언트 업데이트 필요
```

PM은 이 섹션의 항목별 경로를 읽어 repo-Agent 수를 결정한다.
