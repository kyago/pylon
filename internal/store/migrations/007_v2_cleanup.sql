-- v2 cleanup: remove tables no longer needed after spec-kit rewrite
-- message_queue: inbox/outbox protocol removed (same-process communication)
-- blackboard: merged into project_memory
-- dlq: dead letter queue removed
-- topic_subscriptions: pub/sub not needed

DROP TABLE IF EXISTS message_queue;
DROP TABLE IF EXISTS blackboard;
DROP TABLE IF EXISTS dlq;
DROP TABLE IF EXISTS topic_subscriptions;
DROP TABLE IF EXISTS session_archive;
