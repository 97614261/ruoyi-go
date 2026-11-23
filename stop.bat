@echo off
setlocal
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\pod-start-local.ps1" -Stop
set "POD_STOP_EXIT=%errorlevel%"
echo.
pause
exit /b %POD_STOP_EXIT%
