# Settings click-through harness: drives EVERY non-modal settings control
# through the real window with posted WM_LBUTTONUPs (painter-space client
# coords, the exact hit-test path) and asserts each observable effect in
# settings.json. Skips color chips (modal dialog), check-updates (modal
# box) and account (tray menu).
$ErrorActionPreference = 'Stop'
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class W32C {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr a, int x, int y, int w, int ht, uint f);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc cb, IntPtr l);
  public delegate bool EnumWindowsProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern int GetClassNameW(IntPtr h, [MarshalAs(UnmanagedType.LPWStr)] StringBuilder sb, int max);
  [DllImport("user32.dll")] public static extern bool PostMessageW(IntPtr h, uint m, IntPtr w, IntPtr l);
  public struct R { public int Left, Top, Right, Bottom; }
}
"@
[W32C]::SetProcessDPIAware() | Out-Null

$env:LOCALAPPDATA = 'C:\Users\Void_LLM\AppData\Local\Temp\hudprof2'
$dir = Join-Path $env:LOCALAPPDATA 'ZCode Usage HUD'
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Remove-Item (Join-Path $dir 'settings.json') -Force -ErrorAction SilentlyContinue
$exe = Join-Path (Get-Location) 'ZCode-Usage-HUD.exe'
$script:fails = 0

function Get-Wnd([uint32]$procid, [string]$cls) {
  for ($i = 0; $i -lt 20; $i++) {
    $script:hit = [IntPtr]::Zero
    $cb = [W32C+EnumWindowsProc]{ param($h, $l)
      $p2 = 0
      [W32C]::GetWindowThreadProcessId($h, [ref]$p2) | Out-Null
      if ($p2 -eq $procid) {
        $sb = New-Object System.Text.StringBuilder 256
        [W32C]::GetClassNameW($h, $sb, 256) | Out-Null
        if ($sb.ToString() -eq $cls) { $script:hit = $h }
      }
      return $true
    }
    [W32C]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
    if ($script:hit -ne [IntPtr]::Zero) { return $script:hit }
    Start-Sleep -Milliseconds 500
  }
  return [IntPtr]::Zero
}

function Click([IntPtr]$h, [int]$x, [int]$y) {
  $lp = [IntPtr](($y -shl 16) -bor ($x -band 0xFFFF))
  [W32C]::PostMessageW($h, 0x0201, [IntPtr]1, $lp) | Out-Null
  Start-Sleep -Milliseconds 50
  [W32C]::PostMessageW($h, 0x0202, [IntPtr]0, $lp) | Out-Null
}

# Clicks one style button and polls the HUD rect through the morph.
# Returns intermediate heights (strictly between the two endpoints) and
# the settled final height. Empty intermediates means the window SNAPPED,
# which is the defect this harness exists to catch.
function Invoke-StyleClick([IntPtr]$sw, [IntPtr]$hud, [int[]]$btn, [int]$fromH, [int]$toH) {
  $lo = [Math]::Min($fromH, $toH); $hi = [Math]::Max($fromH, $toH)
  $mids = @(); $final = -1
  Click $sw $btn[0] $btn[1]
  $end = (Get-Date).AddMilliseconds(800)
  while ((Get-Date) -lt $end) {
    $r = New-Object W32C+R
    [W32C]::GetWindowRect($hud, [ref]$r) | Out-Null
    $h = $r.Bottom - $r.Top
    if (($h -gt ($lo + 4)) -and ($h -lt ($hi - 4))) { $mids += $h }
    if ([Math]::Abs($h - $toH) -le 2) { $final = $h; break }
    Start-Sleep -Milliseconds 10
  }
  return @{ mids = $mids; final = $final }
}

function Wait-Settings([string]$jsonPath) {
  for ($i = 0; $i -lt 20; $i++) {
    if (Test-Path $jsonPath) { return (Get-Content $jsonPath -Raw | ConvertFrom-Json) }
    Start-Sleep -Milliseconds 150
  }
  return $null
}

function Assert-Field([object]$s, [string]$field, [object]$want, [string]$tag) {
  $got = $s.$field
  if ("$got" -ne "$want") {
    Write-Output ("FAIL {0}: {1}={2} want {3}" -f $tag, $field, $got, $want)
    $script:fails++
  } else {
    Write-Output ("PASS {0}: {1}={2}" -f $tag, $field, $got)
  }
}

