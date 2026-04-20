# Fork 릴리즈 가이드

> `soulpaper/waveterm` fork 전용 릴리즈 운영 문서.
> 업스트림(`wavetermdev/waveterm`)과 별개로, GitHub Releases + electron-updater github provider 조합으로 자동 업데이트까지 무료로 운영합니다.

---

## 개요

- **업데이트 소스**: `github.com/soulpaper/waveterm/releases`
- **자동 업데이트 방식**: 설치된 앱이 GitHub API를 폴링해서 새 릴리즈 감지 → 백그라운드 다운로드 → 재시작 시 설치
- **비용**: 0원 (public fork + GitHub Actions 무료 티어)
- **코드서명**: 없음 (Windows SmartScreen 최초 1회 경고 발생, 업데이트엔 지장 없음)
- **지원 플랫폼**: Windows x64만 (macOS/Linux 추가하려면 `build-fork.yml` 확장 필요)

---

## 사전 세팅 (이미 완료됨)

이미 구축돼 있는 것들 — 다시 할 필요 없음:

1. **`electron-builder.config.cjs`**의 `publish` 블록이 github provider로 설정됨
   ```js
   publish: {
       provider: "github",
       owner: "soulpaper",
       repo: "waveterm",
   }
   ```

2. **`.github/workflows/build-fork.yml`** — 태그 push 시 Windows 빌드 + Release 자동 생성

3. **업스트림 워크플로(`build-helper.yml`, `publish-release.yml`)** — `github.repository_owner == 'wavetermdev'` 가드로 fork에선 안 돌도록 처리됨

---

## 일반 릴리즈 절차

### 1. 작업 내용 커밋
평소처럼 feature/fix 커밋하고 push:
```bash
git commit -am "feat: ..."
git push
```

### 2. 버전 올리기

`package.json` 파일 열어서 `"version"` 필드 수정:
```json
"version": "1.0.0"   →   "version": "1.0.1"
```

