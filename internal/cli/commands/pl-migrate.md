---
description: "구버전 워크스페이스를 Fossil 이력 체계로 마이그레이션"
---

# Workspace Migrate

Fossil 이력 일원화 이후 처음 실행하는 워크스페이스를 정리합니다.
모든 단계는 멱등이므로 여러 번 실행해도 안전합니다. 실제 변경은 전부
pylon CLI 명령으로 수행하고, 판단이 어려운 항목은 삭제하지 말고 사용자에게 보고합니다.

## 사전 확인

```bash
pylon doctor
```

- DB 마이그레이션(legacy `pipeline_state` 테이블 드롭)은 pylon 명령 실행 시 자동 적용됩니다
- doctor가 스크립트·슬래시 커맨드·config 기본값을 최신으로 동기화합니다

## 실행

### Step 1: 종료된 runtime 디렉토리 백필

Fossil 도입 이전에 끝난 파이프라인은 체크포인트 없이 `.pylon/runtime/`에만 남아 있습니다.
각 디렉토리의 상태를 확인합니다:

```bash
for dir in .pylon/runtime/*/; do
  if [ -f "${dir}status.json" ]; then
    echo "${dir}: $(jq -r '.status // "unknown"' "${dir}status.json")"
  else
    echo "${dir}: no-status"
  fi
done
```

종료된 디렉토리가 하나도 없으면 이 단계를 건너뛰고 Step 2로 진행합니다.
`no-status`인 디렉토리는 판단 불가로 취급해 보존하고 사용자에게 보고합니다.

상태별 phase 매핑:

| status.json의 status | phase |
|---|---|
| `completed`, `success` | `completed` |
| `cancelled`, `cleaned` | `cancelled` |
| `failed` | `failed` |
| `running` 또는 판단 불가 | 건드리지 않고 사용자에게 보고 |

종료 상태인 디렉토리마다 체크포인트를 생성하고, **성공한 경우에만** runtime 디렉토리를 삭제합니다.
`<pipeline-id>`는 runtime 디렉토리 이름 그대로입니다 (예: `.pylon/runtime/20260305-user-login/` → `20260305-user-login`).
`&&` 체이닝이 체크포인트 실패 시 삭제를 막아주므로 반드시 아래 형태로 실행합니다:

```bash
BRANCH=$(jq -r '.branch // ""' ".pylon/runtime/<pipeline-id>/status.json")
pylon history checkpoint --pipeline "<pipeline-id>" --phase <phase> && \
  .pylon/scripts/bash/cleanup-pipeline.sh ".pylon/runtime/<pipeline-id>" "$BRANCH" true
```

체크포인트가 실패하면(예: fossil 미설치) 해당 디렉토리는 보존하고 실패 원인을 보고합니다.

### Step 2: 검색을 오염시키는 change 메모리 정리

파일 변경 이력은 이제 Fossil 체크포인트가 담당하므로, 과거에 쌓인
`change` 카테고리 메모리를 일괄 삭제합니다. 먼저 규모를 확인한 뒤 삭제합니다:

```bash
pylon mem prune --category change --dry-run
pylon mem prune --category change --yes
```

### Step 3: 설정과 hook 점검 (선택)

- `.pylon/config.yml`에서 더 이상 사용되지 않는 키를 제거할 수 있습니다(남겨도 무해):
  `memory.session_archive`, `memory.retention_days`, `conversation:` 섹션 전체
- `.claude/settings.json` 등에 수동으로 추가한 `pylon sync-memory --incremental` hook이
  있으면 제거합니다 (이제 아무것도 저장하지 않는 no-op입니다)
- `pylon destroy`는 제거되었습니다 → `pylon uninstall`을 사용합니다

### Step 4: 결과 보고

다음을 요약해 보고합니다:

- 백필·삭제한 파이프라인 수와 각 체크인 ID
- 삭제한 `change` 메모리 엔트리 수
- 보존한 항목(진행 중 파이프라인, 체크포인트 실패 디렉토리)과 그 이유
