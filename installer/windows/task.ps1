# Installed to {app} and run elevated by the installer.
#   -Autostart  register a logon task that runs the host (and start it now)
#   -Lan        listen on all interfaces so the Decky plugin can connect, with a firewall rule
#   -SignIn     install the SpoutSignIn service, which lets paired devices type the sign-in PIN
#   -Uninstall  remove the task, service and firewall rules
param([switch]$Autostart, [switch]$Lan, [switch]$SignIn, [switch]$Uninstall)
$ErrorActionPreference = 'Stop'
$name = 'SpoutRemotePlayHost'
$exe = Join-Path (Split-Path -Parent $PSCommandPath) 'spout-host.exe'
$service = 'SpoutSignIn'

Stop-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue
Unregister-ScheduledTask -TaskName $name -Confirm:$false -ErrorAction SilentlyContinue
# Also drops rules Windows created from its "allow access" prompt for this exe.
Get-NetFirewallApplicationFilter -Program $exe -ErrorAction SilentlyContinue | Get-NetFirewallRule | Remove-NetFirewallRule
Get-NetFirewallRule -DisplayName 'Spout Remote Play Host' -ErrorAction SilentlyContinue | Remove-NetFirewallRule
Get-NetFirewallRule -DisplayName 'Spout Sign-In' -ErrorAction SilentlyContinue | Remove-NetFirewallRule
if (Get-Service -Name $service -ErrorAction SilentlyContinue) {
    Stop-Service -Name $service -Force -ErrorAction SilentlyContinue
    sc.exe delete $service | Out-Null
}
if ($Uninstall) { exit 0 }

if ($Lan) {
    # All profiles: home networks are often classified as Public. Non-local requests still
    # need the API token or a paired device token.
    New-NetFirewallRule -DisplayName 'Spout Remote Play Host' -Direction Inbound -Action Allow -Program $exe `
        -Protocol TCP -LocalPort 47995 -Profile Any | Out-Null
}

if ($Autostart) {
    $user = "$env:USERDOMAIN\$env:USERNAME"
    $hostArgs = '-background'
    if ($Lan) { $hostArgs += ' -listen 0.0.0.0:47995' }
    $action = New-ScheduledTaskAction -Execute $exe -Argument $hostArgs
    $trigger = New-ScheduledTaskTrigger -AtLogOn -User $user
    $principal = New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Highest
    # The schtasks defaults would skip the task on battery and kill the host after 72 hours.
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
        -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew
    Register-ScheduledTask -TaskName $name -Action $action -Trigger $trigger -Principal $principal `
        -Settings $settings -Force | Out-Null
    Start-ScheduledTask -TaskName $name
}

if ($SignIn) {
    # Runs as LocalSystem from boot, before anyone signs in. It only accepts devices paired with
    # this user's host (read from their paired.json) and only types while the sign-in screen shows.
    $cfg = Join-Path $env:APPDATA 'SpoutRemotePlayHost\config.json'
    $bin = '"{0}" -signin-service -config "{1}"' -f $exe, $cfg
    New-Service -Name $service -BinaryPathName $bin -DisplayName 'Spout Remote Play Sign-In' `
        -Description 'Lets devices paired with Spout Remote Play Host type your PIN at the Windows sign-in screen.' `
        -StartupType Automatic | Out-Null
    New-NetFirewallRule -DisplayName 'Spout Sign-In' -Direction Inbound -Action Allow -Program $exe `
        -Protocol TCP -LocalPort 47994 -Profile Any | Out-Null
    Start-Service -Name $service
}
