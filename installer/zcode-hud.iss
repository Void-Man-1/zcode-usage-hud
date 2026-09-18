; ZCode Usage HUD — per-user install wizard (Inno Setup 6)
; Compile: ISCC.exe installer\zcode-hud.iss  (from the project root)
;
; Design notes:
;   - Per-user install (no admin): {autopf} resolves to
;     %LOCALAPPDATA%\Programs, matching the app's own installDir().
;   - The app self-manages its session data in
;     %LOCALAPPDATA%\ZCode Usage HUD; the installer never touches it,
;     so upgrading or reinstalling preserves the signed-in session.
;   - AppMutex lets Setup offer to close a running HUD before replacing
;     the exe (the app creates Local\ZCodeUsageHUD-v1 at startup).
;   - The optional "Start with Windows" task writes the same
;     HKCU\...\Run value the app's own tray-menu toggle uses, so both
;     stay in sync.

#define MyAppName "ZCode Usage HUD"
#define MyAppVersion "1.2.0"
#define MyAppPublisher "ZCode Usage HUD"
#define MyAppExeName "ZCode-Usage-HUD.exe"

[Setup]
AppId={{F37A9A2E-2D71-4F43-B5BE-C3BA77D4C775}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} v{#MyAppVersion}
AppPublisher={#MyAppPublisher}
VersionInfoVersion=1.2.0.0
VersionInfoProductVersion=1.2.0.0
DefaultDirName={autopf}\ZCode Usage HUD
DirExistsWarning=no
AppendDefaultDirName=no
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
PrivilegesRequired=lowest
WizardStyle=modern
SetupIconFile=app.ico
UninstallDisplayIcon={app}\{#MyAppExeName}
UninstallDisplayName={#MyAppName}
OutputDir=output
OutputBaseFilename=ZCode-Usage-HUD-v{#MyAppVersion}-Setup
Compression=lzma2/max
SolidCompression=yes
ArchitecturesInstallIn64BitMode=x64compatible
AppMutex=Local\ZCodeUsageHUD-v1
CloseApplications=yes
RestartApplications=no

[Tasks]
Name: "startwithwindows"; Description: "Start {#MyAppName} with Windows (compact mode)"; \
    GroupDescription: "Startup:"; Flags: checkedonce
Name: "desktopicon"; Description: "Create a &desktop shortcut"; \
    GroupDescription: "Additional icons:"; Flags: unchecked

[Files]
Source: "..\ZCode-Usage-HUD.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.txt"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Parameters: "--hud"; Comment: "Live ZCode token and promotion monitor"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Parameters: "--hud"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; \
    ValueType: string; ValueName: "ZCodeUsageHUD"; \
    ValueData: """{app}\{#MyAppExeName}"" --hud --collapsed --startup"; \
    Tasks: startwithwindows; Flags: uninsdeletevalue

[Run]
Filename: "{app}\{#MyAppExeName}"; Parameters: "--hud"; \
    Description: "Launch {#MyAppName}"; \
    Flags: nowait postinstall skipifsilent runasoriginaluser

[UninstallDelete]
; Intentionally does NOT delete %LOCALAPPDATA%\ZCode Usage HUD —
; the signed-in session and logs survive removal.
