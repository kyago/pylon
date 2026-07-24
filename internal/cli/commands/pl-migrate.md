---
description: "구버전 워크스페이스의 레거시 저장소(SQLite·Fossil)를 폐기"
---

# Workspace Migrate

MD-First 저장소 전환 이후 처음 실행하는 워크스페이스를 정리합니다.
**레거시 데이터(SQLite 메모리, Fossil 이력)는 새 버전으로 이전되지 않고 삭제됩니다.**
새 메모리(`.pylon/memory/`)와 새 이력(`.pylon/history/pipelines/`)은 빈 상태로 시작합니다.

실제 삭제는 사용자의 명시적 확인을 받은 뒤에만 수행하고, 판단이 어려운 항목은
삭제하지 말고 사용자에게 보고합니다.

## Step 1: 레거시 파일 감지

구버전 저장소 파일이 남아 있는지 확인합니다:

```bash
test -f .pylon/pylon.db && echo "발견: .pylon/pylon.db (SQLite 메모리/프로젝트 레지스트리)"
test -f .pylon/history/pylon-history.fossil && echo "발견: .pylon/history/pylon-history.fossil (Fossil 이력)"
test -d .pylon/history/checkout && echo "발견: .pylon/history/checkout (Fossil 체크아웃)"
test -f .pylon/history/state.json && echo "발견: .pylon/history/state.json (Fossil 체크포인트 캐시)"
```

하나도 발견되지 않으면 이미 정리된 상태이므로 Step 4로 진행합니다.

## Step 2: 사용자 고지 및 확인

레거시 파일이 발견되면 다음을 **명시적으로** 알리고 확인을 받습니다:

> 레거시 데이터(SQLite 메모리, Fossil 이력)는 새 버전으로 **이전되지 않고 삭제**됩니다.
> - `.pylon/pylon.db`의 프로젝트 메모리는 복구되지 않습니다.
> - Fossil 작업 이력은 복구되지 않습니다.
> 계속하시겠습니까?

확인을 받지 못하면 여기서 중단하고 아무 파일도 삭제하지 않습니다.

## Step 3: 레거시 파일 삭제 (되돌릴 수 없음)

확인을 받은 뒤에만 실행합니다:

```bash
# 확인 후 실행 (되돌릴 수 없음)
test -f .pylon/pylon.db && rm .pylon/pylon.db
rm -rf .pylon/history/pylon-history.fossil .pylon/history/checkout .pylon/history/state.json
```

삭제 후 `.pylon/history/`에는 새 파일 기반 이력인 `pipelines/`만 남습니다.

## Step 4: 커맨드·설정 동기화

```bash
pylon doctor
```

- doctor가 스크립트·슬래시 커맨드·config 기본값을 최신으로 동기화합니다
  (구버전 커맨드 md가 새 버전으로 교체됩니다).
- `.pylon/config.yml`에서 더 이상 사용되지 않는 키는 제거할 수 있습니다(남겨도 무해):
  `history:` 섹션 전체, `memory.compaction_threshold`, `memory.session_archive`,
  `conversation:` 섹션 전체.
- `.claude/settings.json` 등에 수동으로 추가한 `pylon sync-memory --incremental` hook이
  있으면 제거합니다 (이제 아무것도 저장하지 않는 no-op입니다).

## Step 5: 결과 보고

다음을 요약해 보고합니다:

- 삭제한 레거시 파일 목록
- 보존한 항목(진행 중 파이프라인 등)과 그 이유
- 새 메모리/이력이 빈 상태로 시작한다는 안내
