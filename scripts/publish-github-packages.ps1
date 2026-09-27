<#
.SYNOPSIS
  Publishes the JavaScript client to GitHub Packages.

.DESCRIPTION
  GitHub Packages runs an npm registry, and it authenticates with a GitHub token
  rather than an npm one — so it is reachable while the npm account is suspended,
  and it needs no one-time code.

  GitHub requires a package there to be scoped to the owner, which means the
  name cannot be the bare "svpc-ai-sdk" that npm uses. Rather than change the
  manifest and leave the repository claiming a name it is not published under,
  the package is rebuilt in a scratch directory with the scoped name and
  published from there. The working tree is never modified, so both names stay
  correct: this one here, and the bare one on npm whenever the account is
  usable again.

.EXAMPLE
  $env:GH_TOKEN = "github_pat_..."
  ./scripts/publish-github-packages.ps1 -Version 1.0.0
#>
[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^\d+\.\d+\.\d+$')]
  [string]$Version,
  [string]$Registry = 'https://npm.pkg.github.com',
  # The owner every GitHub package is scoped to. It must be the account that
  # owns this repository, or the registry refuses the scope.
  [string]$Owner = 'sovereignempirex-ux'
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$source = Join-Path $root 'sdk\js'

function Step($msg) { Write-Host "[$(Get-Date -Format 'HH:mm:ss')] $msg" }

# run executes a native command and returns its combined output as one string.
#
# It exists because `$ErrorActionPreference = 'Stop'` turns anything a native tool
# writes to stderr into a terminating error, which happens before any exit-code
# check can look at it. Every native call in this script that is expected to be
# able to fail goes through here, so the failure is reported rather than thrown
# from somewhere unrelated.
function run([scriptblock]$command) {
  $previous = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try {
    return (& $command 2>&1 | Out-String).Trim()
  }
  finally {
    $ErrorActionPreference = $previous
  }
}

$token = $env:GH_TOKEN
if (-not $token) {
  Write-Error @"
GH_TOKEN is not set.

Create a token at https://github.com/settings/tokens with the 'write:packages'
permission on this repository, then:

  `$env:GH_TOKEN = "github_pat_..."

It is used for this run and not stored.
"@
  exit 1
}

$scoped = "@$Owner/svpc-ai-sdk"

# The registry line is always "@owner:registry=…", for an organisation as much as
# for a user account: the "@" is what makes npm read it as a scope at all. Without
# it the line is not applied to a scoped package - it is ignored, the publish goes
# to registry.npmjs.org instead, and the reply is a 404 that never mentions the
# .npmrc. So it is written with the "@" rather than derived from an API call, and
# there would be no point in such a call anyway: /users/<org> resolves on GitHub
# exactly as /users/<user> does, so it could not have told the two apart.
$registryScope = "@$Owner"

# ── Rebuild with the scoped name, in a copy ─────────────────────
# In place would be simpler and wrong: the manifest would then name a registry
# this package is not on, and the next person to publish would have to notice.
$stage = Join-Path ([System.IO.Path]::GetTempPath()) ("svpc-ghpkg-" + [System.Guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Force -Path $stage | Out-Null
try {
  # The test file is copied too, so the tests run against the copy that will be
  # published rather than against the working tree. It is not published: the
  # manifest's `files` lists only client.js and README.md, which is what keeps
  # it out of the tarball.
  foreach ($file in @('client.js', 'client.test.js', 'README.md')) {
    Copy-Item (Join-Path $source $file) (Join-Path $stage $file) -Force
  }

  $manifest = Get-Content (Join-Path $source 'package.json') -Raw | ConvertFrom-Json
  $manifest.name = $scoped
  $manifest.version = $Version
  # The description is what a person reads on the package page, and it should say
  # where the thing came from rather than only what it is. The repository URL says
  # that to anyone reading; the path on this machine says nothing to anyone else.
  $sourceUrl = $manifest.repository.url -replace '^git\+', '' -replace '\.git$', ''
  $manifest.description = "A client for the SVPC AI HTTP API. Published from $sourceUrl."
  [System.IO.File]::WriteAllText(
    (Join-Path $stage 'package.json'),
    ($manifest | ConvertTo-Json -Depth 10)
  )

  # ── The credentials, in a file of their own ────────────────────
  # Written to the temporary directory and removed afterwards, so a GitHub token
  # never lands in the user's own npm configuration. The scope-specific registry
  # line is what makes `npm publish` choose GitHub over npm without a flag.
  #
  # Two details that are each invisible when they go wrong: the scope is written
  # as "${registryScope}" because "$registryScope:registry" parses as a
  # drive-qualified variable and expands to nothing, leaving "=https://…" — a
  # line npm reads as no scope at all. And the two lines are joined before being
  # written: piping an array to Set-Content -NoNewline writes the elements with
  # no separator, so they land on one line and the token line never starts.
  $npmrc = Join-Path $stage '.npmrc'
  $(
    "${registryScope}:registry=$Registry"
    "//npm.pkg.github.com/:_authToken=$token"
  ) -join "`n" | Set-Content $npmrc -NoNewline

  $env:NPM_CONFIG_USERCONFIG = $npmrc

  # Against the GitHub registry explicitly. `npm whoami` on its own asks the
  # default registry, which is npmjs, where this file holds no credential — so it
  # answers ENEEDAUTH and says nothing about whether the GitHub token works.
  $who = run { npm whoami --registry $Registry }
  if ($LASTEXITCODE -ne 0 -or -not $who) {
    Write-Error "npm did not accept the token for ${Registry}: $who"
    exit 1
  }
  Step "who this is: $who"

  # ── Tests run against what will be published, not what is in the tree ──
  # Everything from here to the end of the publish runs inside the scratch copy.
  # It has to: the tests must exercise the copy that will be published, and
  # `npm pack` writes its tarball into the current directory, so running it from
  # the caller's directory would leave the tarball somewhere this script then
  # looks for it in $stage and never finds.
  Push-Location $stage
  try {
    Step 'Running the tests against the copy that will be published'
    $tests = run { node --test }
    # The exit code is the verdict, not the text of the output: the reporter node
    # picks varies with how its output is piped ("# fail 0" under TAP, "fail 0"
    # under the spec reporter), so matching on a line of it either passes when
    # the tests failed or refuses when they passed. A suite that finds no test
    # file also exits non-zero, which is the case this must not wave through —
    # client.test.js is what proves the copy was carried over intact.
    if ($LASTEXITCODE -ne 0) {
      Write-Error "the tests did not pass; nothing was published`n$tests"
      exit 1
    }
    if ($tests -match '(?:#\s*)?pass\s+(\d+)') { Step "  $($Matches[1]) tests passed" }

    Step 'Packing'
    $packed = run { npm pack }
    # The tarball is taken from the directory rather than parsed out of npm's
    # output, which interleaves notice lines with the filename. $stage is created
    # fresh for this run, so whatever is here is what this run packed.
    $tarball = Get-ChildItem -Path $stage -Filter '*.tgz' | Select-Object -Last 1
    if (-not $tarball) {
      Write-Error "npm pack produced nothing`n$packed"
      exit 1
    }
    Step ("  {0}  ({1:N0} bytes)" -f $tarball.Name, $tarball.Length)

    # ── Publish ───────────────────────────────────────────────────
    # The manifest is read from the package root, so the working directory is the
    # scratch copy; that is the whole reason it was built there.
    Step "Publishing $scoped@$Version"
    $out = run { npm publish }
  }
  finally {
    Pop-Location
  }

  if ($out -notmatch '\+\s+' + [regex]::Escape($scoped)) {
    ($out -split "`n" | Where-Object { $_ -notmatch '^\s*$' } | Select-Object -Last 8) |
      ForEach-Object { Write-Host "  $($_.TrimEnd())" }
    Write-Error "npm did not report a success line"
    exit 1
  }

  Step "Published: $scoped@$Version"
  Write-Host ''
  Write-Host 'Install with:'
  Write-Host "  npm install $scoped --registry=$Registry"
  Write-Host ''
  Write-Host 'A consumer needs this in its own .npmrc, once:'
  Write-Host "  ${registryScope}:registry=$Registry"
}
finally {
  $env:NPM_CONFIG_USERCONFIG = $null
  # Removed with the directory it lives in, so the token does not survive the run.
  Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
}
