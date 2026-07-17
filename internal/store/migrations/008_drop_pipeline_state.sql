-- 008_drop_pipeline_state.sql
-- Legacy SQLite pipeline state removal.
-- v2 파이프라인은 .pylon/runtime/ 파일 기반으로 동작하고,
-- 종료 이력은 Fossil history 체크포인트가 담당한다.
DROP TABLE IF EXISTS pipeline_state;
