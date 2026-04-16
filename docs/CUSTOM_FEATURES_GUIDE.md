# Wave Terminal - 추가 기능 사용 설명서

> Wave Terminal v0.14.3에 추가된 커스텀 기능들에 대한 사용 가이드입니다.

---

## 목차

1. [Claude Sessions 뷰어](#1-claude-sessions-뷰어)
2. [웹 위젯 GUI 추가 기능](#2-웹-위젯-gui-추가-기능)
3. [터미널 파일 드래그 앤 드롭](#3-터미널-파일-드래그-앤-드롭)
4. [AI 입력창 파일 드래그 앤 드롭](#4-ai-입력창-파일-드래그-앤-드롭)
5. [Jira Tasks 위젯](#5-jira-tasks-위젯)
6. [Simple Editor 위젯](#6-simple-editor-위젯)
7. [wsh jira CLI](#7-wsh-jira-cli)
8. [터미널 한글 IME 안정화](#8-터미널-한글-ime-안정화)
9. [터미널 헤더 태그 표시](#9-터미널-헤더-태그-표시)

---

## 1. Claude Sessions 뷰어

Claude Code CLI의 세션을 Wave Terminal 안에서 조회하고 이어서 작업할 수 있는 위젯입니다.

### 실행 방법

- 위젯 바에서 **Claude Sessions** 아이콘을 클릭합니다.
- 또는 `wsh` 명령어로 블록을 생성합니다: `wsh view claudesessions`

### 기능 설명

| 기능 | 설명 |
|------|------|
| **세션 목록 조회** | `~/.claude/projects/` 디렉터리에서 모든 Claude Code 세션을 자동으로 스캔합니다 |
| **상태 표시** | 각 세션의 상태를 색상으로 구분합니다 |
| **필터링** | "미완료만" / "전체" 필터를 토글하여 원하는 세션만 볼 수 있습니다 |
| **세션 재개** | 세션 카드를 클릭하면 새 터미널에서 `claude -r <sessionId>`로 해당 세션을 이어서 작업합니다 |
| **새로고침** | 상단 바의 새로고침 버튼으로 세션 목록을 다시 불러옵니다 |

### 세션 상태

| 상태 | 색상 | 조건 |
|------|------|------|
| **미완료** (노란색) | 왼쪽 테두리 노란색 | 마지막 메시지가 사용자 메시지이거나, tool_use/tool_result로 끝난 경우 |
| **완료** (초록색) | 왼쪽 테두리 초록색 | 어시스턴트의 마지막 응답에 "완료", "done", "commit" 등 완료 키워드가 포함된 경우 |
| **중단** (빨간색) | 왼쪽 테두리 빨간색 | 메시지가 2개 이하이고 마지막이 사용자 메시지인 경우 |

### 세션 카드 정보

각 세션 카드에는 다음 정보가 표시됩니다:

- **상태 배지**: 미완료/완료/중단
- **시간**: 마지막 수정 시간 (예: "3시간 전", "2일 전")
- **주제**: 세션 이름 또는 첫 번째 사용자 메시지
- **프로젝트**: 해당 세션이 속한 프로젝트 경로
- **메시지 수**: 세션 내 주고받은 메시지 개수

### 성능 최적화

- 100KB 이상의 큰 세션 파일은 앞부분(8KB)과 뒷부분(16KB)만 읽어 빠르게 파싱합니다.
- 세션 목록은 최근 수정 순으로 정렬됩니다.

---

## 2. 웹 위젯 GUI 추가 기능

자주 사용하는 웹사이트를 위젯 바에 바로 추가할 수 있는 GUI 기능입니다.

### 기본 제공 웹 위젯

위젯 바에 다음 웹 서비스들이 기본으로 제공됩니다:

| 위젯 | 아이콘 | URL |
|------|--------|-----|
| **YouTube** | YouTube 로고 | https://www.youtube.com |
| **Gemini** | Google 로고 | https://gemini.google.com |
| **Claude** | Bot 아이콘 | https://claude.ai |

### 커스텀 웹 위젯 추가 방법

1. **위젯 바에서 우클릭** → "Add Web Widget" 메뉴를 선택합니다.
2. 모달 창에서 다음 정보를 입력합니다:

| 필드 | 필수 | 설명 |
|------|------|------|
| **URL** | 필수 | 웹사이트 주소 (예: `https://github.com`). `http://` 또는 `https://`를 생략하면 자동으로 `https://`가 추가됩니다 |
| **Label** | 선택 | 위젯 이름. 비워두면 도메인 이름이 자동으로 사용됩니다 |
| **Icon** | 선택 | 아이콘 이름. 아이콘 선택 버튼(격자 아이콘)을 클릭하면 30개의 일반 아이콘과 브랜드 아이콘 중에서 선택할 수 있습니다 |

3. **"Add Widget"** 버튼을 클릭하거나 **Enter** 키를 눌러 저장합니다.

### 사용 가능한 아이콘 (일부)

**일반 아이콘**: globe, bookmark, link, code, envelope, comment, video, music, image, chart-line, cloud, book, star, heart, house, magnifying-glass

**브랜드 아이콘**: brands@github, brands@google, brands@youtube, brands@discord, brands@slack, brands@linkedin, brands@reddit, brands@twitter, brands@spotify

### 위젯 관리

- 추가된 위젯은 `~/.waveterm/config/widgets.json` 파일에 저장됩니다.
- 위젯 바에서 우클릭 → "Edit widgets.json"으로 직접 편집할 수도 있습니다.
- 동일한 이름의 위젯이 이미 존재하면 자동으로 `-2`, `-3` 등의 접미사가 붙습니다.

---

## 3. 터미널 파일 드래그 앤 드롭

파일을 터미널 뷰에 드래그하면 파일 경로가 자동으로 입력됩니다.

### 사용 방법

#### OS 파일 탐색기에서 드래그

1. Windows 파일 탐색기(또는 다른 OS 파일 관리자)에서 파일을 선택합니다.
2. 선택한 파일을 Wave Terminal의 터미널 블록 위로 드래그합니다.
3. 드래그 오버 시 터미널에 시각적 피드백(하이라이트)이 표시됩니다.
4. 파일을 드롭하면 **쉘에 안전한 형태로 이스케이프된 파일 경로**가 터미널에 붙여넣기됩니다.
5. 여러 파일을 동시에 드롭하면 공백으로 구분된 경로 목록이 입력됩니다.

#### Wave 파일 브라우저에서 드래그

1. Wave Terminal의 파일 브라우저(files 위젯)에서 파일을 선택합니다.
2. 터미널 블록으로 드래그 앤 드롭합니다.
3. 절대 경로가 자동으로 터미널에 입력됩니다.

### 경로 이스케이프 처리

특수 문자(공백, 따옴표, 괄호, `$`, `*` 등)가 포함된 경로는 자동으로 작은따옴표(`'...'`)로 감싸져 쉘에서 안전하게 사용됩니다.

**예시**:
- `C:/Users/my folder/file.txt` → `'C:/Users/my folder/file.txt'`
- `simple_path.txt` → `simple_path.txt` (특수 문자 없으면 그대로)

### 텍스트 드래그 앤 드롭

파일 외에 일반 텍스트도 터미널에 드래그 앤 드롭할 수 있습니다. 브라우저나 다른 앱에서 선택한 텍스트를 터미널에 드롭하면 해당 텍스트가 그대로 붙여넣기됩니다.

---

## 4. AI 입력창 파일 드래그 앤 드롭

파일을 AI 채팅 패널의 입력창에 드래그하여 AI에게 파일 컨텍스트를 제공할 수 있습니다.

### 사용 방법

#### OS 파일 탐색기에서 드래그

1. 파일 탐색기에서 파일을 선택합니다.
2. AI 패널(waveai)의 입력 영역으로 드래그합니다.
3. 드래그 오버 시 입력 영역에 시각적 피드백이 표시됩니다.
4. 파일을 드롭하면 AI가 해당 파일 내용을 컨텍스트로 사용합니다.

#### Wave 파일 브라우저에서 드래그

1. Wave Terminal의 파일 브라우저에서 파일을 선택합니다.
2. AI 패널의 입력 영역으로 드래그 앤 드롭합니다.
3. 파일 경로가 입력 텍스트에 추가되고, 파일 내용이 AI 컨텍스트에 첨부됩니다.

### 지원 파일 형식

- **이미지 파일**: AI에 이미지로 첨부
- **PDF 파일**: 텍스트 추출 후 AI 컨텍스트로 사용
- **텍스트/코드 파일**: 파일 내용을 그대로 AI에 전달

### 파일 크기 제한

너무 큰 파일은 자동으로 거부되며, 지원하지 않는 파일 형식은 거부 사유와 함께 에러 메시지가 표시됩니다.

---

## 5. Jira Tasks 위젯

Atlassian Jira Cloud에서 본인에게 할당된 이슈를 Wave Terminal 안에서 바로 조회·분석할 수 있는 위젯입니다.

### 실행 방법

- 위젯 바에서 **Jira Tasks** 아이콘 클릭
- 또는 `wsh view jiratasks`

### 주요 기능

| 기능 | 설명 |
|------|------|
| **이슈 카드 리스트** | 요약, 상태, 업데이트 시각, 댓글 수 표시 |
| **카드 확장** | description, attachments, comments 조회 |
| **캐시 새로고침** | `☁️` 버튼 — Go 백엔드가 Jira REST API 호출 |
| **프로젝트/상태/기간 필터** | 상단 툴바에서 설정 (설정은 블록별로 저장) |
| **자동 새로고침** | 1분/5분/15분/30분/1시간 주기 선택 |
| **분석 버튼** | 선택한 CLI(`claude`, `gemini` 등)로 프롬프트 전달, 원하는 GSD skill 지정 가능 |

### 설정

자세한 설정 절차는 **`docs/docs/jira-widget.mdx`** 참고. 요약:

1. `https://id.atlassian.com/manage-profile/security/api-tokens` 에서 API 토큰 발급
2. `~/.config/waveterm/jira.json` 작성:
   ```json
   {
     "baseUrl": "https://<YOUR_SITE>.atlassian.net",
     "cloudId": "<YOUR_CLOUD_ID>",
     "email": "<your@email.com>",
     "apiToken": "<PAT>",
     "jql": "assignee = currentUser() ORDER BY updated DESC",
     "pageSize": 50
   }
   ```
3. 터미널에서 `wsh jira refresh` 로 검증

### 캐시 위치

- 메인 캐시: `~/.config/waveterm/jira-cache.json`
- 첨부파일: `~/.config/waveterm/jira-attachments/`

### Claude에게 설정 맡기기

위젯에서 **"Claude에게 자동 설정 요청"** 버튼을 누르면 설정 프롬프트가 클립보드에 복사됩니다. Claude 세션 터미널에 붙여넣으면 값들을 물어보고 `jira.json`을 올바른 권한으로 저장해 줍니다.

---

## 6. Simple Editor 위젯

Monaco 에디터 기반의 파일 편집기 위젯입니다. Wave의 기본 preview/codeeditor 와 달리 단독 편집 블록으로 띄울 수 있습니다.

### 실행 방법

- 위젯 바에서 **editor** 아이콘 클릭
- 또는 `wsh view simpleeditor`

### 주요 기능

| 기능 | 설명 |
|------|------|
| **Monaco 기반 편집** | VS Code와 동일한 엔진, 문법 하이라이트 |
| **언어 선택** | plaintext부터 Go/Rust/Python/TypeScript/SQL 등 다수 지원 |
| **Diff 모드** | 저장본(saved) 대비 또는 VCS(git/svn) 대비 diff 토글 |
| **키바인딩 reinject** | Wave의 글로벌 단축키가 에디터 내에서도 동작 |

### 지원 언어 (일부)

plaintext, javascript, typescript, python, go, rust, java, c, cpp, csharp, html, css, scss, json, yaml, xml, markdown, sql, shell, powershell, dockerfile, ruby, php, swift 등

---

## 7. wsh jira CLI

`wsh jira` 는 Jira Tasks 위젯과 동일한 캐시·설정 파일을 사용하는 CLI입니다. 터미널에서 직접 캐시를 관리할 때 유용합니다.

### 서브커맨드

| 커맨드 | 설명 |
|--------|------|
| `wsh jira refresh` | `jira.json` 을 읽어 Jira API 호출 후 `jira-cache.json` 갱신. 이슈/첨부/댓글 수와 소요 시간 출력 |
| `wsh jira download <ISSUE-KEY> [filename]` | 특정 이슈의 첨부파일을 `~/.config/waveterm/jira-attachments/` 로 다운로드. `filename` 생략 시 해당 이슈의 모든 첨부 |

### 에러 메시지

위젯 카드와 CLI는 **동일한 한국어 에러 분류**를 공유합니다:

- `설정 파일이 없습니다` → `jira.json` 작성 필요
- `인증 실패` → PAT 또는 email 오타
- `Jira 서버에 연결할 수 없습니다` → 네트워크/VPN 확인
- `Jira 서버가 요청을 제한했습니다` → rate limit, 잠시 후 재시도

### 네트워크 특성

요청은 **RateLimitedTransport + RetryTransport** 를 통해 자동 재시도되며, 429 응답 시 `Retry-After` 헤더를 존중합니다.

---

## 8. 터미널 한글 IME 안정화

xterm.js 의 IME composition 처리를 보강해서 **한글 입력이 빠를 때 글자가 먹히는 문제** 를 수정한 커스텀 패치입니다.

### 해결한 문제

- 한글을 빠르게 연속 입력할 때 조합 중인 글자(`pendingComposedData`)가 다음 `compositionstart` 에 덮여 **data 이벤트가 소실**되던 문제
- `compositionend` 이후 `setTimeout(0)` 으로 넘어오는 확정 데이터를 안전하게 대기 (500ms safety timeout)
- 조합 중인 글자(composing preview)가 터미널에 **시각적으로 보이도록** 처리

### 체감 변화

- 한영 키 토글 직후 또는 빠른 연타 상황에서 입력이 안정적으로 들어감
- ESC 로 조합 취소해도 상태가 고장나지 않음

(구현 세부는 `frontend/app/view/term/termwrap.ts` 의 `handleCompositionStart` / `handleCompositionUpdate` / `handleCompositionEnd` / `resetCompositionState`)

---

## 9. 터미널 헤더 태그 표시

여러 터미널이 같은 `user@host` 프롬프트를 보일 때 구분할 수 있도록 **블록 헤더에 태그** 를 붙입니다.

### 표시 규칙

- `meta["display:name"]` 이 설정돼 있으면 해당 이름
- 없으면 `blockId` 앞 6글자 (`#abc123` 형태)

### 활용처

- 여러 서버 SSH 창을 동시에 띄울 때 헤더로 즉시 구분
- Jira Tasks 위젯의 **"분석용 터미널 선택"** 드롭다운에서 어떤 터미널인지 식별

---

## 빠른 참조

| 기능 | 접근 방법 |
|------|-----------|
| Claude Sessions | 위젯 바 → Claude Sessions 아이콘 |
| 웹 위젯 추가 | 위젯 바 우클릭 → "Add Web Widget" |
| 위젯 설정 편집 | 위젯 바 우클릭 → "Edit widgets.json" |
| 터미널에 파일 드롭 | 파일 → 터미널 블록으로 드래그 |
| AI에 파일 드롭 | 파일 → AI 입력창으로 드래그 |
| 세션 재개 | Claude Sessions → 세션 카드 클릭 |
| Jira Tasks | 위젯 바 → Jira Tasks 아이콘 |
| Jira 캐시 갱신 (CLI) | `wsh jira refresh` |
| Jira 첨부 다운로드 | `wsh jira download <ISSUE-KEY>` |
| Jira 설정 프롬프트 복사 | 위젯 → "Claude에게 자동 설정 요청" |
| Simple Editor | 위젯 바 → editor 아이콘 |
