-- Pipeline extensions for workflow support and pause/resume
-- Adds workflow_name, status, paused_at_stage columns to pipeline_state

ALTER TABLE pipeline_state ADD COLUMN workflow_name TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_state ADD COLUMN status TEXT NOT NULL DEFAULT 'running';
ALTER TABLE pipeline_state ADD COLUMN paused_at_stage TEXT NOT NULL DEFAULT '';
