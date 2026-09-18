ZCode Usage HUD v1.5.0
======================

OPEN SOURCE AND INDEPENDENT
This project is released under the MIT License. You may fork, modify, build,
redistribute, and contribute to it. It is not affiliated with, sponsored by,
or endorsed by ZCode or Z.AI.

Always-on-top ZCode usage HUD that reads ZCode's own data sources:

CONNECTED / SYNCED
  dot + email + active plan name, footer with sync time.

STATUS PANEL
  AVAILABLE (green) or TOKENS EXHAUSTED (orange) computed from the
  real token buckets. Locked state counts down to the next recurring
  refill (bucket PeriodEnd); unlock triggers a tray notification.

TOKEN BUCKET CARDS (one per billing/balance bucket)
  GLM-5.3 / GLM-5.3-Flash (today: Start Plan = 3,000,000 + 5,000,000):
  - % used (used/total) / % left (remaining/total — from
    remaining_units, not 100-used) + used/left/total token counts
  - green/yellow/red progress bar (75%/90% thresholds)
  - Card period comes from the entitlement (DAILY/WEEKLY/ONE-TIME/...)
    matched by entitlement_id — never hardcoded. Buckets sort by
    entitlement priority, and the headlined plan is the active one, not
    Plans[0]. Two buckets may share a model name (e.g. a promo
    ONE-TIME pool next to the DAILY pool of the same model).
  - RESETS IN live countdown to PeriodEnd + exact local refill time

PROMO GRANTS (accepted but not spendable yet)
  Z.AI drops promotions as a new plan + entitlement whose bucket only
  appears once the grant activates (effective_at — the 100M one-time
  offers work this way). While it is pending the HUD shows a PROMO
  GRANTS card with the grant size, models, a live "STARTS IN"
  countdown to effective_at, and the exact activation time; the
  collapsed panel lists a pending row too (blue PROMO qualifier + amber
  activation countdown). When the bucket appears the card is replaced
  by the normal bucket card with usage. The promo plan also gets a
  line in the account panel.

PROMOTION + EXPIRY NOTIFICATION (works with the ZCode app off)
  Every refresh diffs plans[]/entitlements against a persisted set of
  seen grant keys (%LOCALAPPDATA%\ZCode Usage HUD\promotions-seen.json)
  and raises one Windows tray notification per new grant ("ZCode
  promotion received — <plan>: 100,000,000 GLM-5.3-Flash tokens
  (one-time) · starts 15:04"). Keys persist across restarts, so a drop
  that happened while the HUD was closed still notifies on the next
  launch, and the check uses the HUD's own Google session — the ZCode
  app never needs to run. First-ever run baselines silently (an
  existing Start Plan never toasts).
  One-time pools are also watched for EXPIRY: when a promo bucket with
  tokens still in it enters its final 24 hours the HUD warns once per
  (bucket, expiry) — "ZCode promo pool expiring — 67,131,608 tokens
  unused · expires Wed 16 Sep 03:00 local (in 07:00:52) — unused
  tokens are lost" — persisted in expiry-notified.json, so a pool
  never silently dies overnight.

ACCOUNT / PLAN / TOKENS PANEL
  buckets, plan name, granted/used/remaining totals for today,
  one line per plan the account reports (status + expiry/starts
  countdown — promotions show up here), paid-plan subscription with
  renewal countdown (empty for Start-Plan-only accounts), customer
  ID, org/project, coding-plan quota note, plan-expiry countdown
  (EndsAt), server time with clock drift (shown when ≥ 2s) +
  device-mid prefix.

FOOTER
  Connected state, sync time, impersonated ZCode version, bucket names.

DATA SOURCES (all stats the app could find)
  zcode.z.ai /api/v1/zcode-plan/billing/balance?app_version=3.11.2
    (zcodejwttoken Bearer + ZCode headers: User-Agent, X-ZCode-App
    Version, X-Platform, X-Device-Mid, ...):
    server_time, plans[] (user_plan_id, plan_id, name, description,
    priority, status, starts_at, ends_at, entitlements[] with
    grant_units, period, capabilities, effective_at), balances[]
    (bucket_id, show_name, meter, unit_type, total/used/remaining/
    available_units, period_start/end, expires_at).
  api.z.ai /api/biz/customer/getCustomerInfo (oauth access token):
    id, customerNumber, masked email, userType, channel, org/project,
    isNewUser, createTime.
  api.z.ai /api/biz/subscription/list: coding-plan subscriptions
    (empty for Start-Plan-only accounts).
  api.z.ai /api/monitor/usage/quota/limit: coding-plan quota level or
    the "no coding plan" note shown in the HUD. A Start-Plan-only
    account has no coding plan — the HUD shows the friendly line
    "Start Plan only — no coding plan" instead of the raw Chinese
    server message.
  Local session: %LOCALAPPDATA%\ZCode Usage HUD\credentials.json holds
  the HUD's OWN login (same enc:v1 envelope + key layout as ZCode, but
  a separate file). A fresh install therefore starts signed out — it
  never attaches to the ZCode app's session by itself. deviceMid is
  the ZCode app's telemetry id when the app is present, otherwise a
  HUD-owned UUID persisted in device-id.json (the billing API rejects
  requests without it — any id is accepted, so the HUD works app-free).
  "Use ZCode app's session" (tray menu) copies the app's tokens over
  explicitly on request.

