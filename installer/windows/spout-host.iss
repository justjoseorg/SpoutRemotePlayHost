; Build: iscc /DAppVersion=0.1.0 /DHostExe=..\..\dist\spout-host.exe /DDriverDir=..\..\dist\driver spout-host.iss
#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

[Setup]
AppName=SpoutRemotePlayHost
AppVersion={#AppVersion}
AppPublisher=justjoseorg
DefaultDirName={autopf}\SpoutRemotePlayHost
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
OutputBaseFilename=SpoutRemotePlayHost-Setup-v{#AppVersion}
OutputDir=..\..\dist
Compression=lzma2
UninstallDisplayIcon={app}\spout-host.exe
SetupIconFile=..\..\internal\tray\icon.ico

[Components]
Name: "host"; Description: "SpoutRemotePlayHost"; Types: full compact custom; Flags: fixed
Name: "driver"; Description: "SudoVDA virtual display driver (an existing SudoVDA, e.g. from ArtLight or Apollo, is reused and left untouched; otherwise adds a self-signed certificate to the trusted root and publisher stores)"; Types: full

[Tasks]
Name: "autostart"; Description: "Start automatically when I sign in"
Name: "lan"; Description: "Allow the Decky plugin to connect over the local network (listens on port 47995, adds a firewall rule; other machines still need a token)"

[Files]
Source: "{#HostExe}"; DestDir: "{app}"; Components: host
Source: "task.ps1"; DestDir: "{app}"; Components: host
Source: "{#DriverDir}\*"; DestDir: "{app}\driver"; Components: driver; Flags: recursesubdirs
Source: "install-driver.ps1"; DestDir: "{app}\driver"; Components: driver

[InstallDelete]
Type: files; Name: "{autoprograms}\Spout Remote Play Host.url"

[Icons]
Name: "{autoprograms}\Spout Remote Play Host"; Filename: "{app}\spout-host.exe"; Parameters: "{code:HostArgs}"

[Run]
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\driver\install-driver.ps1"""; \
  StatusMsg: "Installing virtual display driver..."; Components: driver; Flags: runhidden waituntilterminated
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\task.ps1""{code:TaskArgs}"; \
  StatusMsg: "Configuring startup..."; Flags: runhidden waituntilterminated
Filename: "{app}\spout-host.exe"; Parameters: "{code:HostArgs}"; Description: "Open Spout Remote Play Host"; \
  Flags: postinstall nowait skipifsilent runasoriginaluser

[UninstallRun]
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\task.ps1"" -Uninstall"; \
  Flags: runhidden waituntilterminated; RunOnceId: "RemoveTask"
Filename: "taskkill.exe"; Parameters: "/F /IM spout-host.exe"; Flags: runhidden waituntilterminated; RunOnceId: "KillHost"
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\driver\install-driver.ps1"" -Uninstall"; \
  Flags: runhidden waituntilterminated; RunOnceId: "RemoveDriver"

[UninstallDelete]
Type: files; Name: "{app}\driver\installed-by-spout"

[Code]
function TaskArgs(Param: String): String;
begin
  Result := '';
  if WizardIsTaskSelected('autostart') then Result := Result + ' -Autostart';
  if WizardIsTaskSelected('lan') then Result := Result + ' -Lan';
end;

function HostArgs(Param: String): String;
begin
  Result := '';
  if WizardIsTaskSelected('lan') then Result := '-listen 0.0.0.0:47995';
end;

// Stop a running host so its exe can be replaced on upgrade.
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  rc: Integer;
begin
  Exec(ExpandConstant('{sys}\schtasks.exe'), '/End /TN SpoutRemotePlayHost', '', SW_HIDE, ewWaitUntilTerminated, rc);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM spout-host.exe', '', SW_HIDE, ewWaitUntilTerminated, rc);
  Result := '';
end;