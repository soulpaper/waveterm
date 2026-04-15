---
phase: 03-wsh-rpc-widget-wireup
reviewed: 2026-04-15T00:00:00Z
depth: standard
files_reviewed: 5
files_reviewed_list:
  - pkg/wshrpc/wshserver/wshserver-jira.go
  - pkg/wshrpc/wshserver/wshserver-jira_test.go
  - cmd/wsh/cmd/wshcmd-jira.go
  - cmd/wsh/cmd/wshcmd-jira_test.go
  - frontend/app/view/jiratasks/jiratasks.tsx
findings:
  critical: 0
  warning: 2
  info: 4
  total: 6
status: issues_found
---

# Phase 3: Code Review Report

**Reviewed:** 2026-04-15
**Depth:** standard
**Files Reviewed:** 5
**Status:** issues_found

## Summary

Phase 3는 `jira.Refresh()`를 `JiraRefreshCommand` wsh RPC로 노출하고, `wsh jira refresh` CLI 서브커맨드를 추가하며, 위젯의 새로고침 버튼을 직접 RPC로 연결했습니다. 전반적으로 보안 경계(토큰 스크러빙, Korean 오류 메시지 매핑, 0/1/2/3 종료 코드)는 설계대로 잘 구현되어 있고 핸들러 테스트도 성공 + 6개 오류 클래스 분기를 모두 커버합니다.

**주요 강점**
- `sanitizeErrMessage` 정규식(`[A-Za-z0-9_=+/\-]{20,}`)은 Korean 래퍼 메시지의 공백·문장부호를 삼키지 않으면서 PAT 모양 문자열(Atlassian `ATATT3…`, JWT, base64 등)은 확실히 `<redacted>`로 치환합니다. T-03 방어 의도가 regex 차원에서 실현되어 있음.
- 오류 메시지가 `widget`과 `CLI`가 소비하는 **prefix 계약**으로 설계되어 있음 (`"인증 실패"`, `"설정 파일이 없습니다"`, `"Jira 서버에 연결할 수 없습니다:"`). `exitCodeForError`가 `strings.HasPrefix`로 매칭하므로 위젯의 substring 렌더링과 일관됨.
- `jiraLoadConfig`/`jiraRefresh` seam 패턴 + `restoreSeams` cleanup이 깔끔하며 테스트 격리가 튼튼함.
- RPC route 선택 로직(`WAVETERM_TABID` 기반)이 standalone terminal vs Wave tab을 모두 처리.

**주된 우려**
- 위젯이 `requestJiraRefresh`에 대한 동시 호출 가드를 가지고 있지 않아, ☁️ 버튼 더블 클릭 시 두 개의 Refresh RPC가 동시에 실행되어 캐시 파일 write race를 유발할 가능성 (Warning).
- 성공 후 `loadFromCache()` 내부 실패가 `errorAtom`을 세팅하면 사용자는 "summary(성공)" + "error banner(실패)"가 동시에 떠 있는 UI 상태를 볼 수 있음 (Warning).
- Phase 3가 `claude "..."` subprocess를 RPC로 대체했지만, `analyzeIssueInNewTerminal`/`analyzeIssueInCurrentTerminal`의 `claude "…"` spawn 경로는 남아 있음. 스크럼 요구사항 상 이건 **분석(Analyze) 경로**로 별개이며 Phase 3 범위가 아니라 의도적 유지로 보임 (Info).

## Warnings

### WR-01: 위젯의 동시 Refresh 호출 가드 부재 — 캐시 write race 가능성

**File:** `frontend/app/view/jiratasks/jiratasks.tsx:383-406`
**Issue:** `requestJiraRefresh()`는 진입 즉시 `loadingAtom=true`를 세팅하지만, 진입 시점에 `loadingAtom`이 이미 `true`인지 확인하지 않습니다. 사용자가 ☁️(cloud-arrow-down) 버튼을 빠르게 두 번 클릭하면 두 개의 `JiraRefreshCommand` RPC가 동시에 실행됩니다. 두 호출 모두 동일한 `~/.config/waveterm/jira-cache.json`에 쓰기 때문에 write-race (last-writer-wins, 혹은 잘린 JSON)가 발생할 수 있고, 네트워크 fetch가 2배로 발생하여 `ErrRateLimited`를 유발할 수 있습니다. 또한 먼저 끝난 호출의 `refreshProgressAtom` setTimeout guard는 summary 문자열이 다르므로 여전히 동작하지만, 두 `summary` 중 어느 것이 살아남을지 비결정적입니다.

