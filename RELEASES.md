# Releases & workspace layout

## Version history

- **v1.5.0** (2026-09-18) — no-prompt close-on-uninstall/upgrade (`--quit`
  handoff + native in-installer close; AppMutex/CloseApplications removed),
  animations 320→160 ms @ 60 fps. Notice: `RELEASE_NOTES_v1.5.0.md`.
- **v1.4.0** (2026-09-18) — color picker, cooldown mode with LIMIT REACHED
  flash + strip shrink, eased transitions; two in-place patches for the
  flash scope and the unlock-path preview clobber.
- **v1.3.0** (2026-09-18) — HUD works without the ZCode app present,
  faster sign-in, GUI-subsystem build (no cmd window).
- **v1.2.2** — rebuild after a console-subsystem exe shipped in v1.2.2's
  first cut. **v1.2.0** — last release published on GitHub.

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
