@echo off
echo Building The Overwatch VPN 2.0...

:: Set the project root directory
set PROJECT_DIR=The-OW-VPN-2.0

:: Create necessary directories
if not exist "%PROJECT_DIR%\src-tauri\binaries" mkdir %PROJECT_DIR%\src-tauri\binaries
if not exist "%PROJECT_DIR%\src-tauri\ips" mkdir %PROJECT_DIR%\src-tauri\ips
if not exist "ips" mkdir ips

:: Step 1: Build the IP Puller
echo Building IP Puller...
cd Ip-Puller
:: Since we're in the Ip-Puller directory, use the correct path to main.go
go build -o ..\%PROJECT_DIR%\src-tauri\binaries\ip-puller.exe cmd\puller\main.go
if %ERRORLEVEL% neq 0 (
    echo Error building IP Puller
    cd ..
    exit /b %ERRORLEVEL%
)
cd ..

:: Step 2: Build the Firewall Sidecar
echo Building Firewall Sidecar...
cd firewall-interaction
:: Since we're in the firewall-interaction directory, use the correct path to main.go
go build -o ..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar.exe cmd\sidecar\main.go
if %ERRORLEVEL% neq 0 (
    echo Error building Firewall Sidecar
    cd ..
    exit /b %ERRORLEVEL%
)

:: Add the administrator manifest using Windows SDK tools
echo Adding UAC manifest to the sidecar...
copy sidecar.manifest ..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar.manifest
:: Try to use mt.exe if available (from Windows SDK)
where mt.exe >nul 2>nul
if %ERRORLEVEL% equ 0 (
    mt.exe -manifest ..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar.manifest -outputresource:..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar.exe;#1
) else (
    :: Fallback to rc.exe + link.exe if available (also from Windows SDK)
    where rc.exe >nul 2>nul && where link.exe >nul 2>nul
    if %ERRORLEVEL% equ 0 (
        echo #define MANIFEST_RESOURCE_ID 1 > ..\%PROJECT_DIR%\src-tauri\binaries\manifest.rc
        echo MANIFEST_RESOURCE_ID RT_MANIFEST "..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar.manifest" >> ..\%PROJECT_DIR%\src-tauri\binaries\manifest.rc
        rc.exe ..\%PROJECT_DIR%\src-tauri\binaries\manifest.rc
        link.exe /SUBSYSTEM:WINDOWS ..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar.exe ..\%PROJECT_DIR%\src-tauri\binaries\manifest.res /OUT:..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar-admin.exe
        move /y ..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar-admin.exe ..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar.exe
        del ..\%PROJECT_DIR%\src-tauri\binaries\manifest.rc ..\%PROJECT_DIR%\src-tauri\binaries\manifest.res
    ) else (
        :: Last resort fallback to windres + ld if they're available (MinGW/MSYS2)
        where windres.exe >nul 2>nul && where ld.exe >nul 2>nul
        if %ERRORLEVEL% equ 0 (
            echo 1 24 "..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar.manifest" > ..\%PROJECT_DIR%\src-tauri\binaries\sidecar.rc
            windres.exe ..\%PROJECT_DIR%\src-tauri\binaries\sidecar.rc -O coff -o ..\%PROJECT_DIR%\src-tauri\binaries\sidecar.res
            ld.exe -r -b pei-i386 -o ..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar-admin.exe ..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar.exe ..\%PROJECT_DIR%\src-tauri\binaries\sidecar.res
            move /y ..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar-admin.exe ..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar.exe
            del ..\%PROJECT_DIR%\src-tauri\binaries\sidecar.rc ..\%PROJECT_DIR%\src-tauri\binaries\sidecar.res
        ) else (
            echo WARNING: Could not find tools to embed manifest. UAC elevation may not work properly.
        )
    )
)
del ..\%PROJECT_DIR%\src-tauri\binaries\ow-firewall-sidecar.manifest >nul 2>nul
cd ..

:: Step 3: [SKIPPED] No longer generating initial IP lists during build
echo Skipping initial IP list generation - will be pulled at runtime

:: Step 4: [SKIPPED] No longer copying IP lists to Tauri resources

:: Step 5: Build the Tauri application
echo Building Tauri application...
cd %PROJECT_DIR%
cargo tauri build
if %ERRORLEVEL% neq 0 (
    echo Error building Tauri application
    cd ..
    exit /b %ERRORLEVEL%
)
cd ..

echo Build complete!
echo The installer can be found in %PROJECT_DIR%\src-tauri\target\release\bundle\