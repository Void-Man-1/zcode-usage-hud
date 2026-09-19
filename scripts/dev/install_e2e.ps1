# E2E: install silently while the HUD runs, verify upgrade + close; then
# uninstall silently while running, verify no prompt + clean removal.
$ErrorActionPreference = 'Stop'
$setup = Join-Path (Get-Location) 'releases/v1.2.1/ZCode-Usage-HUD-v1.2.1-Setup.exe'
$exe = "$env:LOCALAPPDATA\Programs\ZCode Usage HUD\ZCode-Usage-HUD.exe"

# --- Phase 0: fresh install (nothing running) ---
if (Get-Process -Name 'ZCode-Usage-HUD' -ErrorAction SilentlyContinue) {
  & "$env:LOCALAPPDATA\Programs\ZCode Usage HUD\ZCode-Usage-HUD.exe" --quit 2>$null | Out-Null
  Start-Sleep -Seconds 2
}
Start-Process -FilePath $setup -ArgumentList '/VERYSILENT','/NORESTART' -Wait
if (-not (Test-Path $exe)) { Write-Output 'FAIL fresh install did not place exe'; exit 1 }
Write-Output 'PASS fresh install'

# --- Phase 1: install again while a HUD is running ---
Start-Process -FilePath $exe | Out-Null
Start-Sleep -Seconds 3
$running = Get-Process -Name 'ZCode-Usage-HUD' -ErrorAction SilentlyContinue
if (-not $running) { Write-Output 'FAIL could not start HUD for upgrade test'; exit 1 }
Write-Output 'HUD running before upgrade'
$sw = [System.Diagnostics.Stopwatch]::StartNew()
Start-Process -FilePath $setup -ArgumentList '/VERYSILENT','/NORESTART' -Wait
$sw.Stop()
Start-Sleep -Seconds 1
$after = Get-Process -Name 'ZCode-Usage-HUD' -ErrorAction SilentlyContinue
$installed = Test-Path $exe
$ver = if ($installed) { (Get-FileHash $exe -Algorithm SHA256).Hash } else { 'missing' }
$want = (Get-FileHash (Join-Path (Get-Location) 'ZCode-Usage-HUD.exe') -Algorithm SHA256).Hash
Write-Output ("upgrade-install: {0:N1}s installed={1} version={2} processes-after={3}" -f $sw.Elapsed.TotalSeconds, $installed, $ver, ($after | Measure-Object).Count)
if (-not $installed) { Write-Output 'FAIL upgrade install did not place exe'; exit 1 }
if ($ver -ne $want) { Write-Output 'FAIL installed exe does not match the built exe'; exit 1 }
Write-Output 'PASS upgrade over running HUD (silent, no prompts)'

# --- Phase 2: launch again, then uninstall while running ---
Start-Process -FilePath $exe | Out-Null
Start-Sleep -Seconds 3
$running = Get-Process -Name 'ZCode-Usage-HUD' -ErrorAction SilentlyContinue
if (-not $running) { Write-Output 'FAIL could not start HUD for uninstall test'; exit 1 }
Write-Output 'HUD running before uninstall'
$unins = Get-ChildItem "$env:LOCALAPPDATA\Programs\ZCode Usage HUD" -Filter 'unins*.exe' -Recurse | Select-Object -First 1
if (-not $unins) { Write-Output 'FAIL uninstaller not found'; exit 1 }
$sw = [System.Diagnostics.Stopwatch]::StartNew()
Start-Process -FilePath $unins.FullName -ArgumentList '/VERYSILENT','/NORESTART' -Wait
$sw.Stop()
Start-Sleep -Seconds 1
$after2 = Get-Process -Name 'ZCode-Usage-HUD' -ErrorAction SilentlyContinue
$still = Test-Path $exe
Write-Output ("uninstall: {0:N1}s exe-removed={1} processes-after={2}" -f $sw.Elapsed.TotalSeconds, (-not $still), ($after2 | Measure-Object).Count)
if ($still) { Write-Output 'FAIL exe still present after uninstall'; exit 1 }
if (($after2 | Measure-Object).Count -gt 0) { Write-Output 'FAIL HUD process survived uninstall'; exit 1 }
Write-Output 'PASS uninstall while running (silent, no warning)'
