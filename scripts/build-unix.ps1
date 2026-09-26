<#
.SYNOPSIS
  Builds the Linux and macOS archives that the release publishes.

.DESCRIPTION
  These are cross-compiled from any host, so this runs on the machine that
  publishes, which for this project is Windows. The archive names, the flat
  layout and the contents are the ones .goreleaser.yml and the install script
  agree on, and internal/release checks that they still agree.

  The names are written out rather than derived, because a name that is computed
  in two places is a name that can disagree; the check that compares this script
  against the release configuration is what keeps the two honest.

.PARAMETER Version
  Stamped into the binaries with -ldflags, and reported by --version. This is the
  tag without its leading "v", which is what goreleaser puts in {{.Version}}.

.PARAMETER OutDir
  Where the archives are written. The publish script reads this directory.

.EXAMPLE
  scripts/build-unix.ps1 -Version 1.0.0
#>
[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [string]$Version,
  # $PSScriptRoot is the scripts directory; the repository is its parent.
  [string]$OutDir = (Join-Path (Split-Path -Parent $PSScriptRoot) 'dist')
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Step($msg) { Write-Host "[$(Get-Date -Format 'HH:mm:ss')] $msg" }

# The four targets the release publishes. A name here is a name users download.
$targets = @(
  @{ os = 'linux'; arch = 'x86_64'; goos = 'linux'; goarch = 'amd64' },
  @{ os = 'linux'; arch = 'arm64';  goos = 'linux'; goarch = 'arm64' },
  @{ os = 'mac';   arch = 'x86_64'; goos = 'darwin'; goarch = 'amd64' },
  @{ os = 'mac';   arch = 'arm64';  goos = 'darwin'; goarch = 'arm64' }
)

# ── The Windows resource must not reach a Unix build ─────────────
# resource.syso carries the icon and the version resource for the Windows
# executable. Go links any .syso it finds into every build, so an unsuffixed one
# changes a Linux binary too — and only on a host that happened to generate it.
# The same source would then build two different binaries, depending on which
# machine ran the build, and checksums.txt would be describing neither.
#
# Naming it with the platform suffix scopes it to Windows, and Go leaves it out
# of these builds. Verified by building twice: the result is reproducible, and it
# stops being reproducible the moment an unsuffixed copy is added.
foreach ($stray in @('resource.syso')) {
  if (Test-Path $stray) {
    Write-Error @"
$stray is in the repository root, so it would be linked into the Linux and macOS
binaries as well as the Windows one, and only on a host that generated it.

Regenerate it with the platform suffix instead:

  goversioninfo -icon internal/gui/assets/svpc.ico -o resource_windows.syso versioninfo.json

and delete the unsuffixed file. Nothing else has to change: Go embeds a .syso
only for the platform its name names, and *.syso is already ignored by git.
"@
    exit 1
  }
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$stage = Join-Path ([System.IO.Path]::GetTempPath()) ("svpc-unix-" + [System.Guid]::NewGuid().ToString('N').Substring(0, 8))
# The binaries and the two extra files share one directory, which is what the
# archive is made from. Building them in the same place they are packed from
# means there is no copy step to get wrong.
$pack = Join-Path $stage 'pack'
New-Item -ItemType Directory -Force -Path $pack | Out-Null
try {
  # ── Build ──────────────────────────────────────────────────────
  $built = @()
  foreach ($t in $targets) {
    $name = "svpc-$($t.os)-$($t.arch)"
    $exe = Join-Path $pack $name

    $env:GOOS = $t.goos
    $env:GOARCH = $t.goarch
    # Pure Go: no cgo, so one binary runs anywhere and cross-compiles cleanly.
    # This is the flag set .goreleaser.yml uses, so what is built here is what
    # goreleaser would build.
    $env:CGO_ENABLED = '0'

    $ldflags = "-s -w -X github.com/svpc-ai/svpc/internal/version.Version=$Version"
    $out = go build -ldflags $ldflags -o $exe . 2>&1
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $exe)) {
      throw "building $name failed:`n$($out | Out-String)"
    }
    $size = (Get-Item $exe).Length
    Step ("{0,-22} {1,10:N0} bytes" -f $name, $size)
    $built += @{ target = $t; name = $name; path = $exe; size = $size }
  }
  $env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'

  # ── Check each binary is what it claims to be ──────────────────
  # A cross-compiled archive that is the wrong architecture installs cleanly and
  # then fails to run, on the one platform nobody tested. The header says what
  # was actually produced, so it is read rather than assumed.
  Step 'Checking each binary against its own name'
  foreach ($b in $built) {
    $head = New-Object byte[] 20
    $fs = [System.IO.File]::OpenRead($b.path)
    try { $null = $fs.Read($head, 0, 20) } finally { $fs.Close() }

    $goos = $b.target.goos
    $goarch = $b.target.goarch

    if ($goos -eq 'linux') {
      # 0x7F 'E' 'L' 'F', then EI_CLASS, then data, then e_machine at 18.
      if ($head[0] -ne 0x7F -or $head[1] -ne 0x45 -or $head[2] -ne 0x4C -or $head[3] -ne 0x46) {
        throw "$($b.name) is not an ELF binary; it starts $([BitConverter]::ToString($head[0..3]))"
      }
      # e_machine is a 16-bit little-endian value at offset 18: 0x3E is x86-64,
      # 0xB7 is AArch64. Printed low byte first, which is the order it is stored
      # in, and compared as text, because these do not all fit in an Int32 and
      # PowerShell would wrap the larger ones.
      $machine = '{0:X2}{1:X2}' -f $head[18], $head[19]
      $want = if ($goarch -eq 'amd64') { '3E00' } else { 'B700' }
      $kind = 'ELF'
    }
    else {
      # A 64-bit Mach-O: the magic is CF FA ED FE for a little-endian one, and
      # cputype follows at offset 4. 01000007 is x86-64, 0100000C is arm64. Again
      # read low byte first.
      $magic = [BitConverter]::ToString($head[0..3])
      if ($magic -ne 'CF-FA-ED-FE' -and $magic -ne 'FE-ED-FA-CF' -and $magic -ne 'CE-FA-ED-FE') {
        throw "$($b.name) is not a Mach-O binary; its magic is $magic"
      }
      $machine = '{0:X2}{1:X2}{2:X2}{3:X2}' -f $head[4], $head[5], $head[6], $head[7]
      $want = if ($goarch -eq 'amd64') { '07000001' } else { '0C000001' }
      $kind = 'Mach-O'
    }

    if ($machine -ne $want) {
      throw "$($b.name) was built as $goarch but the header says $machine"
    }

    # The version has to be in there, or --version reports nothing and the
    # install script's "already installed" check misreads it as a build.
    $bytes = [System.IO.File]::ReadAllBytes($b.path)
    $text = [System.Text.Encoding]::ASCII.GetString($bytes)
    if (-not $text.Contains($Version)) {
      throw "$($b.name) does not contain the version string $Version"
    }
    Step ("  {0,-22} {1,-7} {2}" -f $b.name, $kind, $machine)
  }

  # ── Package ───────────────────────────────────────────────────
  # Flat, with the binary at the root, because the install script untars into a
  # temporary directory and then looks for the binary by name. A directory level
  # would make it report that the archive was wrong.
  #
  # The binary in an archive is named after the platform, which is what
  # goreleaser's own layout would give, so the two are not interchangeable
  # member for member. Only the licence and the readme are shared, and they are
  # named the same in every one.
  Step 'Packaging'
  $written = @()
  foreach ($b in $built) {
    $archive = Join-Path $OutDir "$($b.name).tar.gz"
    if (Test-Path $archive) { Remove-Item $archive -Force }

    foreach ($extra in @('LICENSE', 'README.md')) {
      Copy-Item (Join-Path $root $extra) (Join-Path $pack $extra) -Force
    }

    # The member is the bare command name, which is the name the install script
    # looks for once it has unpacked; the archive's own name says the platform.
    Rename-Item (Join-Path $pack $b.name) 'svpc' -Force
    $tar = & tar -czf $archive -C $pack svpc LICENSE README.md 2>&1
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $archive)) {
      throw "packaging $($b.name) failed:`n$($tar | Out-String)"
    }
    Rename-Item (Join-Path $pack 'svpc') $b.name -Force

    # What is actually inside, rather than what was asked for.
    $members = @(& tar -tzf $archive 2>$null) | Where-Object { $_ }
    $expected = @('svpc', 'LICENSE', 'README.md')
    $missing = @($expected | Where-Object { $_ -notin $members })
    $extra = @($members | Where-Object { $_ -notin $expected })
    if ($missing -or $extra) {
      throw "$($b.name).tar.gz holds the wrong files; missing [$($missing -join ', ')] unexpected [$($extra -join ', ')]"
    }

    $size = (Get-Item $archive).Length
    Step ("{0,-28} {1,10:N0} bytes  {2}" -f (Split-Path $archive -Leaf), $size, ($members -join ' '))
    $written += $archive
  }

  # ── What is left to check ─────────────────────────────────────
  # The names are not re-derived here. They are the four names written at the top
  # of this file, and internal/release holds the rule that they are the names the
  # release publishes and the install script asks for, as a test that runs. A
  # second, weaker copy of that rule inside the build script would be one more
  # thing to keep in step rather than one fewer.

  Write-Host ''
  Step "Wrote $($written.Count) archive(s) to $OutDir"
  foreach ($w in $written) { Write-Host "  $(Split-Path $w -Leaf)" }
  Write-Host ''
  Write-Host 'Attach them with:'
  Write-Host "  `$env:SVPC_GITHUB_TOKEN = '...'; ./scripts/publish-release.ps1"
}
finally {
  Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
}
