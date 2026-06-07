# Project State

## Current

- Branch: `main`
- Remote: `origin https://github.com/thomas-coding/KiroX.git`
- Latest known base commit before this work: `1f570eb release: v1.0.3 - 多代理池 + 指纹按代理缓存 + Cloud-Mail/MoeMail 强制测试`
- Working tree expected after this handoff: clean after commit/push.

## Done Recently

- Added Kiro CLI account management UI and backend helpers.
  - Files: `kiro_cli.go`, `frontend/js/kiro_cli.js`, `process_windows.go`, `process_other.go`.
  - Supports account listing, real chat precheck before import, import into official Kiro CLI auth DB, delete suspended accounts, custom confirm modal, and persistent account log at `<dataDir>/kiro-cli-account.log`.
- Added Mail Manager email provider for Kiro registration.
  - Files: `internal/email/mailmanager.go`, `internal/task/coordinator.go`, `internal/core/config.go`, `internal/core/registrar.go`, `internal/core/signup_flow.go`, `frontend/index.html`, `frontend/js/ui.js`, `frontend/js/app.js`, `frontend/js/i18n.js`.
  - UI exposes only provider type: `hotmail`, `icloud`, `cf_gmail`, `manual`.
  - Backend uses `project=kiro`, parser `kiro_otp`, validates provider type, records `sent_after_unix` before send-otp, binds long OTP wait to task context, and marks failed leases on failure/cancel.
- Added persistent registration logs.
  - Path: `<dataDir>/kiro-register.log`.
  - Running-log page has a "复制日志路径" button via `GetRegisterLogPath`.
- Fixed Windows console popups for helper processes and app icon packaging.
- Build/test passed before handoff:
  - `go test ./...`
  - `node --check frontend/js/task.js frontend/js/i18n.js`
  - `npm run build`
  - `wails build`

## Configuration

- Mail Manager base URL defaults to `http://43.162.94.131:8097`.
- Mail Manager API key is not committed. Runtime must set:
  - `KIROX_MAIL_MANAGER_API_KEY`
  - Optional override: `KIROX_MAIL_MANAGER_URL`
- Registration output defaults to `storage.GetResultOutputDir()`; app data uses `storage.GetDataDir()`.

## Observed Test Result

- Recent Mail Manager runs did lease mailboxes successfully:
  - `rudzikfusch7152@hotmail.com`
  - `11.layer_midsole@icloud.com`
- Both failed before email submission at `[1] OIDC 注册` due to proxy `504 Gateway Timeout`.
- The issue was proxy/network, not Mail Manager or OTP.

## Risks

- Mail Manager early failures currently call `/fail`, not `/release`, even when failure occurs before submitting email to Kiro. This avoids dangling active leases but may cool down the mailbox depending on server policy.
- Retry currently reuses the same proxy within one task. A proxy 504 repeats on retry unless a new task picks another proxy.
- `go.mod` now uses `go 1.25.0` after dependency changes including `modernc.org/sqlite`; do not revert unless toolchain compatibility requires it.

## Next Step

- For next registration test, ensure `KIROX_MAIL_MANAGER_API_KEY` is set in the environment that launches `kirox.exe`, then test with a different proxy or direct connection.
- If mailbox reuse matters, consider changing pre-email-submit failures from Mail Manager `/fail` to `/release`.

## First Files To Read

- `PROJECT_STATE.md`
- `internal/email/mailmanager.go`
- `internal/task/coordinator.go`
- `app.go`
- `kiro_cli.go`
- `frontend/js/app.js`
- `frontend/js/ui.js`
- `frontend/js/task.js`
