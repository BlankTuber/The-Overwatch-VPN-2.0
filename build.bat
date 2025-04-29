@echo off
echo Building Overwatch VPN components...

echo.
echo Cleaning bin directory...
if exist bin (
    echo - Removing existing bin directory
    rmdir /s /q bin
)
echo - Creating bin directory
mkdir bin

echo.
echo Building IP Puller...
cd Ip-Puller
go build -o ..\bin\ip-puller.exe cmd\puller\main.go
if %errorlevel% neq 0 (
    echo ERROR: Failed to build IP Puller
    exit /b %errorlevel%
)
cd ..
echo - IP Puller built successfully

echo.
echo Building Firewall Sidecar...
cd firewall-interaction
go build -o ..\bin\firewall-sidecar.exe cmd\sidecar\main.go
if %errorlevel% neq 0 (
    echo ERROR: Failed to build Firewall Sidecar
    exit /b %errorlevel%
)
cd ..
echo - Firewall Sidecar built successfully

echo.
echo Building Fyne GUI...
cd fyne-gui
go build -o ..\bin\ow-vpn.exe main.go
if %errorlevel% neq 0 (
    echo ERROR: Failed to build Fyne GUI
    exit /b %errorlevel%
)
cd ..
echo - Fyne GUI built successfully

echo.
echo Running IP Puller to generate initial IP lists...
cd bin
mkdir ips
.\ip-puller.exe
if %errorlevel% neq 0 (
    echo WARNING: IP Puller execution failed
) else (
    echo - IP lists generated successfully
)
cd ..

echo.
echo Build completed successfully!
echo All components are available in the bin directory