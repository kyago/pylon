# Pylon 스펙 보완 연구: 에이전트 간 통신 프로토콜 & 컨텍스트 메모리 처리

**연구일**: 2026-03-06
**목적**: Pylon AI 멀티에이전트 오케스트레이터의 통신 프로토콜과 컨텍스트 메모리 관리 스펙 보완을 위한 심층 조사

---

## 1. 아티클 핵심 요약

### 참조 아티클: QMD 기반 Claude Code 메모리 시스템

참조된 아티클은 **QMD(로컬 검색 엔진)를 활용한 Claude Code 세션 메모리 시스템**에 관한 것으로, 멀티에이전트 통신 프로토콜 자체보다는 **에이전트의 세션 간 컨텍스트 유실 문제 해결**에 초점을 맞추고 있다.

**핵심 내용**:

- **문제 정의**: AI 코딩 에이전트(Claude Code)가 세션이 종료되면 이전 작업 컨텍스트를 완전히 잃어버림. 700개 이상의 세션에서 축적된 지식이 매번 사라지는 "콜드 스타트" 문제
- **해결 방식**: JSONL 형식의 세션 트랜스크립트를 마크다운으로 변환 후 QMD로 인덱싱
- **검색 전략**: BM25 풀텍스트 검색(구조화된 데이터의 80% 처리) + 시맨틱 벡터 검색(비정형 데이터) 하이브리드 방식
- **검색 모드**: 시간 기반(temporal), 주제 기반(topic), 그래프 시각화(graph)
- **자동화**: 터미널 종료 시 훅으로 세션 export + embed 자동 처리
- **도구 독립적 설계**: "컨텍스트만 유지하면" Claude Code, Codex, Gemini CLI 등 어디서든 작동

**Pylon에 주는 시사점**:
1. 에이전트의 세션 메모리를 **구조화하고 검색 가능**하게 만드는 것이 핵심
2. **하이브리드 검색**(BM25 + 벡터)이 단일 검색 방식보다 효과적
3. **도구 독립적 설계** 원칙 -- 메모리 시스템이 특정 LLM에 종속되지 않아야 함
4. **자동 인덱싱** -- 수동 개입 없이 세션 종료 시 자동으로 지식 축적

---

## 2. 통신 프로토콜 분석

### 2.1 현재 Pylon 방식 분석

**현재 아키텍처**:
```
에이전트 A → outbox/msg.json (tmp→mv 원자적 쓰기)
                    ↓
            fsnotify 감지
                    ↓
          오케스트레이터 → SQLite 기록
                    ↓
          에이전트 B의 inbox/msg.json 전달
```

**장점**:
- 단순성: 파일 I/O만으로 동작하여 구현/디버깅이 쉬움
- 안정성: tmp→mv 원자적 쓰기로 메시지 손실 방지
- 복구 가능성: state.json + outbox 스캔으로 SPOF 복구
- 에이전트 독립성: 에이전트는 파일만 읽고 쓰면 됨 (LLM 종류 무관)
- tmux 세션 생존: 오케스트레이터 크래시 시에도 에이전트 프로세스 유지

**약점**:
- 비효율적 폴링: fsnotify는 이벤트 기반이지만, 파일 시스템 의존으로 레이턴시 존재
- 메시지 구조 미정의: 메시지 스키마/프로토콜이 느슨하여 확장성 제한
- 브로드캐스트 부재: 1:N 통신 시 N개 파일을 개별 생성해야 함
- 우선순위 미지원: 긴급 메시지와 일반 메시지 구분 불가
- 메시지 이력 관리: SQLite에 기록하지만 체계적인 쿼리/분석 인터페이스 부재
- 컨텍스트 전달 부족: 메시지와 함께 관련 컨텍스트를 전달하는 메커니즘 미비

### 2.2 업계 표준 프로토콜 비교

2025-2026년 기준으로 에이전트 간 통신을 위한 4대 표준 프로토콜이 부상했다:

| 특성 | MCP | ACP | A2A | ANP |
|------|-----|-----|-----|-----|
| **아키텍처** | 클라이언트-서버 (JSON-RPC) | 레지스트리 브로커 (REST) | P2P (Agent Card) | 완전 분산 (DID) |
| **메시지 형식** | JSON-RPC 2.0 | 멀티파트 MIME | Task/Artifact JSON | JSON-LD |
| **전송 방식** | HTTP, Stdio, SSE | HTTP + 스트리밍 | HTTP + SSE | HTTPS + TLS |
| **디스커버리** | 수동/정적 URL | 중앙 레지스트리 API | Agent Card 검색 | 검색엔진/크롤링 |
| **세션 모델** | 무상태 + 선택적 컨텍스트 | 세션 인식 (상태 추적) | 클라이언트 관리 ID | 무상태 (DID 인증) |
| **적합 시나리오** | LLM-도구 통합 | 다양한 에이전트 상호작용 | 기업 내 워크플로우 | 조직 간 협업 |

