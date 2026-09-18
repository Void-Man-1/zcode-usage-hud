$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class W32 {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr lp);
  public delegate bool EnumProc(IntPtr hWnd, IntPtr lp);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT rect);
  [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr hWnd, int x, int y, int w, int h, bool repaint);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr hWnd, StringBuilder sb, int max);
  public struct RECT { public int Left, Top, Right, Bottom; }
}
"@

[W32]::SetProcessDPIAware() | Out-Null
$exe = (Resolve-Path (Join-Path $PSScriptRoot '..\..\ZCode-Usage-HUD.exe')).Path
if (-not (Test-Path $exe)) { Write-Output "FAIL exe missing: $exe"; exit 2 }

function Find-HudWindow([uint32]$targetPid) {
  $found = [IntPtr]::Zero
  $cb = [W32+EnumProc]{
    param($h, $lp)
    if ([W32]::IsWindowVisible($h)) {
      $p = [uint32]0
      [W32]::GetWindowThreadProcessId($h, [ref]$p) | Out-Null
      if ($p -eq $script:wantPid) {
        $sb = New-Object System.Text.StringBuilder 256
        [W32]::GetClassName($h, $sb, 256) | Out-Null
        if ($sb.ToString().StartsWith('ZCodeUsageHUD')) { $script:found = $h; return $false }
      }
    }
    return $true
  }
  $script:wantPid = $targetPid
  $script:found = [IntPtr]::Zero
  [W32]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
  return $script:found
}

function Wait-Window([System.Diagnostics.Process]$proc, [int]$seconds) {
  $deadline = (Get-Date).AddSeconds($seconds)
  while ((Get-Date) -lt $deadline) {
    if ($proc.HasExited) { throw "HUD process exited early (code $($proc.ExitCode))" }
    $h = Find-HudWindow $proc.Id
    if ($h -ne [IntPtr]::Zero) { return $h }
    Start-Sleep -Milliseconds 300
  }
  throw "HUD window not found within ${seconds}s"
}

function Test-Token($bmp, [int]$x, [int]$y, [int]$er, [int]$eg, [int]$eb, [string]$label) {
  # clamp sample center into the bitmap
  $x = [math]::Min([math]::Max($x, 1), $bmp.Width-2)
  $y = [math]::Min([math]::Max($y, 1), $bmp.Height-2)
  # average a 3x3 patch to be robust against 1px AA edges
  $rs=0; $gs=0; $bs=0; $n=0
  for ($dy=-1; $dy -le 1; $dy++) {
    for ($dx=-1; $dx -le 1; $dx++) {
      $c = $bmp.GetPixel($x+$dx, $y+$dy); $rs+=$c.R; $gs+=$c.G; $bs+=$c.B; $n++
    }
  }
  $r=[int]($rs/$n); $g=[int]($gs/$n); $b=[int]($bs/$n)
  $ok = [math]::Abs($r-$er) -le 3 -and [math]::Abs($g-$eg) -le 3 -and [math]::Abs($b-$eb) -le 3
  $got = '#{0:x2}{1:x2}{2:x2}' -f $r, $g, $b
  $want = '#{0:x2}{1:x2}{2:x2}' -f $er, $eg, $eb
  # Write-Host: report lines must bypass the pipeline, otherwise they
  # get captured by the caller's assignment and failures go silent.
  if ($ok) { Write-Host ("PASS {0,-28} ({1},{2}) = {3}" -f $label, $x, $y, $got) }
  else     { Write-Host ("FAIL {0,-28} ({1},{2}) want {3} got {4}" -f $label, $x, $y, $want, $got) }
  return $ok
}

$failures = 0

