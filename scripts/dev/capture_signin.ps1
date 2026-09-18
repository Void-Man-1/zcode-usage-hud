$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class W32S {
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
[W32S]::SetProcessDPIAware() | Out-Null
$exe = (Resolve-Path (Join-Path $PSScriptRoot '..\..\ZCode-Usage-HUD.exe')).Path

$scratch = Join-Path $env:TEMP ('hudsignin_' + [guid]::NewGuid().ToString('N').Substring(0,8))
$simHome = Join-Path $scratch 'home'
New-Item -ItemType Directory -Force -Path $simHome | Out-Null
# deliberately NO credentials seeding: clean machine, signed-out state

$envBackupUP = $env:USERPROFILE
$envBackupLA = $env:LOCALAPPDATA
$env:USERPROFILE = $simHome
$env:LOCALAPPDATA = $scratch

function Find-HudWindow([uint32]$targetPid) {
  $found = [IntPtr]::Zero
  $cb = [W32S+EnumProc]{
    param($h, $lp)
    if ([W32S]::IsWindowVisible($h)) {
      $p = [uint32]0
      [W32S]::GetWindowThreadProcessId($h, [ref]$p) | Out-Null
      if ($p -eq $script:wantPid) {
        $sb = New-Object System.Text.StringBuilder 256
        [W32S]::GetClassName($h, $sb, 256) | Out-Null
        if ($sb.ToString().StartsWith('ZCodeUsageHUD')) { $script:found = $h; return $false }
      }
    }
    return $true
  }
  $script:wantPid = $targetPid
  $script:found = [IntPtr]::Zero
  [W32S]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
  return $script:found
}

$p = Start-Process -FilePath $exe -ArgumentList '--preview' -PassThru
try {
  $deadline = (Get-Date).AddSeconds(25)
  $h = [IntPtr]::Zero
  while ((Get-Date) -lt $deadline) {
    if ($p.HasExited) { throw "HUD exited early" }
    $h = Find-HudWindow $p.Id
    if ($h -ne [IntPtr]::Zero) { break }
    Start-Sleep -Milliseconds 300
  }
  if ($h -eq [IntPtr]::Zero) { throw "window not found" }
  Start-Sleep -Milliseconds 900
  $r = New-Object W32S+RECT
  [W32S]::GetWindowRect($h, [ref]$r) | Out-Null
  $w = $r.Right - $r.Left; $ht = $r.Bottom - $r.Top
  [W32S]::MoveWindow($h, 60, 60, $w, $ht, $true) | Out-Null
  Start-Sleep -Milliseconds 700
  [W32S]::GetWindowRect($h, [ref]$r) | Out-Null
  $w = $r.Right - $r.Left; $ht = $r.Bottom - $r.Top
  $bmp = New-Object System.Drawing.Bitmap $w, $ht
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($r.Left, $r.Top, 0, 0, (New-Object System.Drawing.Size($w, $ht)))
  $g.Dispose()
  $out = (Resolve-Path (Join-Path $PSScriptRoot '..\..\docs\screenshots')).Path + '\zcode-usage-hud-sign-in.png'
  $bmp.Save($out, [System.Drawing.Imaging.ImageFormat]::Png)
  $bmp.Dispose()
  Write-Output ("captured sign-in {0}x{1} (profile {2})" -f $w, $ht, $scratch)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  $env:USERPROFILE = $envBackupUP
  $env:LOCALAPPDATA = $envBackupLA
  Start-Sleep -Milliseconds 500
  Remove-Item -Recurse -Force $scratch -ErrorAction SilentlyContinue
}
Write-Output 'SIGNIN_DONE'
