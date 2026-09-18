; ZCode Usage HUD — per-user install wizard (Inno Setup 6)
; Compile: ISCC.exe installer\zcode-hud.iss  (from the project root)
;
; Notes for maintainers:
;   - Per-user install (no admin): {autopf} resolves to
;     %LOCALAPPDATA%\Programs, matching the app's own installDir().
;   - The app self-manages its session data in
;     %LOCALAPPDATA%\ZCode Usage HUD; the installer never touches it,
;     so upgrading or reinstalling preserves the signed-in session.
;   - Uninstalling closes a running HUD automatically (graceful --quit first,
;     then a force-kill fallback after a short wait), so removal never blocks.
;   - The optional "Start with Windows" task writes the same
;     HKCU\...\Run value the app's own tray-menu toggle uses, so both
;     stay in sync.

#define MyAppName "ZCode Usage HUD"
#define MyAppVersion "1.5.0"
#define MyAppPublisher "ZCode Usage HUD"
#define MyAppExeName "ZCode-Usage-HUD.exe"

[Setup]
AppId={{F37A9A2E-2D71-4F43-B5BE-C3BA77D4C775}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} v{#MyAppVersion}
AppPublisher={#MyAppPublisher}
VersionInfoVersion=1.5.0.0
VersionInfoProductVersion=1.5.0.0
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
; Versioned per-release folder: releases\v{#MyAppVersion}\ (see RELEASES.md)
OutputDir=..\releases\v{#MyAppVersion}
OutputBaseFilename=ZCode-Usage-HUD-v{#MyAppVersion}-Setup
Compression=lzma2/max
SolidCompression=yes
ArchitecturesInstallIn64BitMode=x64compatible
RestartApplications=no

[Tasks]
Name: "startwithwindows"; Description: "Start {#MyAppName} with Windows in compact mode"; \
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

[Code]
// Close a running HUD without prompting and without depending on the
// installed exe's generation (old versions have no --quit): post
// WM_CLOSE to the HUD window, wait briefly, then force-kill by image
// name so removal never blocks on a running app.
const
  WM_CLOSE_LOCAL = $0010;

function FindWindowW(Cls: string; Title: Longint): HWND;
  external 'FindWindowW@user32.dll stdcall';
procedure PostMessageW(H: HWND; Msg: UINT; W, L: Longint);
  external 'PostMessageW@user32.dll stdcall';

procedure CloseRunningHUD;
var
  Wnd: HWND;
  Res: Integer;
begin
  Wnd := FindWindowW('ZCodeUsageHUDV1', 0);
  if Wnd <> 0 then
  begin
    PostMessageW(Wnd, WM_CLOSE_LOCAL, 0, 0);
    Sleep(1200);
  end;
  Exec('taskkill.exe', '/F /IM ZCode-Usage-HUD.exe', '', SW_HIDE,
       ewWaitUntilTerminated, Res);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := '';
  CloseRunningHUD;
end;

function InitializeUninstall(): Boolean;
begin
  Result := True;
  CloseRunningHUD;
end;

[UninstallDelete]
; Intentionally does NOT delete %LOCALAPPDATA%\ZCode Usage HUD —
; the signed-in session and logs survive removal.
