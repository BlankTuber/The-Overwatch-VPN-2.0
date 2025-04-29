@echo off
echo Building OW Firewall Sidecar...

rem Create the output directory if it doesn't exist
if not exist "build" mkdir build

rem Build the Go executable
go build -o build\ow-firewall-sidecar.exe cmd\sidecar\main.go

rem Use the Windows Resource Compiler to add the manifest
if exist "build\ow-firewall-sidecar.exe" (
    echo Adding manifest to request admin privileges...
    type sidecar.manifest > build\sidecar.manifest
    
    rem Create a .rc file
    echo 1 24 "sidecar.manifest" > build\sidecar.rc
    
    rem Compile the resource
    windres build\sidecar.rc -O coff -o build\sidecar.res
    
    rem Link the resource to the executable
    ld -r -b pei-i386 -o build\ow-firewall-sidecar-admin.exe build\ow-firewall-sidecar.exe build\sidecar.res
    
    rem Cleanup temporary files
    del build\sidecar.manifest
    del build\sidecar.rc
    del build\sidecar.res
    move /y build\ow-firewall-sidecar-admin.exe build\ow-firewall-sidecar.exe
    
    echo Build complete: build\ow-firewall-sidecar.exe
) else (
    echo Build failed.
)