SIGN-IN (real Google flow, no dead ends)
  "Sign in with Google" drives the ZCode app's own CLI OAuth polling
  flow end to end, reverse-engineered from the ZCode bundle:
  - POST zcode.z.ai/api/v1/oauth/cli/init {provider:"zai"} with a
    random poll token returns flow_id, authorize_url, expires_at,
    poll_interval_sec;
  - the HUD opens the Z.AI authorize page in your browser (Google
    sign-in lives there) with the desktop redirect bridge;
  - it polls .../cli/poll/<flow> until ready/failed/expired
    (network/5xx/408/429 retries, other 4xx aborts — same as the app);
  - on ready it resolves the business token via
    api.z.ai/api/auth/z/login and stores the exact credential keys
    the ZCode app uses (oauth:zai:access_token, zcodejwttoken,
    oauth:zai:user_info, oauth:active_provider) in AES-GCM enc:v1.
    The auth call carries Content-Type + x-request-id only, mirroring
    the app (extra ZCode headers break this endpoint); failures log
    a secret-masked response to hud.log.
  The button/menu item toggles to "Cancel sign-in" while waiting, and
  a 15s watcher notices changes to the HUD's own credential store, including
  an explicit "Use ZCode app's session" import.

SIGN-OUT
  Right-click menu "Sign out" cancels any pending login, deletes the
  HUD's five session keys, and resets the HUD to the sign-in panel.
  The ZCode app's separate session is never touched.
  ZCode-Usage-HUD.exe --logout does the same headlessly.

BEHAVIOR
  Minimize collapses to a compact panel listing EVERY pool as a
  stacked full-width row, no abbreviations: the full model name on the
  left with its quota kind right-aligned — "PROMO" in blue for one-time
  grant pools, "DAILY"/"WEEKLY"/"MONTHLY" in gray for regular plan
  quotas — and a green/amber/red progress bar spanning the full width
  with that bucket's remaining % on the second line. Pending promotion
  grants get the same row with an amber "STARTS IN" countdown and the
  grant size instead of a usage bar. The □/× controls live in their own
  slim top strip so the rows stay full-width. The panel grows with the
  row count (four rows + a "+N more" row at most), the left accent bar
  tracks the account-wide remaining share (amber < 25%, red < 10%),
  and clicking anywhere on the panel expands it. When every bucket is
  exhausted the panel becomes a large countdown to the recurring refill
  (one-time pools never count as refill sources). Signed-out state
  distinguishes "SIGN IN" from "OFFLINE" (API unreachable).
  Close exits.
  Companion-aware stacking everywhere: the collapsed strip parks directly
  above a detected companion HUD window instead of overlapping it, and the snapped
  expanded panel stacks above the companion too — both default to the
  same notification-area corner. A 1s restack follows the companion
  when it appears, moves, expands, collapses, or exits in either mode;
  dragging the expanded panel unhooks it (Snap re-hooks), and touching
  edges count as settled so the position is stable.
  Tray icon + right-click menu: Refresh now, Snap, HUD color..., Open ZCode
  folder, Start with Windows (compact), Exit. "HUD color..." opens the standard
  Windows color picker; the choice recolors the HUD panels, applies immediately,
  and persists in %LOCALAPPDATA%\ZCode Usage HUD\accent.json until changed.
  1s countdown repaint, 60s
  API refresh, single instance per session, topmost snap to
  notification-area corner, --preview mode with fake buckets.

