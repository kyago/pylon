# AGENTS.md를 AI provider가 저작하도록 전환 — 설계

- 날짜: 2026-08-07
- 상태: 설계 승인 대기
- 대상 바이너리: `pylon`

## 배경 / 문제

현재 워크스페이스의 루트 시스템 프롬프트는 `buildRootCLAUDEMD`
(`internal/cli/launch_claudemd.go`)가 **Go 코드에서 한국어 프로즈 ~280줄을 하드코딩**해
`CLAUDE.md`로 출력한다. 이 파일은 `runLaunch`에서 **매 launch마다 무조건 덮어쓰이고**
(`launch.go:154`), `.gitignore`에 올라간 **일회성 생성물**이다.

문제:

1. **상시 토큰 과다.** 위임 판단·안티패턴·에이전트 선택·파이프라인 설명 등 상당 부분이
   Claude가 네이티브로 이미 수행하는 규범을 재교육하는 프로즈다. 멀티레포에서 매 세션 상시
   컨텍스트를 차지한다.
2. **저작 주체가 pylon(Go)에 고정.** pylon의 정체성은 "얇은 런처 — Go 코드는 파이프라인을
   돌리지 않는다"인데, 시스템 프롬프트 저작만은 Go가 통째로 쥐고 있어 워크스페이스 실정에
   맞춘 재단이 불가능하고, 문구 변경이 곧 바이너리 변경이다.

## 목표

- 루트 운영 가이드의 **저작을 사용자의 AI provider(= 실행된 claude 세션)에게 위임**한다.
- pylon은 "how to use pylon" **레퍼런스(매뉴얼)만 공급**한다. Go는 LLM을 호출하지 않는다.
- 도구 버전(매뉴얼) 업그레이드 시 **재저작이 트리거**되도록 한다.
- `init`은 오프라인·즉시·결정적으로 유지한다.

### 비목표

- 다른 AI provider용 subprocess/provider 추상화 (백엔드가 `claude-code`로 고정된 현재
  불필요 — 만들지 않는다).
- `.pylon/domain/`, 메모리, config 등 사용자 데이터 형식 변경.
- 파이프라인·에이전트·슬래시 커맨드 동작 변경.

## 확정된 결정 (브레인스토밍 합의)

1. **작성 주체 = 세션 내 저작.** Go는 LLM을 호출하지 않는다. `init`/`launch`는 스캐폴딩과
   부트스트랩 파일만 쓰고, 실제 저작은 실행된 claude 세션이 첫 턴에 수행한다.
2. **지속성 = git-ignore + 전자동 관리, 마커 없음.** 비결정적 AI 출력의 diff/머지 충돌을
   피한다. 사용자 커스터마이징은 `.pylon/domain/`(never-overwritten 사용자 데이터)에 둔다.
3. **파일 분리.**
   - `AGENTS.md` — AI가 저작하는 실제 운영 가이드(프로바이더 중립 파일명). 부트스트랩·버전
     스탬프·재저작 대상.
   - `CLAUDE.md` — Go가 자동 생성하는 **import 마커 한 줄**(`@AGENTS.md`). Claude Code가
     CLAUDE.md를 로드할 때 AGENTS.md를 그대로 import한다.
4. **검증 명령은 온디맨드 위임.** `renderVerificationCommands`를 CLAUDE.md 생성에서 제거한다.
   AI가 필요할 때 각 프로젝트의 `.pylon/verify.yml`을 직접 읽는다(원본이 곧 진실).

## 아키텍처

### 신규/변경 구성요소

#### 1. 임베디드 레퍼런스 `pylon-usage.md`

- 위치: `internal/cli/reference/pylon-usage.md` (`//go:embed`).
- 내용: 현재 `buildRootCLAUDEMD` 프로즈의 *정보*를 **CLAUDE.md 완성본이 아니라 매뉴얼**로
  재구성. 포함: pylon 워크스페이스 개념, 슬래시 커맨드 목록, 도메인 자동감지/라우팅,
  도메인별 파이프라인 단계, 위임 판단 기준, 서브에이전트 선택 기준, 프로젝트 메모리 CLI,
  검증 규약(각 `verify.yml`을 읽어 실행), 상태파일 규약, 안티패턴, 행동 규칙.
