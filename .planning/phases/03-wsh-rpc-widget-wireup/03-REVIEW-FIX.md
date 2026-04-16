---
phase: 03-wsh-rpc-widget-wireup
fixed_at: 2026-04-15T00:00:00Z
review_path: .planning/phases/03-wsh-rpc-widget-wireup/03-REVIEW.md
iteration: 1
findings_in_scope: 2
fixed: 2
skipped: 0
status: all_fixed
---

# Phase 3: Code Review Fix Report

**Fixed at:** 2026-04-15
**Source review:** `.planning/phases/03-wsh-rpc-widget-wireup/03-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 2 (WR-01, WR-02 — Warnings only; IN-01..IN-04 explicitly skipped per scope)
- Fixed: 2
- Skipped: 0

## Fixed Issues

### WR-01: 위젯의 동시 Refresh 호출 가드 부재 — 캐시 write race 가능성

**Files modified:** `frontend/app/view/jiratasks/jiratasks.tsx`
**Commit:** `eda926e2`
**Applied fix:** `requestJiraRefresh()` 시작부에서 `globalStore.get(this.loadingAtom)`를 확인하여 이미 true이면 early return하도록 가드를 추가했습니다. 이로써 ☁️ 버튼을 연속해서 두 번 눌러도 두 번째 호출은 즉시 반환되어 `JiraRefreshCommand` RPC가 하나만 in-flight 상태가 되며, `~/.config/waveterm/jira-cache.json`에 대한 write-race가 발생하지 않습니다. 가드 진입 이후에만 `loadingAtom=true`가 세팅되므로 `finally` 블록의 `loadingAtom=false` 리셋과도 대칭적입니다.

**Verification:**
- Tier 1: 수정 블록 재확인 — 가드 코드 존재, 기존 try/catch/finally 흐름 유지.
- Tier 2: `npx tsc --noEmit` pass (출력 없음 = 에러 없음).

---

### WR-02: 성공 후 `loadFromCache()` 실패 시 UI 불일치 (summary + error 동시 표시)

**Files modified:** `frontend/app/view/jiratasks/jiratasks.tsx`
**Commit:** `000e1c2f`
**Applied fix:** `await this.loadFromCache()` 직후 `globalStore.get(this.errorAtom)`가 non-null인지 확인하고, non-null이면 early return하여 `refreshProgressAtom` summary를 세팅하지 않도록 했습니다. `loadFromCache()`가 자체 try/catch로 예외를 삼키고 `errorAtom`에 "Jira 캐시를 읽을 수 없습니다..." 메시지를 세팅하는 경우, 이제 성공 summary 배너가 렌더링되지 않아 에러 배너와의 동시 표시(라인 1071-1082)가 방지됩니다. `refreshProgressAtom`은 손대지 않으므로 이전 summary가 있더라도 덮어쓰지 않습니다 — D-UI-02의 auto-clear setTimeout만 이전 값을 지웁니다.

**Verification:**
- Tier 1: 수정 블록 재확인 — `errorAtom` 체크 분기가 `loadFromCache()`와 `elapsedSec` 계산 사이에 삽입됨.
- Tier 2: `npx tsc --noEmit` pass (출력 없음).

**Note (logic bug flag):** 이 fix는 단순한 조건 추가이지만 UX semantic을 약간 변경합니다(성공한 Refresh가 더 이상 "N 이슈 · Xs" summary를 표시하지 않는 경우 발생). 사용자 관점에서는 에러 배너가 더 중요하므로 의도된 동작이지만, 실제 UI 확인을 권장합니다.

## Skipped Issues

없음. 모든 in-scope 경고가 적용되었습니다.

**참고 — 범위 밖 항목 (orchestrator 지시에 따라 의도적으로 생략):**
- IN-01: `tokenLikeRegexp` 임계값 — policy tradeoff, 보안 누출 없음.
- IN-02: `isNetworkError` string fallback — 테스트 커버리지 항목.
- IN-03: context cancel/timeout 테스트 갭 — 테스트 커버리지 항목.
- IN-04: Claude subprocess Analyze 경로 — 의도적 유지(dead code 아님), 코드 변경 불필요.

---

_Fixed: 2026-04-15_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