**Fix:**
```ts
async requestJiraRefresh(): Promise<void> {
    if (globalStore.get(this.loadingAtom)) {
        return; // 또는 기존 작업 취소 로직
    }
    globalStore.set(this.loadingAtom, true);
    globalStore.set(this.errorAtom, null);
    // ... 기존 본문
}
```
또는 button의 `disabled` 속성을 `loadingAtom` 값으로 바인딩하여 UI 차원에서 차단하는 것도 가능합니다(`endIconButtons`의 `IconButtonDecl`에 `disabled`가 지원되는지 확인 후).

---

### WR-02: 성공 후 `loadFromCache()` 실패 시 UI 불일치 (summary + error 동시 표시)

**File:** `frontend/app/view/jiratasks/jiratasks.tsx:386-406`, `473-518`
**Issue:** `requestJiraRefresh`는 RPC 성공 후 `await this.loadFromCache()`를 호출합니다. `loadFromCache`는 내부에서 자체 try/catch로 예외를 삼키고 `errorAtom`을 `"Jira 캐시를 읽을 수 없습니다..."`로 세팅한 뒤 정상 반환합니다 (line 514-515). 따라서 이후 `requestJiraRefresh`는 이 실패를 인지하지 못하고 `refreshProgressAtom`에 성공 summary(`"N 이슈 · Xs"`)를 세팅합니다. 결과적으로 렌더링(line 1071-1082)에서 에러 배너와 성공 배너가 동시에 표시되어 사용자에게 혼란을 줍니다. 더구나 `requestJiraRefresh`가 시작할 때 `errorAtom=null`로 초기화했으므로, 이 에러는 정확히 refresh의 부산물임.

**Fix:** `loadFromCache` 호출 직후 `errorAtom` 값을 체크하여 에러가 세팅된 경우 summary를 표시하지 않도록 합니다:
```ts
await this.loadFromCache();
if (globalStore.get(this.errorAtom) !== null) {
    // 캐시 재로드 실패 — summary 표시하지 않음
    return;
}
const elapsedSec = (rtn.elapsedms / 1000).toFixed(1);
// ... 기존 summary 설정
```
또는 `loadFromCache()`가 실패를 boolean/throw로 호출자에게 전달하는 선택적 모드(`loadFromCache({ throwOnError: true })`)를 만드는 것도 가능합니다.

## Info

### IN-01: `tokenLikeRegexp`가 긴 호스트명을 과도하게 redact할 수 있음

**File:** `pkg/wshrpc/wshserver/wshserver-jira.go:48`
**Issue:** 정규식 `[A-Za-z0-9_=+/\-]{20,}`는 `.`를 포함하지 않으므로 안전한 래퍼 메시지를 보존합니다만, 긴 서브도메인(예: `mycompanyverylongsubdomain.atlassian.net`에서 `mycompanyverylongsubdomain` 26자)이 `<redacted>`로 치환되어 네트워크 오류가 모호해질 수 있습니다. 보안 누출은 없고 의도된 tradeoff라는 주석(line 44-47)도 있지만, 디버깅 가능성을 위해 노출된 문제를 기록합니다.

**Fix:** (우선순위 낮음) 필요 시 향후에 `\b[A-Za-z0-9_=+/\-]{32,}\b` 로 임계값을 높이거나 `eyJ`(JWT), `ATATT`(Atlassian) 같은 구체적 접두사 패턴으로 한정하는 것도 옵션. 현재 구현으로도 보안상 충분.

---

### IN-02: `isNetworkError`가 문자열 매칭에 의존하는 마지막 fallback

