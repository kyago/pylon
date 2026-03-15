# Discord Bot 연동 의사결정 문서

> 작성일: 2026-03-09
> 상태: 의사결정 완료 / 구현 전 (미결사항 존재)

## 개요

Pylon에 Discord Bot을 연동하여, 사용자가 Discord 메신저를 통해 로컬 머신의 Claude Code 세션과 대화할 수 있도록 한다.

## 의사결정 요약

| # | 항목 | 결정 | 비고 |
|---|------|------|------|
| 1 | 아키텍처 | 로컬 전용 | 터널링 불필요 |
| 2 | Claude Code 연동 | CLI subprocess (`claude -p`) | 공식 Go SDK 없음 |
| 3 | 세션 모델 | Discord Thread 1:1 매핑 | 스레드 연속성 UX 포함 |
| 4 | 보안 모델 | 허용 채널 / permissionMode / 자유 작업 디렉토리 | 보안 보완 필요 |
| 5 | 통합 범위 | 완전 통합 (Pipeline/Orchestrator 포함) | tmux 추상화 선행 필요 |

---

## 의사결정 1: 아키텍처 — 로컬 전용

### 결정

Discord Bot을 로컬 머신에서 직접 실행한다. 클라우드 서버나 터널링을 사용하지 않는다.

### 근거

- Discord Bot은 Discord Gateway에 **아웃바운드 WebSocket** 연결을 사용하므로 인바운드 포트 오픈이 불필요
- NAT/방화벽 뒤에서도 정상 동작
- Pylon 기존 인프라(SQLite, 메모리, 파일시스템)와 같은 머신에서 직접 접근 가능
- 보안 위험 최소화 (로컬 머신 미노출)

### 구조

```
Discord Gateway (cloud) <==WebSocket(아웃바운드)==> [로컬 머신]
                                                     ├── Discord Bot (discordgo)
                                                     ├── Claude Code (subprocess)
                                                     └── Pylon (Orchestrator, Store, Memory)
```

### 제약

- 로컬 머신 상시 구동 필요 (launchd/systemd 서비스 등록으로 완화 가능)

---

## 의사결정 2: Claude Code 연동 — CLI subprocess

### 결정

`os/exec`로 `claude -p --output-format stream-json`을 subprocess로 실행하고, stdout JSON을 파싱하여 통신한다.

### 근거

- Claude Code Agent SDK 공식 지원: TypeScript, Python만 존재. **Go 공식 SDK 없음**
- Go 기반 메신저 연동 프로젝트들(cc-connect 등)이 동일한 CLI subprocess 방식을 사용하여 검증됨
- 비공식 Go SDK도 내부적으로 CLI subprocess를 래핑한 것에 불과
- cc-connect가 이 방식으로 8개 메신저를 지원 중

### 통신 프로토콜

```bash
# 새 세션
claude -p --output-format stream-json --prompt "메시지" --permission-mode <mode>

# 세션 재개
claude -p --output-format stream-json --resume <session_id> --prompt "메시지"
```

- 출력: NDJSON 스트리밍 (message type별: assistant, tool_use, result 등)
- session_id: 첫 응답에서 추출하여 저장

### 참고 프로젝트

