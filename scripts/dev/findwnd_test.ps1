$ErrorActionPreference = 'Stop'
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class FW {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern IntPtr FindWindowW(string cls, string title);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc cb, IntPtr l);
  public delegate bool EnumWindowsProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern int GetClassNameW(IntPtr h, [MarshalAs(UnmanagedType.LPWStr)] StringBuilder sb, int max);
}
"@
[FW]::SetProcessDPIAware() | Out-Null
$exe = Join-Path (Get-Location) 'ZCode-Usage-HUD.exe'
$p = Start-Process -FilePath $exe -ArgumentList '--preview','--collapsed' -PassThru
Start-Sleep -Seconds 4
# 1) enumerate (ground truth)
$names = @()
$cb = [FW+EnumWindowsProc]{ param($h, $l)
  $procid = 0
  [FW]::GetWindowThreadProcessId($h, [ref]$procid) | Out-Null
  if ($procid -eq $p.Id) {
    $sb = New-Object System.Text.StringBuilder 256
    [FW]::GetClassNameW($h, $sb, 256) | Out-Null
    $script:names += $sb.ToString()
  }
  return $true
}
[FW]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
Write-Output ("enum: {0}" -f ($names -join ', '))
# 2) FindWindowW with same string
$h1 = [FW]::FindWindowW('ZCodeUsageHUDPreviewV1', $null)
Write-Output ("FindWindowW(cls, null): {0}" -f $h1)
$h2 = [FW]::FindWindowW('ZCodeUsageHUDPreviewV1', '')
Write-Output ("FindWindowW(cls, ''): {0}" -f $h2)
& $exe --quit 2>$null | Out-Null
Start-Sleep -Milliseconds 600
if (Get-Process -Id $p.Id -ErrorAction SilentlyContinue) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
