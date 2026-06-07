# Architecture

## App Shape

- Wails desktop app.
- Go backend owns registration, account lifecycle, Gateway operations, subscription checks, and local persistence.
- Frontend lives under `frontend/`; `npm run build` copies assets to `frontend/dist/` for Wails packaging.

## Kiro Registration

- Main batch orchestration: `internal/task/coordinator.go`.
- Registration flow: `internal/core/run.go`, `internal/core/registrar.go`, `internal/core/signup_flow.go`, `internal/core/signup_password.go`.
- Saved successful accounts: `C:\Users\wujin\Documents\Kirox\accounts.json` by default.
- Runtime log: `<dataDir>/kiro-register.log`.

## Email Providers

- Provider selection comes from the register UI and is passed as `StartTaskRequest.EmailProvider`.
- Outlook uses stored Microsoft accounts and IMAP/OAuth polling.
- MoeMail and Cloud-Mail generate temporary or hosted mailboxes from UI-managed configs.
- Mail Manager uses `internal/email/mailmanager.go` and private runtime config at `%APPDATA%\kirox\mail-manager.local.json`.
- SMSB Gmail uses `internal/email/smsb_gmail.go`.
  - Default SMSB mail service is `aws`, domain `gmail.com`, max price `0.05`.
  - Config resolution order: explicit config, `KIROX_SMSB_*` env vars, `%APPDATA%\kirox\smsb.local.json`, defaults.
  - OTP timeout is 30 seconds for SMSB only; timeout cancels activation with `setStatus=2`.
  - OTP success completes activation with `setStatus=3`.

## Kiro Account Lifecycle And Gateway

- Local lifecycle UI/backend: `kiro_cli.go`, `frontend/js/kiro_cli.js`.
- Gateway panel/backend: `kiro_gateway.go`, `frontend/js/kiro_gateway.js`.
- Local lifecycle state: `<dataDir>/kiro-account-state.json`.
- Local Gateway JSON output: `<dataDir>/kiro-gateway-accounts`.
- Remote server credentials must stay outside the repo in `C:\Users\wujin\.codex\kiro_servers.local.json`.

## Private Data Rule

- Do not commit API keys, server passwords, Kiro tokens, refresh tokens, proxy credentials, local account JSON, or private server config.
- Private config files are expected under `%APPDATA%\kirox\*.local.json` or `C:\Users\wujin\.codex\*.local.json`.
