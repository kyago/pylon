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

`$ARGUMENTS` repo pipeline에는 worker 실행 전에 생성된 `criteria.json`과 digest를 가진 `status.json`이
있어야 합니다. 이 명령은 live `verify.yml`을 실행 기준으로 사용하지 않습니다.

## 결과 분석

snapshot/manifest 무결성 실패 또는 snapshot 이후 live config 변경은 fail-closed입니다. 기준 변경이
필요하면 현재 run의 snapshot을 덮어쓰지 말고 새 run에서 다시 승인·생성합니다.

검증 실패 시:
1. `verification.json`의 실패 항목을 분석합니다
2. 에러 메시지를 기반으로 수정 방안을 제시합니다
3. 수정 후 재검증합니다

## 산출물
`$PIPELINE_DIR/verification.json`
