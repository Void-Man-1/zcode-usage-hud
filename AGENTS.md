# AGENTS.md — non-obvious learnings for this repo

Facts not recoverable by reading the code: API contracts, environment quirks,
debugging breakthroughs, and stated preferences. Terse by design.

## Repo & workspace

- The git repo root is `zcode-console/`, NOT the enclosing workspace folder
  (`F:\Projects 2\glm hud`, which also holds the separate Codex HUD project).
  Git commands run elsewhere fail with "not a git repository".
- `.gitignore` has a global `*.png` rule because captures may contain real
  account data. Tracked screenshots in `docs/screenshots/` were force-added
  historically — new ones need `git add -f` or a `!docs/screenshots/**` exception.
- Release/versioned-folder convention and installer runbook: see RELEASES.md.
- Any file the ISS packs counts as payload: editing README.txt (docs included)
  after compiling the setup ships a stale artifact — this bit twice. Run the
  RELEASES.md step-5 freshness check after EVERY change, not just exe changes.
- Files change outside your session (user edits, parallel agents): main.go's
  appVersion was bumped externally mid-session. Re-read before editing, respect
  on-disk bumps, and align all 4 version literals per RELEASES.md.
- The GitHub repo (Void-Man-1/zcode-usage-hud) is PUBLIC: fetch old
  versions via raw.githubusercontent.com/<repo>/<tag>/<file> instead of
  cloning (works even when plan mode blocks writes).
- Installer close-on-uninstall contract: the ISS `[Code]` closes a running
  HUD natively (external FindWindowW/PostMessageW + taskkill fallback) in
  BOTH PrepareToInstall and InitializeUninstall; AppMutex/CloseApplications
  are deliberately ABSENT (they prompt). Never call `{app}\exe --quit` from
  the installer — on upgrades {app} still holds the OLD exe without --quit
  (class/exe names verified stable since v1.2.0).
- Before adding close/kill helpers, grep first: closeRunningHUD() already
  covers both window classes with poll-wait + force-kill; --quit is just a
  3-line wrapper over it (an 85-line duplicate trio was written and deleted).
- Silent install/uninstall E2E on this machine is safe and ~5s: run setup
  or unins000.exe with /VERYSILENT /SUPPRESSMSGBOXES while the HUD is up;
  assert exe mtime changed + 0 processes left (Start-Process -Wait).
- A `go test` FAIL that vanishes on rerun is a torn read of a concurrently-edited
  file, not flaky code — rerun `go test -count=1` and re-grep anchors before
  diagnosing.
- Make multi-site edits to main.go one atomic scripted batch: verify every
  anchor has exactly one match, then a single write. Half-applied batches (from
  concurrent edits or mode interruptions) leave dead helpers whose comments
  claim tests that don't exist — grep for callers before trusting new code.

## ZCode API & credential store (hard-won)

- The billing API rejects any request without `X-Device-Mid` (HTTP 400
  `{"code":3001,"msg":"parameter error"}`) but accepts ANY UUID. The HUD mints
  its own device id (`device-id.json`), preferring the ZCode app's telemetry mid.
- Credential files are AES-256-GCM encrypted with a key derived from HOME PATH +
  username (`zcode-credential-fallback:win32:<home>:<username>`, matching the
  upstream app). Changing `USERPROFILE`/`LOCALAPPDATA` invalidates decryption —
  and the loader SILENTLY SKIPS undecryptable values, so the HUD shows a clean
  signed-out state instead of an error. This misleading symptom cost hours.
- When simulating another machine: re-encrypt the session with the sim home's
  key, and use identical backslash path strings everywhere (forward slashes
  derive a different key). Real paths: app credentials in
  `%USERPROFILE%\.zcode\v2\credentials.json`, HUD store in
  `%LOCALAPPDATA%\ZCode Usage HUD\credentials.json` — seeding the wrong one was
  a repeated harness failure.
- OAuth `state` is generated but never verified locally (the poll API doesn't
  echo it) — upstream limitation; local mitigations are the loginGen flow guard
  and the HTTPS-only authorize URL check (unit-tested).
- OAuth init response carries `poll_interval_sec` + `expires_at`; polls must
  never hot-spin (parseOAuthInit rejects interval <1s). The loop polls
  immediately and sleeps BETWEEN polls; retryable errors (408/429/5xx) back
  off one interval.

## Driving the HUD for tests

