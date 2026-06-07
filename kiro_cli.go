package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/google/uuid"
	"reg_go/internal/data"
	httputil "reg_go/internal/http"
	"reg_go/internal/storage"

	_ "modernc.org/sqlite"
)

const (
	kiroCliExePath = `C:\Users\wujin\AppData\Local\Kiro-Cli\kiro-cli.exe`
	kiroCliDbPath  = `C:\Users\wujin\AppData\Local\Kiro-Cli\data.sqlite3`
)

var kiroCliLogMu sync.Mutex

type kiroCliAccount struct {
	Email        string `json:"email"`
	RefreshToken string `json:"refreshToken"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Region       string `json:"region"`
	Provider     string `json:"provider"`
	Subscription string `json:"subscription"`
	Time         string `json:"time"`
}

type kiroCliToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

func kiroCliLog(action, email, stage, message string) {
	if email == "" {
		email = "<unknown>"
	}
	line := fmt.Sprintf("[Kiro CLI][%s][%s][%s] %s", action, email, stage, message)
	log.Print(line)
	appendKiroCliLogFile(line)
}

func kiroCliLogPath() string {
	return filepath.Join(storage.GetDataDir(), "kiro-cli-account.log")
}

func appendKiroCliLogFile(line string) {
	kiroCliLogMu.Lock()
	defer kiroCliLogMu.Unlock()
	path := kiroCliLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	entry := time.Now().Format("2006-01-02 15:04:05") + " " + line + "\n"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(entry)
}

func kiroCliFail(action, email, stage string, err error) map[string]interface{} {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	kiroCliLog(action, email, stage, "失败: "+truncateKiroCliText(msg, 800))
	return map[string]interface{}{
		"success": false,
		"stage":   stage,
		"error":   msg,
	}
}

func accountFromOutputMap(m map[string]interface{}) kiroCliAccount {
	get := func(k string) string { v, _ := m[k].(string); return v }
	region := get("region")
	if region == "" {
		region = "us-east-1"
	}
	return kiroCliAccount{
		Email:        get("email"),
		RefreshToken: get("refreshToken"),
		ClientID:     get("clientId"),
		ClientSecret: get("clientSecret"),
		Region:       region,
		Provider:     get("provider"),
		Subscription: get("subscription"),
		Time:         get("time"),
	}
}

func findKiroCliAccount(email string) (kiroCliAccount, error) {
	items, err := data.LoadAccounts(storage.GetResultOutputDir())
	if err != nil {
		return kiroCliAccount{}, err
	}
	for _, m := range items {
		acc := accountFromOutputMap(m)
		if acc.Email == email {
			return acc, nil
		}
	}
	return kiroCliAccount{}, fmt.Errorf("未找到账号: %s", email)
}

func validateKiroCliAccount(acc kiroCliAccount) error {
	if acc.Email == "" {
		return fmt.Errorf("账号缺少 email")
	}
	if acc.RefreshToken == "" || acc.ClientID == "" || acc.ClientSecret == "" {
		return fmt.Errorf("账号缺少 refreshToken/clientId/clientSecret")
	}
	if acc.Region == "" {
		acc.Region = "us-east-1"
	}
	return nil
}

func refreshKiroCliToken(acc kiroCliAccount) (kiroCliToken, error) {
	if err := validateKiroCliAccount(acc); err != nil {
		return kiroCliToken{}, err
	}
	payload, _ := json.Marshal(map[string]string{
		"clientId":     acc.ClientID,
		"clientSecret": acc.ClientSecret,
		"refreshToken": acc.RefreshToken,
		"grantType":    "refresh_token",
	})
	client := httputil.NewTLSClient("", true)
	req, _ := fhttp.NewRequest("POST", fmt.Sprintf("https://oidc.%s.amazonaws.com/token", acc.Region), bytes.NewReader(payload))
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-amz-user-agent", "aws-sdk-js/3.980.0 KiroIDE")
	req.Header.Set("user-agent", "aws-sdk-js/3.980.0 ua/2.1 os/windows#10.0.19045 lang/js md/nodejs#22.21.1 api/sso-oidc#3.980.0 m/E KiroIDE")
	req.Header.Set("host", fmt.Sprintf("oidc.%s.amazonaws.com", acc.Region))
	req.Header.Set("amz-sdk-invocation-id", uuid.NewString())
	req.Header.Set("amz-sdk-request", "attempt=1; max=4")
	req.Header.Set("Connection", "close")
	resp, err := client.Do(req)
	if err != nil {
		return kiroCliToken{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return kiroCliToken{}, fmt.Errorf("刷新 token 失败 HTTP %d: %s", resp.StatusCode, truncateKiroCliText(string(body), 1000))
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return kiroCliToken{}, err
	}
	access, _ := raw["accessToken"].(string)
	if access == "" {
		access, _ = raw["access_token"].(string)
	}
	if access == "" {
		return kiroCliToken{}, fmt.Errorf("刷新响应缺少 accessToken")
	}
	refresh := acc.RefreshToken
	if v, _ := raw["refreshToken"].(string); v != "" {
		refresh = v
	} else if v, _ := raw["refresh_token"].(string); v != "" {
		refresh = v
	}
	expiresIn := int64(3600)
	if v, ok := raw["expiresIn"].(float64); ok && v > 0 {
		expiresIn = int64(v)
	} else if v, ok := raw["expires_in"].(float64); ok && v > 0 {
		expiresIn = int64(v)
	}
	return kiroCliToken{AccessToken: access, RefreshToken: refresh, ExpiresIn: expiresIn}, nil
}

func kiroRsMachineID(refreshToken string) string {
	sum := sha256.Sum256([]byte("KotlinNativeAPI/" + refreshToken))
	return hex.EncodeToString(sum[:])
}

func precheckKiroCliChat(acc kiroCliAccount, token kiroCliToken) (map[string]interface{}, error) {
	machineID := kiroRsMachineID(acc.RefreshToken)
	body, _ := json.Marshal(map[string]interface{}{
		"conversationState": map[string]interface{}{
			"agentTaskType":   "vibe",
			"chatTriggerType": "MANUAL",
			"currentMessage": map[string]interface{}{
				"userInputMessage": map[string]interface{}{
					"userInputMessageContext": map[string]interface{}{},
					"content":                 "只输出 OK",
					"modelId":                 "auto",
					"origin":                  "AI_EDITOR",
				},
			},
			"conversationId": uuid.NewString(),
			"history":        []interface{}{},
		},
	})
	host := fmt.Sprintf("q.%s.amazonaws.com", acc.Region)
	client := httputil.NewTLSClient("", true)
	req, _ := fhttp.NewRequest("POST", "https://"+host+"/generateAssistantResponse", bytes.NewReader(body))
	req.Header.Set("content-type", "application/json")
	req.Header.Set("Connection", "close")
	req.Header.Set("x-amzn-codewhisperer-optout", "true")
	req.Header.Set("x-amzn-kiro-agent-mode", "vibe")
	req.Header.Set("x-amz-user-agent", "aws-sdk-js/1.0.34 KiroIDE-0.9.2-"+machineID)
	req.Header.Set("user-agent", "aws-sdk-js/1.0.34 ua/2.1 os/windows#10.0.19045 lang/js md/nodejs#22.21.1 api/codewhispererstreaming#1.0.34 m/E KiroIDE-0.9.2-"+machineID)
	req.Header.Set("host", host)
	req.Header.Set("amz-sdk-invocation-id", uuid.NewString())
	req.Header.Set("amz-sdk-request", "attempt=1; max=3")
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	result := map[string]interface{}{
		"statusCode": resp.StatusCode,
		"machineId":  machineID,
	}
	if resp.StatusCode != 200 {
		result["error"] = string(respBody)
		result["suspended"] = isKiroCliSuspendedText(string(respBody))
		return result, fmt.Errorf("真实 chat 预检失败 HTTP %d: %s", resp.StatusCode, truncateKiroCliText(string(respBody), 600))
	}
	if !strings.Contains(string(respBody), "OK") {
		result["error"] = truncateKiroCliText(string(respBody), 600)
		return result, fmt.Errorf("真实 chat 预检未返回 OK")
	}
	result["ok"] = true
	return result, nil
}

func isKiroCliSuspendedText(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "suspended") ||
		strings.Contains(lower, "locked your account") ||
		strings.Contains(lower, "accessdeniedexception")
}

func truncateKiroCliText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func writeOfficialKiroCliAuth(acc kiroCliAccount, token kiroCliToken) (string, error) {
	if _, err := os.Stat(kiroCliDbPath); err != nil {
		return "", fmt.Errorf("Kiro CLI 数据库不存在: %s", kiroCliDbPath)
	}
	backup := kiroCliDbPath + ".bak-kirox-" + time.Now().Format("20060102-150405")
	if err := copyFile(kiroCliDbPath, backup); err != nil {
		return "", fmt.Errorf("备份 Kiro CLI 数据库失败: %w", err)
	}
	expiresAt := time.Now().UTC().Add(time.Duration(maxInt64(60, token.ExpiresIn-60)) * time.Second).Format("2006-01-02T15:04:05Z")
	device := map[string]interface{}{
		"client_id":                acc.ClientID,
		"client_secret":            acc.ClientSecret,
		"client_secret_expires_at": time.Now().UTC().Add(90 * 24 * time.Hour).Format("2006-01-02T15:04:05Z"),
		"region":                   acc.Region,
		"oauth_flow":               "DeviceCode",
		"scopes": []string{
			"codewhisperer:completions",
			"codewhisperer:analysis",
			"codewhisperer:conversations",
		},
	}
	tokenState := map[string]interface{}{
		"access_token":  token.AccessToken,
		"refresh_token": token.RefreshToken,
		"token_type":    "Bearer",
		"expires_at":    expiresAt,
		"region":        acc.Region,
		"oauth_flow":    "DeviceCode",
	}
	deviceJSON, _ := json.Marshal(device)
	tokenJSON, _ := json.Marshal(tokenState)
	if err := writeKiroCliAuthRows(string(deviceJSON), string(tokenJSON)); err != nil {
		return backup, err
	}
	return backup, nil
}

func writeKiroCliAuthRows(deviceJSON, tokenJSON string) error {
	db, err := sql.Open("sqlite", kiroCliDbPath)
	if err != nil {
		return fmt.Errorf("打开 Kiro CLI 数据库失败: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("设置 SQLite busy_timeout 失败: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开启 SQLite 事务失败: %w", err)
	}
	defer tx.Rollback()

	const upsert = `INSERT INTO auth_kv(key, value) VALUES(?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value`
	if _, err := tx.Exec(upsert, "kirocli:odic:device-registration", deviceJSON); err != nil {
		return fmt.Errorf("写入 device-registration 失败: %w", err)
	}
	if _, err := tx.Exec(upsert, "kirocli:odic:token", tokenJSON); err != nil {
		return fmt.Errorf("写入 token 失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 SQLite 事务失败: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func restoreOfficialKiroCliAuth(backup string) error {
	if backup == "" {
		return fmt.Errorf("缺少 Kiro CLI 数据库备份路径")
	}
	return copyFile(backup, kiroCliDbPath)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func runKiroCli(args ...string) (string, error) {
	cmd := exec.Command(kiroCliExePath, args...)
	hideCommandWindow(cmd)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		return text, fmt.Errorf("%w: %s", err, text)
	}
	if isKiroCliSuspendedText(text) || strings.Contains(text, "Authentication failed") || strings.Contains(text, "session may have expired") {
		return text, fmt.Errorf("Kiro CLI 验证失败: %s", truncateKiroCliText(text, 600))
	}
	return text, nil
}

// LoadKiroCliAccounts 读取输出账号并显示当前官方 Kiro CLI 登录邮箱。
func (a *App) LoadKiroCliAccounts() map[string]interface{} {
	const action = "load"
	kiroCliLog(action, "-", "load-accounts", "开始加载账号列表")
	items, err := data.LoadAccounts(storage.GetResultOutputDir())
	if err != nil {
		return kiroCliFail(action, "-", "load-accounts", err)
	}
	current := ""
	if _, err := os.Stat(kiroCliExePath); err == nil {
		kiroCliLog(action, "-", "detect-current-cli", "读取当前官方 Kiro CLI 登录账号")
		if out, err := runKiroCli("whoami"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "Email:") {
					current = strings.TrimSpace(strings.TrimPrefix(line, "Email:"))
				}
			}
			if current != "" {
				kiroCliLog(action, current, "detect-current-cli", "当前官方 Kiro CLI 账号已识别")
			}
		} else {
			kiroCliLog(action, "-", "detect-current-cli", "读取当前官方 Kiro CLI 账号失败: "+truncateKiroCliText(err.Error(), 500))
		}
	}
	kiroCliLog(action, "-", "load-accounts", fmt.Sprintf("加载完成，共 %d 个账号", len(items)))
	return map[string]interface{}{
		"success":      true,
		"accounts":     items,
		"outputDir":    storage.GetResultOutputDir(),
		"currentEmail": current,
		"logPath":      kiroCliLogPath(),
	}
}

// PrecheckKiroCliAccount 对账号做真实 chat 预检，不写入官方 CLI。
func (a *App) PrecheckKiroCliAccount(email string) map[string]interface{} {
	const action = "precheck"
	kiroCliLog(action, email, "find-account", "开始预检")
	acc, err := findKiroCliAccount(email)
	if err != nil {
		return kiroCliFail(action, email, "find-account", err)
	}
	kiroCliLog(action, email, "refresh-token", "刷新访问令牌")
	token, err := refreshKiroCliToken(acc)
	if err != nil {
		return kiroCliFail(action, email, "refresh-token", err)
	}
	kiroCliLog(action, email, "real-chat-precheck", "执行 kiro.rs 风格真实 chat 预检")
	result, err := precheckKiroCliChat(acc, token)
	if err != nil {
		if result == nil {
			result = map[string]interface{}{}
		}
		result["success"] = false
		result["stage"] = "real-chat-precheck"
		result["error"] = err.Error()
		kiroCliLog(action, email, "real-chat-precheck", "失败: "+truncateKiroCliText(err.Error(), 800))
		return result
	}
	result["success"] = true
	result["stage"] = "done"
	kiroCliLog(action, email, "done", "预检通过")
	return result
}

// ImportKiroCliAccount 先真实预检，通过后替换官方 Kiro CLI 当前登录态。
func (a *App) ImportKiroCliAccount(email string) map[string]interface{} {
	const action = "import"
	kiroCliLog(action, email, "find-account", "开始导入")
	acc, err := findKiroCliAccount(email)
	if err != nil {
		return kiroCliFail(action, email, "find-account", err)
	}
	kiroCliLog(action, email, "refresh-token", "刷新访问令牌")
	token, err := refreshKiroCliToken(acc)
	if err != nil {
		return kiroCliFail(action, email, "refresh-token", err)
	}
	kiroCliLog(action, email, "real-chat-precheck", "执行导入前真实 chat 预检")
	precheck, err := precheckKiroCliChat(acc, token)
	if err != nil {
		if precheck == nil {
			precheck = map[string]interface{}{}
		}
		precheck["success"] = false
		precheck["stage"] = "real-chat-precheck"
		precheck["error"] = err.Error()
		kiroCliLog(action, email, "real-chat-precheck", "失败: "+truncateKiroCliText(err.Error(), 800))
		return precheck
	}
	kiroCliLog(action, email, "write-auth", "写入官方 Kiro CLI 登录态")
	backup, err := writeOfficialKiroCliAuth(acc, token)
	if err != nil {
		return kiroCliFail(action, email, "write-auth", err)
	}
	kiroCliLog(action, email, "write-auth", "写入完成，备份: "+backup)
	kiroCliLog(action, email, "validate-whoami", "验证官方 Kiro CLI whoami")
	if _, err := runKiroCli("whoami"); err != nil {
		_ = restoreOfficialKiroCliAuth(backup)
		res := kiroCliFail(action, email, "validate-whoami", err)
		res["backup"] = backup
		res["rollback"] = true
		return res
	}
	kiroCliLog(action, email, "validate-model-list", "验证官方 Kiro CLI 模型列表")
	if _, err := runKiroCli("chat", "--list-models", "--format", "json-pretty"); err != nil {
		_ = restoreOfficialKiroCliAuth(backup)
		res := kiroCliFail(action, email, "validate-model-list", err)
		res["backup"] = backup
		res["rollback"] = true
		return res
	}
	kiroCliLog(action, email, "validate-minimal-chat", "验证官方 Kiro CLI 最小 chat")
	chatOut, err := runKiroCli("chat", "--no-interactive", "只输出 OK")
	if err != nil {
		_ = restoreOfficialKiroCliAuth(backup)
		res := kiroCliFail(action, email, "validate-minimal-chat", err)
		res["backup"] = backup
		res["rollback"] = true
		return res
	}
	if !strings.Contains(chatOut, "OK") {
		_ = restoreOfficialKiroCliAuth(backup)
		err := fmt.Errorf("Kiro CLI 最小 chat 未返回 OK")
		res := kiroCliFail(action, email, "validate-minimal-chat", err)
		res["backup"] = backup
		res["rollback"] = true
		return res
	}
	kiroCliLog(action, email, "done", "导入成功")
	return map[string]interface{}{"success": true, "email": email, "backup": backup, "stage": "done"}
}

// DeleteKiroCliAccount 从输出 accounts.json 删除账号。
func (a *App) DeleteKiroCliAccount(email string) map[string]interface{} {
	const action = "delete"
	kiroCliLog(action, email, "delete-account", "删除账号")
	removed, err := data.DeleteAccount(storage.GetResultOutputDir(), email)
	if err != nil {
		return kiroCliFail(action, email, "delete-account", err)
	}
	kiroCliLog(action, email, "delete-account", fmt.Sprintf("删除完成 removed=%v", removed))
	return map[string]interface{}{"success": true, "removed": removed}
}

// DeleteSuspendedKiroCliAccounts 删除前端传入的封禁账号。
func (a *App) DeleteSuspendedKiroCliAccounts(emails []string) map[string]interface{} {
	const action = "delete-suspended"
	kiroCliLog(action, "-", "delete-suspended", fmt.Sprintf("开始删除 %d 个已封禁账号", len(emails)))
	removed := 0
	for _, email := range emails {
		kiroCliLog(action, email, "delete-account", "删除账号")
		ok, err := data.DeleteAccount(storage.GetResultOutputDir(), email)
		if err != nil {
			res := kiroCliFail(action, email, "delete-account", err)
			res["removed"] = removed
			return res
		}
		if ok {
			removed++
		}
	}
	kiroCliLog(action, "-", "done", fmt.Sprintf("删除完成 removed=%d", removed))
	return map[string]interface{}{"success": true, "removed": removed}
}

// KiroCliStartCommand 返回当前推荐的官方 CLI 启动命令。
func (a *App) KiroCliStartCommand() string {
	return fmt.Sprintf("& '%s' chat --model claude-sonnet-4.5 --effort high --trust-all-tools", kiroCliExePath)
}