- 상단에 매뉴얼 버전 스탬프(아래 `pylonUsageVersion`와 동일 값)를 문서화한다.
- 런타임 배치: `.pylon/reference/pylon-usage.md`. pylon-owned 리소스로서 소유권 계약에
  편입되어 launch/doctor가 임베디드 버전으로 갱신한다(`syncPylonResources` 대상에
  `reference/` 추가).

#### 2. 버전 스탬프 상수

- `internal/cli/`에 `pylonUsageVersion` 정수(또는 문자열) 상수. **매뉴얼이 바뀔 때만** 증가.
  pylon CalVer 릴리스와 독립 — 재저작 트리거는 "버전 아무거나 오름"이 아니라 "매뉴얼이 바뀜".

#### 3. 부트스트랩 AGENTS.md 생성기 (Go)

`buildRootCLAUDEMD`를 대체하는 `buildBootstrapAgentsMD(root, projects) string`. AGENTS.md가
없거나 stale일 때만 쓰는 **짧은(~20줄)** 파일:

- 최상단 버전 스탬프 주석: `<!-- pylon-usage-version: N -->`
- 저작 지시문(한국어): 이 워크스페이스는 pylon으로 초기화됨(또는 vN으로 업그레이드됨).
  다른 작업을 하기 전에 `.pylon/reference/pylon-usage.md`와 실제 리포·`.pylon/`을 읽고,
  **이 파일(AGENTS.md)을 이 워크스페이스의 운영 가이드로 재작성**하라. 완료 후 상단 버전
  스탬프를 N으로 남겨라.
- 워크스페이스 루트·`.pylon/` 레이아웃·프로젝트 목록 포인터 포함.
- 이 부트스트랩은 **재저작이 끝내 일어나지 않아도 최소 기능하는 폴백**을 겸한다(오프라인/
  비대화형 세션 등).

#### 4. CLAUDE.md 포인터 생성기 (Go)

`buildClaudeMDPointer() string` — Claude Code의 import 마커 **한 줄**:

```
@AGENTS.md
```

`@AGENTS.md`는 Claude Code가 CLAUDE.md를 로드할 때 AGENTS.md 내용을 그대로 import한다.
결정적 상수라 매 launch 재생성해도 무해하다.

#### 5. Stale 판정 (`launch.go` + `doctor.go`)

- 헬퍼 `agentsMDStale(root string) bool`: `AGENTS.md`가 없으면 stale. 있으면 최상단
  `<!-- pylon-usage-version: N -->`를 파싱해 `pylonUsageVersion`과 비교, 낮으면 stale.
  스탬프 파싱 실패도 stale로 간주(안전측).

### 흐름

#### `pylon init`
1. `.pylon/` 스캐폴딩(기존) + `.pylon/reference/pylon-usage.md` 기록.
2. 부트스트랩 `AGENTS.md` 기록.
3. CLAUDE.md 포인터 기록.
4. `.gitignore`에 `AGENTS.md`, `CLAUDE.md` 유지(둘 다 ignore).
5. **AI 호출 없음.**

#### `pylon` (launch / `runLaunch` → `generateClaudeDir`)
1. pylon-owned 리소스 리컨사일(기존) — `.pylon/reference/` 포함.
2. CLAUDE.md 포인터 기록(결정적, 매번).
3. `agentsMDStale(root)` → true면 부트스트랩 `AGENTS.md` 기록, false면 **건드리지 않음**.
   - **`launch.go:154`의 무조건 CLAUDE.md 덮어쓰기는 제거된다.**
