# Offline/first-run behavior check (req 3): the HUD must work standalone.
# Phase A: real (non-preview) mode via --hud, redirected LOCALAPPDATA with
#   NO credentials and NO ZCode dir — fresh-install state. The app must
#   launch, survive >=1 refresh tick, and stay responsive.
# Phase B: credentials.json with a PLAINTEXT bogus JWT (decryptCredential
#   passes non-enc values through) — the real API rejects it. The app must
#   stay alive, never crash, and exit cleanly via --quit.
# NOTE: a bare exe launch without --hud means INSTALL (run-equals-install
#   contract) — phase A originally installed instead of running, leaving a
#   Start Menu folder and uninstall key behind. Both fixed here.
$ErrorActionPreference = 'Stop'
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class W32O {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc cb, IntPtr l);
  public delegate bool EnumWindowsProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern int GetClassNameW(IntPtr h, [MarshalAs(UnmanagedType.LPWStr)] StringBuilder sb, int max);
  [DllImport("user32.dll")] public static extern bool PostMessageW(IntPtr h, uint m, IntPtr w, IntPtr l);
  public struct R { public int Left, Top, Right, Bottom; }
}
"@
[W32O]::SetProcessDPIAware() | Out-Null

$env:LOCALAPPDATA = 'C:\Users\Void_LLM\AppData\Local\Temp\hudoff'
$dir = Join-Path $env:LOCALAPPDATA 'ZCode Usage HUD'
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$exe = Join-Path (Get-Location) 'ZCode-Usage-HUD.exe'
$script:fails = 0

function Get-Wnd([uint32]$procid, [string]$cls) {
  for ($i = 0; $i -lt 20; $i++) {
    $script:hit = [IntPtr]::Zero
    $cb = [W32O+EnumWindowsProc]{ param($h, $l)
      $p2 = 0
      [W32O]::GetWindowThreadProcessId($h, [ref]$p2) | Out-Null
      if ($p2 -eq $procid) {
        $sb = New-Object System.Text.StringBuilder 256
        [W32O]::GetClassNameW($h, $sb, 256) | Out-Null
        if ($sb.ToString() -eq $cls) { $script:hit = $h }
      }
      return $true
    }
    [W32O]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
    if ($script:hit -ne [IntPtr]::Zero) { return $script:hit }
    Start-Sleep -Milliseconds 500
  }
  return [IntPtr]::Zero
}

function Assert-Alive([System.Diagnostics.Process]$proc, [int]$seconds, [string]$tag) {
  $deadline = (Get-Date).AddSeconds($seconds)
  while ((Get-Date) -lt $deadline) {
    if ($proc.HasExited) {
      Write-Output ("FAIL {0}: process exited early (code {1})" -f $tag, $proc.ExitCode)
      $script:fails++
      return $false
    }
    Start-Sleep -Milliseconds 500
  }
  if ($proc.HasExited) {
    Write-Output ("FAIL {0}: process exited during check" -f $tag); $script:fails++; return $false
  }
  Write-Output ("PASS {0}: alive after {1}s" -f $tag, $seconds)
  return $true
}

function Assert-LogClean([string]$logPath, [string]$tag) {
  if (Test-Path $logPath) {
    $content = Get-Content $logPath -Raw
    if ($content -match 'panic|runtime error|fatal') {
      Write-Output ("FAIL {0}: log contains panic/fatal" -f $tag); $script:fails++
    } else {
      Write-Output ("PASS {0}: log clean ({1} lines)" -f $tag, ($content -split "`n").Count)
    }
    return $content
  }
  Write-Output ("PASS {0}: no log written (clean run)" -f $tag)
  return ""
}

# ---------- Phase A: fresh install, zero credentials ----------
Remove-Item (Join-Path $dir 'credentials.json') -Force -ErrorAction SilentlyContinue
Remove-Item (Join-Path $dir 'hud.log') -Force -ErrorAction SilentlyContinue
$pA = Start-Process -FilePath $exe -ArgumentList '--hud' -PassThru
try {
  $w = Get-Wnd $pA.Id 'ZCodeUsageHUDV1'
  if ($w -eq [IntPtr]::Zero) { Write-Output 'FAIL phase A: window not found'; $script:fails++ }
  else { Write-Output 'PASS phase A: real window up without any credentials' }
  if (Assert-Alive $pA 7 'phase A') { }   # startup refresh + throttle window
  Assert-LogClean (Join-Path $dir 'hud.log') 'phase A'
} finally {
  & $exe --quit 2>$null | Out-Null
  Start-Sleep -Milliseconds 2500
  if (-not $pA.HasExited) {
    Write-Output 'FAIL phase A: --quit did not exit the app'; $script:fails++
    Stop-Process -Id $pA.Id -Force -ErrorAction SilentlyContinue
  } else {
    Write-Output 'PASS phase A: clean --quit exit'
  }
}

# ---------- Phase B: plaintext bogus JWT, real API rejects ----------
@'
{"zcodejwttoken":"bogus.jwt.token.for.offline.harness","email":"harness@example.com","name":"Harness"}
'@ | Set-Content (Join-Path $dir 'credentials.json') -Encoding UTF8
Remove-Item (Join-Path $dir 'hud.log') -Force -ErrorAction SilentlyContinue
$pB = Start-Process -FilePath $exe -ArgumentList '--hud' -PassThru
try {
  $w2 = Get-Wnd $pB.Id 'ZCodeUsageHUDV1'
  if ($w2 -eq [IntPtr]::Zero) { Write-Output 'FAIL phase B: window not found'; $script:fails++ }
  else { Write-Output 'PASS phase B: window up with bogus credentials' }
  if (Assert-Alive $pB 9 'phase B') { }
  Assert-LogClean (Join-Path $dir 'hud.log') 'phase B'
} finally {
  & $exe --quit 2>$null | Out-Null
  Start-Sleep -Milliseconds 2500
  if (-not $pB.HasExited) {
    Write-Output 'FAIL phase B: --quit did not exit the app'; $script:fails++
    Stop-Process -Id $pB.Id -Force -ErrorAction SilentlyContinue
  } else {
    Write-Output 'PASS phase B: clean --quit exit'
  }
}

Write-Output ("RESULT failures={0}" -f $script:fails)
exit ([int]($script:fails -gt 0))
