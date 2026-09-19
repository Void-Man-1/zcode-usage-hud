# Behavioral harness for bar-only cooldown: watches the real window rect
# over 12.5s. Mode 'bar': --preview --collapsed --locked must flash on the
# bar, shrink to 240x40, then ease back. Mode 'expanded': --preview --locked
# must NEVER resize and NEVER show the alarm. Mode 'interleave': publishes a
# snapshot mid-flash and refills mid-flash (lock window 4s) — the bar must
# blink, NEVER become 240x40, and regain 340x142 when the flash ends.
param([string]$mode = 'bar')
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class W32L {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr lp);
  public delegate bool EnumProc(IntPtr hWnd, IntPtr lp);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT rect);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetWindowText(IntPtr hWnd, StringBuilder sb, int max);
  public struct RECT { public int Left, Top, Right, Bottom; }
}
"@
[W32L]::SetProcessDPIAware() | Out-Null

$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$exe = Join-Path $root 'ZCode-Usage-HUD.exe'
if (-not (Test-Path $exe)) { Write-Output "FAIL exe missing: $exe"; exit 2 }

function Find-HudWindow([uint32]$targetPid) {
  $script:found = [IntPtr]::Zero
  $cb = [W32L+EnumProc]{
    param($h, $lp)
    if ([W32L]::IsWindowVisible($h)) {
      $p = [uint32]0
      [W32L]::GetWindowThreadProcessId($h, [ref]$p) | Out-Null
      if ($p -eq $targetPid) {
        $sb = New-Object System.Text.StringBuilder 128
        [W32L]::GetWindowText($h, $sb, 128) | Out-Null
        if ($sb.Length -gt 0) { $script:found = $h; return $false }
      }
    }
    return $true
  }
  [W32L]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
  return $script:found
}

function Test-RedFrame($bmp) {
  for ($x = 0; $x -lt $bmp.Width; $x += 2) {
    for ($y = 0; $y -lt $bmp.Height; $y += 2) {
      $c = $bmp.GetPixel($x, $y)
      if ([Math]::Abs($c.R - 196) -le 14 -and [Math]::Abs($c.G - 57) -le 14 -and [Math]::Abs($c.B - 67) -le 14) { return $true }
    }
  }
  return $false
}

$argList = @('--preview','--locked')
if ($mode -eq 'bar' -or $mode -eq 'interleave') { $argList += '--collapsed' }
if ($mode -eq 'interleave') {
  $env:ZCODE_PREVIEW_SCRIPT = '1400,4500'
  $env:ZCODE_PREVIEW_LOCK_SECS = '4'
}
$watchSecs = 12.5
if ($mode -eq 'interleave') { $watchSecs = 14 }
$p = Start-Process -FilePath $exe -ArgumentList $argList -PassThru
$failures = 0
$initialW = 0; $initialH = 0
$sawRed = $false
$sawStrip = $false
$sawRestore = $false
$sawRegained = $false
$anyResizeFromExpanded = $false
$timeline = New-Object System.Collections.ArrayList

