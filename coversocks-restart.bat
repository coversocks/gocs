@echo off
cd /d %~dp0
nssm stop "coversocks"
nssm start "coversocks"