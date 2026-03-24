---
description: "빌드/테스트/린트 검증 실행"
---

# Verification

코드 변경사항을 검증합니다.

## 입력
- `$ARGUMENTS`: 파이프라인 디렉토리 경로

## 실행

```bash
.pylon/scripts/bash/run-verification.sh "$ARGUMENTS"
```

## 결과 분석

검증 실패 시:
1. `verification.json`의 실패 항목을 분석합니다
2. 에러 메시지를 기반으로 수정 방안을 제시합니다
3. 수정 후 재검증합니다

## 산출물
`$PIPELINE_DIR/verification.json`
