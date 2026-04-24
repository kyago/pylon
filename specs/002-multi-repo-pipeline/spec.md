# Feature Specification: Multi-Repo Pipeline Harness

**Feature Branch**: `002-multi-repo-pipeline`
**Created**: 2026-04-25
**Status**: Draft
**Input**: User description: "@claudedocs/analysis-multi-repo-pipeline-harness.md"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 단일 Repo 워크플로우 호환성 유지 (Priority: P1)

기존 단일 git repo 환경에서 파이프라인을 실행하는 사용자는 아무런 설정 변경 없이 현재와 동일하게 파이프라인을 실행할 수 있어야 한다. 변경 이후에도 단일 repo 워크플로우는 완전히 동일하게 동작해야 한다.

**Why this priority**: 기존 사용자가 회귀 없이 계속 사용할 수 있는 것이 가장 중요하다. 이 기능이 실패하면 현재 모든 사용자에게 영향을 준다.

**Independent Test**: 기존 단일 repo 프로젝트에서 pl-pipeline을 실행하여 브랜치 생성 → 태스크 실행 → PR 생성의 전체 흐름이 정상 완료되는지 확인.

**Acceptance Scenarios**:

1. **Given** 단일 git repo 프로젝트에서, **When** pl-pipeline을 기존 방식대로 실행하면, **Then** 브랜치가 생성되고, 태스크가 실행되고, PR이 생성된다.
2. **Given** `git.repo` 설정이 없는 단일 repo 환경에서, **When** pl-pipeline을 실행하면, **Then** git root를 자동으로 탐지하여 정상 동작한다.
3. **Given** 단일 repo 환경에서 생성된 tasks.json에서, **When** repo 필드 값이 `"."` 이면, **Then** 현재 repo를 대상으로 처리된다.

---

### User Story 2 - 다중 Repo 프로젝트에서 자동 서브파이프라인 스폰 (Priority: P1)

다중 git repo가 포함된 워크스페이스에서 파이프라인을 실행하는 사용자는, PM이 영향 받는 각 repo를 파악하고 각 repo별 독립 서브파이프라인을 자동으로 병렬 실행하여 결과를 집계하는 것을 경험한다.

**Why this priority**: 이 기능이 이 스펙의 핵심 목표이다. 다중 repo 환경에서 현재 파이프라인이 실패하는 근본 문제를 해결한다.

**Independent Test**: 두 개 이상의 독립 git repo가 포함된 워크스페이스에서 pl-pipeline을 실행하고 각 repo에 별도 브랜치가 생성되며 작업이 분리 실행되는지 확인.

**Acceptance Scenarios**:

1. **Given** service-a와 service-b 두 repo를 포함한 워크스페이스에서, **When** 두 repo 모두에 변경이 필요한 요구사항으로 파이프라인을 실행하면, **Then** 각 repo에 독립적으로 브랜치가 생성되고 병렬로 작업이 실행된다.
2. **Given** 아키텍처 분석 결과 두 repo가 영향을 받는 상황에서, **When** PM이 태스크를 분해하면, **Then** tasks.json에 각 태스크에 repo 필드가 포함되고, repo별로 에이전트가 스폰된다.
3. **Given** 두 repo가 병렬로 작업 중일 때, **When** 한 repo의 작업이 실패하면, **Then** 다른 repo 작업은 계속 진행되며, 최종 보고 시 성공/실패 상태가 분리 표시된다.

---

### User Story 3 - 각 Repo별 독립 PR 생성 (Priority: P2)

파이프라인 실행이 완료된 후, 각 repo에서 수행된 작업에 대해 repo별로 독립적인 PR이 생성되어 각 프로젝트 팀이 별도로 리뷰하고 머지할 수 있어야 한다.

**Why this priority**: PR 분리는 각 repo 팀의 독립적 리뷰 워크플로우를 위해 필요하지만, 핵심 파이프라인 실행 자체보다는 후속 단계이다.

**Independent Test**: 다중 repo 파이프라인 완료 후 각 repo에 대해 별도 PR이 생성되었는지 확인.

**Acceptance Scenarios**:

1. **Given** service-a와 service-b의 모든 서브파이프라인이 성공한 상황에서, **When** PM이 PR 생성을 진행하면, **Then** service-a와 service-b 각각에 별도 PR이 생성되고 URL 목록이 보고된다.
2. **Given** service-a는 성공, service-b는 실패한 상황에서, **When** PM이 PR 생성을 진행하면, **Then** service-a에만 PR이 생성되고, service-b 실패 원인이 함께 보고된다.
3. **Given** PR이 생성될 때, **When** PR 본문이 작성되면, **Then** 전체 요구사항 분석과 아키텍처 문서를 기반으로 한 맥락이 있는 설명이 포함된다.