# Control coordinates (client space of the 560-wide settings window).
# APPEARANCE: style buttons at y 52..82 (bars {20..270}, gauge {290..530}).
# ANIMATION section: title 424..462, anim row 462..486; MINIMIZED BAR
# title 518..556, barW row 556..580, barH row 588..612 (set/clear home
# rows are scanned, not fixed); BEHAVIOR title 728..766, notif 766..790,
# refresh 798..822. Steppers: minus (300..330), plus (388..418).
$animMinus = @(315, 474); $animPlus = @(403, 474)
$barWMinus = @(315, 568); $barWPlus = @(403, 568)
$barHMinus = @(315, 600); $barHPlus = @(403, 600)
$notif     = @(32, 778)
$refMinus  = @(315, 810); $refPlus = @(403, 810)

$p = Start-Process -FilePath $exe -ArgumentList '--preview','--collapsed' -PassThru
try {
  $hud = Get-Wnd $p.Id 'ZCodeUsageHUDPreviewV1'
  if ($hud -eq [IntPtr]::Zero) { Write-Output 'FAIL no hud'; exit 1 }
  for ($i = 0; $i -lt 20; $i++) {
    $r = New-Object W32C+R
    [W32C]::GetWindowRect($hud, [ref]$r) | Out-Null
    if (($r.Right - $r.Left) -lt 500) { break }
    Start-Sleep -Milliseconds 500
  }
  $lp = [IntPtr]((9 -shl 16) -bor (($r.Right - $r.Left - 90) -band 0xFFFF))
  [W32C]::PostMessageW($hud, 0x0201, [IntPtr]1, $lp) | Out-Null
  Start-Sleep -Milliseconds 50
  [W32C]::PostMessageW($hud, 0x0202, [IntPtr]0, $lp) | Out-Null
  $sw = Get-Wnd $p.Id 'ZCodeHUDSettingsWnd'
  if ($sw -eq [IntPtr]::Zero) { Write-Output 'FAIL settings did not open'; exit 1 }
  Start-Sleep -Milliseconds 700
  $jp = Join-Path $dir 'settings.json'

  # STYLE BUTTONS (bars height 142 -> gauge 74 in preview). Each click
  # must morph the collapsed bar through eased intermediate rects — an
  # instant snap is the req-7 defect — and persist the style.
  $m = Invoke-StyleClick $sw $hud @(410, 67) 142 74
  if ($m.mids.Count -gt 0) { Write-Output ("PASS gauge morph mid-flight samples: {0} (e.g. {1})" -f $m.mids.Count, ($m.mids -join ',')) }
  else { Write-Output 'FAIL gauge morph snapped: no intermediate rect observed'; $script:fails++ }
  if ([Math]::Abs($m.final - 74) -le 2) { Write-Output "PASS gauge morph settles at $($m.final)" }
  else { Write-Output "FAIL gauge morph final height $($m.final), want 74"; $script:fails++ }
  Assert-Field (Wait-Settings $jp) 'style' 1 'gauge style click'

  $m = Invoke-StyleClick $sw $hud @(145, 67) 74 142
  if ($m.mids.Count -gt 0) { Write-Output ("PASS bars morph mid-flight samples: {0} (e.g. {1})" -f $m.mids.Count, ($m.mids -join ',')) }
  else { Write-Output 'FAIL bars morph snapped: no intermediate rect observed'; $script:fails++ }
  if ([Math]::Abs($m.final - 142) -le 2) { Write-Output "PASS bars morph settles at $($m.final)" }
  else { Write-Output "FAIL bars morph final height $($m.final), want 142"; $script:fails++ }
  Assert-Field (Wait-Settings $jp) 'style' 0 'bars style click'

  # ANIMATION steppers: 160 -> -3x20 -> 100 -> +20 -> 120.
  foreach ($c in @($animMinus, $animMinus, $animMinus, $animPlus)) { Click $sw $c[0] $c[1]; Start-Sleep -Milliseconds 120 }
  Assert-Field (Wait-Settings $jp) 'animMs' 120 'anim steppers'

  # Bar width dead-band: minus from auto(0) stays 0; plus steps by 20.
  Click $sw $barWMinus[0] $barWMinus[1]; Start-Sleep -Milliseconds 120
  Assert-Field (Wait-Settings $jp) 'barW' 0 'barW minus at auto floor'
  Click $sw $barWPlus[0] $barWPlus[1]; Start-Sleep -Milliseconds 120
  Assert-Field (Wait-Settings $jp) 'barW' 20 'barW plus from auto'
  foreach ($i in 1..5) { Click $sw $barWPlus[0] $barWPlus[1]; Start-Sleep -Milliseconds 120 }
  Assert-Field (Wait-Settings $jp) 'barW' 120 'barW plus x5'

  # Bar height dead-band + stepping.
  Click $sw $barHMinus[0] $barHMinus[1]; Start-Sleep -Milliseconds 120
  Assert-Field (Wait-Settings $jp) 'barH' 0 'barH minus at auto floor'
  foreach ($i in 1..7) { Click $sw $barHPlus[0] $barHPlus[1]; Start-Sleep -Milliseconds 120 }
  Assert-Field (Wait-Settings $jp) 'barH' 28 'barH plus x7'

  # Set home: move the HUD to (150,150) first, then click. The two
  # buttons' y positions depend on the whole content flow above them,
  # so scan y until the click lands (real hit-test path all the way).
  [W32C]::SetWindowPos($hud, [IntPtr]::Zero, 150, 150, 200, 120, 0x0004) | Out-Null
  Start-Sleep -Milliseconds 300
  $setHitY = -1
  foreach ($y in 630..700) {
    Click $sw 280 $y; Start-Sleep -Milliseconds 100
    $s = Wait-Settings $jp
    if ($null -ne $s -and $null -ne $s.homeX) { $setHitY = $y; break }
  }
  if ($setHitY -lt 0) { Write-Output 'FAIL set home never fired scanning y 630..700'; $script:fails++ }
  else { Write-Output ("PASS set home (y={0})" -f $setHitY) }
  $s = Wait-Settings $jp
  Assert-Field $s 'homeX' 150 'set home x'
  Assert-Field $s 'homeY' 150 'set home y'

  # Clear home (scan below the set-home row); homeX must disappear.
  $clearHit = $false
  foreach ($y in ($setHitY + 30)..($setHitY + 90)) {
    Click $sw 280 $y; Start-Sleep -Milliseconds 100
    $s = Wait-Settings $jp
    if ($null -eq $s.homeX) { $clearHit = $true; break }
  }
  if (-not $clearHit) { Write-Output 'FAIL clear home never fired'; $script:fails++ }
  else { Write-Output 'PASS clear home removed homeX' }

  # Notification toggle both ways.
  Click $sw $notif[0] $notif[1]; Start-Sleep -Milliseconds 120
  Assert-Field (Wait-Settings $jp) 'notifications' 'False' 'notif off'
  Click $sw $notif[0] $notif[1]; Start-Sleep -Milliseconds 120
  Assert-Field (Wait-Settings $jp) 'notifications' 'True' 'notif on'

  # Refresh floor at 2, then step up.
  foreach ($i in 1..4) { Click $sw $refMinus[0] $refMinus[1]; Start-Sleep -Milliseconds 120 }
  Assert-Field (Wait-Settings $jp) 'refreshSecs' 2 'refresh floor'
  Click $sw $refMinus[0] $refMinus[1]; Start-Sleep -Milliseconds 120
  Assert-Field (Wait-Settings $jp) 'refreshSecs' 2 'refresh floor holds'
  Click $sw $refPlus[0] $refPlus[1]; Start-Sleep -Milliseconds 120
  Assert-Field (Wait-Settings $jp) 'refreshSecs' 3 'refresh plus'

  Write-Output ("RESULT failures={0}" -f $script:fails)
  exit ([int]($script:fails -gt 0))
} finally {
  & $exe --quit 2>$null | Out-Null
  Start-Sleep -Milliseconds 800
  if (Get-Process -Id $p.Id -ErrorAction SilentlyContinue) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
}
