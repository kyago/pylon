---
description: "PM 태스크 분해 단독 실행"
---

# Task Breakdown

요구사항과 아키텍처 분석을 기반으로 태스크를 분해합니다.

## 입력
- `$ARGUMENTS`: 파이프라인 디렉토리 경로

## 실행

1. `requirement-analysis.md`와 `architecture.md`를 읽습니다
2. 구현 태스크를 분해합니다:
   - 각 태스크에 ID(T001~), 제목, 설명, 담당 에이전트, 의존성 부여
   - 의존성 없는 태스크는 병렬 실행 가능하도록 표시
3. `tasks.json`에 저장합니다

## 산출물
`$PIPELINE_DIR/tasks.json`
