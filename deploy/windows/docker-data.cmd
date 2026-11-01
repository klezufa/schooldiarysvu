@echo off
rem Runs docker-data.ps1 with a process-scoped ExecutionPolicy Bypass (system policy is not changed).
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0docker-data.ps1" %*
exit /b %ERRORLEVEL%
