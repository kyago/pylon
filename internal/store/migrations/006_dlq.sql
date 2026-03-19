-- Dead Letter Queue for failed pipelines
CREATE TABLE IF NOT EXISTS dlq (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id     TEXT    NOT NULL,
    workflow_name   TEXT    NOT NULL DEFAULT '',
    stage           TEXT    NOT NULL,
    error_message   TEXT    NOT NULL DEFAULT '',
    error_output    TEXT    NOT NULL DEFAULT '',
    original_state_json TEXT NOT NULL DEFAULT '{}',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_dlq_pipeline_id ON dlq(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_dlq_created_at ON dlq(created_at DESC);