4. `.claude/`(agents 심링크/commands/hooks/settings) 생성(기존).
5. `exec claude`. 세션 첫 턴이 stale/부트스트랩 상태의 AGENTS.md를 저작/재저작.

#### `pylon doctor`
1. 리컨사일 + stale 체크(launch와 동일 로직 공유).
2. stale/누락이면 부트스트랩 `AGENTS.md`를 (재)기록 → 다음 launch가 재저작하도록 하고,
   그 사실을 보고(doctor는 신뢰 가능한 리포팅 채널).
3. doctor 자체는 AI를 호출하지 않는다(세션 내 저작 결정과 일관).

### 제거되는 것
- `buildRootCLAUDEMD` 프로즈 빌더 전체.
- `renderVerificationCommands` / `writeVerifySteps`의 CLAUDE.md 주입 경로.
- 메모리 인덱스 proactive 주입의 CLAUDE.md 경로.
- 이 정보들은 삭제가 아니라 **레퍼런스가 "어디서 찾는지"를 안내**하고 AI가 라이브로
  수집하는 방식으로 이동한다(verify.yml 직접 읽기, `pylon mem list`, 프로젝트 디렉토리 `ls`).

## 파일명 정책

- 백엔드가 `claude-code`이므로 저작 대상은 **AGENTS.md**, 유도 포인터는 **CLAUDE.md**.
- 향후 다른 백엔드가 생기면 동일 메커니즘에서 포인터 파일명만 바꿔 대응한다(지금은 미구현).

## 에러/degradation

- 세션이 비대화형이거나 재저작이 일어나지 않으면 AGENTS.md는 부트스트랩 상태로 남는다.
  부트스트랩 자체가 `.pylon/reference/pylon-usage.md` 포인터 + 워크스페이스 팩트를 담아
  **그 자체로 최소 기능**한다. 하드 실패 없음.
- `init`은 네트워크/auth와 무관하게 항상 성공한다(AI 미호출).

## 테스트 계획

- `launch_claudemd_test.go` 재작성:
  - `buildBootstrapAgentsMD`: 버전 스탬프·핵심 지시문·프로젝트 포인터 포함 검증.
  - `buildClaudeMDPointer`: AGENTS.md 참조 문구 포함 검증(결정적).
  - `agentsMDStale`: 누락/저버전/스탬프없음 → stale, 최신 → not stale.
- `launch` 통합 테스트: 최신 AGENTS.md가 있으면 **덮어쓰지 않음**을, 누락/stale이면
  부트스트랩이 기록됨을 검증.
- `doctor` 테스트: stale 감지 시 부트스트랩 재기록 + 보고.
- `syncPylonResources`: `.pylon/reference/pylon-usage.md`가 임베디드 버전으로 갱신됨을 검증.

## 영향 범위(코드)

- `internal/cli/launch_claudemd.go` — 대체(부트스트랩+포인터+stale 헬퍼).
- `internal/cli/launch.go` — `generateClaudeDir`의 CLAUDE.md 무조건 덮어쓰기 제거,
  포인터/stale-gated AGENTS.md 기록으로 교체. `.gitignore` 항목에 `AGENTS.md` 추가.
- `internal/cli/init_cmd.go` — reference 임베드 기록 + 부트스트랩/포인터 기록.
- `internal/cli/doctor.go` — stale 체크·부트스트랩 재기록·보고. `syncPylonResources`에
  `reference/` 편입.
- `internal/cli/reference/pylon-usage.md` — 신규 임베디드 매뉴얼.
- `internal/layout/` — 필요 시 `AGENTS.md` / `.pylon/reference/` 경로 헬퍼 추가.

## 미해결/후속

- CLAUDE.md의 `@AGENTS.md` import 마커가 AGENTS.md를 그대로 끌어오므로 Claude Code의
  AGENTS.md 네이티브 자동 로드 여부와 무관하게 v1이 성립한다. 구현 시 `@AGENTS.md` import가
  실제로 동작하는지(상대경로 해석 포함) 통합 검증 스텝에서 확인한다.
