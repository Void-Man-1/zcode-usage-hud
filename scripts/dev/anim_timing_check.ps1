# Animation timing check: drives --preview --locked --collapsed and polls
# the real window rect every 40ms. Asserts the full-bar -> cooldown-strip
# transition passes through intermediate rects (animated, not snapped) and
# completes within ~400ms.
$ErrorActionPreference = 'Stop'
Add-Type @"
using System; using System.Runtime.InteropServices;
public class W32A {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern IntPtr FindWindowW([MarshalAs(UnmanagedType.LPWStr)] string c, IntPtr t);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  public struct RECT { public int Left, Top, Right, Bottom; }
}
"@
[W32A]::SetProcessDPIAware() | Out-Null
$exe = Join-Path $PSScriptRoot '..\..\ZCode-Usage-HUD.exe'
$exe = [System.IO.Path]::GetFullPath($exe)
# Default lock window (8s): locked publish ~0.6s -> 5.5s flash -> strip at
# ~6.1s -> refill 8.4s -> restore. A short lock aborts the flash mid-way
# (interleave semantics), so we must NOT override it here.
$p = Start-Process -FilePath $exe -ArgumentList '--preview','--locked','--collapsed' -PassThru
try {
  $deadline = (Get-Date).AddSeconds(10.5)
  $rects = @()
  while ((Get-Date) -lt $deadline) {
    $h = [W32A]::FindWindowW('ZCodeUsageHUDPreviewV1', [IntPtr]::Zero)
    if ($h -ne 0) {
      $r = New-Object W32A+RECT
      [W32A]::GetWindowRect($h, [ref]$r) | Out-Null
      $w = $r.Right - $r.Left; $ht = $r.Bottom - $r.Top
      $rects += ,@($w, $ht)
    }
    Start-Sleep -Milliseconds 40
  }
  $uniq = @($rects | ForEach-Object { "$($_[0])x$($_[1])" } | Select-Object -Unique)  # width x height
  Write-Output ("distinct-rects: " + ($uniq -join ' '))
  $stripIdx = -1
  for ($i = 0; $i -lt $rects.Count; $i++) { if ($rects[$i][0] -le 250) { $stripIdx = $i; break } }
  $fullIdx = -1
  for ($i = $stripIdx; $i -ge 0; $i--) { if ($rects[$i][0] -ge 330) { $fullIdx = $i; break } }
  if ($stripIdx -lt 0 -or $fullIdx -lt 0) { Write-Output "FAIL strip or full-bar rect never observed"; exit 1 }
  $between = $stripIdx - $fullIdx - 1
  Write-Output ("samples-from-full-to-strip: " + $between + " (~" + ($between*40) + "ms observed span)")
  if ($between -ge 1) { Write-Output "PASS transition animated (intermediate rects seen)" } else { Write-Output "FAIL no intermediate rect: snapped, not animated" }
  if (($between*40) -le 400) { Write-Output "PASS transition fast (<=~400ms)" } else { Write-Output "FAIL transition slow" }
} finally {
  Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
}
