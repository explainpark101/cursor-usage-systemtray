# Cursor Tray

Windows 시스템 트레이 전용 Cursor 사용량 모니터. 작업표시줄에 표시되지 않으며,
10초마다 `cursor.com` 사용량을 가져와 트레이 아이콘에 사용률(%)을 표시합니다.

## 특징

- **트레이 전용**: `-H windowsgui` 빌드, 작업표시줄/콘솔 창 없음
- **아이콘에 사용률 표시**: 16×16 아이콘에 숫자 + 색상 바(70% 노랑, 90% 빨강)
- **10초 폴 + 지수 백오프**: 오류 시 10s→20s→…→최대 5분, 복구 시 즉시 10s 복귀
- **401/403 감지**: 로그인 만료 시 아이콘 "!" 상태로 표시
- **WebView2 로그인**: 설정에서 cursor.com 로그인 → 세션 쿠키 자동 캡처(검증까지 수행)
- **쿠키는 DPAPI 암호화** 저장 (`%LOCALAPPDATA%\cursor-tray\cookies.bin`)
- **최소 자원**: 상주 RAM 목표 ~5–15MB, WebView2는 로그인/설정 창이 열린 동안에만 존재
- **고급: 쿠키 수동 입력** — Cookie 헤더 또는 Netscape/curl `cookies.txt` 파일/붙여넣기 지원

## 사용법

1. `dist\cursor-tray-x86_64-<version>.exe`(또는 arm64)를 아무 폴더에 배치하고 실행
2. 트레이에 아이콘 출현 → 우클릭 → **설정…**
3. **cursor.com 로그인…** 클릭 → 로그인 완료되면 자동으로 캡처되어 저장
   (로그인 창은 쿠키 검증 성공 즉시 닫힘)
4. 트레이 아이콘이 사용률(%)로 갱신됨

### 트레이 메뉴

- **사용량**: 최근 Total/Auto/API 표시 (읽기 전용)
- **새로고침**: 즉시 다시 폴
- **설정…**: 로그인/로그아웃, 쿠키 수동 입력, 폴 주기(기본 10초), 시작 프로그램 등록
- **종료**

### 저장 위치

- `%LOCALAPPDATA%\cursor-tray\config.json` — 설정
- `%LOCALAPPDATA%\cursor-tray\cookies.bin` — DPAPI 암호화 쿠키
- `%LOCALAPPDATA%\cursor-tray\webview\` — 설정 창 WebView2 프로필
- `%LOCALAPPDATA%\cursor-tray\webview-login\` — 로그인 창 WebView2 프로필(설정과 분리)
- exe 옆 `cursor-tray.log` — 로그

## 빌드

Windows + Go 1.23+ + [Inno Setup 6](https://jrsoftware.org/isdl.php):

```
build.bat
```

버전은 `go.mod`의 `// Version: x.y.z` 한 줄에서 읽습니다. 빌드 산출물 파일명에도 버전이 포함됩니다.

산출물 (`dist\`, 예: 버전 `1.0.1`):

- `cursor-tray-x86_64-1.0.1.exe` / `cursor-tray-arm64-1.0.1.exe` — 포터블 실행 파일
- `cursor-tray-setup-x86_64-1.0.1.exe` — Inno Setup 설치 파일 (x86_64)

또는 수동 빌드만:

```
go build -ldflags="-s -w -H windowsgui" -o cursor-tray.exe ./cmd/cursor-tray
```

## 아키텍처

```
cmd/cursor-tray/      진입점 (트레이 + 폴리 조립)
internal/tray/        트레이 아이콘 렌더링(3x5 픽셀 폰트), 툴팁, 메뉴
internal/poller/      10s 폴, 지수 백오프, 401 처리
internal/cursorapi/   POST /api/dashboard/get-current-period-usage
internal/auth/        WebView2 로그인 + 쿠키 캡처(DevTools Network.getAllCookies)
internal/settings/    설정 창 (HTML UI, Go 바인딩)
internal/storage/     config + DPAPI(CryptProtectData)
internal/autostart/   HKCU Run 키 등록
internal/webview2/    go-webview2 내장 포크(COM 바인딩 확장 포함, MIT)
internal/win32/       Win32 API 바인딩 (go-webview2 내장)
```

`internal/webview2`, `internal/win32`는 [go-webview2](https://github.com/jchv/go-webview2)(MIT, John Chadwick)의
`pkg/edge`, `internal/w32`, `webviewloader`를 기반으로 하며, DevTools 프로토콜 콜백 바인딩이 추가되었습니다.
라이선스 원문은 `internal/webview2/LICENSE-*` 참조.
