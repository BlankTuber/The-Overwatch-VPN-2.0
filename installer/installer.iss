[Setup]
AppName=Overwatch VPN
AppVersion=2.0
DefaultDirName={autopf}\Overwatch VPN
DefaultGroupName=Overwatch VPN
UninstallDisplayIcon={app}\ow-vpn.exe
OutputDir=..\bin
OutputBaseFilename=overwatch-vpn-setup
Compression=lzma
SolidCompression=yes
PrivilegesRequired=admin

[Files]
Source: "..\bin\ow-vpn.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\bin\ip-puller.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\bin\firewall-sidecar.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: ".\overwatch-vpn.ico"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\Overwatch VPN"; Filename: "{app}\ow-vpn.exe"; IconFilename: "{app}\overwatch-vpn.ico"
Name: "{commondesktop}\Overwatch VPN"; Filename: "{app}\ow-vpn.exe"; IconFilename: "{app}\overwatch-vpn.ico"

[Dirs]
Name: "{app}\ips"; Permissions: everyone-full

[Run]
Filename: "{app}\ow-vpn.exe"; Description: "Launch Overwatch VPN"; Flags: nowait postinstall skipifsilent