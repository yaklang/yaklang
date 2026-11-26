param(
    [Parameter(Mandatory = $true)][string]$Directory,
    [Parameter(Mandatory = $true)][string]$Downloads
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
New-Item -ItemType Directory -Force -Path $Directory, $Downloads | Out-Null

function Get-PinnedRelease([string]$Url, [string]$Name, [string]$Digest) {
    $destination = Join-Path $Downloads $Name
    if (-not (Test-Path -LiteralPath $destination)) {
        curl.exe --fail --location --silent --show-error --retry 4 --retry-all-errors --connect-timeout 15 --max-time 180 --output $destination $Url
    }
    if ((Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash -ne $Digest) {
        throw "Release SHA-256 did not match $Name"
    }
    return $destination
}

$memfit = Get-PinnedRelease 'https://github.com/yaklang/yakit/releases/download/v1.1.1-1120-memfit/MemfitAI-1.1.1-1120-windows-amd64.exe' 'memfit-1.1.1-1120.exe' '1be6abd8ed0d7075cf442cc4c0bf8fdd654e0991171d1d87108e4b9860a2f1ef'
$oldPlugin = Get-PinnedRelease 'https://github.com/yaklang/ytray/releases/download/extension-v0.2.11/chrome-enterprise-0.2.11.zip' 'chrome-enterprise-0.2.11.zip' '9c739c1b89db6fe47c6c2e6c317c8e78f88f2c0c53cf5bce70a02e39ab20cffd'
$ytraySetup = Get-PinnedRelease 'https://github.com/yaklang/ytray/releases/download/v0.3.4/YTray-0.3.4-windows-amd64-setup.exe' 'ytray-0.3.4-setup.exe' '86625ecff61c93b07bcc9f07741d3776f9225a214a90b4b80d8192fe866c58b7'
$oldEngine = Get-PinnedRelease 'https://aliyun-oss.yaklang.com/yak/1.4.9-beta7/yak_windows_amd64.exe' 'yak-1.4.9-beta7.exe' '7d838b661fe642e8c4dd7fa14d90b26ddc4208d31ef97cd418a9ccf578d6e4b4'
$alpha = Get-PinnedRelease 'https://aliyun-oss.yaklang.com/yak/1.4.9-alpha1123/yak_windows_amd64.exe' 'yak-1.4.9-alpha1123.exe' 'b5d4b3d1db710cb37ee176991cad6f8c90d6cf48d27d55c322a95997ad295e3d'
Copy-Item -LiteralPath $oldEngine -Destination (Join-Path $Directory 'legacy-yak.exe')
Copy-Item -LiteralPath $alpha -Destination (Join-Path $Directory 'alpha1123-yak.exe')

$zip = (Get-Command 7z.exe -ErrorAction Stop).Source
& $zip e -y "-o$Directory" $memfit '$PLUGINSDIR\app-64.7z' | Out-Null
& $zip e -y "-o$Directory" (Join-Path $Directory 'app-64.7z') 'resources\app.asar' | Out-Null
Move-Item -LiteralPath (Join-Path $Directory 'app.asar') -Destination (Join-Path $Directory 'memfit.asar')
Expand-Archive -LiteralPath $oldPlugin -DestinationPath (Join-Path $Directory 'old-plugin')

# Install the released host on the disposable Windows runner. Inno Setup's
# silent mode skips opening the tray app, and the extension is embedded in it.
$ytrayHome = Join-Path $Directory 'ytray'
$installer = Start-Process -FilePath $ytraySetup -ArgumentList @(
    '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/NOCLOSEAPPLICATIONS',
    ('/DIR="' + $ytrayHome + '"')
) -Wait -PassThru -WindowStyle Hidden
if ($installer.ExitCode -ne 0) { throw "YTray fixture installer exited with $($installer.ExitCode)" }
$hostExecutable = Join-Path $ytrayHome 'YTray.exe'
$assembly = [System.Reflection.Assembly]::LoadFile($hostExecutable)
$stream = $assembly.GetManifestResourceStream('YTray.Assets.BundledExtension.yakit-browser-agent.zip')
if (-not $stream) { throw 'YTray release did not contain its Browser Agent package' }
$newZip = Join-Path $Directory 'new-plugin.zip'
try {
    $output = [System.IO.File]::Create($newZip)
    try { $stream.CopyTo($output) } finally { $output.Dispose() }
} finally {
    $stream.Dispose()
}
Expand-Archive -LiteralPath $newZip -DestinationPath (Join-Path $Directory 'new-plugin')

$hostName = 'com.yaklang.ytray_engine'
$hostManifest = Join-Path $Directory "$hostName.json"
@{
    name = $hostName
    description = 'Pinned YTray browser connection fixture'
    path = $hostExecutable
    type = 'stdio'
    allowed_origins = @('chrome-extension://mcnaombmlombekhbonfndagbcfhmoail/')
} | ConvertTo-Json | Set-Content -LiteralPath $hostManifest -Encoding utf8NoBOM
foreach ($browser in @('Google\Chrome', 'Google\ChromeForTesting', 'Chromium')) {
    $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey("Software\$browser\NativeMessagingHosts\$hostName")
    try { $key.SetValue('', $hostManifest) } finally { $key.Dispose() }
}
