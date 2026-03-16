-- 프로젝트 레지스트리 (pylon init / add-project 시 등록)
CREATE TABLE IF NOT EXISTS projects (
    project_id  TEXT PRIMARY KEY,
    path        TEXT NOT NULL,
    stack       TEXT DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
