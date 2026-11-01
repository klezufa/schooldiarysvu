@echo off
rem Runs diary.ps1 with a process-scoped ExecutionPolicy Bypass (system policy is not changed).
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0diary.ps1" %*
exit /b %ERRORLEVEL%
