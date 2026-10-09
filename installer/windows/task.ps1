# Installed to {app} and run elevated by the installer.
#   -Autostart  register a logon task that runs the host (and start it now)
#   -Lan        listen on all interfaces so the Decky plugin can connect, with a firewall rule
#   -Uninstall  remove the task and firewall rule
param([switch]$Autostart, [switch]$Lan, [switch]$Uninstall)
$ErrorActionPreference = 'Stop'
$name = 'SpoutRemotePlayHost'
$exe = Join-Path (Split-Path -Parent $PSCommandPath) 'spout-host.exe'

Stop-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue
Unregister-ScheduledTask -TaskName $name -Confirm:$false -ErrorAction SilentlyContinue
Get-NetFirewallRule -DisplayName 'Spout Remote Play Host' -ErrorAction SilentlyContinue | Remove-NetFirewallRule
if ($Uninstall) { exit 0 }

if ($Lan) {
    # Non-local requests still need the API token or a paired device token.
    New-NetFirewallRule -DisplayName 'Spout Remote Play Host' -Direction Inbound -Action Allow -Program $exe `
        -Protocol TCP -LocalPort 47995 -Profile Domain, Private | Out-Null
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