# ---------- Phase 1: expanded preview ----------
$p1 = Start-Process -FilePath $exe -ArgumentList '--preview' -PassThru
try {
  $h = Wait-Window $p1 25
  $r = New-Object W32+RECT
  [W32]::GetWindowRect($h, [ref]$r) | Out-Null
  $w = $r.Right - $r.Left; $ht = $r.Bottom - $r.Top
  Write-Output ("window {0}x{1} at ({2},{3}) pid {4}" -f $w, $ht, $r.Left, $r.Top, $p1.Id)
  [W32]::MoveWindow($h, 40, 40, $w, $ht, $true) | Out-Null
  Start-Sleep -Milliseconds 700
  [W32]::GetWindowRect($h, [ref]$r) | Out-Null
  $wd = $r.Right-$r.Left; $hd = $r.Bottom-$r.Top
  $bmp = New-Object System.Drawing.Bitmap $wd, $hd
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($r.Left, $r.Top, 0, 0, (New-Object System.Drawing.Size($wd, $hd)))
  # layout coordinates are for the 720-wide expanded canvas (scale 1:
  # verified by the device-pixel window size)
  if ($wd -ne 720) { Write-Host ("NOTE window width {0}, expected 720" -f $wd) }

  # canvas + panels (client == window: WS_POPUP has no frame)
  # Expectations = original palette render (v1.3.0+), sampled from the
  # real window and cross-checked against HEAD color literals.
  $ok = Test-Token $bmp 10 10 0x0d 0x0e 0x11 'canvas rgb(13,14,17)'
  if (-not $ok) { $failures++ }
  $ok = Test-Token $bmp 584 11 0x1b 0x1d 0x23 'version chip fill'
  if (-not $ok) { $failures++ }
  $ok = Test-Token $bmp 150 100 0x16 0x18 0x1d 'status panel rgb(22,24,29)'
  if (-not $ok) { $failures++ }
  $ok = Test-Token $bmp 24 120 0x52 0xc1 0x7a 'status accent success text'
  if (-not $ok) { $failures++ }
  $ok = Test-Token $bmp 150 240 0x4b 0xaa 0x6e 'bucket-1 fill success'
  if (-not $ok) { $failures++ }
  $ok = Test-Token $bmp 400 241 0x2b 0x2e 0x37 'bar track rgb(43,46,55)'
  if (-not $ok) { $failures++ }
  $ok = Test-Token $bmp 691 260 0x16 0x18 0x1d 'card interior surface-1'
  if (-not $ok) { $failures++ }
  $ok = Test-Token $bmp 150 386 0xcd 0x9d 0x39 'bucket-2 fill amber'
  if (-not $ok) { $failures++ }
  $ok = Test-Token $bmp 24 560 0x60 0xa5 0xfa 'promo accent rgb(96,165,250)'
  if (-not $ok) { $failures++ }
  $g.Dispose(); $bmp.Save((Join-Path $env:TEMP 'hud_expanded.png')); $bmp.Dispose()
} finally {
  if (-not $p1.HasExited) { Stop-Process -Id $p1.Id -Force }
}
Start-Sleep -Milliseconds 800

# ---------- Phase 2: collapsed preview ----------
$p2 = Start-Process -FilePath $exe -ArgumentList '--preview','--collapsed' -PassThru
try {
  $h = Wait-Window $p2 25
  $r = New-Object W32+RECT
  [W32]::GetWindowRect($h, [ref]$r) | Out-Null
  $w = $r.Right - $r.Left; $ht = $r.Bottom - $r.Top
  Write-Output ("collapsed {0}x{1} at ({2},{3})" -f $w, $ht, $r.Left, $r.Top)
  [W32]::MoveWindow($h, 40, 40, $w, $ht, $true) | Out-Null
  Start-Sleep -Milliseconds 700
  [W32]::GetWindowRect($h, [ref]$r) | Out-Null
  $wd = $r.Right-$r.Left; $hd = $r.Bottom-$r.Top
  $bmp = New-Object System.Drawing.Bitmap $wd, $hd
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($r.Left, $r.Top, 0, 0, (New-Object System.Drawing.Size($wd, $hd)))
  if ($wd -ne 340) { Write-Host ("NOTE window width {0}, expected 340" -f $wd) }

  $ok = Test-Token $bmp 10 10 0x0d 0x0e 0x11 'collapsed canvas'
  if (-not $ok) { $failures++ }
  $ok = Test-Token $bmp 1 68 0x5a 0xbe 0x79 'collapsed success bar'
  if (-not $ok) { $failures++ }
  $ok = Test-Token $bmp 230 47 0x2b 0x2e 0x37 'collapsed row-1 track'
  if (-not $ok) { $failures++ }
  $ok = Test-Token $bmp 230 85 0x2b 0x2e 0x37 'collapsed row-2 track'
  if (-not $ok) { $failures++ }
  $g.Dispose(); $bmp.Save((Join-Path $env:TEMP 'hud_collapsed.png')); $bmp.Dispose()
} finally {
  if (-not $p2.HasExited) { Stop-Process -Id $p2.Id -Force }
}

Write-Output ("RESULT failures={0}" -f $failures)
exit ([math]::Min($failures, 1))
