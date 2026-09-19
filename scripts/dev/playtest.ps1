# Careless-user playtest: drives the real windows the way a first user
# muddles through — rapid clicks mid-animation, launching twice, quitting
# when idle, hammering the gear. Uses posted clicks (the app's hit-test
# path). Every failure here is something a user would hit.
$ErrorActionPreference = 'Stop'
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class W32P {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc cb, IntPtr l);
  public delegate bool EnumWindowsProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern int GetClassNameW(IntPtr h, [MarshalAs(UnmanagedType.LPWStr)] StringBuilder sb, int max);
  [DllImport("user32.dll")] public static extern bool PostMessageW(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  public struct R { public int Left, Top, Right, Bottom; }
}
"@
[W32P]::SetProcessDPIAware() | Out-Null

$env:LOCALAPPDATA = 'C:\Users\Void_LLM\AppData\Local\Temp\hudplay'
$dir = Join-Path $env:LOCALAPPDATA 'ZCode Usage HUD'
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Remove-Item (Join-Path $dir 'settings.json') -Force -ErrorAction SilentlyContinue
$exe = Join-Path (Get-Location) 'ZCode-Usage-HUD.exe'
$script:fails = 0
function Fail([string]$m) { Write-Output "FAIL $m"; $script:fails++ }

function Get-Wnd([uint32]$procid, [string]$cls) {
  for ($i = 0; $i -lt 20; $i++) {
    $script:hit = [IntPtr]::Zero
    $cb = [W32P+EnumWindowsProc]{ param($h, $l)
      $p2 = 0
      [W32P]::GetWindowThreadProcessId($h, [ref]$p2) | Out-Null
      if ($p2 -eq $procid) {
        $sb = New-Object System.Text.StringBuilder 256
        [W32P]::GetClassNameW($h, $sb, 256) | Out-Null
        if ($sb.ToString() -eq $cls) { $script:hit = $h }
      }
      return $true
    }
    [W32P]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
    if ($script:hit -ne [IntPtr]::Zero) { return $script:hit }
    Start-Sleep -Milliseconds 500
  }
  return [IntPtr]::Zero
}

function Click([IntPtr]$h, [int]$x, [int]$y) {
  $lp = [IntPtr](($y -shl 16) -bor ($x -band 0xFFFF))
  [W32P]::PostMessageW($h, 0x0201, [IntPtr]1, $lp) | Out-Null
  Start-Sleep -Milliseconds 30
  [W32P]::PostMessageW($h, 0x0202, [IntPtr]0, $lp) | Out-Null
}

function Wait-Height([IntPtr]$h, [int]$want, [int]$tol) {
  $end = (Get-Date).AddMilliseconds(1500)
  while ((Get-Date) -lt $end) {
    $r = New-Object W32P+R
    [W32P]::GetWindowRect($h, [ref]$r) | Out-Null
    if ([Math]::Abs(($r.Bottom - $r.Top) - $want) -le $tol) { return $true }
    Start-Sleep -Milliseconds 25
  }
  return $false
}

