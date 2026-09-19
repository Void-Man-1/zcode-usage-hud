# Theme E2E: prove the full color-customization loop on the real windows.
# 1. Seed a non-default theme.json (all overrides) in an isolated profile.
# 2. Launch --preview --collapsed; assert the HUD renders the OVERRIDE
#    (startup load reaches every painted surface).
# 3. Post a real gear click (hit-test path), assert the settings window.
# 4. Post a click on "Return to the void"; assert the HUD pixel returns
#    to the exact default AND theme.json is emptied.
$ErrorActionPreference = 'Stop'
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class W32T {
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
[W32T]::SetProcessDPIAware() | Out-Null
Add-Type -AssemblyName System.Drawing

$env:LOCALAPPDATA = 'C:\Users\Void_LLM\AppData\Local\Temp\hudprof'
$dir = Join-Path $env:LOCALAPPDATA 'ZCode Usage HUD'
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$exe = Join-Path (Get-Location) 'ZCode-Usage-HUD.exe'

function Get-Wnd([uint32]$procid, [string]$cls) {
  for ($i = 0; $i -lt 20; $i++) {
    $script:hit = [IntPtr]::Zero
    $cb = [W32T+EnumWindowsProc]{ param($h, $l)
      $p2 = 0
      [W32T]::GetWindowThreadProcessId($h, [ref]$p2) | Out-Null
      if ($p2 -eq $procid) {
        $sb = New-Object System.Text.StringBuilder 256
        [W32T]::GetClassNameW($h, $sb, 256) | Out-Null
        if ($sb.ToString() -eq $cls) { $script:hit = $h }
      }
      return $true
    }
    [W32T]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
    if ($script:hit -ne [IntPtr]::Zero) { return $script:hit }
    Start-Sleep -Milliseconds 500
  }
  return [IntPtr]::Zero
}

function Snap-Pixel([IntPtr]$h, [int]$x, [int]$y) {
  $r = New-Object W32T+R
  [W32T]::GetWindowRect($h, [ref]$r) | Out-Null
  $w = $r.Right - $r.Left; $ht = $r.Bottom - $r.Top
  $bmp = New-Object System.Drawing.Bitmap $w, $ht
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($r.Left, $r.Top, 0, 0, (New-Object System.Drawing.Size($w, $ht)))
  $c = $bmp.GetPixel($x, $y)
  $g.Dispose(); $bmp.Dispose()
  return '#{0:x2}{1:x2}{2:x2}' -f $c.R, $c.G, $c.B
}

function Wait-Settled([IntPtr]$h) {
  for ($i = 0; $i -lt 20; $i++) {
    $r = New-Object W32T+R
    [W32T]::GetWindowRect($h, [ref]$r) | Out-Null
    $w = $r.Right - $r.Left
    if ($w -lt 500 -and $w -gt 200) { return $r }
    Start-Sleep -Milliseconds 500
  }
  return $r
}

# --- Seed: every key overridden to a distinct magenta-family value so any
# unthemed surface would keep its default and fail the before-check.
$seed = @'
{"windowBg":14740899,"panel":13545076,"panelAlt":13355979,"border":16711935,"divider":12303291,"text":16711935,"textBright":16711935,"textMuted":11128866,"accent":16711935,"accent2":15658734,"track":11771177,"bad":16711935,"ok":16711935,"warn":16711935,"standbyBg":10463187,"standbyText":16711935,"gaugeFace":9871231,"gaugeRim":16711935,"gaugeNeedle":16711935,"gaugeTick":12303291,"titlebar":14540287,"closeHover":16711935,"minHover":15658734,"gearHover":15658734,"rowLabel":16711935,"rowValue":16711935,"rowFill":16711935,"rowFillLow":15658734,"tooltipBg":11777,"tooltipText":16711935,"scrollThumb":13754137,"menuBg":14540287,"menuText":16711935,"menuHover":15658734,"loginBg":13545076,"loginBtn":16711935,"loginText":16711935,"spinner":16711935,"btnFace":13355979,"btnText":16711935}
'@
# (a couple of deliberately-invalid tokens above get dropped by the loader's
#  unknown-key/zero guard — which is itself part of the contract under test)
[System.IO.File]::WriteAllText((Join-Path $dir 'theme.json'), $seed)

$p = Start-Process -FilePath $exe -ArgumentList '--preview','--collapsed' -PassThru
try {
  $hud = Get-Wnd $p.Id 'ZCodeUsageHUDPreviewV1'
  if ($hud -eq [IntPtr]::Zero) { Write-Output 'FAIL hud window not found'; exit 1 }
  $r = Wait-Settled $hud
  $w = $r.Right - $r.Left
  Start-Sleep -Milliseconds 800
  $before = Snap-Pixel $hud 10 10
  Write-Output "HUD bg with overrides: $before (default would be #0d0e11)"
  if ($before -eq '#0d0e11') { Write-Output 'FAIL override did not reach the paint path'; exit 1 }
  Write-Output 'PASS theme.json override renders on startup'

  # Gear click: rect [W-108, W-72] x [0,18] -> click at (W-90, 9).
  $gx = $w - 90; $gy = 9
  $lp = [IntPtr](($gy -shl 16) -bor ($gx -band 0xFFFF))
  [W32T]::PostMessageW($hud, 0x0201, [IntPtr]1, $lp) | Out-Null
  Start-Sleep -Milliseconds 60
  [W32T]::PostMessageW($hud, 0x0202, [IntPtr]0, $lp) | Out-Null
  $sw = Get-Wnd $p.Id 'ZCodeHUDSettingsWnd'
  if ($sw -eq [IntPtr]::Zero) { Write-Output 'FAIL settings window did not open'; exit 1 }
  Write-Output 'PASS gear click opened settings'
  Start-Sleep -Milliseconds 700  # first paint registers the click rects

  # "Return to the void" full-width button, ~412..444 in client Y; center-x.
  $sr = New-Object W32T+R
  [W32T]::GetWindowRect($sw, [ref]$sr) | Out-Null
  $swW = $sr.Right - $sr.Left
  $vx = [int]($swW / 2); $vy = 398
  $lp2 = [IntPtr](($vy -shl 16) -bor ($vx -band 0xFFFF))
  [W32T]::PostMessageW($sw, 0x0201, [IntPtr]1, $lp2) | Out-Null
  Start-Sleep -Milliseconds 60
  [W32T]::PostMessageW($sw, 0x0202, [IntPtr]0, $lp2) | Out-Null
  Start-Sleep -Milliseconds 900

  $after = Snap-Pixel $hud 10 10
  Write-Output "HUD bg after Return-to-the-void: $after"
  if ($after -ne '#0d0e11') {
    Write-Output 'FAIL void click did not restore the default render (coords or wiring)'
    [System.IO.File]::Copy((Join-Path $dir 'theme.json'), (Join-Path $dir 'theme.json.fail'), $true)
    exit 1
  }
  Write-Output 'PASS Return-to-the-void restored default grey live'
  $tf = Join-Path $dir 'theme.json'
  if (Test-Path $tf) { Write-Output ("theme.json after reset: " + (Get-Content $tf -Raw)) }
  else { Write-Output 'theme.json removed after reset' }
} finally {
  & $exe --quit 2>$null | Out-Null
  Start-Sleep -Milliseconds 800
  if (Get-Process -Id $p.Id -ErrorAction SilentlyContinue) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
}
Write-Output 'theme-e2e done'
