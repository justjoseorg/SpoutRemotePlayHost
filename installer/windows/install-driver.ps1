# Installed to {app}\driver and run elevated by the installer.
param([switch]$Uninstall)
$ErrorActionPreference = 'Stop'
$dir = Split-Path -Parent $PSCommandPath
$hwid = 'root\sudomaker\sudovda'
$class = '4D36E968-E325-11CE-BFC1-08002BE10318'
$cer = Join-Path $dir 'spout-driver.cer'
$nef = Join-Path $dir 'nefconc.exe'

& $nef --remove-device-node --hardware-id $hwid --class-guid $class | Out-Null

if ($Uninstall) {
    $thumb = (New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 $cer).Thumbprint
    foreach ($store in 'Root', 'TrustedPublisher') {
        Remove-Item "Cert:\LocalMachine\$store\$thumb" -ErrorAction SilentlyContinue
    }
    exit 0
}

# Windows only loads the self-signed catalog if its certificate is trusted.
Import-Certificate -FilePath $cer -CertStoreLocation Cert:\LocalMachine\Root | Out-Null
Import-Certificate -FilePath $cer -CertStoreLocation Cert:\LocalMachine\TrustedPublisher | Out-Null

& $nef --create-device-node --class-name Display --class-guid $class --hardware-id $hwid
if ($LASTEXITCODE) { throw 'create-device-node failed' }
& $nef --install-driver --inf-path (Join-Path $dir 'SudoVDA.inf')
if ($LASTEXITCODE) { throw 'install-driver failed' }
