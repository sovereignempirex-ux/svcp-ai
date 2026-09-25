# Builds the SVPC AI Android client into a signed APK.
#
# The pipeline is the SDK's own tools rather than Gradle: aapt2 for resources,
# javac for the single activity, d8 for the dex, zipalign and apksigner for the
# finished package. That is a few seconds and needs nothing from Maven, so the
# app can be rebuilt on a machine that has only the SDK installed.
#
# Usage:  pwsh -File android/build-apk.ps1
# Output: android/dist/svpc-ai.apk

param(
  [switch]$Debug
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

function Step($msg) { Write-Host "[$(Get-Date -Format 'HH:mm:ss')] $msg" }

# A native tool writing to stderr is not a failure, and several of these do, so
# the preference is relaxed around each call and the exit code is checked.
function Run-Native($exe, [string[]]$NativeArgs, [string]$cwd = $null) {
  $previous = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try {
    if ($cwd) { Push-Location $cwd; try { $out = & $exe @NativeArgs 2>&1 } finally { Pop-Location } }
    else { $out = & $exe @NativeArgs 2>&1 }
    $code = $LASTEXITCODE
  } finally {
    $ErrorActionPreference = $previous
  }
  if ($code -ne 0) {
    throw "$([System.IO.Path]::GetFileName($exe)) $($NativeArgs -join ' ') failed ($code):`n$($out -join "`n")"
  }
  return $out
}

# ── Toolchain ───────────────────────────────────────────────────
$javaHome = Join-Path $env:LOCALAPPDATA 'AndroidToolchain\jdk'
$sdkHome = Join-Path $env:LOCALAPPDATA 'Android\Sdk'
$buildTools = Join-Path $sdkHome 'build-tools\34.0.0'
$androidJar = Join-Path $sdkHome 'platforms\android-34\android.jar'

# d8 and apksigner ship as batch files that look for java themselves, so the
# environment has to carry these rather than just this script's own variables.
$env:JAVA_HOME = $javaHome
$env:ANDROID_HOME = $sdkHome
$env:ANDROID_SDK_ROOT = $sdkHome
$env:Path = "$javaHome\bin;$env:Path"

foreach ($required in @(
  (Join-Path $javaHome 'bin\javac.exe'),
  (Join-Path $javaHome 'bin\keytool.exe'),
  (Join-Path $buildTools 'aapt2.exe'),
  (Join-Path $buildTools 'd8.bat'),
  (Join-Path $buildTools 'zipalign.exe'),
  (Join-Path $buildTools 'apksigner.bat'),
  $androidJar
)) {
  if (-not (Test-Path $required)) { throw "missing tool: $required" }
}

$here = Split-Path -Parent $MyInvocation.MyCommand.Definition
$out = Join-Path $here 'build'
$dist = Join-Path $here 'dist'
foreach ($dir in @($out, $dist)) {
  if (Test-Path $dir) { Remove-Item $dir -Recurse -Force }
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
}

# ── 1. Resources ────────────────────────────────────────────────
Step 'Compiling resources with aapt2...'
$compiled = Join-Path $out 'res.zip'
Run-Native (Join-Path $buildTools 'aapt2.exe') @('compile', '--dir', (Join-Path $here 'res'), '-o', $compiled) | Out-Null

Step 'Linking resources and the manifest...'
$unsigned = Join-Path $out 'unsigned.apk'
$gen = Join-Path $out 'gen'
$linkArgs = @(
  'link',
  '-o', $unsigned,
  '-I', $androidJar,
  '--manifest', (Join-Path $here 'AndroidManifest.xml'),
  '--java', $gen,
  '--min-sdk-version', '24',
  '--target-sdk-version', '34',
  # Release builds are minified by R8 in a Gradle project. With one class there
  # is nothing to strip, and --minify would only risk breaking reflection on
  # the activity name in the manifest.
  '--no-version-vectors'
)
Run-Native (Join-Path $buildTools 'aapt2.exe') ($linkArgs + @($compiled)) | Out-Null

# ── 2. Java ─────────────────────────────────────────────────────
Step 'Compiling the activity...'
$classes = Join-Path $out 'classes'
New-Item -ItemType Directory -Force -Path $classes | Out-Null

$sources = @(Get-ChildItem (Join-Path $here 'src') -Recurse -Filter *.java | Select-Object -ExpandProperty FullName)
$sources += @(Get-ChildItem $gen -Recurse -Filter *.java | Select-Object -ExpandProperty FullName)
if ($sources.Count -eq 0) { throw 'no java sources found' }

Run-Native (Join-Path $javaHome 'bin\javac.exe') (@(
  '-source', '17', '-target', '17',
  # android.jar goes on the classpath rather than the bootclasspath: the JDK
  # rejects --boot-class-path once the target is 17. d8 desugars the result, so
  # compiling against the JDK's own java.* alongside android.jar is fine.
  '-classpath', $androidJar,
  '-d', $classes,
  '-nowarn'
) + $sources) | Out-Null

# ── 3. Dex ──────────────────────────────────────────────────────
Step 'Converting to dex with d8...'
$dexDir = Join-Path $out 'dex'
New-Item -ItemType Directory -Force -Path $dexDir | Out-Null
$classFiles = @(Get-ChildItem $classes -Recurse -Filter *.class | Select-Object -ExpandProperty FullName)
# d8's --output is a directory or an archive, never a bare .dex path; it writes
# classes.dex inside whatever it is given.
Run-Native (Join-Path $buildTools 'd8.bat') (@(
  '--lib', $androidJar,
  '--min-api', '24',
  '--output', $dexDir
) + $classFiles) | Out-Null

$dex = Join-Path $dexDir 'classes.dex'
if (-not (Test-Path $dex)) { throw "d8 did not produce classes.dex in $dexDir" }

# ── 4. Package ──────────────────────────────────────────────────
# An APK is a zip. Java's jar tool writes one that Android accepts, and it is
# already on the machine.
Step 'Adding the dex to the package...'
Push-Location $dexDir
try {
  Run-Native (Join-Path $javaHome 'bin\jar.exe') @('uf', $unsigned, 'classes.dex') | Out-Null
} finally { Pop-Location }

# ── 5. Sign ─────────────────────────────────────────────────────
# The key lives outside build/ on purpose. Android refuses to update an
# installed app signed with a different key, so wiping the build directory must
# not silently mint a new identity: that would strand every user on the very
# first release. Back this file up somewhere safe.
$keystoreDir = Join-Path $here 'keystore'
New-Item -ItemType Directory -Force -Path $keystoreDir | Out-Null
$keystore = Join-Path $keystoreDir 'svpc.keystore'
$alias = 'svpc'
$storePass = 'svpc-android'
$keyPass = 'svpc-android'

if (-not (Test-Path $keystore)) {
  Step 'No signing key found; generating one.'
  Step "IMPORTANT: back this up now -> $keystore"
  Run-Native (Join-Path $javaHome 'bin\keytool.exe') @(
    '-genkeypair',
    '-keystore', $keystore,
    '-storepass', $storePass,
    '-keypass', $keyPass,
    '-alias', $alias,
    '-keyalg', 'RSA',
    '-keysize', '2048',
    '-validity', '10000',
    '-dname', 'CN=SVPC AI, OU=SVPC AI, O=SVPC AI, L=, ST=, C=',
    '-storetype', 'PKCS12'
  ) | Out-Null
} else {
  Step 'Reusing the existing signing key (android/keystore).'
}

Step 'Aligning...'
$aligned = Join-Path $out 'aligned.apk'
Run-Native (Join-Path $buildTools 'zipalign.exe') @('-p', '-f', '4', $unsigned, $aligned) | Out-Null

Step 'Signing...'
$apk = Join-Path $dist 'svpc-ai.apk'
if ($Debug) {
  $apk = Join-Path $dist 'svpc-ai-debug.apk'
}
Run-Native (Join-Path $buildTools 'apksigner.bat') (@(
  'sign',
  '--ks', $keystore,
  '--ks-pass', "pass:$storePass",
  '--key-pass', "pass:$keyPass",
  '--ks-key-alias', $alias,
  '--min-sdk-version', '24',
  '--out', $apk,
  $aligned
)) | Out-Null

# ── 6. Verify ───────────────────────────────────────────────────
Step 'Verifying the signature...'
$verify = Run-Native (Join-Path $buildTools 'apksigner.bat') @('verify', '--print-certs', $apk)
Step 'Signature verified.'

$badging = Run-Native (Join-Path $buildTools 'aapt2.exe') @('dump', 'badging', $apk)
$line = $badging | Where-Object { $_ -match '^package:' } | Select-Object -First 1
Step "package: $line"

$size = [math]::Round((Get-Item $apk).Length / 1KB, 1)
Step "SUCCESS  $apk  ($size KB)"
