# Build Windows MSI installer for i2plan
# PowerShell script
param(
    [string]$Version = $env:VERSION
)

if (-not $Version) {
    $Version = "1.0.0"
}

# MSI version must be in format X.X.X.X where X is 0-65534
# Convert "0.0.0-experimental" to "0.0.0.0"
$MsiVersion = $Version -replace '-.*$', ''
if ($MsiVersion -notmatch '^\d+\.\d+\.\d+$') {
    Write-Error "Invalid version format: $Version"
    exit 1
}
# Append .0 if only three components
$MsiVersion = "$MsiVersion.0"

$Arch = "x64"
$MsiFile = "i2plan-${Version}-windows-amd64.msi"

Write-Host "Building Windows MSI: $MsiFile"
Write-Host "MSI Version: $MsiVersion"

# Verify WiX is installed
if (-not (Get-Command candle.exe -ErrorAction SilentlyContinue)) {
    Write-Error "WiX Toolset not found. Install with: choco install wixtoolset"
    exit 1
}

# Verify binary exists
if (-not (Test-Path "i2plan.exe")) {
    Write-Error "i2plan.exe not found. Build the binary first."
    exit 1
}

# Compile WXS to WIXOBJ
Write-Host "Compiling WXS..."
candle.exe -dVersion=$MsiVersion -arch $Arch installer/windows/i2plan.wxs
if ($LASTEXITCODE -ne 0) {
    Write-Error "candle.exe failed with exit code $LASTEXITCODE"
    exit $LASTEXITCODE
}

# Link WIXOBJ to MSI
Write-Host "Linking MSI..."
light.exe -ext WixUIExtension -out $MsiFile i2plan.wixobj
if ($LASTEXITCODE -ne 0) {
    Write-Error "light.exe failed with exit code $LASTEXITCODE"
    exit $LASTEXITCODE
}

# Clean up intermediate files
Remove-Item i2plan.wixobj -ErrorAction SilentlyContinue
Remove-Item i2plan.wixpdb -ErrorAction SilentlyContinue

Write-Host "MSI package built: $MsiFile"
Get-Item $MsiFile | Select-Object Name, Length, LastWriteTime
