---
description: "파이프라인 상태 조회"
---

# Pipeline Status

현재 실행 중인 파이프라인의 상태를 조회합니다.

## 실행

1. `.pylon/runtime/` 아래의 파이프라인 디렉토리를 탐색합니다
2. 각 파이프라인의 `status.json`을 읽습니다
3. 산출물 존재 여부로 진행 단계를 판단합니다:
   - `requirement.md` → init 완료
   - `requirement-analysis.md` → PO 분석 완료
   - `architecture.md` → architect 분석 완료
   - `tasks.json` → PM 태스크 분해 완료
   - `execution-log.json` → 에이전트 실행 완료
   - `verification.json` → 검증 완료
   - `pr.json` → PR 생성 완료

4. 각 파이프라인의 상태를 요약하여 출력합니다
