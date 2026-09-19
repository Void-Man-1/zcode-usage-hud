# Settings-window live check: launch the HUD preview, click the gear,
# assert the settings window exists, then close it and quit the HUD.
$ErrorActionPreference = 'Stop'
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class W32S {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint dx, uint dy, uint d, UIntPtr e);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool PostMessageW(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc cb, IntPtr l);
  public delegate bool EnumWindowsProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern int GetClassNameW(IntPtr h, [MarshalAs(UnmanagedType.LPWStr)] StringBuilder sb, int max);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  public struct R { public int Left, Top, Right, Bottom; }
}
"@
[W32S]::SetProcessDPIAware() | Out-Null

# FindWindowW misbehaves against this process in this environment; enum by
# pid+class is ground truth and is what the other harnesses use.
function Get-HudWindow([uint32]$procid, [string]$cls) {
  for ($i = 0; $i -lt 20; $i++) {
    $script:hit = [IntPtr]::Zero
    $cb = [W32S+EnumWindowsProc]{ param($h, $l)
      $pid2 = 0
      [W32S]::GetWindowThreadProcessId($h, [ref]$pid2) | Out-Null
      if ($pid2 -eq $procid) {
        $sb = New-Object System.Text.StringBuilder 256
        [W32S]::GetClassNameW($h, $sb, 256) | Out-Null
        if ($sb.ToString() -eq $cls) { $script:hit = $h }
      }
      return $true
    }
    [W32S]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
    if ($script:hit -ne [IntPtr]::Zero) { return $script:hit }
    Start-Sleep -Milliseconds 500
  }
  return [IntPtr]::Zero
}

$exe = Join-Path (Get-Location) 'ZCode-Usage-HUD.exe'
$p = Start-Process -FilePath $exe -ArgumentList '--preview','--collapsed' -PassThru
try {
  $hud = Get-HudWindow $p.Id 'ZCodeUsageHUDPreviewV1'
  if ($hud -eq [IntPtr]::Zero) { Write-Output 'FAIL hud window not found'; exit 1 }
  # The snap/restack timer re-snaps the window and multi-monitor DPI
  # skews cursor synthesis, so physical clicking is unreliable. Post the
  # button-up directly with painter-space client coords: the exact
  # hit-test path the gear uses (rect [W-108,W-72] x [0,18]).
  $w = 0; $h2 = 0
  # Wait for the launch collapse animation to finish: the gear only
  # exists in collapsed geometry, so clicking early hits the expanded
  # title strip instead.
  $r = New-Object W32S+R
  for ($i = 0; $i -lt 20; $i++) {
    [W32S]::GetWindowRect($hud, [ref]$r) | Out-Null
    if (($r.Right - $r.Left) -lt 500) { break }
    Start-Sleep -Milliseconds 500
  }
  $w = $r.Right - $r.Left
  $gx = $w - 90; $gy = 9
  $lp = [IntPtr](($gy -shl 16) -bor ($gx -band 0xFFFF))
  [W32S]::PostMessageW($hud, 0x0201, [IntPtr]1, $lp) | Out-Null  # WM_LBUTTONDOWN
  Start-Sleep -Milliseconds 60
  [W32S]::PostMessageW($hud, 0x0202, [IntPtr]0, $lp) | Out-Null  # WM_LBUTTONUP
  $sw = [IntPtr]::Zero
  for ($i = 0; $i -lt 10; $i++) {
    Start-Sleep -Milliseconds 300
    $sw = Get-HudWindow $p.Id 'ZCodeHUDSettingsWnd'
    if ($sw -ne [IntPtr]::Zero) { break }
  }
  if ($sw -eq [IntPtr]::Zero) { Write-Output 'FAIL settings window did not open after gear click'; exit 1 }
  $sr = New-Object W32S+R
  [W32S]::GetWindowRect($sw, [ref]$sr) | Out-Null
  Write-Output ("PASS settings window opened {0}x{1} at ({2},{3})" -f ($sr.Right-$sr.Left), ($sr.Bottom-$sr.Top), $sr.Left, $sr.Top)
  [W32S]::PostMessageW($sw, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null  # WM_CLOSE
  Start-Sleep -Milliseconds 400
  $sw2 = Get-HudWindow $p.Id 'ZCodeHUDSettingsWnd'
  if ($sw2 -ne [IntPtr]::Zero -and [W32S]::IsWindowVisible($sw2)) { Write-Output 'WARN settings window still visible after WM_CLOSE' }
  else { Write-Output 'PASS settings window closes via WM_CLOSE' }
} finally {
  & $exe --quit 2>$null | Out-Null
  Start-Sleep -Milliseconds 800
  if (Get-Process -Id $p.Id -ErrorAction SilentlyContinue) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
}
Write-Output 'settings-check done'