**Pylon과의 관계**:
- Pylon은 단일 머신에서 동작하므로 ANP(인터넷 스케일)나 A2A(기업 간)는 과도함
- ACP의 레지스트리 개념은 Pylon의 에이전트 등록/발견에 참고 가능
- MCP는 이미 Claude Code가 사용하는 프로토콜이므로, Pylon이 MCP 서버로서 에이전트에게 도구를 제공하는 패턴이 자연스러움

### 2.3 멀티에이전트 프레임워크 통신 방식 비교

| 프레임워크 | 통신 패러다임 | 상태 관리 | 에이전트 관계 |
|-----------|-------------|----------|-------------|
| **CrewAI** | 역할 기반 구조적 통신 | Crews(동적) + Flows(이벤트) | 팀원-팀장 계층 |
| **LangGraph** | 그래프 노드 간 상태 전달 | 공유 상태 그래프 | 노드-엣지 연결 |
| **AutoGen** | 대화 기반 (메시지 교환) | 비동기 이벤트 + 토픽 구독 | P2P 대화 |
| **OpenAI Swarm/SDK** | 핸드오프 함수 기반 | 무상태 (클라이언트 관리) | 전환 체인 |
| **Google ADK** | 세션 상태 공유 + 핸드오프 | 계층적 (Working/Session/Memory) | 부모-자식 위임 |
| **Pylon** | 파일 기반 inbox/outbox | SQLite + state.json | 오케스트레이터 중심 허브 |

**핵심 인사이트**:

1. **Google ADK의 "Narrative Casting"**: 에이전트 핸드오프 시 이전 에이전트의 assistant 메시지를 "컨텍스트"로 재구성하여 새 에이전트의 혼란 방지. Pylon에서 태스크 위임 시 적용 가치 높음.

2. **AutoGen의 토픽 구독 모델**: 에이전트가 관심 토픽을 구독하고 해당 이벤트에만 반응. Pylon의 현재 1:1 메시징보다 유연한 pub/sub 패턴.

3. **OpenAI Swarm의 경량 핸드오프**: 명시적 `transfer_to_agent()` 함수로 제어권 이전. 단순하면서도 추적 가능. Pylon의 태스크 위임에 적합.

### 2.4 블랙보드 패턴 -- Pylon에 가장 적합한 패턴

블랙보드(Blackboard) 패턴은 Pylon의 현재 파일 기반 방식과 **철학적으로 가장 유사**하면서도 체계적으로 발전된 형태다:

```
┌─────────────────────────────────────────────┐
│              블랙보드 (공유 지식 저장소)        │
│  ┌──────────┬──────────┬──────────┐         │
│  │ 가설     │ 증거     │ 결과     │          │
│  │ (초기 분석)│ (데이터) │ (검증됨) │          │
│  └──────────┴──────────┴──────────┘         │
│  ┌──────────┬──────────┐                    │
│  │ 제약조건  │ 태스크   │                    │
│  │ (규칙)   │ (진행상태)│                    │
│  └──────────┴──────────┘                    │
└────────┬──────────┬──────────┬──────────────┘
         │          │          │
    ┌────▼───┐ ┌───▼────┐ ┌──▼─────┐
    │Agent A │ │Agent B │ │Agent C │
    │(읽기/쓰기)│(읽기/쓰기)│(읽기/쓰기)│
    └────────┘ └────────┘ └────────┘
```

**블랙보드 패턴의 핵심 구성요소**:
- **공유 상태(Blackboard)**: 모든 발견, 가설, 중간 결과를 저장하는 중앙 저장소
- **지식 소스(Agents)**: 독립적으로 블랙보드를 모니터링하고 기여
- **제어 컴포넌트**: 어떤 에이전트가 다음에 행동할지 결정 (Pylon 오케스트레이터 역할)

**Pylon 적용 시 이점**:
- 에이전트 간 직접 통신 없이 비동기 협업 (현재 방식과 호환)
- 동적 에이전트 참여/이탈 (에이전트 확장성)
- 자연스러운 병렬 처리
- 감사 추적(audit trail) 용이

**구현 방식 옵션**:
1. **인메모리**: 빠르지만 비영속적 (단일 세션용)
2. **데이터베이스 기반**: SQLite로 영속적 (Pylon의 기존 SQLite와 연계)
3. **이벤트 소싱**: 모든 쓰기가 append-only 이벤트 (완전한 이력 + 리플레이)

---

## 3. 컨텍스트/메모리 분석

### 3.1 현재 Pylon에 부재한 메모리 관리 전략

현재 Pylon 스펙에서 **명시적으로 정의되지 않은** 메모리 관련 영역:

1. **에이전트별 컨텍스트 윈도우 관리**: 에이전트의 Claude Code 세션이 길어질 때 컨텍스트 윈도우를 어떻게 관리할지 미정의
2. **장기 메모리 저장소**: 프로젝트 수준의 학습/결정 이력 저장 메커니즘 부재
3. **태스크 간 지식 전이**: 한 태스크에서 얻은 인사이트를 다른 태스크로 전달하는 방법 미정의
4. **공유 컨텍스트**: 여러 에이전트가 공유해야 하는 프로젝트 지식 관리 미정의
5. **컨텍스트 압축/요약**: 긴 대화 이력을 압축하여 핵심만 유지하는 전략 부재

