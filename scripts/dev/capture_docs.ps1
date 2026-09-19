$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class W32C {
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
[W32C]::SetProcessDPIAware() | Out-Null
$exe = (Resolve-Path (Join-Path $PSScriptRoot '..\..\ZCode-Usage-HUD.exe')).Path
$outDir = (Resolve-Path (Join-Path $PSScriptRoot '..\..\docs\screenshots')).Path
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

function Find-HudWindow([uint32]$targetPid) {
  $found = [IntPtr]::Zero
  $cb = [W32C+EnumProc]{
    param($h, $lp)
    if ([W32C]::IsWindowVisible($h)) {
      $p = [uint32]0
      [W32C]::GetWindowThreadProcessId($h, [ref]$p) | Out-Null
      if ($p -eq $script:wantPid) {
        $sb = New-Object System.Text.StringBuilder 256
        [W32C]::GetClassName($h, $sb, 256) | Out-Null
        if ($sb.ToString().StartsWith('ZCodeUsageHUD')) { $script:found = $h; return $false }
      }
    }
    return $true
  }
  $script:wantPid = $targetPid
  $script:found = [IntPtr]::Zero
  [W32C]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
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

function Capture([string]$mode, [string]$outName) {
  $args = @('--preview'); if ($mode -eq 'compact') { $args += '--collapsed' }
  $p = Start-Process -FilePath $exe -ArgumentList $args -PassThru
  try {
    $h = Wait-Window $p 25
    Start-Sleep -Milliseconds 900   # let first paint + tray settle
    $r = New-Object W32C+RECT
    [W32C]::GetWindowRect($h, [ref]$r) | Out-Null
    $w = $r.Right - $r.Left; $ht = $r.Bottom - $r.Top
    [W32C]::MoveWindow($h, 60, 60, $w, $ht, $true) | Out-Null
    Start-Sleep -Milliseconds 700
    [W32C]::GetWindowRect($h, [ref]$r) | Out-Null
    $w = $r.Right - $r.Left; $ht = $r.Bottom - $r.Top
    $bmp = New-Object System.Drawing.Bitmap $w, $ht
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.CopyFromScreen($r.Left, $r.Top, 0, 0, (New-Object System.Drawing.Size($w, $ht)))
    $g.Dispose()
    $out = Join-Path $outDir $outName
    $bmp.Save($out, [System.Drawing.Imaging.ImageFormat]::Png)
    $bmp.Dispose()
    Write-Output ("captured {0} -> {1} ({2}x{3})" -f $mode, $out, $w, $ht)
  } finally {
    if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  }
  Start-Sleep -Milliseconds 800
}

Capture 'expanded' 'zcode-usage-hud-expanded.png'
Capture 'compact'  'zcode-usage-hud-compact.png'
Write-Output 'CAPTURE_DONE'