- `--dump` is the headless E2E lever: it runs the real fetch pipeline against
  the live API and prints the snapshot (email, customer id, device id — never
  tokens). Assert on SignedIn, balances, Error.
- `--preview` fabricates a signed-in state (green 62%-left / amber 19%-left
  buckets) with its own mutex + window class, so it runs alongside an installed
  HUD. It cannot show the signed-out panel — capture that with the real exe
  under a redirected profile containing no credentials.
- A second normal launch exits silently (production mutex `Local\ZCodeUsageHUD-v1`).
  The self-installer shows a confirmation dialog that hangs a synchronous shell —
  run detached, verify via `tasklist` + exe timestamp.
- Pixel audits (see `scripts/dev/behavior_check.ps1`): GDI-capture the real
  `--preview` window, assert token colors ±3 tolerance. Never assume coordinates
  or DPI scale — dump raw pixel runs for ground truth first; every harness
  "failure" was an assumption about scale/offsets, not a product bug.
- "When to refresh" has three interacting owners: the 60s WM_TIMER(2) tick, the
  15s WM_TIMER(3) signed-out watcher, and refreshNow's 1.5s throttle, which now
  RESCHEDULES via AfterFunc (it used to silently drop — post-login refresh
  could stall ~15s). Change them together, never one in isolation.
- fetchSnapshot's signed-in path has no automated coverage: unit tests never
  reach its HTTP layer, and its concurrent fetch goroutines only execute with
  live credentials. Prove changes there with an httptest.Server or a real
  sign-in before shipping.
- The cooldown flash/collapse/expand animation is UI-thread-only and fires only
  on the locked→unlocked bucket transition. `--preview --locked` fabricates
  exhausted buckets (same bucket count as normal preview — a real exhausted
  account keeps its list, so panel size never changes) and auto-refills at 8s.
  Drive the lifecycle with scripts/dev/cooldown_check.ps1 (bar, expanded,
  and interleave modes); never needs a real exhausted account.
- The harness's standard refill lands after the flash ends — the refill-mid-
  flash interleaving is reachable only via mode 'interleave' (env seam:
  ZCODE_PREVIEW_SCRIPT = comma-separated ms publish delays,
  ZCODE_PREVIEW_LOCK_SECS shortens the exhausted beat). A refill DURING the
  5-blink flash must leave the bar at 340x142 — shrinking is a regression.
  Full-lifecycle probes need the DEFAULT 8s lock: overriding
  ZCODE_PREVIEW_LOCK_SECS=4 makes refill beat flash-end and aborts the
  shrink (interleave semantics) — the strip never appears.
- Collapse/expand animate 160ms @ 60fps tick (user: eased but NOT slow).
  scripts/dev/anim_timing_check.ps1 polls the real window at 40ms and
  asserts intermediate rects; the 300ms harness can't see a 160ms
  transition.
- Persistence functions writing under appDataDir() are directly round-trip-
  testable with t.Setenv("LOCALAPPDATA", t.TempDir()) (auto-restored). That test
  caught saveAccentPref missing MkdirAll — it only worked where credentials
  bootstrap had already created the dir; give every new writer there that guard.
- publishSnapshot runs on BACKGROUND fetch goroutines — anything touching
  windows (resize/anim hooks) must hook WM_APP_REFRESH on the UI thread, not
  publishSnapshot, or it moves windows off-thread.