**File:** `pkg/wshrpc/wshserver/wshserver-jira.go:123-126`
**Issue:** `*net.OpError`, `*url.Error`로 `errors.As` 체크 후 fallback으로 `strings.Contains(msg, "dial tcp") || strings.Contains(msg, "i/o timeout")` 문자열 매칭을 합니다. 로케일/Go 버전 변경 시 에러 문자열이 번역되거나 변경되면 fallback이 실패할 수 있습니다. 현재 Go 표준 라이브러리는 영어 고정이므로 실무상 문제 없으나, 테스트에서 이 fallback 경로는 직접 커버되지 않습니다(`network` 서브테스트는 `*net.OpError` 경로로 커버).

**Fix:** (선택) fallback을 제거하거나 별도 서브테스트 추가:
```go
t.Run("network_string_fallback", func(t *testing.T) {
    restoreSeams(t)
    jiraLoadConfig = func() (jira.Config, error) { return jira.Config{BaseUrl: "x"}, nil }
    jiraRefresh = func(ctx context.Context, opts jira.RefreshOpts) (*jira.RefreshReport, error) {
        return nil, errors.New("unexpected dial tcp failure")
    }
    _, err := ws.JiraRefreshCommand(context.Background(), wshrpc.CommandJiraRefreshData{})
    if !strings.HasPrefix(err.Error(), "Jira 서버에 연결할 수 없습니다:") { t.Error(...) }
})
```

---

### IN-03: 테스트 커버리지 갭 — context cancel/timeout 경로

**File:** `pkg/wshrpc/wshserver/wshserver-jira_test.go`
**Issue:** 7개 서브테스트(명시된 6개 + `config_incomplete`)가 모든 오류 클래스를 커버하지만, `ctx.Done()` 취소·타임아웃 시 handler가 에러를 어떻게 전파하는지, 그리고 `mapJiraError`가 `context.DeadlineExceeded`/`context.Canceled`를 어떤 카테고리로 분류하는지 직접 확인하는 테스트가 없습니다. 현재 구현은 둘 다 `default` 분기로 가서 `"refresh failed: context deadline exceeded"`가 되는데, D-ERR-01에 명시된 카테고리와 다를 수 있으므로 의도를 명시하는 게 좋습니다.

**Fix:** 서브테스트 추가:
```go
t.Run("context_canceled", func(t *testing.T) {
    restoreSeams(t)
    jiraLoadConfig = func() (jira.Config, error) { return jira.Config{...}, nil }
    jiraRefresh = func(ctx context.Context, opts jira.RefreshOpts) (*jira.RefreshReport, error) {
        return nil, context.Canceled
    }
    _, err := ws.JiraRefreshCommand(context.Background(), wshrpc.CommandJiraRefreshData{})
    // 의도한 분기 확인 (현재는 "refresh failed:" prefix)
})
```
또한 `concurrent_call`(핸들러 내 global state 없음을 확인) 테스트는 seam이 package-level 변수이므로 `t.Parallel()` 사용 시 오히려 위험할 수 있음 — 명시적 테스트는 불필요하지만 주석으로 명기하면 좋음.

---

### IN-04: Dead code 확인 — Claude subprocess 경로는 Analyze 전용으로 의도적 유지

**File:** `frontend/app/view/jiratasks/jiratasks.tsx:568-632`
**Issue:** Phase 3가 "Refresh"를 `claude "..."` subprocess에서 `JiraRefreshCommand` RPC로 전환했지만, `analyzeIssueInNewTerminal`/`analyzeIssueInCurrentTerminal`은 여전히 `${cli} "${prompt...}"` 명령을 터미널에 주입합니다. 이는 **Analyze(이슈 분석) 경로**로 Refresh와 별개이며, 사용자가 `analyzeCli` prefs로 CLI를 선택(`claude`, `gemini` 등)할 수 있도록 설계된 기능입니다. 따라서 dead code가 아니며 의도적 유지입니다. (README/CONTEXT 재확인 완료)

**Fix:** 코드 변경 불필요. 본 리뷰에서는 phase-specific audit 결과만 기록합니다. 향후 Phase 4+ 작업에서 Analyze 경로도 RPC 기반으로 전환할 계획이 있다면 별도 phase plan 필요.

---

_Reviewed: 2026-04-15_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
