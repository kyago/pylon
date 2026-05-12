## 개요

`pylon add-project`를 git submodule 기반에서 독립 `git clone` 기반으로 전환한다.
spec 002 다중 repo 파이프라인은 결합 방식에 중립이므로 변경 없이 동작한다.

## 주요 변경

- `pylon add-project`가 `git clone` 사용. `.gitmodules` 미생성, 워크스페이스 `git init` 불필요
- `pylon init`이 워크스페이스를 `git init`하지 않음
- `pylon migrate-project <name>` 신규 명령: 기존 submodule을 안전하게 clone으로 전환
- `detectProjectCoupling` 헬퍼로 `add-project --force`, `uninstall`, `doctor`, `migrate-project`가 동일한 결합 감지 기준 공유
- 기존 submodule 프로젝트는 동작 그대로 유지 (회귀 없음)

## 안전성 (`pylon migrate-project`)

spec 003 §5.1의 4가지 차단 조건을 모두 점검:
- working tree dirty
- unpushed local commits
- local-only branches
- 핀 SHA와 origin tip 불일치

`--dry-run`은 어떤 상태도 변경하지 않는다.
`--force`는 안전 검사를 우회한다.

## 테스트

- 단위/통합 테스트 추가: coupling 6종, add-project clone 경로, force 보호, migrate-project 안전 검사 4종 + happy path + dry-run, uninstall 분기, doctor 일반화, init git init 제거
- 전체 회귀 `go test ./... -race` 통과
- spec §8 acceptance 7항목 모두 자동 검증 완료

## 호환성

- 기존 submodule 프로젝트가 등록된 워크스페이스는 그대로 동작
- `pylon add-project --force`가 submodule 디렉토리에 대해 차단되어 실수 방지
- `--force --migrate` 조합으로 명시적 변환 + 재clone 가능

## 문서

README.md, pylon-spec.md, docs/MIGRATION-V2.md, docs/v2-rewrite/MIGRATION.md, docs/v2-rewrite/CAPABILITY-INVENTORY.md, IMPLEMENTATION_PLAN.md 모두 갱신.

## 참조

- Spec: `specs/003-add-project-clone/spec.md`
- Plan: `specs/003-add-project-clone/plan.md`
- Tasks: `specs/003-add-project-clone/tasks.md`

## 머지

squash 금지. `--merge` 사용. (사용자 메모: no-squash-merge)
