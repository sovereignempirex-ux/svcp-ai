# Publishing the clients

Both clients are versioned independently of the agent: a fix to a client does not
need a new agent release, and a new agent release does not force a client bump.

Neither is published automatically. A name on npm and a name on PyPI cannot be
reused once taken, so a publish is a decision and not a build step.

## What is published, and what is not

| Registry | Name | Contents |
|---|---|---|
| PyPI | `svpc-ai` | `sdk/python` — the client, and nothing else |
| npm | `svpc-ai-sdk` | `sdk/js` — `client.js`, `README.md`, `package.json` |

Neither package contains its tests. A client is what someone imports; a test file
in a distribution is dead weight in someone's dependency tree, and the tests run in
CI against the working tree anyway.

## Before either publish

The tests, because they are the only thing that knows the clients work:

```bash
go test ./...                      # the agent
node --test sdk/js                 # 18 tests, against a real HTTP server
python -m unittest discover -s sdk/python   # 22 tests, likewise
```

Then check the artefacts before they are public, not after. A distribution can
never be taken back, only replaced by a new version.

```bash
# Python
cd sdk/python
python -m build --no-isolation
python -m twine check dist/*
#   and read the wheel's contents: it should hold svpc/__init__.py, three
#   dist-info files, and nothing else.

# JavaScript
cd sdk/js
npm pack --dry-run
#   three files: client.js, README.md, package.json
```

`twine check` does not catch everything. It passed a distribution that PyPI
rejected, because the licence was declared in the metadata and absent from the
archive; the registry's own complaint was the only thing that found it.

## PyPI

Straightforward. The token needs the `pypi.org` project scope.

```bash
cd sdk/python
python -m build --no-isolation

export TWINE_USERNAME="__token__"
export TWINE_PASSWORD="pypi-..."
python -m twine upload --repository-url https://upload.pypi.org/legacy/ dist/*
```

Verify by installing what was published, into a directory of its own, and running
the test suite against that copy rather than the working tree. Anything else tests
the code you already had.

## npm

Two things about npm that PyPI does not have, and both have bitten this project.

**Two-factor authentication.** With 2FA on, a granular access token cannot publish
on its own: the registry answers

```
403 Forbidden - Two-factor authentication or granular access tokens are not
supported for this operation
```

A direct-capable token plus a one-time code does work:

```bash
cd sdk/js
npm publish --otp=123456
```

**Staging tokens.** A token created for the staging flow can only publish to a
staging area, and only for a name that already exists:

```
403 Forbidden - Cannot publish "svpc-ai-sdk": this token can only publish to a
staging area, and "svpc-ai-sdk" does not exist yet. Create it first with a
direct-capable token, then use `npm stage publish`. (E_STAGE_REQUIRED)
```

So a staging token is useless for a first publish, whatever it is named. The name
is created by the first direct publish, and staging tokens take over afterwards.

**Scoped names.** `@svpc-ai/client` was the intended name and cannot be published:
a scope belongs to an organisation, and this account is not in one. Publishing to
it fails with a permission error that looks nothing like a missing organisation.
Either create the organisation at npmjs.com and add the account, or publish
unscoped. The name in `package.json` and the names in the two READMEs have to
agree with whichever was chosen, or the documentation sends people to a package
that cannot exist.

## Publishing npm from a release asset

The tarball is attached to every release as `svpc-ai-sdk-<version>.tgz`, so the
publish does not depend on a working tree and the artefact is downloadable whether
or not it ever reached the registry.

```bash
npm publish svpc-ai-sdk-1.0.0.tgz --otp=123456
```

The name printed by `npm pack` and the version in `package.json` are the same, and
`sha1sum` of the tarball is the shasum npm reports, so a publish can be checked
before and after:

```bash
npm pack
sha1sum svpc-ai-sdk-*.tgz
```

`scripts/publish-release.ps1` gathers both `*.tar.gz` and `*.tgz` from `dist/`, and
writes a SHA-256 for each into `checksums.txt`, so the release page is enough to
verify what was uploaded.


## Verifying afterwards

Both registries answer for themselves, and the answer is the one to trust — not the
output of the upload command.

```bash
# PyPI
curl -s https://pypi.org/pypi/svpc-ai/json | python -m json.tool | head -40

# npm
npm view svpc-ai-sdk
npm view svpc-ai-sdk dist.tarball
```

Then install the published package into a clean environment and use it. Anything
short of that is a claim rather than a check.
