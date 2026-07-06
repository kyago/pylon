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
   - 관련 project_memory 건수
   - `--purge`면 삭제될 클론 디렉터리 경로
5. `--force`가 아니고 `--json`이 아니면 `[y/N]` 확인 프롬프트. 거부 시 "취소되었습니다" 후 종료
6. 실행:
   - **기본**: 단일 트랜잭션으로 `projects` + `project_memory`(해당 `project_id`) 삭제 후,
     프로젝트의 `.pylon/` 마커 디렉터리를 제거한다. 클론된 소스 코드는 보존한다.
   - **`--purge`**: DB 삭제 성공 후 클론 디렉터리 전체를 `os.RemoveAll`

### `.pylon/` 마커를 제거하는 이유

`sync-projects`는 config.yml과 함께 워크스페이스 하위에서 `.pylon/` 디렉터리를 가진
디렉터리를 스캔해 projects 테이블을 재구성한다(`config.DiscoverProjects`). 따라서 기본
삭제가 `.pylon/`을 남기면 다음 `sync-projects` 실행 시 프로젝트가 되살아난다. 등록을
확실히 해제하기 위해 기본 삭제에서도 `.pylon/` 마커를 함께 제거한다. add-project는
config.yml을 건드리지 않으므로(DB에만 등록) config.yml 정리는 불필요하다.

## store 계층 추가 (`internal/store/projects.go`)

- `GetProject(projectID string) (*ProjectRecord, error)`
  - 존재 확인 및 `--purge`용 `path` 조회. 미존재 시 `sql.ErrNoRows` 래핑하여 반환.
- `DeleteProject(projectID string) (DeleteProjectResult, error)`
  - 단일 트랜잭션에서 `project_memory`, `projects` 순으로 삭제
  - 각 테이블 삭제 건수를 담은 결과 구조체 반환

```go
type DeleteProjectResult struct {
    Projects int64
    Memory   int64
}
```

참고: `blackboard` 테이블은 migration 007에서 삭제되어 더 이상 존재하지 않으므로 대상에서 제외한다.

## 안전장치

- 디스크를 건드리기 전, DB의 `path`가 워크스페이스 루트 하위(루트 자신 제외)에 실제로
  존재하는 디렉터리인지 검증한다(`resolveProjectDir`). 벗어나면 파일을 건드리지 않고 경고만
  출력한다. `--force`/`--json` 경로에서도 경고를 출력한다.
- DB 삭제를 먼저 수행하고, 성공 시에만 디렉터리/마커를 삭제한다. 삭제 실패는 경고로 처리(레지스트리는 이미 정리됨).
- `.git/info/exclude`의 `.pylon/` 항목은 건드리지 않는다. (`--purge`면 디렉터리째 사라져 무의미, 기본이면 `.pylon/`째 제거되므로 무의미)

## 범위 밖 (YAGNI)

- `--dry-run`: 요약 출력 + 확인 프롬프트로 충분하므로 이번 범위에서 제외.

## 테스트

`internal/cli/delete_project_test.go` 신규 작성. `add_project_test.go` 패턴을 따른다.

- 미등록 프로젝트 삭제 시 에러
- 기본 삭제: projects/project_memory DB 레코드 제거 + `.pylon/` 마커 제거, 소스는 보존
- `--purge`: 클론 디렉터리 전체 제거
- 확인 프롬프트 거부 시 등록 유지
- `resolveProjectDir`: 워크스페이스 하위/루트/밖/미존재 경로 판정
