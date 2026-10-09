# Builds SudoVDA from source and signs it with a self-signed certificate.
# Run on a Windows machine with Visual Studio (MSVC + MSBuild) and the WDK.
# Output: <OutDir>\SudoVDA.{dll,inf,cat} and spout-driver.cer
param(
    [Parameter(Mandatory = $true)][string]$OutDir,
    [string]$PfxPath,
    [string]$PfxPassword
)
$ErrorActionPreference = 'Stop'

$sudovdaCommit = 'a4b09fa2aa731a964d0cb5d139cb1e6240e4da12' # SudoMaker/SudoVDA (MIT)
$work = Join-Path $env:TEMP 'sudovda-build'
Remove-Item $work -Recurse -Force -ErrorAction SilentlyContinue
git clone https://github.com/SudoMaker/SudoVDA.git $work
git -C $work checkout $sudovdaCommit

$sln = Join-Path $work 'Virtual Display Driver (HDR)\SudoVDA.sln'
$vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
$msbuild = & $vswhere -latest -requires Microsoft.Component.MSBuild -find 'MSBuild\**\Bin\MSBuild.exe' | Select-Object -First 1
if (-not $msbuild) { throw 'MSBuild not found' }
# The WDK ships MSBuild tasks for VS 17 only; the VS 18 runner image needs them under the 18.0 name.
Get-ChildItem 'C:\Program Files (x86)\Windows Kits\10\build' -Directory | ForEach-Object {
    $bin = Join-Path $_.FullName 'bin'
    $src = Join-Path $bin 'Microsoft.DriverKit.Build.Tasks.17.0.dll'
    $dst = Join-Path $bin 'Microsoft.DriverKit.Build.Tasks.18.0.dll'
    if ((Test-Path $src) -and -not (Test-Path $dst)) { Copy-Item $src $dst }
}
& $msbuild $sln /p:Configuration=Release /p:Platform=x64 /m
if ($LASTEXITCODE) { throw 'msbuild failed' }

New-Item -ItemType Directory -Force $OutDir | Out-Null
$built = Get-ChildItem $work -Recurse -Filter SudoVDA.dll | Where-Object { $_.FullName -match 'x64.*Release' } | Select-Object -First 1
if (-not $built) { throw 'SudoVDA.dll not found after build' }
$pkg = $built.Directory.FullName
Copy-Item "$pkg\SudoVDA.dll", "$pkg\SudoVDA.inf" $OutDir -Force

if ($PfxPath) {
    $pwd = ConvertTo-SecureString $PfxPassword -AsPlainText -Force
    $cert = Import-PfxCertificate -FilePath $PfxPath -CertStoreLocation Cert:\CurrentUser\My -Password $pwd
} else {
    $cert = New-SelfSignedCertificate -Type CodeSigningCert -Subject 'CN=SpoutRemotePlayHost driver' `
        -CertStoreLocation Cert:\CurrentUser\My -NotAfter (Get-Date).AddYears(5)
}
Export-Certificate -Cert $cert -FilePath (Join-Path $OutDir 'spout-driver.cer') | Out-Null

& Inf2Cat.exe /driver:$OutDir /os:10_X64
if ($LASTEXITCODE) { throw 'Inf2Cat failed' }
& signtool.exe sign /sha1 $cert.Thumbprint /fd SHA256 (Join-Path $OutDir 'SudoVDA.cat')
if ($LASTEXITCODE) { throw 'signtool failed' }
Write-Host "Driver package ready in $OutDir"
