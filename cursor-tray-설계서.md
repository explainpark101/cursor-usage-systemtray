# Cursor Tray — 설계서 (v1)

Windows 시스템 트레이 전용 Cursor 사용량 모니터. 작업표시줄 미표시, 10초마다
`cursor.com/api/dashboard/get-current-period-usage`를 폴리하여 트레이 아이콘에 사용률(%) 표시.

## 1. 언어/기술 선택

| 항목 | 선택 | 이유 |
|---|---|---|
| 언어 | **Go** | 단일 정적 exe, idle RAM ~5–15MB, GC 오버헤드 무시 가능 |
| 트레이 | `fyne-io/systray` (Windows 순수 Go) | CGO 불필요, 작업표시줄 창 없이 트레이 전용 |
| 임베디드 브라우저 | WebView2 (`jchv/go-webview2`, CGO-free) | 로그인용. Win11 기본 탑재 |
| 쿠키 암호화 | DPAPI (`golang.org/x/sys/windows` CryptProtectData) | 사용자 키 기반 암호화, 평문 디스크 저장 방지 |
| 저장 경로 | `%LOCALAPPDATA%\cursor-tray\` | config.json + cookies.bin(DPAPI) |
| 빌드 | `go build -ldflags="-s -w -H windowsgui"` | 콘솔 없는 단일 exe (~6–8MB) |

배제: C#(idle RAM 30–60MB), Python/Electron(50MB+).

## 2. 아키텍처

```
cursor-tray.exe  (단일 바이너리, windowsgui, 작업표시줄 미표시)
┌──────────────────────────────────────────────────────┐
│  ┌───────────┐  usage(chan)   ┌─────────────────┐    │
│  │  Poller   │ ─────────────► │  Tray 모듈      │    │
│  │  10s tick │                │  · 아이콘에 %   │    │
│  └─────┬─────┘                │    렌더링(GDI)  │    │
│        │ POST + Cookie       │  · 툴팁/메뉴    │    │
│  ┌─────▼─────┐               └───────▲─────────┘    │
│  │ Cursor API│                       │ 이벤트       │
│  └───────────┘               ┌───────┴─────────┐    │
│                              │ Settings        │    │
│  ┌───────────┐  저장/로드    │  · 로그인(WV2)  │    │
│  │ Storage   │ ◄─────────── │  · 폴 주기 등   │    │
│  │ DPAPI     │              └─────────────────┘    │
│  └───────────┘                                      │
└──────────────────────────────────────────────────────┘
```

### 모듈 구성
- `cmd/cursor-tray/main.go` — 진입점, 고루틴 조립
- `internal/tray/` — 트레이 아이콘(사용률 렌더링), 툴팁, 컨텍스트 메뉴
- `internal/poller/` — 10s 폴, 지수 백오프(최대 5분), 401/403 처리
- `internal/cursorapi/` — API 클라이언트 (`planUsage` 파싱)
- `internal/auth/` — WebView2 로그인, 쿠키 캡처, 쿠키 수동 입력 fallback
- `internal/storage/` — config, DPAPI 암호화 쿠키 저장/로드
- `internal/settings/` — 설정창, 시작프로그램 등록

## 3. 주요 동작

### 폴리
- `POST https://cursor.com/api/dashboard/get-current-period-usage`, body `{}`, 헤더에 쿠키
- 타임아웃 5s. 실패 시 10s→20s→…→최대 300s 지수 백오프, 성공 시 10s 복귀
- 401/403 → 아이콘 "!" 상태 + 메뉴 "다시 로그인 필요"
- `planUsage.totalPercentUsed` / `autoPercentUsed` / `apiPercentUsed` 추출

### 트레이
- 아이콘: 16×16 비트맵에 사용률 숫자 렌더링. 70% 노랑, 90% 빨강
- 툴팁: `Total 42% · Auto 35% · API 8%`
- 좌클릭: 즉시 새로고침. 우클릭 메뉴: 사용량(읽기 전용) / 새로고침 / 설정 / 종료

### 로그인 (쿠키 획득)
cursor.com 세션 쿠키는 HttpOnly일 수 있으므로 `document.cookie` 스크래핑 대신:
1. 설정 → "Cursor 로그인" → WebView2 창 (`cursor.com/login`)
2. 로그인 완료 후 대시보드 네비게이션 감지
3. `ICoreWebView2CookieManager`로 `.cursor.com` 쿠키 전체 추출(HttpOnly 포함)
4. 창 즉시 파괴, 쿠키를 DPAPI 암호화 후 저장
- Fallback: 설정에서 쿠키 문자열 수동 붙여넣기

### 자원 목표
- 상주: RAM ~5–15MB, CPU ~0% (10초당 수 KB 요청 1회)
- WebView2/설정창은 사용 시에만 생성, 닫으면 즉시 해제

## 4. 마일스톤
1. M1 트레이 셸 + 더미 데이터 렌더링
2. M2 쿠키 수동 입력 → 실제 fetch → 갱신, 백오프·401
3. M3 WebView2 자동 로그인 + 쿠키 캡처 + DPAPI
4. M4 설정창 + 자동시작 + 로그아웃
5. M5 아이콘 다듬기, 배포 빌드 스크립트
