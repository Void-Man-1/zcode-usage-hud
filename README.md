# ZCode Usage HUD

<p align="center">
  <a href="https://github.com/Void-Man-1/zcode-usage-hud/releases/latest/download/ZCode-Usage-HUD-v1.2.0-Setup.exe">
    <img src="https://img.shields.io/badge/Download-latest%20Windows%20installer-2ea44f?style=for-the-badge&logo=windows&logoColor=white" alt="Download the latest Windows installer">
  </a>
</p>

<p align="center">
  <a href="https://github.com/Void-Man-1/zcode-usage-hud/releases"><img src="https://img.shields.io/github/v/release/Void-Man-1/zcode-usage-hud?display_name=tag&sort=semver&logo=github" alt="Latest release"></a>
  <a href="https://github.com/Void-Man-1/zcode-usage-hud/releases"><img src="https://img.shields.io/github/downloads/Void-Man-1/zcode-usage-hud/total?logo=github" alt="Total downloads"></a>
  <a href="https://github.com/Void-Man-1/zcode-usage-hud"><img src="https://img.shields.io/github/repo-size/Void-Man-1/zcode-usage-hud?logo=github" alt="Repository size"></a>
  <img src="https://img.shields.io/badge/platform-Windows%20x86--64-0078d4?logo=windows&logoColor=white" alt="Windows x86-64">
  <img src="https://img.shields.io/badge/Go-1.23-00ADD8?logo=go&logoColor=white" alt="Go 1.23">
</p>

ZCode Usage HUD is a small Windows companion app for keeping an eye on your
ZCode usage without opening the ZCode app itself. It stays on top of other
windows, shows how much of each token pool remains, and can sit neatly above
the installed Codex Usage HUD.

The HUD reads ZCode's billing and account APIs, then turns the response into a
clear view of:

- the connected account, active plan, and sync status;
- daily, weekly, monthly, and one-time token pools;
- used, remaining, and total tokens with color-coded progress bars;
- live reset, activation, expiry, and subscription countdowns;
- accepted promotions that have not activated yet;
- Windows notifications for new promotions and expiring one-time pools.

## Preview

<p align="center">
  <img src="docs/screenshots/zcode-usage-hud-preview.png" alt="ZCode Usage HUD showing separate daily and promotional token buckets" width="680">
</p>

It has its own encrypted local credential store and starts signed out on a
fresh install. It never silently borrows the ZCode app's session; copying that
session is an explicit tray-menu action. Tokens are sent only to the
hard-coded ZCode and Z.AI HTTPS endpoints, and the application does not
collect telemetry or include an update service.

## Getting started

This is a Windows x86-64 Go application.

```powershell
go test ./...
go vet ./...
go build -trimpath -ldflags "-H=windowsgui -s -w" .
```

The resulting executable is `ZCode-Usage-HUD.exe`. For a per-user installer,
build it with Inno Setup 6:

```powershell
ISCC.exe installer\zcode-hud.iss
```

The installer can add the HUD to Windows startup, create a desktop shortcut,
and launch it after installation. Uninstalling removes the program and
shortcuts but leaves the local data directory so a reinstall does not discard
the saved session.

## Useful diagnostics

```powershell
ZCode-Usage-HUD.exe --dump
ZCode-Usage-HUD.exe --logout
```

`--dump` prints the current fetched snapshot as JSON. `--logout` removes the
HUD's local session without changing the ZCode app's session. Runtime logs and
session data live under `%LOCALAPPDATA%\ZCode Usage HUD`.

The Python files in this repository are development and investigation tools
for probing the public ZCode endpoints; they are not compiled into the HUD.

## Privacy and security

Credentials are stored in an AES-GCM `enc:v1` envelope in the HUD's own local
data directory. Log output masks token-shaped values. The app reads the
device identifier from ZCode telemetry state, but does not use the ZCode app's
credentials unless the user explicitly chooses **Use ZCode app's session**.
Deleting `%LOCALAPPDATA%\ZCode Usage HUD` removes the saved session and logs.
