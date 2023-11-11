@echo off
cd /d %~dp0
mkdir logs
nssm install "coversocks" %CD%\coversocks.exe -s -f %CD%\default-server.json
nssm set "coversocks" AppStdout %CD%\logs\out.log
nssm set "coversocks" AppStderr %CD%\logs\err.log
nssm start "coversocks"
pause