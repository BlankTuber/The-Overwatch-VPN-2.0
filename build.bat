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
cd firewall-interaction
go build -o ..\src-tauri\binaries\ow-firewall-sidecar.exe cmd\sidecar\main.go
if %ERRORLEVEL% neq 0 (
    echo Error building Firewall Sidecar
    exit /b %ERRORLEVEL%
)

:: Add the administrator manifest using Windows SDK tools
echo Adding UAC manifest to the sidecar...
copy sidecar.manifest ..\src-tauri\binaries\ow-firewall-sidecar.manifest
:: Try to use mt.exe if available (from Windows SDK)
where mt.exe >nul 2>nul
if %ERRORLEVEL% equ 0 (
    mt.exe -manifest ..\src-tauri\binaries\ow-firewall-sidecar.manifest -outputresource:..\src-tauri\binaries\ow-firewall-sidecar.exe;#1
) else (
    :: Fallback to rc.exe + link.exe if available (also from Windows SDK)
    where rc.exe >nul 2>nul && where link.exe >nul 2>nul
    if %ERRORLEVEL% equ 0 (
        echo #define MANIFEST_RESOURCE_ID 1 > ..\src-tauri\binaries\manifest.rc
        echo MANIFEST_RESOURCE_ID RT_MANIFEST "..\src-tauri\binaries\ow-firewall-sidecar.manifest" >> ..\src-tauri\binaries\manifest.rc
        rc.exe ..\src-tauri\binaries\manifest.rc
        link.exe /SUBSYSTEM:WINDOWS ..\src-tauri\binaries\ow-firewall-sidecar.exe ..\src-tauri\binaries\manifest.res /OUT:..\src-tauri\binaries\ow-firewall-sidecar-admin.exe
        move /y ..\src-tauri\binaries\ow-firewall-sidecar-admin.exe ..\src-tauri\binaries\ow-firewall-sidecar.exe
        del ..\src-tauri\binaries\manifest.rc ..\src-tauri\binaries\manifest.res
    ) else (
        :: Last resort fallback to windres + ld if they're available (MinGW/MSYS2)
        where windres.exe >nul 2>nul && where ld.exe >nul 2>nul
        if %ERRORLEVEL% equ 0 (
            echo 1 24 "..\src-tauri\binaries\ow-firewall-sidecar.manifest" > ..\src-tauri\binaries\sidecar.rc
            windres.exe ..\src-tauri\binaries\sidecar.rc -O coff -o ..\src-tauri\binaries\sidecar.res
            ld.exe -r -b pei-i386 -o ..\src-tauri\binaries\ow-firewall-sidecar-admin.exe ..\src-tauri\binaries\ow-firewall-sidecar.exe ..\src-tauri\binaries\sidecar.res
            move /y ..\src-tauri\binaries\ow-firewall-sidecar-admin.exe ..\src-tauri\binaries\ow-firewall-sidecar.exe
            del ..\src-tauri\binaries\sidecar.rc ..\src-tauri\binaries\sidecar.res
        ) else (
            echo WARNING: Could not find tools to embed manifest. UAC elevation may not work properly.
        )
    )
)
del ..\src-tauri\binaries\ow-firewall-sidecar.manifest >nul 2>nul
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