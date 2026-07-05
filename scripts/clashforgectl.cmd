@echo off
rem clashforgectl.ps1 requires pwsh (PS7+) -- it uses `??` throughout, which
rem Windows PowerShell 5.1 cannot even parse, so a fallback to `powershell`
rem would only ever fail. Fail fast with actionable guidance instead.
where pwsh >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    pwsh -NoLogo -NoProfile -File "%~dp0clashforgectl.ps1" %*
    exit /b %ERRORLEVEL%
)
echo.
echo   [clashforgectl] PowerShell 7 (pwsh) is required but not installed.
echo.
echo   Install it:
echo     winget install Microsoft.PowerShell
echo   Or download: https://github.com/PowerShell/PowerShell/releases/latest
echo.
exit /b 1