### 3.2 업계 메모리 아키텍처 분석

#### Anthropic의 4가지 컨텍스트 관리 전략

Anthropic이 제시한 에이전트 컨텍스트 엔지니어링 4대 전략:

| 전략 | 설명 | Pylon 적용 |
|------|------|-----------|
| **Write** | 에이전트가 스스로 메모를 작성하여 외부 저장소에 유지 | NOTES.md 패턴, 에이전트 자체 메모리 파일 |
| **Select** | 필요한 컨텍스트만 선별적으로 로드 | 태스크별 관련 컨텍스트만 주입 |
| **Compress** | 오래된 대화를 요약하여 재시작 | Compaction -- 긴 세션을 요약으로 압축 |
| **Isolate** | 하위 에이전트에게 깨끗한 컨텍스트 제공 | 태스크별 격리된 컨텍스트 윈도우 |

**핵심 원칙**: "가장 적은 수의 고신호(high-signal) 토큰으로 원하는 결과의 가능성을 최대화"

#### Google ADK의 4계층 컨텍스트 모델

```
┌─────────────────────────────────────────┐
│ Working Context (작업 컨텍스트)           │
│ = 각 LLM 호출 시 실제 전달되는 프롬프트   │
│ = 매 호출마다 재구성                      │
├─────────────────────────────────────────┤
│ Session (세션)                           │
│ = 이벤트의 시간순 목록                    │
│ = 단일 상호작용의 전체 기록               │
├─────────────────────────────────────────┤
│ Memory (메모리)                          │
│ = 세션을 넘어서는 장기 검색 가능 지식      │
│ = 학습된 선호도, 패턴, 결정 이력          │
├─────────────────────────────────────────┤
│ Artifacts (아티팩트)                      │
│ = 이름 있는 버전 관리 바이너리/텍스트 객체  │
│ = 프롬프트와 별도 관리                    │
└─────────────────────────────────────────┘
```

**핵심 인사이트**: 컨텍스트를 "풍부한 상태 시스템 위의 컴파일된 뷰"로 취급. 모든 것을 프롬프트에 넣지 않고, 계층별로 분리하여 필요 시 조합.

#### 메모리 접근 방식

| 방식 | 설명 | 적용 시나리오 |
|------|------|-------------|
| **반응적 회상 (Reactive)** | 에이전트가 지식 부족을 인식하고 명시적으로 도구를 호출하여 검색 | 알려지지 않은 API, 이전 결정 확인 |
| **선제적 회상 (Proactive)** | 전처리기가 입력 기반 유사도 검색을 실행하여 관련 정보를 사전 주입 | 관련 과거 세션, 유사 태스크 결과 |

### 3.3 Compaction (압축) 전략 상세

컨텍스트 윈도우가 한계에 가까워질 때의 핵심 전략:

**1. 대화 압축 (Conversation Compaction)**
```
원본 대화 (10,000 토큰)
    ↓ 요약 LLM 호출
압축된 요약 (1,500 토큰)
    ↓ 새 컨텍스트 윈도우로 재시작
에이전트 계속 작업 (성능 저하 최소)
```

Claude Code의 실제 구현: 아키텍처 결정, 미해결 버그, 구현 세부사항은 보존하고, 중복 도구 출력이나 메시지는 제거.

**2. 도구 결과 클리어링 (Tool Result Clearing)**
- 가장 가벼운 형태의 압축
- 처리 완료된 도구 출력을 과거 메시지 이력에서 제거
- 원시 출력은 처리 후 불필요하므로 삭제

**3. 비동기 압축 (Asynchronous Compaction)**
- 임계값 도달 시 비동기 프로세스가 이전 이벤트를 요약
- 요약을 새로운 세션 이벤트로 기록
- 매우 장시간 실행되는 대화도 물리적으로 관리 가능

### 3.4 에이전트 간 컨텍스트 전달 패턴

**Google ADK의 "Narrative Casting" 패턴**:

에이전트 핸드오프 시:
1. 이전 에이전트의 assistant 메시지를 "서사적 문맥"으로 재구성
2. 도구 호출을 기여 표시하여 새 에이전트의 혼란 방지
3. 새 에이전트의 관점에서 신선한 Working Context 구축
4. 세션 이력은 보존

```
에이전트 A 완료 → 핸드오프 트리거
    ↓
Narrative Casting: A의 출력을 "컨텍스트"로 변환
    ↓
에이전트 B에게 전달:
  - A의 결과 (재구성된 컨텍스트)
  - 태스크 설명
  - 필요한 도구/리소스 목록
  - B의 시스템 프롬프트
```

