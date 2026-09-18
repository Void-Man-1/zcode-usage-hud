# ZCode Usage HUD v1.5.0 — update notice

Covers everything since the v1.2.0 release (v1.3.0 → v1.5.0).
Baseline for this diff: GitHub tag `v1.2.0` vs release v1.5.0 — 15 files changed, +1179/−166 lines.

## 🛠 Patches & fixes

**Uninstalling (or upgrading) no longer blocks on a running HUD — v1.5.0.**
Previously the installer detected the running app through an `AppMutex` check and warned
"app is still running," forcing you to exit the HUD by hand first. Now the installer closes
the HUD itself, with no prompt at any step:

- `AppMutex` / `CloseApplications` were removed from the setup, so the "close these
  applications?" dialog can never appear — for uninstall *and* for install-over-upgrade.
- Before touching any file, the installer posts a graceful `WM_CLOSE` to the HUD window
  (the same message the tray → Exit uses, so state is saved cleanly), waits briefly, and
  only if something refused to close runs a `taskkill /F` fallback by image name.
- The close runs in both `PrepareToInstall` (upgrades) and `InitializeUninstall` (removal),
  and talks to the window class directly rather than exec'ing the installed exe — so it
  works even when the installed build predates the new `--quit` flag.
- The app itself gained a matching headless `--quit` flag (graceful close of any running
  instance) for scripts and manual use.

Verified end-to-end on a live machine: upgrade-installed and uninstalled while the HUD was
running — both finish in ~5 s with zero prompts, zero leftover processes.

**The HUD works without the ZCode desktop app present — v1.3.0.**
Starting or signing in no longer fails when ZCode desktop is not installed. The HUD keeps
its own encrypted credential store and does a browser sign-in on its own; copying the
ZCode app's session remains an explicit tray action ("Use ZCode app's session"), never
automatic.

**No more stray cmd window at startup — v1.2.2/v1.3.0.**
One earlier installer shipped a console-subsystem build, so launching the HUD (including
the Windows-startup autostart) opened an empty cmd window that just sat there. Builds are
GUI-subsystem again and verified (`file` reports `(GUI)`, no conhost children ever spawn).

**Stale-installer class closed — v1.4.x.**
Two releases had shipped payloads older than their sources (a corrected binary behind a
stale installer, then corrected docs behind a fresh binary). The release runbook now has a
mandatory payload-freshness check: the setup exe must be newer than every file it packs.

**Animation/transitions twice as fast — v1.5.0.**
Collapse/expand went from 320 ms at ~30 fps to 160 ms at ~60 fps, keeping the ease-in-out
curve (nothing snaps; it's just snappy). Proven by a 40 ms-resolution probe that captures
intermediate window rects mid-transition.

**Cooldown sequence corrected — v1.4.0 patches.**
The "limit reached" alarm originally painted across the entire expanded panel; it was
re-scoped to the minimized bar only (see below), the text was made steady (only the
background pulses), and a refill arriving mid-flash can no longer leave the bar stuck small.

## ✨ New features

- **Cooldown mode (v1.4.0, corrected in patches).** When every token bucket is exhausted,
  the minimized bar flashes **LIMIT REACHED** five times slowly (background pulse only —
  the text stays steady), then eases down to the compact 240×40 bar so it stops hogging
  space when there are no percentage bars to display. On refill it eases back open to the
  full gauges/buckets panel. The expanded view is never resized or alarmed.
- **Color picker (v1.4.0).** Tray menu → "HUD color…": the standard Windows color dialog,
  applied live and persisted (`accent.json`); with no pick the HUD keeps the original
  palette, which is byte-exact vs earlier versions (pixel-audited).
- **`--preview --locked` + scripted publishes (v1.4.x).** A preview variant that fabricates
  an exhausted account (plus env-driven publish schedules) so the full cooldown lifecycle —
  flash, shrink, refill-restore — can be driven and asserted headlessly. Also the lever
  behind the new `scripts/dev/cooldown_check.ps1` and `anim_timing_check.ps1` harnesses.

## 🧹 Quality of life

- Faster sign-in path (v1.3.0).
- The cooldown bar restores your expanded panel's position after refill (v1.4.x).
- Quieter upgrades: the installer closes the HUD for you instead of asking (v1.5.0).
- Versioned release folders (`releases/vX.Y.Z/`, superseded builds archived) and a
  written release runbook with a freshness gate (v1.4.x).
- Docs updated to match reality: the old "the installer will not replace a running copy"
  troubleshooting note now reads "the installer closes a running HUD for you" (v1.5.0).
