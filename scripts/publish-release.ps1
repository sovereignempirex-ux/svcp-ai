# Uploads the Windows executable and the Android package as a GitHub release.
#
# Needs a token with the `repo` scope. It is read from SVPC_GITHUB_TOKEN, or from
# the first argument, and is never written to disk or echoed.
#
# Usage:
#   $env:SVPC_GITHUB_TOKEN = "ghp_..."
#   pwsh -File scripts/publish-release.ps1
#
#   pwsh -File scripts/publish-release.ps1 -Tag v1.0.0 -DryRun

param(
  [string]$Tag = "v1.0.0",
  [string]$Repo = "sovereignempirex-ux/svcp-ai",
  [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

function Step($msg) { Write-Host "[$(Get-Date -Format 'HH:mm:ss')] $msg" }

# ── Checksums ───────────────────────────────────────────────────
# Written before anything is uploaded, and from the same list that is about to
# be published, so it cannot describe a different set of files. A hand-kept
# checksum file goes stale the moment an asset is replaced, and a stale one is
# worse than none: it looks authoritative while being wrong.
function Write-Checksums($paths, $target) {
  $lines = @()
  foreach ($p in $paths) {
    # GitHub rewrites unsafe characters in an asset name, so the entry has to
    # carry the name as it will be downloadable.
    $name = (Get-UploadName (Split-Path $p -Leaf))
    $lines += ("{0}  {1}" -f (Get-FileHash $p -Algorithm SHA256).Hash.ToLower(), $name)
  }
  # A trailing newline, because that is what sha256sum writes and what a
  # verifier expects.
  [System.IO.File]::WriteAllText($target, ($lines -join "`n") + "`n")
  return $lines.Count
}

# GitHub rewrites characters it considers unsafe in an asset name — a space
# becomes a dot, so "SVPC AI.exe" is stored and downloaded as "SVPC.AI.exe".
# Comparing against the local name would never match, and a re-run would push
# the whole file again for no reason.
function Get-UploadName([string]$name) { return ($name -replace '[^A-Za-z0-9._-]', '.') }

$token = $env:SVPC_GITHUB_TOKEN
if (-not $token) {
  Write-Error @"
SVPC_GITHUB_TOKEN is not set.

Create a token at https://github.com/settings/tokens with the `repo` scope, then:

  `$env:SVPC_GITHUB_TOKEN = "ghp_..."

The token is only used for this script and is not stored.
"@
  exit 1
}

# ── What to publish ─────────────────────────────────────────────
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Definition)

# The signed executable and the Android package are always here. The Unix
# archives are whatever .goreleaser.yml last produced, gathered into dist/;
# they are optional so a release can ship a subset rather than fail.
$assets = @(
  (Join-Path $root 'SVPC AI.exe'),
  (Join-Path $root 'android\dist\svpc-ai.apk')
)
$archives = @(Get-ChildItem (Join-Path $root 'dist') -Filter '*.tar.gz' -ErrorAction SilentlyContinue)
$assets += @($archives | Select-Object -ExpandProperty FullName)

$required = @($assets | Where-Object { $_ -notlike '*.tar.gz' })
$missing = $required | Where-Object { -not (Test-Path $_) }
if ($missing) {
  Write-Error "missing: $($missing -join ', ')`nBuild them first: go build -o 'SVPC AI.exe' . and android/build-apk.ps1"
  exit 1
}

# The checksums cover every asset, so the file is part of the set and is
# uploaded alongside them.
$checksumPath = Join-Path $root 'dist\checksums.txt'
$count = Write-Checksums $assets $checksumPath
$assets += $checksumPath

foreach ($a in $assets) {
  $size = [math]::Round((Get-Item $a).Length / 1MB, 2)
  Step "$(Split-Path $a -Leaf)  ($size MB)"
}
Step "$count checksums written to dist\checksums.txt"

if ($DryRun) {
  Step "Dry run: would publish $Tag to $Repo with $($assets.Count) assets."
  exit 0
}

# ── API helpers ─────────────────────────────────────────────────
$headers = @{
  Authorization = "Bearer $token"
  Accept        = "application/vnd.github+json"
  'X-GitHub-Api-Version' = '2022-11-28'
  'User-Agent'  = 'svpc-release'
}

function Invoke-GitHub($method, $uri, $body) {
  $previous = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try {
    $params = @{
      Method      = $method
      Uri         = "https://api.github.com$uri"
      Headers     = $headers
      ContentType = 'application/json'
    }
    if ($body) { $params.Body = ($body | ConvertTo-Json -Depth 5) }
    $response = Invoke-RestMethod @params
  } catch {
    $detail = $_.ErrorDetails.Message
    throw "GitHub $method $uri failed: $($_.Exception.Message) $detail"
  } finally {
    $ErrorActionPreference = $previous
  }
  return $response
}

# ── Release ─────────────────────────────────────────────────────
$notes = @"
## SVPC AI $Tag

One agent, four ways to reach it. The executable and the archives below are the
same program; what changes is where the interface is drawn.

| Platform | File | Getting it |
|---|---|---|
| Windows | `SVPC.AI.exe` | Download and run. Double-click opens the window; `svpc` from a shell opens the terminal UI. |
| Linux | `svpc-linux-x86_64.tar.gz`, `svpc-linux-arm64.tar.gz` | `./install`, or unpack and put `svpc` on your PATH. |
| macOS | `svpc-mac-x86_64.tar.gz`, `svpc-mac-arm64.tar.gz` | `./install`, or unpack and put `svpc` on your PATH. |
| Android | `svpc-ai.apk` | Install it, then run `svpc --serve 0.0.0.0` on the computer. |

`checksums.txt` carries a SHA-256 for every file here.

### Using it

    svpc                          the terminal interface
    svpc --gui                    the desktop window
    svpc --serve 0.0.0.0          serve the same interface to a phone
    svpc -p "explain this repo"    one non-interactive prompt

The Android app is a client. The agent, its tools and its sessions run on the
machine that started the bridge, which is why a phone can drive a build but
cannot run one: those tools need a shell and a filesystem.

`--serve` refuses to start without a token. The bridge runs commands, writes
files and deploys, so it is not something to leave unauthenticated on a network.

### Note on the Windows file

Windows Smart App Control blocks executables that are not signed by a
certificate Microsoft trusts, so this build carries a self-signed one. If
Windows refuses to run it, that is why. The Unix builds are unaffected: they
carry no signature and are governed by your own policy instead.
"@

$existing = $null
try { $existing = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/tags/$Tag" -Headers $headers } catch { }

if ($existing) {
  Step "Release $Tag already exists; refreshing its notes and uploading to it."
  # The notes are the only part a re-run can meaningfully change, and leaving a
  # stale install table on a published release is worse than rewriting it.
  $release = Invoke-GitHub 'PATCH' "/repos/$Repo/releases/$($existing.id)" @{
    name        = $existing.name
    body        = $notes
    draft       = $existing.draft
    prerelease  = $existing.prerelease
  }
} else {
  Step "Creating release $Tag..."
  $release = Invoke-GitHub 'POST' "/repos/$Repo/releases" @{
    tag_name    = $Tag
    name        = "SVPC AI $Tag"
    body        = $notes
    draft       = $false
    prerelease  = $false
    target_commitish = 'main'
  }
}

Step "Uploading $($assets.Count) asset(s)..."

# GitHub rewrites characters it considers unsafe in an asset name — a space
# becomes a dot, so "SVPC AI.exe" is stored and downloaded as "SVPC.AI.exe".
# Comparing against the local name would therefore never match, and a re-run
# would push the whole file again for no reason. Everything below uses the name
# GitHub will actually hold.
function Get-UploadName([string]$name) { return ($name -replace '[^A-Za-z0-9._-]', '.') }

foreach ($path in $assets) {
  $name = Get-UploadName (Split-Path $path -Leaf)
  $localSize = (Get-Item $path).Length

  # A re-run should not push 61 MB again. The release's own record is
  # authoritative; the download URL may still be serving a cached copy.
  $existing = (Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/tags/$Tag" -Headers $headers).assets |
    Where-Object { $_.name -eq $name }

  if ($existing -and $existing.size -eq $localSize) {
    Step "  skip  $name  already published, $localSize bytes"
    continue
  }

  if ($existing) {
    # A different size means it was rebuilt, so the old one has to go first:
    # GitHub will not overwrite an asset in place.
    Step "  replace $name  ($($existing.size) -> $localSize bytes)"
    Invoke-RestMethod -Method Delete -Uri "https://api.github.com/repos/$Repo/releases/assets/$($existing.id)" -Headers $headers | Out-Null
    Start-Sleep -Seconds 2
  }

  $url = "https://uploads.github.com/repos/$Repo/releases/$($release.id)/assets?name=$([uri]::EscapeDataString($name))"
  Step "  upload $name"

  $previous = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try {
    # The upload endpoint wants the raw bytes, not a JSON body.
    Invoke-RestMethod -Method Post -Uri $url -Headers $headers -InFile $path -ContentType 'application/octet-stream' | Out-Null
  } catch {
    $ErrorActionPreference = $previous
    throw "upload of $name failed: $($_.Exception.Message) $($_.ErrorDetails.Message)"
  } finally {
    $ErrorActionPreference = $previous
  }
}

Step "Verifying what was published..."
# A download URL is served from a CDN that can still be holding the previous
# asset, so the check reads the release's own record rather than the bytes the
# CDN hands back. A size that disagrees means the upload has not landed yet.
$problems = @()
foreach ($path in $assets) {
  $name = Get-UploadName (Split-Path $path -Leaf)
  $localSize = (Get-Item $path).Length

  $published = $null
  for ($attempt = 1; $attempt -le 5; $attempt++) {
    $current = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/tags/$Tag" -Headers $headers
    $published = $current.assets | Where-Object { $_.name -eq $name }
    if ($published) { break }
    Start-Sleep -Seconds 2
  }

  if (-not $published) {
    $problems += "$name is not attached to the release"
  } elseif ($published.size -ne $localSize) {
    $problems += "$name is $localSize bytes locally but $($published.size) on the release"
  } else {
    Step "  ok  $name  $localSize bytes"
  }
}

if ($problems) {
  Write-Warning @"
A published asset does not match the local file:
  $($problems -join "`n  ")

If a file was replaced, give it a moment and re-run. The bytes on GitHub can be
correct while a cached download URL still serves the previous one.
"@
}

Step "Done: https://github.com/$Repo/releases/tag/$Tag"