# ---------- A: rapid toggle clicks mid-animation ----------
$p = Start-Process -FilePath $exe -ArgumentList '--preview','--collapsed' -PassThru
try {
  $hud = Get-Wnd $p.Id 'ZCodeUsageHUDPreviewV1'
  if ($hud -eq [IntPtr]::Zero) { Fail 'A: window never appeared'; exit 1 }
  if (-not (Wait-Height $hud 142 4)) { Fail 'A: never settled collapsed (142)' }
  # 5 rapid clicks on the title strip, 120ms apart — inside the 160ms anim.
  # Parity from collapsed: expand, collapse, expand, collapse, expand.
  foreach ($i in 1..5) { Click $hud 60 9; Start-Sleep -Milliseconds 120 }
  if (Wait-Height $hud 958 12) { Write-Output 'PASS A: rapid toggles end expanded, window consistent' }
  else {
    $r = New-Object W32P+R; [W32P]::GetWindowRect($hud, [ref]$r) | Out-Null
    Fail ("A: rapid toggles left window at {0}x{1} (want expanded ~958)" -f ($r.Right-$r.Left), ($r.Bottom-$r.Top))
  }
  # window must still respond after the mashing. In EXPANDED geometry only
  # the - button collapses (minRc ~ [W-92..W-46] x [0..43]); the title strip
  # drags the window instead — by design.
  $r = New-Object W32P+R; [W32P]::GetWindowRect($hud, [ref]$r) | Out-Null
  Click $hud ($r.Right - $r.Left - 69) 21; Start-Sleep -Milliseconds 400
  if (Wait-Height $hud 142 4) { Write-Output 'PASS A: window still responsive after mashing' }
  else { Fail 'A: window unresponsive after rapid toggles' }

  # ---------- D: gear click during expand animation ----------
  Click $hud 60 9            # start expanding
  Start-Sleep -Milliseconds 60
  if (-not (Wait-Height $hud 958 12)) { Fail 'D: expand never completed for D' }
  Click $hud 60 9; Start-Sleep -Milliseconds 60
  $r = New-Object W32P+R; [W32P]::GetWindowRect($hud, [ref]$r) | Out-Null
  # gear rect exists in collapsed strip; click anyway mid-flight: app must
  # not crash or corrupt; settings may or may not open (geometry-dependent).
  Click $hud ($r.Right - $r.Left - 90) 9
  Start-Sleep -Milliseconds 500
  if ($p.HasExited) { Fail 'D: app died after gear-during-animation click'; exit 1 }
  Write-Output 'PASS D: gear-mid-animation click survived'

  # ---------- E: rapid gear clicks x3 -> exactly one settings window ----------
  # Gear only exists in collapsed geometry: wait for it, then read a FRESH
  # rect (a stale rect after the mash is what made these clicks dead before).
  if (-not (Wait-Height $hud 142 4)) { Fail 'E: never collapsed for gear test' }
  $r = New-Object W32P+R; [W32P]::GetWindowRect($hud, [ref]$r) | Out-Null
  foreach ($i in 1..3) { Click $hud ($r.Right - $r.Left - 90) 9; Start-Sleep -Milliseconds 100 }
  Start-Sleep -Milliseconds 800
  $count = 0
  $cb2 = [W32P+EnumWindowsProc]{ param($h, $l)
    $p2 = 0
    [W32P]::GetWindowThreadProcessId($h, [ref]$p2) | Out-Null
    if ($p2 -eq $p.Id) {
      $sb = New-Object System.Text.StringBuilder 256
      [W32P]::GetClassNameW($h, $sb, 256) | Out-Null
      if ($sb.ToString() -eq 'ZCodeHUDSettingsWnd' -and [W32P]::IsWindowVisible($h)) { $script:sc = $script:sc + 1 }
    }
    return $true
  }
  $script:sc = 0
  [W32P]::EnumWindows($cb2, [IntPtr]::Zero) | Out-Null
  if ($script:sc -eq 1) { Write-Output 'PASS E: three rapid gear clicks -> exactly one settings window' }
  else { Fail ("E: {0} settings windows after 3 rapid gear clicks" -f $script:sc) }
} finally {
  & $exe --quit 2>$null | Out-Null
  Start-Sleep -Milliseconds 1200
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
}

# ---------- B: double-launch handoff ----------
$p1 = Start-Process -FilePath $exe -ArgumentList '--preview' -PassThru
Start-Sleep -Milliseconds 2500
$p2 = Start-Process -FilePath $exe -ArgumentList '--preview' -PassThru
Start-Sleep -Milliseconds 2500
try {
  if (-not $p2.HasExited) { Fail 'B: second instance did not exit immediately' }
  else { Write-Output 'PASS B: second instance exits immediately (handoff)' }
  if (-not $p1.HasExited) { Write-Output 'PASS B: first instance still alive' }
  else { Fail 'B: first instance died on double-launch' }
  $w = Get-Wnd $p1.Id 'ZCodeUsageHUDPreviewV1'
  if ($w -ne [IntPtr]::Zero) { Write-Output 'PASS B: original window intact' }
  else { Fail 'B: original window lost' }
} finally {
  & $exe --quit 2>$null | Out-Null
  Start-Sleep -Milliseconds 1200
  if (-not $p1.HasExited) { Stop-Process -Id $p1.Id -Force -ErrorAction SilentlyContinue }
}

# ---------- C: --quit with nothing running ----------
$out = & $exe --quit 2>&1
$code = $LASTEXITCODE
if ($code -eq 0) { Write-Output 'PASS C: --quit with nothing running exits 0' }
else { Fail "C: --quit idle exited $code" }

Write-Output ("RESULT failures={0}" -f $script:fails)
exit ([int]($script:fails -gt 0))
