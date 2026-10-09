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
Source: "{#HostExe}"; DestDir: "{app}"; Components: host; Flags: ignoreversion
Source: "task.ps1"; DestDir: "{app}"; Components: host; Flags: ignoreversion
Source: "{#DriverDir}\*"; DestDir: "{app}\driver"; Components: driver; Flags: recursesubdirs ignoreversion
Source: "install-driver.ps1"; DestDir: "{app}\driver"; Components: driver; Flags: ignoreversion

[InstallDelete]
; Remove files from a previous install. The driver marker (installed-by-spout) is kept so
; uninstall still knows whether the driver is ours.
Type: files; Name: "{autoprograms}\Spout Remote Play Host.url"
Type: files; Name: "{app}\spout-host.exe"
Type: files; Name: "{app}\task.ps1"
Type: files; Name: "{app}\driver\*.inf"
Type: files; Name: "{app}\driver\*.cat"
Type: files; Name: "{app}\driver\*.sys"
Type: files; Name: "{app}\driver\*.cer"
Type: files; Name: "{app}\driver\*.ps1"
Type: files; Name: "{app}\driver\*.txt"

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

// Stop a running host and remove its exe, so an upgrade never keeps the old one.
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  rc, i: Integer;
  exe: String;
begin
  Result := '';
  exe := ExpandConstant('{app}\spout-host.exe');
  Exec(ExpandConstant('{sys}\schtasks.exe'), '/End /TN SpoutRemotePlayHost', '', SW_HIDE, ewWaitUntilTerminated, rc);
  for i := 1 to 20 do
  begin
    Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /T /IM spout-host.exe', '', SW_HIDE, ewWaitUntilTerminated, rc);
    if not FileExists(exe) or DeleteFile(exe) then
      Exit;
    Sleep(500);
  end;
  Result := 'Could not stop the running Spout Remote Play Host (' + exe + ' is in use). Close it and run Setup again.';
end;