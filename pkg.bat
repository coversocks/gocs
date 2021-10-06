@echo off
set srv_name=coversocks
set srv_ver=1.3.0
set OS=%1
del /s /a /q build\%srv_name%
mkdir build
mkdir build\%srv_name%
set GOOS=windows
set GOARCH=%1
go build -o build\%srv_name%\csocks.exe github.com/coversocks/gocs/csocks
if NOT %ERRORLEVEL% EQU 0 goto :efail
xcopy win-%OS%\nssm.exe build\%srv_name%
xcopy cert.bat build\%srv_name%
xcopy coversocks-conf.bat build\%srv_name%
xcopy coversocks-install-server.bat build\%srv_name%
xcopy coversocks-install-client.bat build\%srv_name%
xcopy coversocks-uninstall.bat build\%srv_name%
xcopy default-client.json /F build\%srv_name%
xcopy default-server.json /F build\%srv_name%
xcopy csuser.json /F build\%srv_name%
xcopy sysproxy.exe /F build\%srv_name%
xcopy sysproxy64.exe /F build\%srv_name%
xcopy gfwlist.txt /F build\%srv_name%
xcopy abp.js /F build\%srv_name%


if NOT %ERRORLEVEL% EQU 0 goto :efail

cd build
del /s /a /q %srv_name%-%srv_ver%-Win-%OS%.zip
7z a -r %srv_name%-%srv_ver%-Win-%OS%.zip %srv_name%
if NOT %ERRORLEVEL% EQU 0 goto :efail
cd ..\
goto :esuccess

:efail
echo "Build fail"
pause
exit 1

:esuccess
echo "Build success"
pause