# Android client

SVPC AI for Android is a thin client. The agent, its tools and the session store
run on the computer that started the bridge; the app shows the same interface the
desktop application does.

That split is deliberate. A phone cannot run the build, shell, container or
language-server tools the agent depends on — those need a shell and a filesystem
worth the name — so the tools stay where the code is, and the phone is the
window.

## Using it

On the computer that holds your project:

```
svpc --serve 0.0.0.0 --serve-port 8080
```

It prints a port and a token once. Open the app, enter this machine's address in
the local network together with those two values, and it connects.

The token is saved under the user config directory, so it is shown only on the
first run. Deleting that file revokes every paired device; editing the value in it
issues a new one.

`--serve` refuses to start without a token. The bridge can run commands, write
files and deploy, so an open port is not a small thing, and the failure mode of
getting it wrong is an unauthenticated agent on your network.

To use the desktop window and the phone at the same time, pass both:

```
svpc --gui --serve 0.0.0.0
```

## Building the APK

The build uses the SDK's own tools — `aapt2`, `javac`, `d8`, `zipalign` and
`apksigner` — rather than Gradle. That keeps it to a few seconds and needs
nothing from Maven, so the app can be rebuilt on a machine that only has the SDK.

```
pwsh -File android/build-apk.ps1
```

Output is `android/dist/svpc-ai.apk`, signed with a key generated on first run
and kept in `android/build/`. Both are build output and are not committed.

### Prerequisites

A JDK 17 and the Android SDK, both inside the user profile so no administrator
rights are needed:

| | Path |
|---|---|
| JDK | `%LOCALAPPDATA%\AndroidToolchain\jdk` |
| SDK | `%LOCALAPPDATA%\Android\Sdk` |
| Platform | `android-34` |
| Build tools | `34.0.0` |

Install the command-line tools from
<https://developer.android.com/studio#command-tools>, unzip them so that
`sdkmanager.bat` ends up at `cmdline-tools\latest\bin\`, then:

```
sdkmanager --licenses
sdkmanager --install "platform-tools" "platforms;android-34" "build-tools;34.0.0"
```

## Limits worth knowing

- **Cleartext HTTP.** The bridge is reached over `http` on a local network, so the
  manifest allows cleartext and the app does not complain about it. The token is
  the credential, not TLS. Put it behind a tunnel if the network is not one you
  trust.
- **Landscape and rotation** are handled without recreating the activity, so the
  conversation survives a turn of the phone.
- **No offline mode.** With the computer off, the app cannot reach the agent.
- **One bridge at a time.** The app holds a single address; the setup screen comes
  back from the back button when there is no history left to go back to.
