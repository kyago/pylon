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
.pylon/builtin/scripts/bash/run-verification.sh "$ARGUMENTS" --git-root <프로젝트 상대경로>
```

검증 결과의 `tree`는 깨끗한 작업 트리에서 검증한 HEAD에만 기록됩니다. PR·cleanup의 근거로 쓸 결과라면 변경을
먼저 커밋합니다. 미커밋 변경이 있으면 검증은 돌지만 `tree` 대신 `tree_reason`이 남습니다. 검증 명령이 새로 만든
추적 안 된 파일은 결과 파일 옆 `verify-artifacts/`(결과 파일이 없으면 `<repo>/.pylon/runtime/verify-artifacts/`)로
옮겨지고 마지막 검증분만 남습니다 — 목록은 결과의 `artifacts`에 있습니다.

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
