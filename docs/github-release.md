# GitHub Release 가이드

이 프로젝트는 `go.mod`의 `// Version: x.y.z`를 읽어 `vX.Y.Z` 태그와 GitHub Release를 만들고, **설치형 installer**와 **포터블 exe**를 Release 자산으로 올립니다. Actions Artifact는 사용하지 않습니다.

## 전체 흐름

1. `go.mod`의 `// Version:` 을 올린다 (예: `1.1.0`)
2. 변경을 `main`에 푸시한다 (또는 Actions에서 **Build and Release**를 `workflow_dispatch`로 실행)
3. Actions가 `build.bat`으로 빌드한다
4. 태그 `v{version}` Release를 생성/갱신하고 installer + portable을 업로드한다

## 릴리스할 때 내가 할 일

1. 버전 한 줄만 올린다:

   ```text
   // Version: 1.1.0
   ```

2. 커밋 후 `main`에 푸시한다.

3. GitHub **Actions**에서 **Build and Release** 성공을 확인한다.

4. GitHub **Releases**에서 `v1.1.0` 자산이 있는지 확인한다:
   - `cursor-tray-setup-x86_64-1.1.0.exe` (Inno Setup 설치형)
   - `cursor-tray-x86_64-1.1.0.exe` (포터블)
   - `cursor-tray-arm64-1.1.0.exe` (포터블)

## 이 프로젝트 설정

- 워크플로: `.github/workflows/release.yml`
- 버전 소스: `go.mod` → `// Version: x.y.z`
- 빌드: `build.bat` (Go amd64/arm64 + Inno Setup)
- 업로드: installer + portable x86_64/arm64 (Actions Artifact 없음)
- Runner: `windows-latest`
- PR: 빌드 검증만 하고 Release는 만들지 않음

## 주의

- 같은 버전으로 `main`에 다시 푸시하면 기존 `vX.Y.Z` Release 자산이 갱신됩니다.
- 웹 UI에서 Release를 직접 만들면 자동화와 겹칠 수 있으니, 버전 bump + `main` 푸시를 권장합니다.
- 수동 태그 푸시(`git tag vX.Y.Z`)는 필수가 아닙니다. 태그는 워크플로가 `go.mod` 버전으로 만듭니다.
