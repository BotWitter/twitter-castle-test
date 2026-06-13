@echo off
setlocal enabledelayedexpansion

cd /d "%~dp0"

set LDFLAGS=-s -w
set GOFLAGS=-trimpath
set CGO_ENABLED=0
set OUTDIR=%~dp0bin

if not exist "%OUTDIR%" mkdir "%OUTDIR%"

echo ========================================
echo  Building castle-verify-go
echo  ldflags: %LDFLAGS%
echo  trimpath: ON
echo ========================================
echo.

:: castle-verify
echo [1/2] castle-verify ...
go build -ldflags="%LDFLAGS%" -o "%OUTDIR%\castle-verify.exe" ./cmd/castle-verify
if errorlevel 1 (
    echo FAIL: castle-verify
    goto :err
)
echo       OK

:: castle-verify-mt
echo [2/2] castle-verify-mt ...
go build -ldflags="%LDFLAGS%" -o "%OUTDIR%\castle-verify-mt.exe" ./cmd/castle-verify-mt
if errorlevel 1 (
    echo FAIL: castle-verify-mt
    goto :err
)
echo       OK

echo.

:: UPX compression (optional)
where upx >nul 2>&1
if %errorlevel%==0 (
    echo [UPX] Compressing executables ...
    upx --best --lzma "%OUTDIR%\castle-verify.exe"
    upx --best --lzma "%OUTDIR%\castle-verify-mt.exe"
    echo [UPX] Done.
) else (
    echo [UPX] Not found - skipping compression. Install: https://github.com/upx/upx/releases
)

echo.
echo ========================================
echo  Output:
for %%f in ("%OUTDIR%\castle-verify.exe" "%OUTDIR%\castle-verify-mt.exe") do (
    if exist "%%f" (
        for %%s in ("%%f") do echo   %%~nxf  %%~zs bytes
    )
)
echo ========================================

goto :done

:err
echo.
echo BUILD FAILED
exit /b 1

:done
echo BUILD OK
exit /b 0
