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
3. 정리 스크립트를 실행합니다:

```bash
.pylon/scripts/bash/cleanup-pipeline.sh "$PIPELINE_DIR" "$BRANCH"
```

4. 정리 결과를 보고합니다