**Agno 프레임워크의 선택적 컨텍스트 패턴**:
- `add_history_to_context`: 선택적 이력 주입 (기본 off)
- `add_memories_to_context`: 선택적 메모리 주입 (기본 off)
- `num_history_runs=3`: 최근 N개 턴만 유지
- "에이전트가 실제로 알아야 하는 것만 선택적으로 추가"

### 3.5 QMD 스타일 세션 메모리 시스템

참조 아티클의 QMD 메모리 시스템이 Pylon에 주는 구체적 시사점:

**1. 세션 트랜스크립트 → 검색 가능 지식**
```
Claude Code 세션 JSONL
    ↓ 자동 변환
마크다운 파일 (프로젝트/날짜-슬러그-id.md)
    ↓ 인덱싱
BM25 풀텍스트 인덱스 + 벡터 임베딩
    ↓ 검색
시간 기반 / 주제 기반 / 그래프 시각화
```

**2. 하이브리드 검색의 효과**: 구조화된 노트의 80%는 BM25(키워드 매칭)로 처리하고, 나머지 비정형 데이터는 시맨틱 검색으로 보완

**3. 자동화 파이프라인**: 세션 종료 시 자동으로 export → embed → index. 수동 개입 없이 지식 축적

---

## 4. Pylon 스펙 보완 제안

### 4.1 통신 프로토콜 개선안

#### P1 (최우선): 구조화된 메시지 스키마 정의

현재 inbox/outbox JSON 파일의 스키마를 명확하게 정의:

```go
// MessageEnvelope - 모든 에이전트 간 메시지의 공통 봉투
type MessageEnvelope struct {
    // 헤더
    ID          string    `json:"id"`           // UUID v7 (시간순 정렬 가능)
    Type        MsgType   `json:"type"`         // task_assign, result, query, broadcast, heartbeat
    Priority    Priority  `json:"priority"`     // critical, high, normal, low
    From        AgentID   `json:"from"`         // 발신 에이전트
    To          AgentID   `json:"to"`           // 수신 에이전트 ("*" = 브로드캐스트)
    ReplyTo     string    `json:"reply_to"`     // 원본 메시지 ID (응답 시)
    Timestamp   time.Time `json:"timestamp"`

    // 본문
    Subject     string    `json:"subject"`      // 메시지 제목/요약
    Body        any       `json:"body"`         // 타입별 구조화된 본문

    // 컨텍스트
    Context     *MsgContext `json:"context,omitempty"` // 관련 컨텍스트
    Attachments []Artifact  `json:"attachments,omitempty"` // 첨부 아티팩트

    // 메타
    TTL         Duration  `json:"ttl,omitempty"` // 메시지 유효 기간
    Trace       []string  `json:"trace"`        // 메시지 전달 경로 추적
}

type MsgContext struct {
    TaskID      string            `json:"task_id,omitempty"`
    ProjectID   string            `json:"project_id,omitempty"`
    References  []string          `json:"references,omitempty"`  // 관련 파일/문서
    Summary     string            `json:"summary,omitempty"`     // 이전 대화 요약
    Decisions   []Decision        `json:"decisions,omitempty"`   // 관련 결정 이력
    Constraints []string          `json:"constraints,omitempty"` // 제약 조건
}
```

**이유**: 현재 느슨한 JSON 구조를 표준화하면 메시지 라우팅, 우선순위 처리, 컨텍스트 전달, 추적이 모두 체계화됨.

#### P1: SQLite 메시지 큐 고도화

현재 SQLite를 단순 기록용에서 **메시지 큐** 역할로 강화:

```sql
CREATE TABLE message_queue (
    id          TEXT PRIMARY KEY,     -- UUID v7
    type        TEXT NOT NULL,
    priority    INTEGER DEFAULT 2,    -- 0=critical, 1=high, 2=normal, 3=low
    from_agent  TEXT NOT NULL,
    to_agent    TEXT NOT NULL,
    subject     TEXT,
    body        TEXT,                 -- JSON
    context     TEXT,                 -- JSON (MsgContext)
    status      TEXT DEFAULT 'queued', -- queued, delivered, acked, failed, expired
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    delivered_at DATETIME,
    acked_at    DATETIME,
    ttl_seconds INTEGER,
    reply_to    TEXT,
    trace       TEXT                  -- JSON array
);

CREATE INDEX idx_mq_to_status ON message_queue(to_agent, status);
CREATE INDEX idx_mq_priority ON message_queue(priority, created_at);
CREATE INDEX idx_mq_task ON message_queue(json_extract(context, '$.task_id'));
```

