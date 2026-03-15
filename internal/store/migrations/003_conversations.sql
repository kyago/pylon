-- conversations 테이블
CREATE TABLE IF NOT EXISTS conversations (
    id              TEXT PRIMARY KEY,
    title           TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active',
    session_id      TEXT,
    pipeline_id     TEXT,
    projects        TEXT,
    task_id         TEXT,
    ambiguity_score REAL DEFAULT 0.0,
    clarity_scores  TEXT,
    started_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at    DATETIME,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('active', 'completed', 'cancelled'))
);

CREATE INDEX IF NOT EXISTS idx_conv_status ON conversations(status);
CREATE INDEX IF NOT EXISTS idx_conv_pipeline ON conversations(pipeline_id);

-- message_queue 추가 인덱스 (IsResultProcessed 조회 최적화)
CREATE INDEX IF NOT EXISTS idx_mq_from_type_status ON message_queue(from_agent, type, status);
