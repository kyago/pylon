---
description: "프로젝트를 워크스페이스에 추가 (git clone + .pylon/ 구성)"
---

# Add Project

Git 저장소를 워크스페이스에 독립 clone으로 추가하고, 코드베이스를 분석해
프로젝트 수준 `.pylon/` 설정(`context.md`, 기본 에이전트 정의)을 생성합니다.

## 입력
- `$ARGUMENTS`: git URL (필수)과 선택 플래그
  - `--name <dir>`: 프로젝트 디렉토리 이름 (기본: URL에서 추론)
  - `--force`: 기존 디렉토리를 제거하고 다시 clone
  - `--skip-clone`: clone을 생략하고 기존 디렉토리로 `.pylon/` 구성만 수행

## 실행

1. 입력에서 git URL을 확인합니다. URL이 없으면 사용자에게 요청합니다.
2. 프로젝트를 추가합니다:

   ```bash
   pylon add-project $ARGUMENTS
   ```

3. 출력(클론 위치, 생성된 `.pylon/` 구성)을 요약해 보고합니다.
4. 실패 시 원인을 설명하고 적절한 플래그를 안내합니다:
   - 디렉토리가 이미 존재 → `--force`(재clone) 또는 `--skip-clone`(구성만)
   - submodule 잔재 감지 → 안내 메시지에 따라 정리 후 재시도

## 산출물
- `<project>/` — clone된 프로젝트 디렉토리
- `<project>/.pylon/context.md` — 코드베이스 분석 컨텍스트
- `<project>/.pylon/agents/` — 기본 에이전트 정의
