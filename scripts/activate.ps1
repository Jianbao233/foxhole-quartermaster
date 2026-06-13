# 一次性激活本工作区的工具链（Windows PowerShell）
# 用法：在工作区根目录执行  . ./scripts/activate.ps1

$workspace = (Resolve-Path "$PSScriptRoot/..").Path
$env:GOROOT = "$workspace\_toolchain\go"
$env:GOPATH = "$workspace\_gopath"
$env:GOPROXY = "https://goproxy.cn,direct"
$env:GOMODCACHE = "$env:GOPATH\pkg\mod"

$pnpmDir = "J:\Tools\MCPControl\npm-global"

$env:Path = "$env:GOROOT\bin;$pnpmDir;$env:Path"

if (-not (Test-Path $env:GOPATH)) {
  New-Item -ItemType Directory -Force -Path $env:GOPATH | Out-Null
}

Write-Host "Foxhole Quartermaster toolchain activated:" -ForegroundColor Green
go version
node --version
pnpm --version
git --version