**Go 라이브러리 참고**: [goqite](https://github.com/maragudk/goqite) -- SQLite 기반 Go 메시지 큐 라이브러리 (AWS SQS 영감, 18,500 msg/s 처리)

**이점**:
- 메시지 전달 보장 (ack 기반)
- 우선순위 큐잉 (critical 메시지 우선 처리)
- 메시지 만료 (TTL)
- 전달 이력 추적
- 기존 SQLite 인프라 재활용

#### P2: 블랙보드 패턴 레이어 도입

기존 inbox/outbox 위에 **프로젝트 블랙보드**를 추가:

```sql
CREATE TABLE blackboard (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    category    TEXT NOT NULL,        -- hypothesis, evidence, decision, constraint, result
    key         TEXT NOT NULL,
    value       TEXT,                 -- JSON
    confidence  REAL DEFAULT 0.5,    -- 0.0 ~ 1.0
    author      TEXT NOT NULL,        -- 작성 에이전트
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME,
    superseded_by TEXT,               -- 이후 업데이트된 항목 ID

    UNIQUE(project_id, category, key)
);

CREATE TABLE blackboard_subscriptions (
    agent_id    TEXT NOT NULL,
    project_id  TEXT NOT NULL,
    category    TEXT NOT NULL,         -- 구독할 카테고리
    PRIMARY KEY (agent_id, project_id, category)
);
```

**사용 시나리오**:
- Architect가 기술 결정을 블랙보드에 기록 → 프로젝트 에이전트가 자동으로 인식
- QA 에이전트가 발견한 버그를 블랙보드에 게시 → 관련 개발 에이전트에게 알림
- PM이 우선순위 변경을 블랙보드에 반영 → 모든 에이전트에게 전파

#### P2: 토픽 기반 구독 (Pub/Sub) 패턴

AutoGen의 토픽 구독 모델을 참고하여 추가:

```go
type TopicSubscription struct {
    AgentID   string   `json:"agent_id"`
    Topics    []string `json:"topics"`     // e.g., ["task.assigned", "decision.architecture", "bug.critical"]
    Filter    string   `json:"filter"`     // 선택적 필터 (예: project_id = "abc")
}

// 오케스트레이터가 관리하는 토픽 라우터
type TopicRouter struct {
    subscriptions map[string][]TopicSubscription // topic -> subscribers
}
```

**토픽 계층 예시**:
```
task.*                    -- 모든 태스크 이벤트
task.assigned             -- 태스크 할당
task.completed            -- 태스크 완료
decision.*                -- 모든 결정
decision.architecture     -- 아키텍처 결정
decision.requirement      -- 요구사항 결정
bug.*                     -- 버그 관련
code.review.requested     -- 코드 리뷰 요청
```

#### P3: 핸드오프 프로토콜 (Narrative Casting)

Google ADK의 패턴을 참고한 태스크 위임 시 컨텍스트 핸드오프:

```go
type TaskHandoff struct {
    TaskID          string          `json:"task_id"`
    FromAgent       AgentID         `json:"from_agent"`
    ToAgent         AgentID         `json:"to_agent"`

    // Narrative Casting
    NarrativeContext string         `json:"narrative_context"`  // 이전 작업의 서사적 요약
    KeyDecisions    []Decision      `json:"key_decisions"`      // 핵심 결정 사항
    UnresolvedIssues []string       `json:"unresolved_issues"`  // 미해결 이슈
    RelevantFiles   []string        `json:"relevant_files"`     // 관련 파일 목록
    Constraints     []string        `json:"constraints"`        // 제약 조건

    // 명시적으로 포함하지 않을 것
    ExcludeRawLogs  bool            `json:"exclude_raw_logs"`   // 원시 로그 제외
}
```

### 4.2 컨텍스트/메모리 시스템 신규 스펙

#### P1: 3계층 메모리 아키텍처

Google ADK와 Anthropic의 모델을 Pylon에 맞게 재설계:

```
┌──────────────────────────────────────────────────┐
│          Layer 1: Working Context (작업 메모리)     │
│  = 에이전트의 현재 Claude Code 세션 컨텍스트 윈도우  │
│  = 매 LLM 호출마다 재구성                          │
│  = 관리: 에이전트 자체 (Claude Code 내장)           │
│  = 수명: 단일 LLM 호출                             │
├──────────────────────────────────────────────────┤
│          Layer 2: Session Memory (세션 메모리)      │
│  = 현재 태스크 실행 동안의 대화/결정 이력            │
│  = Compaction 전략으로 압축 관리                    │
│  = 관리: 오케스트레이터 + 에이전트 협력              │
│  = 수명: 태스크 완료까지                            │
├──────────────────────────────────────────────────┤
│          Layer 3: Project Memory (프로젝트 메모리)   │
│  = 프로젝트 수준의 장기 지식 저장소                  │
│  = 아키텍처 결정, 학습된 패턴, 코드베이스 이해       │
│  = 관리: 오케스트레이터 (SQLite + 인덱스)           │
│  = 수명: 프로젝트 전체                              │
└──────────────────────────────────────────────────┘
```

#### P1: Compaction 전략 구현

에이전트의 컨텍스트 윈도우가 한계에 도달하기 전에 자동 압축:

```go
type CompactionConfig struct {
    // 트리거 조건
    TokenThreshold    int     `json:"token_threshold"`     // 압축 시작 토큰 수 (예: 150,000)
    TokenLimit        int     `json:"token_limit"`         // 최대 허용 토큰 (예: 200,000)
    MessageCountLimit int     `json:"message_count_limit"` // 최대 메시지 수

    // 압축 전략
    Strategy          string  `json:"strategy"`            // "summary", "selective", "hybrid"

    // 보존 규칙
    PreserveDecisions bool    `json:"preserve_decisions"`  // 결정 사항 항상 보존
    PreserveBugs      bool    `json:"preserve_bugs"`       // 미해결 버그 보존
    PreserveRecent    int     `json:"preserve_recent"`     // 최근 N개 턴 보존

    // 제거 대상
    ClearToolOutputs  bool    `json:"clear_tool_outputs"`  // 처리 완료된 도구 출력 제거
    ClearRedundant    bool    `json:"clear_redundant"`     // 중복 정보 제거
}
```

**Compaction 프로세스**:
```
1. 모니터링: 에이전트의 컨텍스트 사용량 추적
2. 트리거: 임계값 도달 시 오케스트레이터에게 알림
3. 추출: 보존해야 할 핵심 정보 식별
   - 아키텍처 결정
   - 미해결 버그/이슈
   - 현재 태스크 상태
   - 제약 조건
4. 요약: LLM을 사용하여 나머지 대화를 압축 요약
5. 저장: 원본을 Session Memory에 아카이브
6. 재시작: 요약 + 보존 정보로 새 컨텍스트 구성
```

#### P2: 프로젝트 메모리 저장소

```sql
-- 프로젝트 수준 장기 메모리
CREATE TABLE project_memory (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    category    TEXT NOT NULL,         -- architecture, pattern, decision, learning, codebase
    key         TEXT NOT NULL,
    content     TEXT NOT NULL,         -- 메모리 내용
    embedding   BLOB,                 -- 벡터 임베딩 (선택적)
    metadata    TEXT,                  -- JSON 메타데이터
    author      TEXT,                  -- 작성 에이전트
    confidence  REAL DEFAULT 0.8,
    access_count INTEGER DEFAULT 0,   -- 접근 횟수 (중요도 지표)
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME,
    expires_at  DATETIME              -- 선택적 만료
);

-- BM25 풀텍스트 검색 인덱스
CREATE VIRTUAL TABLE project_memory_fts USING fts5(
    key, content, category,
    content='project_memory',
    content_rowid='rowid'
);

CREATE INDEX idx_pm_project ON project_memory(project_id, category);
CREATE INDEX idx_pm_access ON project_memory(access_count DESC);
```

**메모리 카테고리**:

| 카테고리 | 설명 | 예시 |
|---------|------|------|
| `architecture` | 아키텍처 결정 및 근거 | "REST API 대신 gRPC 선택: 이유..." |
| `pattern` | 코드 패턴 및 컨벤션 | "에러 핸들링은 sentinel error 패턴 사용" |
| `decision` | 기술/비즈니스 결정 | "PostgreSQL 선택 근거..." |
| `learning` | 실패/성공에서 학습한 교훈 | "테스트 X가 실패한 원인과 해결법" |
| `codebase` | 코드베이스 구조 이해 | "모듈 A와 B의 의존 관계" |
| `requirement` | 요구사항 해석 | "사용자가 말한 X는 Y를 의미" |

#### P2: 에이전트 간 지식 전이 메커니즘

```go
// 태스크 완료 시 자동 지식 추출
type KnowledgeExtraction struct {
    TaskID      string   `json:"task_id"`
    AgentID     string   `json:"agent_id"`

    // 추출된 지식
    Decisions   []Decision   `json:"decisions"`    // 내린 결정들
    Patterns    []Pattern    `json:"patterns"`     // 발견한 패턴
    Learnings   []Learning   `json:"learnings"`    // 학습한 교훈
    Issues      []Issue      `json:"issues"`       // 발견한 이슈

    // 추천 전파 범위
    Scope       string       `json:"scope"`        // "project", "team", "global"
}

// 지식 전이 프로세스
// 1. 태스크 완료 시 에이전트가 KnowledgeExtraction 생성
// 2. 오케스트레이터가 project_memory에 저장
// 3. 관련 에이전트의 다음 태스크 시작 시 관련 메모리 주입
```

#### P2: 2단계 메모리 접근 (Reactive + Proactive)

```go
type MemoryAccess struct {
    // Proactive (선제적): 태스크 시작 시 자동 주입
    ProactiveConfig struct {
        InjectRelevantDecisions bool   // 관련 아키텍처 결정 자동 주입
        InjectRecentLearnings   bool   // 최근 학습 교훈 주입
        InjectCodebaseContext   bool   // 코드베이스 구조 정보 주입
        MaxTokenBudget          int    // 선제적 주입 최대 토큰 (예: 2000)
        SimilarityThreshold     float64 // 관련성 임계값 (예: 0.7)
    }

    // Reactive (반응적): 에이전트가 필요 시 요청
    ReactiveConfig struct {
        SearchEndpoint   string   // 메모리 검색 API
        MaxResults       int      // 최대 검색 결과 수
        AllowedCategories []string // 접근 가능한 카테고리
    }
}
```

**선제적 주입 프로세스**:
```
1. 새 태스크 할당 시
2. 태스크 설명에서 키워드/의도 추출
3. project_memory에서 관련 항목 검색 (BM25 + 벡터)
4. 관련성 점수 > 임계값인 항목 선별
5. 토큰 예산 내에서 에이전트 시스템 프롬프트에 주입
6. 에이전트가 추가 정보 필요 시 reactive 검색 도구 사용
```

#### P3: 세션 메모리 자동 아카이빙

QMD 패턴을 참고한 자동 세션 아카이빙:

```go
type SessionArchiver struct {
    // 아카이브 트리거
    TriggerOnTaskComplete  bool  // 태스크 완료 시
    TriggerOnAgentShutdown bool  // 에이전트 종료 시
    TriggerOnCompaction    bool  // 압축 발생 시

    // 아카이브 내용
    ArchiveFormat          string // "jsonl", "markdown"
    IncludeToolOutputs     bool   // 도구 출력 포함 여부
    IncludeRawMessages     bool   // 원시 메시지 포함 여부

    // 인덱싱
    EnableBM25Index        bool   // BM25 풀텍스트 인덱스
    EnableVectorEmbedding  bool   // 벡터 임베딩 (선택적)
    EmbeddingModel         string // 로컬 임베딩 모델
}
```

### 4.3 통합 아키텍처 제안

```
┌─────────────────────────────────────────────────────────────────┐
│                     Pylon 오케스트레이터                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │
│  │메시지 큐  │  │토픽 라우터│  │블랙보드  │  │메모리 매니저  │   │
│  │(SQLite)  │  │(Pub/Sub) │  │(SQLite)  │  │(검색+주입)   │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬───────┘   │
│       │              │            │                │            │
│  ┌────▼──────────────▼────────────▼────────────────▼─────────┐ │
│  │              통합 SQLite 데이터베이스                        │ │
│  │  messages | topics | blackboard | project_memory | sessions │ │
│  └───────────────────────────────────────────────────────────┘ │
│       │                                                        │
│  ┌────▼─────────────────────────────────────────┐              │
│  │           fsnotify + 파일 I/O 레이어          │              │
│  │    (기존 inbox/outbox 패턴 호환 유지)          │              │
│  └──────┬──────────┬──────────┬─────────────────┘              │
└─────────┼──────────┼──────────┼────────────────────────────────┘
          │          │          │
   ┌──────▼──┐ ┌────▼────┐ ┌──▼──────┐
   │ PO/PM   │ │Architect│ │프로젝트  │
   │에이전트  │ │에이전트  │ │에이전트  │
   │(tmux)   │ │(tmux)   │ │(tmux)   │
   └─────────┘ └─────────┘ └─────────┘
```

**설계 원칙**:
1. **하위 호환**: 기존 파일 기반 inbox/outbox 패턴은 유지하되, 내부적으로 SQLite 큐를 통해 관리
2. **점진적 도입**: 메시지 스키마 → 메시지 큐 → 블랙보드 → Pub/Sub 순서로 단계적 구현
3. **단일 SQLite**: 모든 데이터를 하나의 SQLite DB에 통합 (WAL 모드로 동시 읽기 지원)
4. **에이전트 투명성**: 에이전트는 여전히 파일만 읽고 쓰면 됨 (오케스트레이터가 추상화)

### 4.4 구현 우선순위 로드맵

| 단계 | 항목 | 우선순위 | 복잡도 | 영향도 |
|------|------|---------|--------|--------|
| **Phase 1** | 메시지 스키마 표준화 | P1 | 낮음 | 높음 |
| **Phase 1** | SQLite 메시지 큐 고도화 | P1 | 중간 | 높음 |
| **Phase 1** | Compaction 전략 기본 구현 | P1 | 중간 | 높음 |
| **Phase 1** | 3계층 메모리 아키텍처 기본 구조 | P1 | 중간 | 높음 |
| **Phase 2** | 블랙보드 테이블 + API | P2 | 중간 | 중간 |
| **Phase 2** | 프로젝트 메모리 저장소 + BM25 검색 | P2 | 중간 | 높음 |
| **Phase 2** | 에이전트 간 지식 전이 | P2 | 높음 | 높음 |
| **Phase 2** | Reactive + Proactive 메모리 접근 | P2 | 높음 | 높음 |
| **Phase 2** | 토픽 기반 Pub/Sub | P2 | 중간 | 중간 |
| **Phase 3** | 핸드오프 프로토콜 (Narrative Casting) | P3 | 높음 | 중간 |
| **Phase 3** | 벡터 임베딩 기반 시맨틱 검색 | P3 | 높음 | 중간 |
| **Phase 3** | 세션 자동 아카이빙 + 인덱싱 | P3 | 중간 | 중간 |

---

## 5. 추가 참고 자료

### 프로토콜 및 아키텍처
- [A Survey of Agent Interoperability Protocols (MCP, ACP, A2A, ANP)](https://arxiv.org/html/2505.02279v1) -- 4대 에이전트 통신 프로토콜 종합 비교 논문
- [Top 5 Open Protocols for Building Multi-Agent AI Systems 2026](https://onereach.ai/blog/power-of-multi-agent-ai-open-protocols/) -- 2026년 주요 오픈 프로토콜 개요
- [AI Agent Protocols 2026: The Complete Guide](https://www.ruh.ai/blogs/ai-agent-protocols-2026-complete-guide) -- AI 에이전트 프로토콜 완전 가이드
- [Four Design Patterns for Event-Driven Multi-Agent Systems](https://www.confluent.io/blog/event-driven-multi-agent-systems/) -- 이벤트 기반 4대 설계 패턴

### 프레임워크 비교
- [CrewAI vs LangGraph vs AutoGen (DataCamp)](https://www.datacamp.com/tutorial/crewai-vs-langgraph-vs-autogen) -- 3대 프레임워크 상세 비교
- [CrewAI vs LangGraph vs AutoGen vs OpenAgents 2026](https://openagents.org/blog/posts/2026-02-23-open-source-ai-agent-frameworks-compared) -- 2026년 최신 프레임워크 비교
- [OpenAI Swarm (GitHub)](https://github.com/openai/swarm) -- 경량 멀티에이전트 핸드오프 프레임워크
- [Google ADK Multi-Agent Systems](https://google.github.io/adk-docs/agents/multi-agents/) -- Google ADK 멀티에이전트 문서
- [Google ADK Context Management](https://google.github.io/adk-docs/sessions/) -- 세션, 상태, 메모리 관리

### 컨텍스트 엔지니어링
- [Effective Context Engineering for AI Agents (Anthropic)](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents) -- Anthropic 공식 컨텍스트 엔지니어링 가이드
- [Context Engineering in Multi-Agent Systems (Agno)](https://www.agno.com/blog/context-engineering-in-multi-agent-systems) -- 멀티에이전트 컨텍스트 전략
- [Architecting Efficient Context-Aware Multi-Agent Framework (Google)](https://developers.googleblog.com/architecting-efficient-context-aware-multi-agent-framework-for-production/) -- 프로덕션 컨텍스트 인식 프레임워크
- [Context Engineering (LangChain)](https://blog.langchain.com/context-engineering-for-agents/) -- LangChain의 컨텍스트 엔지니어링
- [Memory for AI Agents: Context Engineering Paradigm (The New Stack)](https://thenewstack.io/memory-for-ai-agents-a-new-paradigm-of-context-engineering/) -- 메모리 패러다임

### 메모리 시스템
- [What Is AI Agent Memory? (IBM)](https://www.ibm.com/think/topics/ai-agent-memory) -- AI 에이전트 메모리 개요
- [AWS AgentCore Long-Term Memory](https://aws.amazon.com/blogs/machine-learning/building-smarter-ai-agents-agentcore-long-term-memory-deep-dive/) -- AWS 장기 메모리 아키텍처
- [AI Agent Memory with Redis](https://redis.io/blog/build-smarter-ai-agents-manage-short-term-and-long-term-memory-with-redis/) -- Redis 기반 단기/장기 메모리
- [Memory Overview (LangChain)](https://docs.langchain.com/oss/python/concepts/memory) -- LangChain 메모리 개념
- [OpenAI Agents SDK Session Memory](https://cookbook.openai.com/examples/agents_sdk/session_memory) -- OpenAI 세션 메모리

### 블랙보드 패턴
- [Blackboard Pattern for Multi-Agent Systems (ReputAgent)](https://reputagent.com/patterns/blackboard-pattern) -- 블랙보드 패턴 상세 설명
- [Building Multi-Agent Systems with Blackboard Pattern](https://medium.com/@dp2580/building-intelligent-multi-agent-systems-with-mcps-and-the-blackboard-pattern-to-build-systems-a454705d5672) -- MCP + 블랙보드 구현
- [LLM Multi-Agent Systems Based on Blackboard Architecture (arXiv)](https://arxiv.org/html/2507.01701v1) -- 학술 논문

### 세션 메모리 (QMD 관련)
- [QMD Sessions: Claude Code Memory](https://www.williambelk.com/blog/qmd-sessions-claude-code-memory-with-qmd-20260303/) -- QMD 세션 메모리 상세
- [QMD GitHub](https://github.com/tobi/qmd) -- QMD 로컬 검색 엔진
- [Ghost: Session Memory for Claude Code](https://github.com/notkurt/ghost) -- Git 통합 세션 메모리

### Go 구현 참고
- [goqite: Go Queue Library Built on SQLite](https://github.com/maragudk/goqite) -- SQLite 기반 Go 메시지 큐 (18,500 msg/s)
- [liteq: Go Persistent Job Queues on SQLite](https://github.com/khepin/liteq) -- SQLite 기반 Go 작업 큐
