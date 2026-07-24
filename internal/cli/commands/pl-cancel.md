---
description: "파이프라인 취소 및 정리"
---

# Pipeline Cancel

실행 중인 파이프라인을 취소하고 리소스를 정리합니다.

## 입력
- `$ARGUMENTS`: 파이프라인 ID 또는 디렉토리 경로

## 실행

1. 파이프라인 디렉토리를 찾습니다
2. `status.json`을 `cancelled`로 업데이트합니다
3. 취소 시점의 이력 체크포인트를 생성합니다:

```bash
pylon history checkpoint --pipeline "$(basename "$PIPELINE_DIR")" --phase cancelled
```

4. 정리 스크립트를 실행합니다. 체크포인트가 성공했으면 세 번째 인자로 `true`를 넘겨 runtime 디렉토리를 삭제합니다(이력은 .pylon/history/에 보존됨). 체크포인트가 실패했으면 `true`를 넘기지 마세요:

```bash
.pylon/scripts/bash/cleanup-pipeline.sh "$PIPELINE_DIR" "$BRANCH" true
```

5. 정리 결과를 보고합니다
