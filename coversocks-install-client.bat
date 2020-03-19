@echo off
cd /d %~dp0
mkdir logs
nssm install "coversocks" %CD%\csocks.exe -c -f %CD%\default-client.json
nssm set "coversocks" AppStdout %CD%\logs\out.log
nssm set "coversocks" AppStderr %CD%\logs\err.log
nssm start "coversocks"
pause