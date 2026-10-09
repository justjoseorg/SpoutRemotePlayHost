# Downloads nefconc.exe (nefarius/nefcon, MIT) pinned by version and SHA-256 of the release zip.
param([Parameter(Mandatory = $true)][string]$OutDir)
$ErrorActionPreference = 'Stop'

$version = 'v1.17.40'
$sha256 = '812bae7ed7dfb7d6d2284bc7de2f8ccebc92ed2a0b1ae893c53b337096e50c1a'
$zip = Join-Path $env:TEMP "nefcon_$version.zip"
Invoke-WebRequest "https://github.com/nefarius/nefcon/releases/download/$version/nefcon_$version.zip" -OutFile $zip
if ((Get-FileHash $zip -Algorithm SHA256).Hash.ToLower() -ne $sha256) { throw 'nefcon hash mismatch' }

$tmp = Join-Path $env:TEMP 'nefcon-extract'
Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
Expand-Archive $zip $tmp
New-Item -ItemType Directory -Force $OutDir | Out-Null
Copy-Item (Join-Path $tmp 'x64\nefconc.exe') $OutDir -Force