버전 규칙은 [버전 규칙](#버전-규칙) 참고.

### 3. 버전 커밋 + 태그 + push

```bash
git commit -am "chore: bump version to 1.0.1"
git tag v1.0.1
git push
git push origin v1.0.1
```

> ⚠️ `git push --tags` 대신 **`git push origin v1.0.1`** 로 특정 태그만 push하세요. `--tags`는 업스트림 태그(`v0.14.4` 등)도 같이 밀어서 rejected 에러 노이즈가 발생합니다.

### 4. Actions 진행 확인

```
https://github.com/soulpaper/waveterm/actions
```

`Build v1.0.1 (fork)` 워크플로가 자동 시작됨. ~10~15분 소요.

- 🟢 **성공**: Releases 페이지에 `v1.0.1` 자동 생성 (`.exe`, `.msi`, `.zip`, `latest.yml`)
- 🔴 **실패**: 로그 확인 후 [문제 해결](#문제-해결) 참조

### 5. 사용자 관점

이미 앱을 설치한 사용자는 **아무 조작도 필요 없습니다**:
- 앱 시작 시 자동 체크 + 이후 1시간마다 체크
- 새 버전 발견 → 백그라운드 다운로드
- 다운로드 완료 → 앱 안에 업데이트 알림 배너 표시
- "지금 재시작" 클릭 → 자동 설치 + 재시작

---

## 버전 규칙

Fork는 **독립 버전 트랙**(`1.x.x` 이상)을 사용합니다. 업스트림은 `0.x.x`를 쓰므로 절대 겹치지 않습니다.

SemVer 적용:

| 변경 유형 | 버전 증가 | 예시 |
|----------|----------|------|
| 버그 수정 | patch | `1.0.0` → `1.0.1` |
| 기능 추가 (하위 호환) | minor | `1.0.1` → `1.1.0` |
| 큰 변경 / 호환성 깸 | major | `1.1.0` → `2.0.0` |

beta/rc 릴리즈:
```
v1.1.0-beta.0
v1.1.0-rc.1
```
→ `build-fork.yml`이 자동으로 prerelease로 표시함.

---

## 빠른 요약 (치트시트)

```bash
# 1. package.json의 "version" 수정

# 2. 커밋 + 태그 + push
git commit -am "chore: bump version to X.Y.Z"
git tag vX.Y.Z
git push
git push origin vX.Y.Z

# 3. 브라우저에서 Actions 확인
# https://github.com/soulpaper/waveterm/actions
```

---

## 문제 해결

### Actions 빌드 실패

1. `https://github.com/soulpaper/waveterm/actions` 접속
2. 실패한 워크플로 클릭 → 빨간 ✗ 단계 클릭
3. 자주 발생하는 에러:

| 에러 | 원인 | 해결 |
|------|------|------|
| `Error: Process completed with exit code 1` in "Build Windows" | 보통 Go/Node 의존성 깨짐, 또는 electron-builder 캐시 | 캐시 삭제 후 재시도 (Actions UI → Re-run) |
| `npm ci` 실패 | `package-lock.json` 불일치 | 로컬에서 `npm install` 후 lockfile 커밋 |
| `fpm not found` | Ruby gem 설치 실패 | 러너 이슈, Re-run |
| Release creation 403 | token 권한 | `permissions: contents: write` 확인 (이미 설정됨) |

### `latest.yml` 누락

- 증상: 앱이 새 버전을 감지 못 함
- 원인: Release에 `latest.yml` 파일이 없음 (electron-updater 필수 파일)
- 해결: Release 페이지 → 편집 → `make/latest.yml` 수동 업로드. 또는 해당 태그 삭제 후 재태그.

### 버전 번호 잘못 찍음

이미 태그 + Release까지 만들어졌다면:

```bash
# 1. 로컬 태그 삭제
git tag -d vX.Y.Z

# 2. 원격 태그 삭제
git push origin --delete vX.Y.Z

# 3. 브라우저 Releases 페이지에서 해당 Release 삭제 (휴지통 아이콘)

# 4. package.json 고치고, 올바른 버전으로 재시도
```

### 사용자가 자동 업데이트 못 받는 경우

- 앱 설정에서 `autoupdate` 관련 값 확인 (`emain/updater.ts` 참고)
- 메뉴: 앱 → 수동으로 업데이트 체크 (`Check for Updates`)
- GitHub API rate limit (시간당 60회 IP당 unauth) — 여러 명이 한 IP 뒤에 있으면 영향 가능

---

## 실수 복구 참고

### 커밋 메시지/버전을 잘못 바꾼 경우

force push는 가급적 피함. 새 커밋으로 수정하는 방식을 권장:
```bash
# package.json 다시 고치고
git commit -am "chore: correct version to X.Y.Z"
```

### Actions이 잘못된 태그로 돌고 있음

1. Actions 페이지에서 해당 워크플로 클릭 → **Cancel workflow**
2. 잘못 생성된 Release는 Releases 페이지에서 삭제 (휴지통 아이콘)
3. 잘못된 태그 삭제 (위 "버전 번호 잘못 찍음" 참조)

---

## 확장 방안 (선택사항)

### macOS 지원 추가

`build-fork.yml`의 `strategy.matrix`에 `macos-latest` 추가 가능. 단:
- **Apple Developer 계정 없으면 설치 자체가 막힘** (Gatekeeper 차단)
- 우클릭 → 열기 workaround는 가능하지만 사용자 경험 나쁨
- 유료 인증서 필요 (연 $99)

### Linux 지원 추가

Linux는 서명 필수가 아니라 비교적 쉬움. `AppImage`/`deb`/`rpm` 중 선택해서 matrix 확장.

### Windows 코드서명 추가

SmartScreen 경고 제거 목적. 비용:
- Sectigo/DigiCert OV 인증서 연 $50~$300
- EV 인증서 연 $300+ (즉시 평판 획득)
- 환경변수 `SM_CODE_SIGNING_CERT_SHA1_HASH` 등 secret 등록 후 `electron-builder.config.cjs`의 `win.signtoolOptions` 활성화

---

## 참고 파일

- `electron-builder.config.cjs` — 빌드 + publish 설정
- `.github/workflows/build-fork.yml` — Fork 릴리즈 워크플로
- `emain/updater.ts` — 앱 내 자동 업데이트 로직
- `Taskfile.yml` — `task package` 명령 정의
