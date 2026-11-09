@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d %~dp0

if "%GOEXE%"=="" set GOEXE=go

rem Read app version from go.mod comment: // Version: x.y.z
set "APP_VERSION="
for /f "tokens=3" %%v in ('findstr /r /c:"^// Version:" go.mod') do set "APP_VERSION=%%v"
if not defined APP_VERSION (
  echo ERROR: could not read "// Version: x.y.z" from go.mod
  exit /b 1
)

for /f "tokens=3" %%v in ('%GOEXE% version ^| findstr /r "go[0-9.]*"') do set GOVERSION=%%v
echo Building cursor-tray !APP_VERSION! with %GOVERSION% ...

if not exist dist mkdir dist

set "LDFLAGS=-s -w -H windowsgui -X cursor-tray/internal/version.Version=!APP_VERSION!"
set "OUT_AMD=dist\cursor-tray-x86_64-!APP_VERSION!.exe"
set "OUT_ARM=dist\cursor-tray-arm64-!APP_VERSION!.exe"

set GOARCH=amd64
%GOEXE% build -ldflags="%LDFLAGS%" -o "!OUT_AMD!" .\cmd\cursor-tray || exit /b 1
set GOARCH=arm64
%GOEXE% build -ldflags="%LDFLAGS%" -o "!OUT_ARM!" .\cmd\cursor-tray || exit /b 1
set GOARCH=amd64

echo.
echo Building installer with Inno Setup ...

set "ISCC="
where ISCC >nul 2>&1 && for /f "delims=" %%i in ('where ISCC') do (
  if not defined ISCC set "ISCC=%%i"
)
if not defined ISCC if exist "%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe" set "ISCC=%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe"
if not defined ISCC if exist "%ProgramFiles%\Inno Setup 6\ISCC.exe" set "ISCC=%ProgramFiles%\Inno Setup 6\ISCC.exe"
if not defined ISCC if exist "%LocalAppData%\Programs\Inno Setup 6\ISCC.exe" set "ISCC=%LocalAppData%\Programs\Inno Setup 6\ISCC.exe"

if not defined ISCC (
  echo ERROR: Inno Setup 6 ISCC.exe not found.
  echo Install from https://jrsoftware.org/isdl.php then re-run build.bat
  exit /b 1
)

echo Using "%ISCC%"
"%ISCC%" /DMyAppVersion=!APP_VERSION! "%~dp0installer\cursor-tray.iss" || exit /b 1

echo.
echo Done. Artifacts in dist\
dir dist
