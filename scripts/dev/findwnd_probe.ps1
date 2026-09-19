$ErrorActionPreference = 'Stop'
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class FP {
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern IntPtr FindWindowW(string cls, string title);
  [DllImport("user32.dll", CharSet = CharSet.Unicode, EntryPoint = "FindWindowW")] public static extern IntPtr FindWindowU([MarshalAs(UnmanagedType.LPWStr)] string cls, [MarshalAs(UnmanagedType.LPWStr)] string title);
}
"@
$h1 = [FP]::FindWindowW('Shell_TrayWnd', $null)
Write-Output ("Shell_TrayWnd via FindWindowW: {0}" -f $h1)
$h2 = [FP]::FindWindowU('Shell_TrayWnd', $null)
Write-Output ("Shell_TrayWnd via FindWindowU: {0}" -f $h2)
$h3 = [FP]::FindWindowW('NotAClassXYZ123', $null)
Write-Output ("bogus class (expect 0): {0}" -f $h3)
