# Releases & workspace layout

## Version history

- **v1.2.1** (2026-09-19) — reset of everything between v1.2.0 and this
  release (that work was never published; its tag and GitHub release were
  removed, so the last public version is v1.2.0). Full settings window
  (Appearance/Animation/Layout/Behavior/Updates/Account), full theme
  customization with a "return to the void" reset, bars↔speedometer
  gauge style with animated morphing, out-of-usage standby mode
  (fade → 5× red flash → countdown strip, expanded view never resized),
  bar resize + set-home, gear icon, X-button quit confirmation,
  notification toggle, check-for-updates with silent update, silent
  close-on-uninstall/upgrade, preview refresh clobber fix.
- **v1.2.0** — previous published release (GitHub tag `v1.2.0`).

## Versioned release folders

Every installer build lands in its own folder named after the version —
no flat piles, no mixing versions:

```
releases/
  v1.2.2/                      ← current release
    ZCode-Usage-HUD-v1.2.2-Setup.exe
  archive/
    v1.2.0/                    ← superseded builds kept locally
      ZCode-Usage-HUD-v1.2.0-Setup.exe
```

The installer script writes straight into the right folder — `OutputDir`
in `installer/zcode-hud.iss` is `..\releases\v{#MyAppVersion}`, so a
version bump re-files the output automatically. `releases/` is
gitignored; installers are attached to GitHub Releases instead of being
tracked.

## Cutting a release

1. Bump `MyAppVersion`, `VersionInfoVersion`, and
   `VersionInfoProductVersion` in `installer/zcode-hud.iss` (all three
   must match), plus `appVersion` in `main.go` and the READMEs.
2. Verify and build:

   ```powershell
   gofmt -l .
   go vet ./...
   go test ./...
   go build -trimpath -ldflags "-H=windowsgui -s -w" .
   ```

3. Compile the installer (Inno Setup 6):

   ```powershell
   & "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe" installer\zcode-hud.iss
   ```

4. The setup exe appears in `releases\v<version>\`.
5. **Payload-freshness check (mandatory):** every file the ISS packs
   (`ZCode-Usage-HUD.exe`, `README.txt`) must be OLDER than the setup
   exe's LastWriteTime. Recompile ISCC if any payload was touched after
   it — a stale setup shipped a corrected binary behind stale docs once
   (and a stale binary behind a fixed source once before that).
6. Move superseded installers into `releases\archive\v<old>\` rather
   than deleting them.

## Dev scripts

`scripts/dev/` holds everything that is not part of the shipped exe:

- `test_api.py`, `probe_full.py`, `probe_params.py`, `inspect_tokens.py`
  — manual API exploration used to reverse-engineer the Z.AI endpoints
  (documented in README.txt; they read local credentials at runtime).
- `settings_check.ps1` — gear click opens the settings window; WM_CLOSE works.
- `settings_clickthrough.ps1` — drives EVERY non-modal settings control
  (steppers, toggle, set/clear home, refresh) via real posted clicks and
  asserts each effect in settings.json.
- `prefs_live_check.ps1` — writes settings.json directly and asserts the
  live bar honors gauge style, home snap and bar-size override.
- `theme_e2e.ps1` — full color loop: theme.json override renders on
  startup, gear -> Return-to-the-void restores default grey live.
- `install_e2e.ps1` — silent fresh install, upgrade over a running HUD,
  and uninstall while running; zero prompts.
- `standby_timeline.ps1` — polls the window rect through the standby
  sequence for animation debugging.
- `offline_check.ps1` — first-run/standalone proof: real-mode app with no
  credentials and with a rejected JWT must stay alive, log clean, and exit
  cleanly; paired with `--dump` assertions for SIGN IN / OFFLINE states.
  NOTE: launching the bare exe without `--hud` means INSTALL, not run.
- `behavior_check.ps1` — pixel-level render audit of the real exe in
  `--preview` mode.
- `cooldown_check.ps1 -mode bar|expanded|interleave` — drives the full cooldown
  lifecycle via `--preview --locked`: bar mode asserts flash-on-bar →
  240x40 strip → eased restore; expanded mode asserts the expanded view
  is NEVER resized or alarmed; interleave drives a refill mid-flash.
- `anim_timing_check.ps1` — polls the real window at 40 ms and asserts the
  collapse/expand transition animates through intermediate rects and
  completes fast (the 300 ms harness cannot see a 160 ms transition).
- `capture_docs.ps1`, `capture_signin.ps1` — README/docs screenshot
  capture harnesses.
- `captures/` — local capture output (gitignored; may contain real
  account data, never commit).
- `graphify/` — knowledge-graph analysis scratch (gitignored).
