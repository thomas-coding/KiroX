# Project State

## Current

- Branch: `main`
- Remote: `origin https://github.com/thomas-coding/KiroX.git`
- Last completed work: Kiro account lifecycle UI, Gateway panel, Mail Manager local config fallback.
- Working tree target: clean after commit/push.

## Done Recently

- Reworked Kiro account management into a lifecycle page.
  - Files: `kiro_cli.go`, `frontend/js/kiro_cli.js`, `frontend/index.html`, `frontend/css/style.css`.
  - Tracks local lifecycle in `<dataDir>/kiro-account-state.json`.
  - Supports precheck, Gateway JSON generation, upload to Gateway, local CLI import, mark limited/suspended/retired, delete, and persistent account logs.
  - Gateway JSON output directory: `<dataDir>/kiro-gateway-accounts`.
- Added remote Kiro Gateway panel.
  - Files: `kiro_gateway.go`, `frontend/js/kiro_gateway.js`.
  - Reads private server config from `C:\Users\wujin\.codex\kiro_servers.local.json`.
  - Shows health, container status, remote account JSON files, per-account failures, and request stats.
  - Reads stats from configured host state path first, then falls back to container `/app/state.json`.
  - Supports remote JSON delete, Gateway restart, and chat smoke test.
- Added one-click Gateway upload from the local lifecycle page.
  - Upload uses `pscp`, restarts the configured Gateway container, and marks local lifecycle `gateway_uploaded`.
  - Server credentials remain outside the repo.
- Added Mail Manager local private config fallback.
  - File: `internal/email/mailmanager.go`.
  - Resolution order: explicit config, `KIROX_MAIL_MANAGER_*` env vars, `%APPDATA%\kirox\mail-manager.local.json`, then default URL.
  - API key is not committed.
- Increased Wails default window size.
  - Default: `1280x820`; minimum: `1100x680`.

## Local Runtime State

- Kiro registration logs: `<dataDir>/kiro-register.log`.
- Kiro account logs: `<dataDir>/kiro-cli-account.log`.
- Local lifecycle state: `<dataDir>/kiro-account-state.json`.
- Local Gateway JSON output: `<dataDir>/kiro-gateway-accounts`.
- Private Mail Manager config on this machine: `%APPDATA%\kirox\mail-manager.local.json`.
- Private old-server config on this machine: `C:\Users\wujin\.codex\kiro_servers.local.json`.

## Observed Results

- `11.layer_midsole@icloud.com` is uploaded on Gateway and has nonzero request stats.
- `DarrellJohnson2520@outlook.pt` precheck, Gateway JSON export, Gateway upload, and container restart completed successfully.
- Latest Mail Manager failure at `2026-06-07 16:04:21` was caused by missing runtime API key; local private config fallback was added and local config was created afterward.

## Risks

- Remote Gateway delete only removes the server JSON; it does not remove the local KiroX account.
- Local KiroX account delete removes local lifecycle and local Gateway JSON, but it does not remove remote Gateway JSON.
- Manually uploaded legacy Gateway JSON may not include `email`; the Gateway panel can still show it by file name.
- Mail Manager early failures currently call `/fail`, not `/release`, even when failure occurs before submitting email to Kiro.

## Verification

- `go test ./...`
- `npm run build`
- `wails build`

## Next Step

- After pulling fresh code, launch `build/bin/kirox.exe`, use Gateway panel `刷新状态`, and confirm request stats load from `/app/state.json` fallback.
- For future account replacement: register account, precheck, generate Gateway JSON, upload Gateway, then verify on Gateway panel.

## First Files To Read

- `PROJECT_STATE.md`
- `kiro_cli.go`
- `kiro_gateway.go`
- `internal/email/mailmanager.go`
- `frontend/js/kiro_cli.js`
- `frontend/js/kiro_gateway.js`
- `frontend/index.html`