---

### User Story 4 - 서브파이프라인별 파일시스템 격리 (Priority: P2)

여러 repo에 대한 서브파이프라인이 병렬 실행될 때, 각 repo-Agent는 자신의 repo 범위 내에서만 파일을 수정하며 다른 repo의 작업과 충돌하지 않아야 한다.

**Why this priority**: 병렬 에이전트 간 충돌은 데이터 손실이나 잘못된 커밋을 초래할 수 있어 신뢰성 측면에서 중요하다.

**Independent Test**: 동시에 두 개의 repo-Agent가 실행될 때 각자의 파일만 수정하고 서로의 작업에 간섭하지 않는지 확인.

**Acceptance Scenarios**:

1. **Given** service-a와 service-b repo-Agent가 병렬 실행 중일 때, **When** 각 Agent가 코드를 수정하면, **Then** 각 Agent의 변경사항은 해당 repo의 파일에만 적용된다.
2. **Given** 두 repo-Agent가 동시에 실행될 때, **When** 한 Agent가 오류를 만나 중단되면, **Then** 다른 Agent의 진행 중인 작업에 영향을 미치지 않는다.

---

### User Story 5 - 스크립트 --git-root 인자로 대상 Repo 명시 지정 (Priority: P2)

파이프라인 오케스트레이터는 init, verify, PR 생성 스크립트를 호출할 때 `--git-root` 인자를 통해 대상 git repo를 명시적으로 지정할 수 있어야 한다.

**Why this priority**: AI 에이전트의 동작에 의존하지 않고 스크립트 수준에서 올바른 repo를 강제 지정하는 것이 신뢰성의 핵심이다.

**Independent Test**: `init-pipeline.sh --git-root services/service-a`를 실행하여 service-a repo에 브랜치가 생성되는지 확인.

**Acceptance Scenarios**:

1. **Given** 다중 repo 워크스페이스에서, **When** `init-pipeline.sh --git-root services/service-a`를 실행하면, **Then** service-a 디렉토리의 git repo에 브랜치가 생성된다.
2. **Given** `--git-root` 인자가 제공될 때, **When** 스크립트가 실행되면, **Then** config.yml의 git.repo 설정과 자동 탐지 결과를 무시하고 명시된 경로를 사용한다.
3. **Given** 잘못된 경로가 `--git-root`로 전달될 때, **When** 스크립트가 실행되면, **Then** 명확한 오류 메시지와 함께 실패한다.

---

### Edge Cases

- 영향 받는 repo가 1개뿐인 경우: 단일 서브파이프라인으로 처리되며 기존 단일 repo 동작과 동일해야 한다.
- 동일한 태스크가 두 repo에 의존성을 가지는 경우: PM이 의존성 순서를 반영하여 스폰 순서를 결정해야 한다.
- 특정 repo에 git 권한이 없는 경우: 해당 repo 서브파이프라인은 실패로 처리되고 나머지는 계속 진행된다.
- 두 repo에서 동일한 브랜치명이 이미 존재하는 경우: 오류를 보고하고 사용자에게 안내한다.
- 아키텍처 분석 결과 영향 받는 repo가 없는 경우: 파이프라인이 조기 종료되며 이유를 보고한다.
- 서브파이프라인 실행 중 네트워크 오류로 원격 push가 실패하는 경우: 해당 서브파이프라인을 실패로 기록하고 PR 생성을 건너뛴다.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 파이프라인 시작 시 PM은 아키텍처 분석 결과에서 영향 받는 repo 목록(`affected_repos`)을 읽어 각 repo별 서브파이프라인을 식별해야 한다.
- **FR-002**: PM은 repo별로 독립된 에이전트를 단일 응답에서 병렬로 스폰할 수 있어야 한다.
- **FR-003**: tasks.json의 각 태스크는 `repo` 필드를 포함해야 하며, 이 필드는 워크스페이스 루트 기준 상대 경로여야 한다.
- **FR-004**: tasks.json에서 `status` 필드는 제거되어야 하며, 런타임 상태는 status.json의 `sub_pipelines` 배열에서 관리되어야 한다.
- **FR-005**: `init-pipeline.sh`, `run-verification.sh`, `create-pr.sh` 스크립트는 `--git-root <path>` 인자를 지원해야 한다.
- **FR-006**: `--git-root` 인자가 전달되면 스크립트는 config.yml 설정과 자동 탐지보다 이 값을 우선 사용해야 한다.
- **FR-007**: 각 repo-Agent는 지정된 repo에 브랜치를 생성하고, 태스크를 구현하고, 검증을 실행하고, 결과(성공/실패)를 반환해야 한다.
- **FR-008**: repo-Agent는 PR을 직접 생성해서는 안 되며, 코드 변경 및 브랜치 push까지만 수행해야 한다.
- **FR-009**: PM은 모든 서브파이프라인의 결과를 수집한 후, 성공한 repo에 대해서만 per-repo PR을 생성해야 한다.
- **FR-010**: 파이프라인 루트 디렉토리 아래에 repo별 서브파이프라인 디렉토리가 생성되어야 한다.
- **FR-011**: git root 자동 탐지는 커스텀 함수 없이 표준 git 명령으로 수행되어야 하며, git worktree 환경에서도 올바르게 동작해야 한다.
- **FR-012**: 단일 repo 환경에서는 서브파이프라인이 1개(`repo: "."`)로 구성되어 다중 repo와 동일한 코드 경로로 처리되어야 한다.
- **FR-013**: 아키텍처 분석 산출물은 영향 받는 repo 목록을 명시하는 섹션을 포함해야 한다.
- **FR-014**: 파이프라인 완료 보고는 repo별 성공/실패 상태와 생성된 PR URL 목록을 포함해야 한다.
- **FR-015**: 서브파이프라인 일부가 실패해도 전체 파이프라인이 중단되지 않아야 하며, PM은 성공/실패 결과를 집계하여 보고해야 한다.

