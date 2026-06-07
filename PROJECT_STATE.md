# Project State

## Current

- Branch: `main`
- Remote: `origin https://github.com/thomas-coding/KiroX.git`
- Last completed work: SMSB Gmail provider for Kiro registration, defaulting to `aws/gmail.com`.
- Working tree target: clean after commit/push.

## Done Recently

- Reworked Kiro account management into a lifecycle page.
  - Files: `kiro_cli.go`, `frontend/js/kiro_cli.js`, `frontend/index.html`, `frontend/css/style.css`.
  - Tracks local lifecycle in `<dataDir>/kiro-account-state.json`.
  - Supports precheck, Gateway JSON generation, one-click Gateway upload, local CLI import, mark limited/suspended/retired, delete, and persistent account logs.
- Added remote Kiro Gateway panel.
  - Files: `kiro_gateway.go`, `frontend/js/kiro_gateway.js`.
  - Reads private server config from `C:\Users\wujin\.codex\kiro_servers.local.json`.
  - Shows health, container status, remote account JSON files, per-account failures, and request stats.
- Added Mail Manager local private config fallback.
  - File: `internal/email/mailmanager.go`.
  - Resolution order: explicit config, `KIROX_MAIL_MANAGER_*` env vars, `%APPDATA%\kirox\mail-manager.local.json`, then default URL.
- Added SMSB Gmail provider for Kiro registration.
  - Files: `internal/email/smsb_gmail.go`, `internal/task/coordinator.go`, `internal/core/signup_flow.go`.
  - Default service is `aws`, domain `gmail.com`, `maxPrice=0.05`; local override lives in `%APPDATA%\kirox\smsb.local.json`.
  - Each mailbox waits 30 seconds for OTP, cancels with SMSB `setStatus=2` on timeout, and retries up to 3 mailboxes.
  - Successful OTP capture completes the activation with SMSB `setStatus=3`.

## Local Runtime State

- Kiro registration logs: `<dataDir>/kiro-register.log`.
- Kiro account logs: `<dataDir>/kiro-cli-account.log`.
- Local lifecycle state: `<dataDir>/kiro-account-state.json`.
- Local Gateway JSON output: `<dataDir>/kiro-gateway-accounts`.
- Private Mail Manager config: `%APPDATA%\kirox\mail-manager.local.json`.
- Private SMSB config: `%APPDATA%\kirox\smsb.local.json`.
- Private old-server config: `C:\Users\wujin\.codex\kiro_servers.local.json`.

## Observed Results

- `11.layer_midsole@icloud.com` is uploaded on Gateway and has nonzero request stats.
- `DarrellJohnson2520@outlook.pt` precheck, Gateway JSON export, Gateway upload, and container restart completed successfully.
- SMSB `am/gmail.com` repeatedly timed out on Kiro OTP.
- SMSB `aws/gmail.com` succeeded on the second mailbox at `2026-06-07 18:40:42`; saved account `dufrene899@gmail.com`.

## Risks

- Remote Gateway delete only removes the server JSON; it does not remove the local KiroX account.
- Local KiroX account delete removes local lifecycle and local Gateway JSON, but it does not remove remote Gateway JSON.
- Manually uploaded legacy Gateway JSON may not include `email`; the Gateway panel can still show it by file name.
- Mail Manager early failures currently call `/fail`, not `/release`, even when failure occurs before submitting email to Kiro.
- SMSB Gmail is short-lived; once OTP is received, the mailbox is completed and cannot be used for future account repair.

## Verification

- `go test ./...`
- `npm run build`
- `wails build`

## Next Step

- Launch `build/bin/kirox.exe`, run one `SMSB Gmail` registration with count `1` and concurrency `1`, then confirm logs show `aws/gmail.com`, 30-second timeout/release behavior, and account save on success.
- For future account replacement: register account, precheck, generate Gateway JSON, upload Gateway, then verify on Gateway panel.

## First Files To Read

- `PROJECT_STATE.md`
- `ARCHITECTURE.md`
- `internal/email/smsb_gmail.go`
- `internal/task/coordinator.go`
- `internal/core/signup_flow.go`
- `kiro_cli.go`
- `kiro_gateway.go`
