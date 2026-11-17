# Cursor Tray (macOS / RunCat Neo)

이 브랜치(`macos/runcat-neo`)는 Windows 트레이 UI 대신 **[RunCat Neo](https://github.com/runcat-dev/RunCatNeo) Custom Metrics** JSON을 갱신합니다.

Cursor 사용량을 폴링한 뒤, RunCat Neo가 감시할 수 있는 스키마의 JSON 파일을 원자적으로 기록합니다.

스키마: [CustomMetricsSchema.md](https://github.com/runcat-dev/RunCatNeo/blob/main/docs/CustomMetricsSchema.md)

## 출력 예시

```json
{
  "title": "Cursor",
  "symbol": "sparkles",
  "metricsBarValue": "42%",
  "metrics": [
    { "title": "Total", "formattedValue": "42%", "normalizedValue": 0.42 },
    { "title": "Auto",  "formattedValue": "35%", "normalizedValue": 0.35 },
    { "title": "API",   "formattedValue": "8%",  "normalizedValue": 0.08 }
  ],
  "lastUpdatedDate": "2026-11-17T04:50:40Z"
}
```

기본 출력 경로: `~/Library/Application Support/cursor-tray/runcat-usage.json`  
(`RUNCAT_OUT_FILE` 또는 `-out`으로 변경 가능)

## 설치 / 사용

### 1. 빌드

```bash
chmod +x build-mac.sh
./build-mac.sh
```

산출물 (`dist/`):

- `cursor-tray-darwin-arm64-<ver>` / `amd64` / `universal` — ad-hoc 서명된 바이너리
- `cursor-tray-macos-<ver>.zip` — 설치용 패키지 (`bin/`, `install.sh`, launchd plist)
- `cursor-tray` — 현재 머신용 바로 실행 바이너리

```bash
# zip 풀고 설치
unzip dist/cursor-tray-macos-*.zip
cd dist/stage-macos-*
./install.sh            # → ~/.local/bin/cursor-tray
./install.sh --launchd  # 로그인 시 자동 실행까지

# 또는 빌드와 동시에 설치
./build-mac.sh --install
```

### 2. 쿠키 저장

macOS에는 WebView2 로그인이 없으므로 Cookie 헤더 또는 Netscape/`cookies.txt`를 넣습니다.

```bash
cursor-tray set-cookie -f ~/Downloads/cookies.txt
# 또는
cursor-tray set-cookie 'WorkosCursorSessionToken=...'
# 또는
export CURSOR_COOKIE='WorkosCursorSessionToken=...'
```

### 3. 실행

```bash
# 한 번만 갱신
cursor-tray once

# 백그라운드 폴링 (기본 10초, 실패 시 지수 백오프)
cursor-tray
```

### 4. RunCat Neo에 연결

1. RunCat Neo → **Settings → Metrics → Custom Metrics**
2. **Add Custom Metrics Source**
3. `~/Library/Application Support/cursor-tray/runcat-usage.json` 선택
4. (선택) Metrics Bar에서 해당 소스 토글을 켜면 `metricsBarValue`(Total %)가 메뉴바에 표시됩니다

### 5. launchd (로그인 시 자동 실행)

```bash
cp launchd/dev.cursor-tray.runcat.plist ~/Library/LaunchAgents/
# plist 안의 /Users/YOU 경로를 본인 홈·바이너리 경로로 수정
launchctl load ~/Library/LaunchAgents/dev.cursor-tray.runcat.plist
```

## CLI

| 명령 | 설명 |
|------|------|
| `cursor-tray` | 폴링 데몬 |
| `cursor-tray once [-out PATH]` | 1회 갱신 후 종료 |
| `cursor-tray set-cookie [-f file \| cookie…]` | 쿠키 저장 |
| `cursor-tray clear-cookie` | 쿠키 삭제 |
| `cursor-tray version` | 버전 출력 |

플래그: `-out`, `-interval` (초)

## 저장 위치

- `~/Library/Application Support/cursor-tray/config.json`
- `~/Library/Application Support/cursor-tray/cookies.bin` (macOS에서는 평문 저장; 파일 모드 0600)
- `~/Library/Application Support/cursor-tray/runcat-usage.json` — RunCat Neo가 읽는 메트릭
- `~/Library/Application Support/cursor-tray/cursor-tray.log`

## 아키텍처

```
cursorapi  →  poller  →  runcat.WriteAtomic(runcat-usage.json)
                              ↑
                     RunCat Neo (FSEvents watch)
```

- `internal/runcat` — Custom Metrics 스키마 + atomic write (`tmp` → `mv`)
- `internal/cursorapi` / `internal/poller` — Windows 브랜치와 동일 API 폴링
- `cmd/cursor-tray/main_darwin.go` — macOS 진입점

Windows 시스템 트레이 버전은 `main` 브랜치를 사용하세요.
