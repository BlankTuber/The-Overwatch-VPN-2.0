@echo off
echo Building The Overwatch VPN 2.0...

:: Create necessary directories
if not exist "src-tauri\binaries" mkdir src-tauri\binaries
if not exist "src-tauri\ips" mkdir src-tauri\ips

:: Step 1: Build the IP Puller
echo Building IP Puller...
cd Ip-Puller
go build -o ..\src-tauri\binaries\ip-puller.exe cmd\puller\main.go
if %ERRORLEVEL% neq 0 (
    echo Error building IP Puller
    exit /b %ERRORLEVEL%
)
cd ..

:: Step 2: Build the Firewall Sidecar
echo Building Firewall Sidecar...
cd ow-firewall-sidecar
go build -o ..\src-tauri\binaries\ow-firewall-sidecar.exe cmd\sidecar\main.go

:: Add the administrator manifest
echo Adding UAC manifest to the sidecar...
type sidecar.manifest > ..\src-tauri\binaries\sidecar.manifest
echo 1 24 "sidecar.manifest" > ..\src-tauri\binaries\sidecar.rc
windres ..\src-tauri\binaries\sidecar.rc -O coff -o ..\src-tauri\binaries\sidecar.res
ld -r -b pei-i386 -o ..\src-tauri\binaries\ow-firewall-sidecar-admin.exe ..\src-tauri\binaries\ow-firewall-sidecar.exe ..\src-tauri\binaries\sidecar.res
move /y ..\src-tauri\binaries\ow-firewall-sidecar-admin.exe ..\src-tauri\binaries\ow-firewall-sidecar.exe
del ..\src-tauri\binaries\sidecar.manifest ..\src-tauri\binaries\sidecar.rc ..\src-tauri\binaries\sidecar.res

if %ERRORLEVEL% neq 0 (
    echo Error building Firewall Sidecar
    exit /b %ERRORLEVEL%
)
cd ..

:: Step 3: Run IP Puller once to generate initial IP lists
echo Generating initial IP lists...
.\src-tauri\binaries\ip-puller.exe
if %ERRORLEVEL% neq 0 (
    echo Warning: Failed to generate initial IP lists
    echo The app will attempt to generate them at first run
)

:: Step 4: Copy IP lists to the Tauri resources directory
echo Copying IP lists to Tauri resources...
xcopy /y /i ips\*.txt src-tauri\ips\

:: Step 5: Build the Tauri application
echo Building Tauri application...
cd src-tauri
cargo tauri build
if %ERRORLEVEL% neq 0 (
    echo Error building Tauri application
    exit /b %ERRORLEVEL%
)
cd ..

echo Build complete!
echo The installer can be found in src-tauri\target\release\bundle\