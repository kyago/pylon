-- Pylon SQLite Schema v1
-- Spec Reference: Section 8 "SQLite Schema"

-- ─── 메시지 큐 (ack 기반 전달 보장) ────────────────
CREATE TABLE IF NOT EXISTS message_queue (
    id          TEXT PRIMARY KEY,          -- UUID v7
    type        TEXT NOT NULL,             -- task_assign, result, query, broadcast, heartbeat
    priority    INTEGER DEFAULT 2,         -- 0=critical, 1=high, 2=normal, 3=low
    from_agent  TEXT NOT NULL,
    to_agent    TEXT NOT NULL,
    subject     TEXT,
    body        TEXT NOT NULL,             -- JSON
    context     TEXT,                      -- JSON (컨텍스트)
    status      TEXT DEFAULT 'queued',     -- queued → delivered → acked → (expired | failed)
    reply_to    TEXT,
    ttl_seconds INTEGER,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    delivered_at DATETIME,
    acked_at    DATETIME
);

CREATE INDEX IF NOT EXISTS idx_mq_to_status ON message_queue(to_agent, status);
CREATE INDEX IF NOT EXISTS idx_mq_priority ON message_queue(priority, created_at);

-- ─── 파이프라인 상태 ───────────────────────────────
CREATE TABLE IF NOT EXISTS pipeline_state (
    pipeline_id TEXT PRIMARY KEY,
    stage       TEXT NOT NULL,
    state_json  TEXT NOT NULL,
    updated_at  DATETIME NOT NULL
);

-- ─── 블랙보드 (프로젝트 공유 지식) ─────────────────
CREATE TABLE IF NOT EXISTS blackboard (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    category    TEXT NOT NULL,             -- hypothesis, evidence, decision, constraint, result
    key         TEXT NOT NULL,
    value       TEXT,                      -- JSON
    confidence  REAL DEFAULT 0.5,          -- 0.0 ~ 1.0
    author      TEXT NOT NULL,             -- 작성 에이전트
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME,
    superseded_by TEXT,                    -- 이후 업데이트된 항목 ID
    UNIQUE(project_id, category, key)
);

-- ─── 토픽 구독 ─────────────────────────────────────
CREATE TABLE IF NOT EXISTS topic_subscriptions (
    agent_id    TEXT NOT NULL,
    topic       TEXT NOT NULL,             -- e.g., "task.completed", "decision.architecture"
    filter      TEXT,                      -- 선택적 필터 (JSON)
    PRIMARY KEY (agent_id, topic)
);

-- ─── 프로젝트 메모리 (장기 지식) ────────────────────
CREATE TABLE IF NOT EXISTS project_memory (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    category    TEXT NOT NULL,             -- architecture, pattern, decision, learning, codebase
    key         TEXT NOT NULL,
    content     TEXT NOT NULL,
    metadata    TEXT,                      -- JSON
    author      TEXT,
    confidence  REAL DEFAULT 0.8,
    access_count INTEGER DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME,
    expires_at  DATETIME
);

-- BM25 풀텍스트 검색 인덱스
CREATE VIRTUAL TABLE IF NOT EXISTS project_memory_fts USING fts5(
    key, content, category,
    content='project_memory',
    content_rowid='rowid'
);

CREATE INDEX IF NOT EXISTS idx_pm_project ON project_memory(project_id, category);

-- ─── 세션 아카이브 ──────────────────────────────────
CREATE TABLE IF NOT EXISTS session_archive (
    id          TEXT PRIMARY KEY,
    agent_name  TEXT NOT NULL,
    task_id     TEXT NOT NULL,
    summary     TEXT NOT NULL,             -- 압축된 세션 요약
    raw_path    TEXT,                      -- 원본 파일 경로 (runtime/sessions/)
    token_count INTEGER,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
