-- v2에서 대화 상태 코드 경로가 제거되어 고아가 된 conversations 테이블 삭제.
-- 관련 인덱스(idx_conv_status, idx_conv_pipeline)는 테이블과 함께 삭제된다.
DROP TABLE IF EXISTS conversations;