try {
  $h = [IntPtr]::Zero
  $deadline = (Get-Date).AddSeconds(8)
  while ((Get-Date) -lt $deadline -and $h -eq [IntPtr]::Zero) {
    Start-Sleep -Milliseconds 150
    $h = Find-HudWindow $p.Id
  }
  if ($h -eq [IntPtr]::Zero) { Write-Output 'FAIL no window appeared'; exit 3 }

  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  while ($sw.Elapsed.TotalSeconds -lt $watchSecs) {
    $r = New-Object W32L+RECT
    [W32L]::GetWindowRect($h, [ref]$r) | Out-Null
    $w = $r.Right - $r.Left; $ht = $r.Bottom - $r.Top
    $t = [math]::Round($sw.Elapsed.TotalSeconds, 1)
    [void]$timeline.Add("t=${t}s ${w}x${ht}")
    # Skip mid-animation frames: 'initial' is the first settled rect
    # (unchanged across two consecutive 150ms samples).
    if ($initialW -eq 0) {
      if ($lastW -eq $w -and $lastH -eq $ht) { $initialW = $w; $initialH = $ht }
      else { $lastW = $w; $lastH = $ht; continue }
    }

    if ($mode -ne 'expanded') {
      # alarm blinks ON the minimized bar itself
      if (-not $sawRed -and $t -ge 0.7 -and $t -le 6.2 -and ($w -le 410)) {
        $bmp = New-Object System.Drawing.Bitmap $w, $ht
        $g = [System.Drawing.Graphics]::FromImage($bmp)
        $g.CopyFromScreen($r.Left, $r.Top, 0, 0, (New-Object System.Drawing.Size($w, $ht)))
        if (Test-RedFrame $bmp) { $sawRed = $true }
        $g.Dispose(); $bmp.Dispose()
      }
      if ($w -ge 238 -and $w -le 242 -and $ht -ge 38 -and $ht -le 42) { $sawStrip = $true }
      if ($sawStrip -and $t -gt 8.6 -and $w -gt 300) { $sawRestore = $true }
      if ($w -ge 335 -and $w -le 345 -and $ht -ge 138 -and $ht -le 146 -and $t -gt 6.5) { $sawRegained = $true }
    } else {
      # expanded view: ANY rect change after the first sample is a violation
      if ($initialW -gt 0 -and $t -ge 0.7 -and $t -le 9.5 -and ($w -ne $initialW -or $ht -ne $initialH)) { $anyResizeFromExpanded = $true }
      # and no alarm anywhere: sample the bottom-right corner region
      if (-not $sawRed -and $t -ge 0.7) {
        $bmp = New-Object System.Drawing.Bitmap 120, 60
        $g = [System.Drawing.Graphics]::FromImage($bmp)
        $g.CopyFromScreen($r.Right - 120, $r.Bottom - 60, 0, 0, (New-Object System.Drawing.Size(120, 60)))
        if (Test-RedFrame $bmp) { $sawRed = $true }
        $g.Dispose(); $bmp.Dispose()
      }
    }
    Start-Sleep -Milliseconds 250
  }
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item Env:ZCODE_PREVIEW_SCRIPT -ErrorAction SilentlyContinue
  Remove-Item Env:ZCODE_PREVIEW_LOCK_SECS -ErrorAction SilentlyContinue
}

$timeline | Select-Object -First 55 | ForEach-Object { Write-Output $_ }

if ($mode -eq 'bar') {
  if ($initialW -le 400 -and $initialH -le 200) { Write-Output ("PASS started as minimized bar {0}x{1}" -f $initialW, $initialH) } else { Write-Output ("FAIL started {0}x{1}, expected a bar" -f $initialW, $initialH); $failures++ }
  if ($sawRed) { Write-Output 'PASS bar flashed LIMIT REACHED on itself' } else { Write-Output 'FAIL no alarm on the bar'; $failures++ }
  if ($sawStrip) { Write-Output 'PASS bar shrank to the 240x40 cooldown strip' } else { Write-Output 'FAIL never reached the 240x40 strip'; $failures++ }
  if ($sawRestore) { Write-Output 'PASS bar eased back open after refill' } else { Write-Output 'FAIL bar never restored its size'; $failures++ }
} elseif ($mode -eq 'interleave') {
  if ($initialW -le 400 -and $initialH -le 200) { Write-Output ("PASS started as minimized bar {0}x{1}" -f $initialW, $initialH) } else { Write-Output ("FAIL started {0}x{1}, expected a bar" -f $initialW, $initialH); $failures++ }
  if ($sawRed) { Write-Output 'PASS alarm blinked on the bar' } else { Write-Output 'FAIL no alarm on the bar'; $failures++ }
  if (-not $sawStrip) { Write-Output 'PASS refill mid-flash: bar NEVER became the 240x40 strip' } else { Write-Output 'FAIL bar shrank even though the account refilled mid-flash'; $failures++ }
  if ($sawRegained) { Write-Output 'PASS bar regained 340x142 when the aborted flash ended' } else { Write-Output 'FAIL bar never regained its gauge size after the aborted flash'; $failures++ }
} else {
  if ($initialW -ge 600) { Write-Output ("PASS started expanded {0}x{1}" -f $initialW, $initialH) } else { Write-Output ("FAIL started {0}x{1}, expected expanded" -f $initialW, $initialH); $failures++ }
  if (-not $anyResizeFromExpanded) { Write-Output 'PASS expanded view NEVER resized' } else { Write-Output 'FAIL expanded view resized during cooldown'; $failures++ }
  if (-not $sawRed) { Write-Output 'PASS no alarm overlay in the expanded view' } else { Write-Output 'FAIL alarm appeared in the expanded view'; $failures++ }
}
Write-Output ("RESULT mode={0} failures={1}" -f $mode, $failures)
exit $failures
