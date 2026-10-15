<#
.SYNOPSIS
  Builds the Microsoft Store package (.msixbundle) from GoReleaser's
  Windows zip archives.

.DESCRIPTION
  For each architecture it extracts portop.exe, stages it with the tile
  assets and a rendered AppxManifest.xml, packs an .msix with makeappx and
  finally bundles them. The result is unsigned on purpose: Partner Center
  signs Store submissions itself. To sideload a local test build, sign it
  with a self-signed certificate whose subject equals -Publisher.

.EXAMPLE
  ./scripts/build-msix.ps1 -Version 0.4.0 -DistDir dist `
    -IdentityName 12345LucaPadovan.portop `
    -Publisher "CN=00000000-0000-0000-0000-000000000000" `
    -PublisherDisplayName "Luca Padovan"
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)] [string] $Version,
    [Parameter(Mandatory)] [string] $IdentityName,
    [Parameter(Mandatory)] [string] $Publisher,
    [Parameter(Mandatory)] [string] $PublisherDisplayName,
    [string] $DistDir = "dist",
    [string] $OutDir = "dist/msix"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$template = Join-Path $root "packaging/msix/AppxManifest.xml"
$assets = Join-Path $root "packaging/msix/Assets"

# MSIX versions are four numeric parts and the last one must be 0 for
# Store submissions: v1.2.3 and 1.2.3-rc1 both become 1.2.3.0.
$core = ($Version -replace '^v', '') -replace '[-+].*$', ''
if ($core -notmatch '^\d+\.\d+\.\d+$') { throw "Unsupported version '$Version'" }
$msixVersion = "$core.0"

function Find-MakeAppx {
    $cmd = Get-Command makeappx.exe -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $kits = "${env:ProgramFiles(x86)}\Windows Kits\10\bin"
    $found = Get-ChildItem $kits -Recurse -Filter makeappx.exe -ErrorAction SilentlyContinue |
        Where-Object { $_.FullName -match '\\x64\\' } |
        Sort-Object FullName -Descending | Select-Object -First 1
    if (-not $found) { throw "makeappx.exe not found: install the Windows 10/11 SDK" }
    return $found.FullName
}

$makeappx = Find-MakeAppx
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$packages = Join-Path $OutDir "packages"
New-Item -ItemType Directory -Force -Path $packages | Out-Null

$arches = @{ "amd64" = "x64"; "arm64" = "arm64" }
foreach ($goarch in $arches.Keys) {
    $zip = Join-Path $DistDir "portop_${core}_windows_${goarch}.zip"
    if (-not (Test-Path $zip)) {
        $zip = Get-ChildItem $DistDir -Filter "portop_*_windows_${goarch}.zip" | Select-Object -First 1 -ExpandProperty FullName
    }
    if (-not $zip) { throw "No Windows $goarch zip in $DistDir" }

    $stage = Join-Path $OutDir "stage-$goarch"
    if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
    New-Item -ItemType Directory -Force -Path $stage | Out-Null
    Expand-Archive -Path $zip -DestinationPath $stage
    Copy-Item -Recurse $assets (Join-Path $stage "Assets")

    $manifest = (Get-Content -Raw $template).
        Replace("__IDENTITY_NAME__", $IdentityName).
        Replace("__PUBLISHER_DISPLAY_NAME__", [Security.SecurityElement]::Escape($PublisherDisplayName)).
        Replace("__PUBLISHER__", [Security.SecurityElement]::Escape($Publisher)).
        Replace("__VERSION__", $msixVersion).
        Replace("__ARCH__", $arches[$goarch])
    Set-Content -Path (Join-Path $stage "AppxManifest.xml") -Value $manifest -Encoding UTF8

    $msix = Join-Path $packages "portop_${core}_$($arches[$goarch]).msix"
    & $makeappx pack /o /d $stage /p $msix
    if ($LASTEXITCODE -ne 0) { throw "makeappx pack failed for $goarch" }
}

$bundle = Join-Path $OutDir "portop_${core}.msixbundle"
& $makeappx bundle /o /bv $msixVersion /d $packages /p $bundle
if ($LASTEXITCODE -ne 0) { throw "makeappx bundle failed" }
Write-Host "Store package: $bundle"
