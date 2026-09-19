# Drives the settings.json -> behavior loop: gauge style geometry,
# home-position snap, and bar resize. Each phase launches fresh.
$ErrorActionPreference = 'Stop'
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class W32P2 {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc cb, IntPtr l);
  public delegate bool EnumWindowsProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern int GetClassNameW(IntPtr h, [MarshalAs(UnmanagedType.LPWStr)] StringBuilder sb, int max);
  public struct R { public int Left, Top, Right, Bottom; }
}
"@
[W32P2]::SetProcessDPIAware() | Out-Null
$dir = "$env:LOCALAPPDATA\ZCode Usage HUD"
$cfg = Join-Path $dir 'settings.json'
$exe = Join-Path (Get-Location) 'ZCode-Usage-HUD.exe'
$failures = 0

function Find-Hud([uint32]$procid) {
  for ($i = 0; $i -lt 16; $i++) {
    $script:hit = [IntPtr]::Zero
    $cb = [W32P2+EnumWindowsProc]{ param($h, $l)
      $p2 = 0
      [W32P2]::GetWindowThreadProcessId($h, [ref]$p2) | Out-Null
      if ($p2 -eq $procid) {
        $sb = New-Object System.Text.StringBuilder 256
        [W32P2]::GetClassNameW($h, $sb, 256) | Out-Null
        if ($sb.ToString() -eq 'ZCodeUsageHUDPreviewV1') { $script:hit = $h }
      }
      return $true
    }
    [W32P2]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
    if ($script:hit -ne [IntPtr]::Zero) { return $script:hit }
    Start-Sleep -Milliseconds 500
  }
  return [IntPtr]::Zero
}

function Measure-Collapsed([string]$json, [string]$label) {
  if (Test-Path $cfg) { Remove-Item $cfg -Force }
  if ($json) { [System.IO.File]::WriteAllText($cfg, $json) }
  $p = Start-Process -FilePath $exe -ArgumentList '--preview','--collapsed' -PassThru
  $h = Find-Hud $p.Id
  if ($h -eq [IntPtr]::Zero) { Write-Output "FAIL $label no window"; $script:failures++; & $exe --quit 2>$null | Out-Null; return $null }
  Start-Sleep -Milliseconds 2500   # settle past launch animation
  $r = New-Object W32P2+R
  [W32P2]::GetWindowRect($h, [ref]$r) | Out-Null
  $out = @{ w = $r.Right - $r.Left; x = $r.Left; y = $r.Top; h = $r.Bottom - $r.Top }
  Write-Host ("{0}: {1}x{2} at ({3},{4})" -f $label, $out.w, $out.h, $r.Left, $r.Top)
  & $exe --quit 2>$null | Out-Null
  Start-Sleep -Milliseconds 1200
  return $out
}

# Phase 1: gauge style -> bar height follows gaugeRowHeight, not 142.
$g = Measure-Collapsed '{"style":1}' 'gauge-style bar'
if ($g -and ($g.h -ge 100 -or $g.h -le 30)) { Write-Output 'FAIL gauge-style bar height not gauge-row sized'; $failures++ }
elseif ($g) { Write-Output 'PASS gauge-style bar resized to gauge row' }
# Phase 2: home snap.
$hm = Measure-Collapsed '{"homeX":200,"homeY":300}' 'home snap'
if ($hm -and ($hm.x -ne 200 -or $hm.y -ne 300)) { Write-Output 'FAIL bar did not snap to home (200,300)'; $failures++ }
elseif ($hm) { Write-Output 'PASS bar snapped to home' }
# Phase 3: user bar size.
$bs = Measure-Collapsed '{"barW":280,"barH":60}' 'user bar size'
if ($bs -and ($bs.w -ne 280)) { Write-Output 'FAIL bar width override ignored'; $failures++ }
elseif ($bs) { Write-Output 'PASS bar width override honored' }

if (Test-Path $cfg) { Remove-Item $cfg -Force }
Write-Output ("RESULT prefs-live failures={0}" -f $failures)
exit $failures