### Key Entities

- **파이프라인 (Pipeline)**: 사용자 요구사항 하나를 처리하는 전체 실행 단위. 루트 파이프라인 디렉토리와 상태 파일을 가진다.
- **서브파이프라인 (Sub-pipeline)**: 특정 repo를 대상으로 하는 독립 실행 단위. 고유 디렉토리, 브랜치, 상태 정보를 가진다.
- **태스크 (Task)**: PM이 분해한 개별 작업 항목. 담당 repo, 담당 에이전트 유형, 의존성 정보를 가진다.
- **repo-Agent**: 특정 repo를 담당하는 에이전트. 해당 repo 내 모든 태스크를 순서대로 처리한다.
- **태스크 명세 (tasks.json)**: PM이 생성하는 정적 태스크 명세. 생성 후 변경되지 않는 계획 스냅샷이다.
- **파이프라인 상태 (status.json)**: 오케스트레이터가 관리하는 런타임 상태. 서브파이프라인별 실행 결과를 포함한다.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 단일 repo 환경에서 기존 파이프라인 실행 성공률이 변경 전과 동일하게 유지된다 (회귀 0건).
- **SC-002**: 2개 이상의 repo를 포함하는 다중 repo 요구사항에 대해 파이프라인이 중단 없이 완전히 실행 완료된다.
- **SC-003**: 다중 repo 파이프라인 실행 시 병렬 repo-Agent가 서로 다른 repo를 대상으로 충돌 없이 독립적으로 실행된다.
- **SC-004**: 스크립트에 `--git-root` 인자를 전달했을 때 100%의 경우 지정된 repo에서 실행된다.
- **SC-005**: 다중 repo 파이프라인 완료 후 각 성공 repo에 대해 별도 PR이 자동 생성되며 PR URL이 최종 보고서에 포함된다.
- **SC-006**: 서브파이프라인 일부가 실패해도 성공한 repo에 대한 PR 생성이 정상 완료되는 부분 성공 시나리오가 올바르게 처리된다.
- **SC-007**: 파이프라인 완료 보고서에 repo별 성공/실패 상태와 실패 원인이 명확하게 포함된다.

## Assumptions

- 다중 repo 워크스페이스에서 각 repo는 독립적인 git 저장소를 가진다.
- 모든 repo는 PR 생성이 가능한 원격 저장소와 연결되어 있다.
- PM 에이전트는 아키텍처 분석 산출물에서 영향 받는 repo 목록을 자율적으로 파악할 수 있다.
- repo 경로는 워크스페이스 루트 기준 상대 경로로 표현된다.
- 단일 repo 환경에서 `repo: "."` 는 워크스페이스 루트를 가리킨다.
- repo-Agent는 PR 생성 권한이 없으며, 코드 변경 및 브랜치 push까지만 담당한다.

## Out of Scope

- 3개 이상의 레이어로 중첩된 서브파이프라인 (repo-Agent 내부에서 추가 에이전트 스폰 없음)
- 실패한 서브파이프라인의 자동 재시도
- 파이프라인 실행 중 실시간 진행 상황 UI 대시보드
- 서로 다른 repo 간 태스크 의존성 그래프 관리