| 프로젝트 | 언어 | 방식 | 지원 메신저 |
|----------|------|------|-------------|
| [cc-connect](https://github.com/chenhg5/cc-connect) | Go | CLI subprocess | Discord, Telegram, Slack 등 8개 |
| [praktor](https://github.com/mtzanidakis/praktor) | Go | Docker + SDK | Telegram |
| [claude-code-discord](https://github.com/zebbern/claude-code-discord) | TypeScript | 공식 TS SDK | Discord |

---

## 의사결정 3: 세션 모델 — Thread 1:1 매핑

### 결정

`/claude` 명령 실행 시 Discord Thread를 생성하고, Thread와 Claude Code Session을 1:1로 매핑한다. 스레드가 길어지면 새 스레드로 이어갈 수 있는 UX를 제공한다.

### 동작 흐름

```
1. 사용자: /claude "로그인 기능 구현해줘"
2. Bot: Discord Thread 생성 ("로그인 기능 구현")
3. Bot: claude -p --prompt "..." → session_id 획득
4. Store: thread_id ↔ session_id 매핑 저장

--- 후속 대화 ---
5. 사용자: Thread 내에서 "JWT로 해줘"
6. Bot: claude -p --resume <session_id> --prompt "JWT로 해줘"
7. Bot: 응답을 같은 Thread에 전송

--- 스레드 연속 ---
8. 스레드가 길어지면 "새 스레드로 이어가기" 버튼/명령 제공
9. 새 Thread 생성 + 이전 세션 컨텍스트 요약 주입 또는 --resume 유지
```

### 세션 저장

```
SQLite: discord_sessions 테이블
├── thread_id (Discord Thread ID)
├── session_id (Claude Code Session ID)
├── channel_id (원본 채널)
├── work_dir (작업 디렉토리)
├── pipeline_id (연결된 파이프라인, nullable)
├── created_at
└── last_active_at
```

---

## 의사결정 4: 보안 모델

### 결정

| 항목 | 결정 | 설명 |
|------|------|------|
| 접근 제어 | 허용 채널 제한 | config에 명시된 채널에서만 `/claude` 사용 가능 |
| 도구 제한 | permissionMode 설정 | default / acceptEdits / bypassPermissions 중 선택 |
| 작업 디렉토리 | 자유 지정 | 사용자가 요청 시 workDir 지정 가능 |

### 설정 예시 (`.pylon/config.yml` 확장)

```yaml
discord:
  token: "${DISCORD_BOT_TOKEN}"
  allowed_channels:
    - "1234567890"
    - "0987654321"
  permission_mode: "acceptEdits"
```

---

## 의사결정 5: 통합 범위 — 완전 통합

### 결정

Discord에서 들어온 요청도 기존 Pylon 파이프라인(8단계)을 타고, Orchestrator/Store/Memory/Git/Protocol을 모두 공유한다.

### 공유 범위

| 패키지 | 공유 방식 |
|--------|-----------|
| `orchestrator` | 파이프라인 실행 & 상태 관리 |
| `store` | SQLite 저장소 (+ discord_sessions 테이블 추가) |
| `memory` | BM25 검색 기반 프로젝트 메모리 |
| `git` | 브랜치/PR 생성 |
| `config` | 설정 로드 (Discord 섹션 추가) |
| `protocol` | 에이전트 간 통신 (inbox/outbox) |

### 신규 패키지

```
internal/discord/
├── bot.go          # discordgo 봇 초기화/실행
├── handler.go      # 메시지/슬래시커맨드 핸들러
├── session.go      # Discord Thread ↔ Claude Code 세션 매핑
├── executor.go     # Claude Code subprocess 실행/스트리밍
├── response.go     # 응답 포맷팅/청킹 (2000자 제한 대응)
└── security.go     # 허용 채널 검증
```

### CLI 확장

```bash
pylon discord    # Discord Bot 실행
```

---

## 충돌 분석 결과

코드 기반 검증에서 발견된 충돌 및 보안 갭.

### 충돌 1: subprocess × 완전 통합 — 심각도 높음

**문제**: Orchestrator/Runner/Lifecycle이 tmux에 하드 커플링되어 있어, subprocess 방식의 에이전트를 관리할 수 없음.

**충돌 지점**:

| 위치 | 문제 |
|------|------|
| `orchestrator.go:20` | `Orchestrator.Tmux` 필드가 `tmux.SessionManager` 고정 |
| `runner.go:76` | `Runner.Start()`가 `r.Tmux.Create()` 호출 |
| `pipeline.go:43` | `AgentStatus.TmuxSession` 필드 |
| `lifecycle.go:33` | `Lifecycle.TmuxSession` 필드 |
| `orchestrator.go:109` | `Recover()`가 tmux 세션 존재 여부로 에이전트 생존 판단 |

**해결 방안**: 에이전트 실행 계층을 인터페이스로 추상화

```
ProcessManager (interface)
├── TmuxProcessManager   (기존: tmux 세션 기반)
└── SubprocessManager    (신규: os/exec 기반)
```

`AgentStatus.TmuxSession` → `AgentStatus.ProcessHandle` (범용 필드)로 변경.

### 충돌 2: Thread 1:1 × 완전 통합 — 심각도 중간

**문제**: Thread = Session 1:1 매핑이 8단계 멀티에이전트 파이프라인과 개념 불일치.

- 하나의 파이프라인에서 PO, Architect, PM, Dev 등 복수 에이전트가 복수 세션을 생성
- 단순 질문("이 함수 설명해줘")도 8단계를 거쳐야 하는 오버헤드

**해결 방안**:

- Thread = Pipeline (1:1), Session은 파이프라인 내 단계별로 복수 생성
- 라우팅 분기:
  - `/ask "질문"` → 단일 subprocess, 파이프라인 없이 직접 응답
  - `/request "요구사항"` → 풀 파이프라인 실행

### 보안 갭: 자유 작업 디렉토리 — 심각도 낮음

**문제**: 허용 채널 제한은 "누가"만 제어하고, "어디서"는 제어하지 않음. `workDir: /etc` 같은 위험 경로 차단 수단 없음.

**권장 보완**: 허용 디렉토리 화이트리스트(`allowed_work_dirs`) 설정 추가.

---

## 미결사항

### 선행 작업 (구현 전 해결 필수)

| # | 항목 | 설명 | 관련 충돌 |
|---|------|------|-----------|
| P-1 | tmux 추상화 리팩토링 | Orchestrator/Runner/Lifecycle의 tmux 의존성을 ProcessManager 인터페이스로 추상화 | 충돌 1 |
| P-2 | 라우팅 분기 설계 | 단순 질문(`/ask`) vs 파이프라인 실행(`/request`) 분리 기준 및 UX 확정 | 충돌 2 |

### 설계 미확정

| # | 항목 | 설명 |
|---|------|------|
| D-1 | 스레드 연속성 UX | 스레드가 길어질 때 새 스레드로 이어가는 구체적인 인터랙션 설계 (버튼? 명령어? 자동?) |
| D-2 | 서브 에이전트 결과 전달 | 서브 에이전트(Architect, PM, Dev) 결과를 Discord Thread에 어떻게 표시할지 |
| D-3 | 파이프라인 진행 상황 표시 | 8단계 파이프라인 진행 중 Discord에서 상태를 어떻게 시각화할지 |
| D-4 | 동시 세션 수 제한 | 로컬 리소스 보호를 위한 최대 동시 subprocess 수 설정 |
| D-5 | 작업 디렉토리 화이트리스트 | 보안 갭 보완을 위한 허용 디렉토리 설정 여부 |

### 기술 검증 필요

| # | 항목 | 설명 |
|---|------|------|
| T-1 | `claude -p --resume` 동작 검증 | subprocess 종료 후 resume으로 세션 재개가 정상 동작하는지 로컬 테스트 |
| T-2 | stream-json 파싱 | NDJSON 스트리밍 출력에서 session_id, 응답 텍스트, 비용 정보 추출 검증 |
| T-3 | discordgo Thread API | Thread 생성, 메시지 전송, 버튼 인터랙션 등 Discord API 제약 확인 |
| T-4 | 동시성 안전성 | 같은 Thread에서 이전 subprocess 실행 중 새 메시지 도착 시 처리 방식 |
