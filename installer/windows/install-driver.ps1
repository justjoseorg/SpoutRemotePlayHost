# Installed to {app}\driver and run elevated by the installer.
# An existing SudoVDA (e.g. from ArtLight or Apollo) is reused and never touched; the host
# talks to whichever SudoVDA device is present. Only a driver installed here is removed again.
param([switch]$Uninstall)
$ErrorActionPreference = 'Stop'
$dir = Split-Path -Parent $PSCommandPath
$hwid = 'root\sudomaker\sudovda'
$class = '4D36E968-E325-11CE-BFC1-08002BE10318'
$cer = Join-Path $dir 'spout-driver.cer'
$nef = Join-Path $dir 'nefconc.exe'
$marker = Join-Path $dir 'installed-by-spout'

function Get-SudoVDA {
    Get-PnpDevice -PresentOnly -ErrorAction SilentlyContinue | Where-Object { $_.HardwareID -contains $hwid }
}
function Get-DriverInf($dev) {
    (Get-PnpDeviceProperty -InstanceId $dev.InstanceId -KeyName DEVPKEY_Device_DriverInfPath -ErrorAction SilentlyContinue).Data
}

# The marker holds the driver store INF (oemNN.inf) we installed. The device is only ours while
# it still uses that INF: if ArtLight/Apollo reinstalled their driver since, leave it alone.
$present = @(Get-SudoVDA)
$ours = $false
if (Test-Path $marker) {
    $inf = (Get-Content $marker -ErrorAction SilentlyContinue | Select-Object -First 1)
    $ours = $present.Count -eq 0 -or $inf -eq 'pending' -or ($inf -and ($present | Where-Object { (Get-DriverInf $_) -eq $inf }))
}

if ($Uninstall) {
    if (-not (Test-Path $marker)) { exit 0 }
    if ($ours) { & $nef --remove-device-node --hardware-id $hwid --class-guid $class | Out-Null }
    $thumb = (New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 $cer).Thumbprint
    foreach ($store in 'Root', 'TrustedPublisher') {
        Remove-Item "Cert:\LocalMachine\$store\$thumb" -ErrorAction SilentlyContinue
    }
    Remove-Item $marker -ErrorAction SilentlyContinue
    exit 0
}

if ($present.Count -and -not $ours) {
    Write-Host "SudoVDA is already installed by another app ($($present[0].InstanceId)); using it."
    exit 0
}

# Reinstall/upgrade of our own driver: drop the old device node first.
if ($ours) { & $nef --remove-device-node --hardware-id $hwid --class-guid $class | Out-Null }

# Windows only loads the self-signed catalog if its certificate is trusted.
Import-Certificate -FilePath $cer -CertStoreLocation Cert:\LocalMachine\Root | Out-Null
Import-Certificate -FilePath $cer -CertStoreLocation Cert:\LocalMachine\TrustedPublisher | Out-Null

& $nef --create-device-node --class-name Display --class-guid $class --hardware-id $hwid
if ($LASTEXITCODE) { throw 'create-device-node failed' }
Set-Content -Path $marker -Value 'pending'
& $nef --install-driver --inf-path (Join-Path $dir 'SudoVDA.inf')
if ($LASTEXITCODE) { throw 'install-driver failed' }
$dev = @(Get-SudoVDA)
Set-Content -Path $marker -Value ($(if ($dev.Count) { Get-DriverInf $dev[0] } else { 'pending' }))
