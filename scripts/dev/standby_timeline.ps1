# Timeline probe: launch --preview --locked --collapsed and print every
# distinct rect with a timestamp, plus the hud.log tail afterwards.
$ErrorActionPreference = 'Stop'
Add-Type @"
using System; using System.Runtime.InteropServices;
public class W32T {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern IntPtr FindWindowW([MarshalAs(UnmanagedType.LPWStr)] string c, IntPtr t);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  public struct RECT { public int Left, Top, Right, Bottom; }
}
"@
[W32T]::SetProcessDPIAware() | Out-Null
$exe = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\zcode-usage-hud.exe'))
$p = Start-Process -FilePath $exe -ArgumentList '--preview','--locked','--collapsed' -PassThru
$last = ""
try {
  $deadline = (Get-Date).AddSeconds(13)
  while ((Get-Date) -lt $deadline) {
    $h = [W32T]::FindWindowW('ZCodeUsageHUDPreviewV1', [IntPtr]::Zero)
    if ($h -ne 0) {
      $r = New-Object W32T+RECT
      [W32T]::GetWindowRect($h, [ref]$r) | Out-Null
      $cur = "$($r.Right-$r.Left)x$($r.Bottom-$r.Top)"
      if ($cur -ne $last) { Write-Output ("{0:hh\:mm\:ss\.fff}  {1}" -f (Get-Date), $cur); $last = $cur }
    }
    Start-Sleep -Milliseconds 40
  }
} finally { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
Write-Output "=== hud.log tail ==="
Get-Content "$env:LOCALAPPDATA\zcode-usage-hud\hud.log" -Tail 12 -ErrorAction SilentlyContinue
