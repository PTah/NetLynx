$ErrorActionPreference = "Stop"
$repo = Resolve-Path (Join-Path $PSScriptRoot "..")
$src = Join-Path $repo ".githooks\commit-msg"
$dstDir = Join-Path $repo ".git\hooks"
$dst = Join-Path $dstDir "commit-msg"
if (-not (Test-Path $src)) {
  Write-Error "Not found: $src"
}
New-Item -ItemType Directory -Force -Path $dstDir | Out-Null
Copy-Item -Force $src $dst
$gitbash = "C:\Program Files\Git\bin\bash.exe"
if (Test-Path $gitbash) {
  & $gitbash -lc "chmod +x '$(($dst -replace '\\', '/'))'"
}
Write-Host "Installed: $dst"
Write-Host "Hook enforces trailer: Made-with: Brain, Google and AI"
