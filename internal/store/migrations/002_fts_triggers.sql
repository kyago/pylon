-- FTS5 content-sync 트리거: project_memory ↔ project_memory_fts 자동 동기화
-- INSERT/UPDATE/DELETE 시 FTS 인덱스를 자동으로 갱신한다.

-- INSERT 트리거: 새 행 삽입 시 FTS 인덱스에 추가
CREATE TRIGGER IF NOT EXISTS project_memory_fts_insert
AFTER INSERT ON project_memory BEGIN
    INSERT INTO project_memory_fts(rowid, key, content, category)
    VALUES (new.rowid, new.key, new.content, new.category);
END;

-- DELETE 트리거: 행 삭제 전 FTS 인덱스에서 제거 (external content FTS5 삭제 프로토콜)
CREATE TRIGGER IF NOT EXISTS project_memory_fts_delete
BEFORE DELETE ON project_memory BEGIN
    INSERT INTO project_memory_fts(project_memory_fts, rowid, key, content, category)
    VALUES ('delete', old.rowid, old.key, old.content, old.category);
END;

-- UPDATE 트리거: 행 갱신 시 기존 FTS 항목 제거 후 새 항목 삽입
CREATE TRIGGER IF NOT EXISTS project_memory_fts_update
AFTER UPDATE ON project_memory BEGIN
    INSERT INTO project_memory_fts(project_memory_fts, rowid, key, content, category)
    VALUES ('delete', old.rowid, old.key, old.content, old.category);
    INSERT INTO project_memory_fts(rowid, key, content, category)
    VALUES (new.rowid, new.key, new.content, new.category);
END;
