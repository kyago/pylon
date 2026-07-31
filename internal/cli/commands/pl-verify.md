---
description: "빌드/테스트/린트 검증 실행"
---

# Verification

코드 변경사항을 검증합니다.

## 입력
- `$ARGUMENTS`: 파이프라인 디렉토리 경로

## 실행

검증 대상 프로젝트를 `--git-root`로 지정합니다. 생략하면 워크스페이스 루트에서 실행되어
하위 프로젝트의 `.pylon/verify.yml`을 보지 못합니다:

```bash
.pylon/scripts/bash/run-verification.sh "$ARGUMENTS" --git-root <프로젝트 상대경로>
```

## 결과 분석

`{"ok":false, "reason":"검증 설정을 찾을 수 없습니다..."}`인 경우 검증이 **수행되지 않은** 것입니다.
통과로 처리하지 말고 `.pylon/verify.yml`을 작성하거나 `--git-root`를 바로잡아 재실행합니다.

검증 실패 시:
1. `verification.json`의 실패 항목을 분석합니다
2. 에러 메시지를 기반으로 수정 방안을 제시합니다
3. 수정 후 재검증합니다

## 산출물
`$PIPELINE_DIR/verification.json`
