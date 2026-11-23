@echo off
setlocal
if /I "%~1"=="-Check" goto check

"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\pod-start-local.ps1"
set "POD_START_EXIT=%errorlevel%"
echo.
pause
exit /b %POD_START_EXIT%

:check
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\pod-start-local.ps1" -Check
exit /b %errorlevel%
