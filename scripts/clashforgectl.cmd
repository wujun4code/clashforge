@echo off
pwsh -NoLogo -File "%~dp0clashforgectl.ps1" %*
exit /b %ERRORLEVEL%
