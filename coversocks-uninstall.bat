@echo off
cd /d %~dp0
nssm stop "coversocks"
nssm remove "coversocks" confirm
pause