COOLDOWN MODE (minimized bar only)
  When every bucket is exhausted (computeUnlock locked), the MINIMIZED bar
  pulses its background under a steady "LIMIT REACHED" five times slowly
  (550ms on / 550ms off), then eases down (ease-in-out cubic, 160ms) to the
  original Codex-HUD bar size (240 wide, taskbar height) — there are no
  percentage bars left to display. When buckets refill, it eases back open to
  fit all gauges and buckets. The EXPANDED view is never resized or alarmed by
  cooldown state.
  Collapse/expand transitions are animated everywhere — no snapping, and the
  restacker never fights a running animation.

BUILD
  Target: Windows x86-64 GUI PE
  go test ./...
  go vet ./...
  go build -trimpath -ldflags "-H=windowsgui -s -w" .
  INSTALL WIZARD (Inno Setup 6, per-user, no admin):
    ISCC.exe installer\zcode-hud.iss
    -> installer\output\ZCode-Usage-HUD-v1.2.0-Setup.exe
  The wizard installs to %LOCALAPPDATA%\Programs (matching the app's
  own installDir), offers "Start with Windows (compact mode)" (writes
  the same HKCU Run value the tray-menu toggle manages) and a desktop
  shortcut, and can launch the HUD on finish. Uninstalling removes the
  program, shortcuts and registry entries but preserves the HUD's data
  folder, so a reinstall keeps the signed-in session.

DIAGNOSTICS
  ZCode-Usage-HUD.exe --dump prints the live fetched snapshot as JSON
  without opening a window. Stacking decisions are logged to
  %LOCALAPPDATA%\ZCode Usage HUD\hud.log ("stack collapse/restack").
  ZCode-Usage-HUD.exe --preview opens a synthetic UI preview without
  signing in. ZCode-Usage-HUD.exe --logout clears the HUD session, and
  --uninstall removes a self-installed copy.
  --quit closes an already-running instance (used by the uninstaller).

PRIVACY & SECURITY
  Where your data lives:
    - The HUD's session is %LOCALAPPDATA%\ZCode Usage HUD\
      credentials.json (AES-256-GCM "enc:v1" envelope, with a key derived
      from the current Windows user's profile and username; any process
      running as your Windows user can read it, exactly like the app's
      own store). A fresh install starts signed out — it never attaches
      to the ZCode app's session and never reads or writes
      %USERPROFILE%\.zcode\v2 except reading telemetry-state.json for
      the device id and, on explicit "Use ZCode app's session",
      copying that session into the HUD's own store.
    - promotions-seen.json / expiry-notified.json hold plan and
      entitlement IDs only — no tokens.
    - hud.log masks token-shaped values automatically.
  Where your data goes: tokens are sent only to zcode.z.ai and
    api.z.ai over HTTPS; every URL is a hardcoded constant. The app
    sends no telemetry of its own and has no update channel.
  Uninstall removes the program, Start Menu shortcuts and registry
    entries, but leaves the HUD's data folder (delete
    %LOCALAPPDATA%\ZCode Usage HUD to wipe sessions and logs).
  ZCode-Usage-HUD.exe --dump prints the live snapshot (including your
    email, customer id and device id) to stdout — a manual diagnostic,
    not used by the app.

DEV SCRIPTS (not part of the shipped exe)
  scripts/dev/test_api.py, probe_full.py, probe_params.py and
  inspect_tokens.py
  are manual exploration scripts used to reverse-engineer the Z.AI
  endpoints. They read credentials from the local machine at runtime,
  hit hardcoded public API hosts only (zcode.z.ai, api.z.ai), and are
  never compiled into the binary. Security scanners flag their URL
  handling as SSRF by pattern; there is no request-forgery surface —
  every URL is a hardcoded public constant and nothing listens locally.
