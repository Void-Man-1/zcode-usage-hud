# Contributing to ZCode Usage HUD

ZCode Usage HUD is an independent open-source project. Contributions,
experiments, documentation improvements, and forks are welcome.

## Before opening a change

Run the same checks used by continuous integration:

```powershell
go test ./...
go vet ./...
go build -trimpath -ldflags "-H=windowsgui -s -w" .
```

Keep real credentials, account identifiers, API responses, and personal
screenshots out of commits. Use the built-in `--preview` mode when creating
documentation images. The repository's `.gitignore` intentionally excludes
local executable builds, captures, logs, and Mimosa scanner state.

## Pull requests

Please describe the user-facing behavior, the Windows version or environment
used for testing, and any changes that depend on ZCode response formats. Keep
changes focused and update the README when a control, data source, or
privacy behavior changes.

## Scope

This project is not affiliated with ZCode or Z.AI. Contributions must not
imply endorsement by those organizations or claim access to private upstream
implementation details.
