# Diagnostic: enumerate all top-level windows of the HUD process.
$ErrorActionPreference = 'Stop'
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class EnumW {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc cb, IntPtr l);
  public delegate bool EnumWindowsProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern int GetClassNameW(IntPtr h, [MarshalAs(UnmanagedType.LPWStr)] StringBuilder sb, int max);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
}
"@
[EnumW]::SetProcessDPIAware() | Out-Null
$exe = Join-Path (Get-Location) 'ZCode-Usage-HUD.exe'
$p = Start-Process -FilePath $exe -ArgumentList '--preview','--collapsed' -PassThru
Start-Sleep -Seconds 4
$found = @()
$cb = [EnumW+EnumWindowsProc]{ param($h, $l)
  $procid = 0
  [EnumW]::GetWindowThreadProcessId($h, [ref]$procid) | Out-Null
  if ($procid -eq $p.Id) {
    $sb = New-Object System.Text.StringBuilder 256
    [EnumW]::GetClassNameW($h, $sb, 256) | Out-Null
    $script:found += ('{0} vis={1}' -f $sb.ToString(), [EnumW]::IsWindowVisible($h))
  }
  return $true
}
[EnumW]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
$found | ForEach-Object { Write-Output "window: $_" }
& $exe --quit 2>$null | Out-Null
Start-Sleep -Milliseconds 600
if (Get-Process -Id $p.Id -ErrorAction SilentlyContinue) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
