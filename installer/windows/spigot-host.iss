; Build: iscc /DAppVersion=0.1.0 /DHostExe=..\..\dist\spigot-host.exe /DDriverDir=..\..\dist\driver spigot-host.iss
#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

[Setup]
AppName=SpigotRemotePlayHost
AppVersion={#AppVersion}
AppPublisher=justjoseorg
DefaultDirName={autopf}\SpigotRemotePlayHost
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
OutputBaseFilename=SpigotRemotePlayHost-Setup-v{#AppVersion}
OutputDir=..\..\dist
Compression=lzma2
UninstallDisplayIcon={app}\spigot-host.exe

[Components]
Name: "host"; Description: "SpigotRemotePlayHost"; Types: full compact custom; Flags: fixed
Name: "driver"; Description: "SudoVDA virtual display driver (adds a self-signed certificate to the Windows trusted root and publisher stores)"; Types: full

[Files]
Source: "{#HostExe}"; DestDir: "{app}"; Components: host
Source: "{#DriverDir}\*"; DestDir: "{app}\driver"; Components: driver; Flags: recursesubdirs
Source: "install-driver.ps1"; DestDir: "{app}\driver"; Components: driver

[Run]
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\driver\install-driver.ps1"""; \
  StatusMsg: "Installing virtual display driver..."; Components: driver; Flags: runhidden waituntilterminated
Filename: "schtasks.exe"; Parameters: "/Create /F /SC ONLOGON /RL HIGHEST /TN SpigotRemotePlayHost /TR ""\""{app}\spigot-host.exe\"""""; \
  Flags: runhidden waituntilterminated
Filename: "schtasks.exe"; Parameters: "/Run /TN SpigotRemotePlayHost"; Flags: runhidden nowait

[UninstallRun]
Filename: "schtasks.exe"; Parameters: "/End /TN SpigotRemotePlayHost"; Flags: runhidden; RunOnceId: "EndTask"
Filename: "schtasks.exe"; Parameters: "/Delete /F /TN SpigotRemotePlayHost"; Flags: runhidden; RunOnceId: "DelTask"
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\driver\install-driver.ps1"" -Uninstall"; \
  Flags: runhidden waituntilterminated; RunOnceId: "RemoveDriver"