- Long python heredocs get silently truncated by the terminal tool ("here-doc
  delimited by end-of-file", exit 0, nothing ran). Write .py via the file tool
  and execute it; keep heredocs for short one-liners.
- Verify palette restores by multiset-comparing all `rgb(...)` literals against
  `git show HEAD:main.go` — per-site eyeballing misses the sites where a design
  pass merged two original colors into one token.
- Cooldown belongs ONLY to the minimized bar (user, emphatic): the bar pulses
  its background and shrinks to the 240x40 strip when exhausted, eases back on
  refill. The expanded view is NEVER resized or alarmed by cooldown state, and
  "LIMIT REACHED" text never flashes — only the background pulses (both
  corrections came from watching it live, after two wrong implementations).
- Pixel-audit checks must sample fills/frames, never text glyphs: a version bump
  re-renders anti-aliased chip text and turns a color check into a false FAIL
  (cost three audit runs to diagnose).

## Shell & tooling quirks (this machine)

- Terminal tool runs Git Bash: `tasklist //FI` needs double slashes; prefer
  PowerShell `Get-ItemProperty` over `reg query` (MSYS mangles `/s`-style flags).
- PowerShell one-liners through bash need `\$` escaping for `$vars`. Detach
  long-running servers with `Start-Process -WindowStyle Hidden` (the tool's
  BACKGROUND mode is unimplemented). `$home` is read-only in PowerShell.
- Heredocs through the terminal tool mangle backslashes/`\\` — create test files
  with the file-writing tool, not `cat <<'EOF'` appends. Even Go string
  literals inside python heredocs lose a backslash level (build error:
  "unknown escape sequence") — build such strings with chr(92) and always
  gofmt+vet after a batch script.
- Chain the atomic batch INTO the gates with && : a newline after the
  heredoc runs gates even when the batch died, printing a misleading
  green build of unchanged files.
- gofmt REALIGNS var-block spacing after each batch — literal anchors
  hand-aligned to yesterday's formatting miss on the second edit; use
  regex `\s+` anchors for aligned blocks (bit twice).
- `write_doc` in this build is a markdown-note tool, not a file writer —
  use write_file for actual files.
- PowerShell P/Invoke structs: reuse the proven RECT pattern from
  cooldown_check.ps1 (named Left/Top/Right/Bottom + scalar temps); other
  field layouts throw op_Subtraction on the out-struct.
- Inno Setup compiler lives at `%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe`;
  the ISS packs `..\ZCode-Usage-HUD.exe` from the project root — build the exe
  under that name.
- ALWAYS build with `go build -trimpath -ldflags "-H=windowsgui -s -w" .` — a
  plain `go build` links a console subsystem, so every launch (incl. `--startup`)
  opens an empty cmd window that just sits there. Verify with `file` (must say
  `(GUI)`); a GUI exe never has conhost children. Caught once by shipping a
  plain-build exe in the v1.2.2 installer.
- PowerShell trap: `Write-Output` inside a function merges into its return
  value — a check harness reported exit 0 with no PASS/FAIL lines (false pass).
  Emit reports via `Write-Host`, return only booleans.
- write_file occasionally drops its `instructions` field through no fault of
  the content — when it errors on that field, just re-send the same call.
- Freebuff restarts kill all processes (dev/preview servers, the HUD) but leave
  files intact — on resume, restart what you need instead of assuming it survived.
- Freshly-written exes cost ~1.1 s just to START on this machine (real-time AV
  scans them first — a python no-op baseline times the same). Never attribute
  that floor to product latency; compare against a non-Go baseline.
- bash `time ./gui-exe` lies (~30 ms): bash doesn't wait for GUI-subsystem
  processes. Time with PowerShell `Start-Process -Wait -PassThru` + Stopwatch.
- Hand paths between bash and PowerShell in explicit Windows form only (msys
  `/tmp` ≠ `C:\msys64\tmp`) — the mismatch silently produced "file not found"
  probes that looked like product failures.
- python subprocess defaults to cp1252 on this host and crashes decoding `git
  show` output containing smart quotes — always pass `encoding='utf-8',
  errors='replace'`.
- PowerShell script-scope trap: a callback writing `$script:x` while the
  function returns a LOCAL of the same name returns the uninitialized local —
  initialize as `$script:` and return that (cost a "no window appeared" chase).
- checkUnlockNotification calls `go refreshNow()` on unlock — in preview mode
  that replaced the fabricated snapshot with real signed-out API data (panel
  shrank after the scripted refill). Preview paths must not trigger live fetches.
- Plan mode refuses all file writes mid-turn and even read-only-looking git
  probes (`git stash list`); while gated, inspect with `git status`/`git diff`
  only and finish the plan before editing.

## Preview tab (web page verification)

- `register_preview` with `htmlPath` serves ONLY the registered file — sibling
  assets 404. Serve the directory with a real static server and register
  `url` + server `pid`.
- `preview_screenshot` fullPage stitches badly on pages with smooth scrolling or
  ambient animation, and "produced no frames" can be compositor flakiness rather
  than a broken page — assert via `preview_evaluate` (DOM, computed styles,
  image `naturalWidth`) and treat screenshot failures as suspect.
- The preview webview runs with `prefers-reduced-motion: reduce`: a page that
  honors it renders fully static (reveals at opacity 1, no ambient motion) —
  don't misread that as broken CSS.

## User preferences

- Keep the API probe scripts in the repo (documented in README.txt); a Strix
  security scan was declined (no Docker daemon / LLM key) and manual-review
  findings were accepted as-is.
- Every version gets its own folder named for that version — no flat piles of
  build artifacts, ever.
