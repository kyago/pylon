# delete-project 명령 설계

## 목표

`pylon add-project`로 등록한 프로젝트를 워크스페이스에서 제거하는 `pylon delete-project` 명령을 추가한다. `add-project`와 대칭되는 안전한 기본 동작을 제공한다.

## 명령 사양

```
pylon delete-project <name> [--purge] [--force]
```

- `<name>`: 제거할 프로젝트 이름 (필수, 정확히 1개)
- `--purge`: 디스크의 클론 디렉터리까지 삭제 (기본은 DB 등록만 해제)
- `--force` / `-f`: 확인 프롬프트 생략
- `--json`: 결과를 JSON으로 출력 (기존 mem 명령들과 일관)

## 동작 흐름

1. `openWorkspaceStore()`로 워크스페이스 루트/스토어 확보 (없으면 에러)
2. `validateProjectName(name)`으로 이름 검증 (경로 traversal 방지, 기존 함수 재사용)
3. `store.GetProject(name)`으로 조회 → 미등록이면 `project "%s" is not registered` 에러
4. 삭제 대상 요약 출력:
   - projects 레코드 1건
   - 관련 project_memory / blackboard 건수
   - `--purge`면 삭제될 클론 디렉터리 경로
5. `--force`가 아니고 `--json`이 아니면 `[y/N]` 확인 프롬프트. 거부 시 "취소되었습니다" 후 종료
6. 실행:
   - **기본**: 단일 트랜잭션으로 `projects` + `project_memory` + `blackboard`(해당 `project_id`) 삭제
   - **`--purge`**: DB 삭제 성공 후 클론 디렉터리 `os.RemoveAll`

## store 계층 추가 (`internal/store/projects.go`)

- `GetProject(projectID string) (*ProjectRecord, error)`
  - 존재 확인 및 `--purge`용 `path` 조회. 미존재 시 `sql.ErrNoRows` 래핑하여 반환.
- `DeleteProject(projectID string) (DeleteProjectResult, error)`
  - 단일 트랜잭션에서 `project_memory`, `blackboard`, `projects` 순으로 삭제
  - 각 테이블 삭제 건수를 담은 결과 구조체 반환

```go
type DeleteProjectResult struct {
    Projects   int64
    Memory     int64
    Blackboard int64
}
```

## 안전장치

- `--purge` 디렉터리 삭제 전, DB의 `path`가 워크스페이스 루트 하위인지 검증하여 임의 경로 삭제를 방지한다. 벗어나면 삭제를 건너뛰고 경고만 출력한다.
- DB 삭제를 먼저 수행하고, 성공 시에만 디렉터리를 삭제한다. 디렉터리 삭제 실패는 경고로 처리(레지스트리는 이미 정리됨).
- `.git/info/exclude`의 `.pylon/` 항목은 건드리지 않는다. (`--purge`면 디렉터리째 사라져 무의미, 아니면 사용자 파일이므로 보존)

## 범위 밖 (YAGNI)

- `--dry-run`: 요약 출력 + 확인 프롬프트로 충분하므로 이번 범위에서 제외.

## 테스트

`internal/cli/delete_project_test.go` 신규 작성. `add_project_test.go` 패턴을 따른다.

- 미등록 프로젝트 삭제 시 에러
- 기본 삭제: projects/project_memory/blackboard DB 레코드 제거, 디렉터리는 보존
- `--purge`: 클론 디렉터리까지 제거
- `--force`로 프롬프트 생략 동작
- 워크스페이스 밖 경로는 `--purge` 시 디렉터리 삭제를 건너뜀
