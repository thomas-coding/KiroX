package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultKiroGatewayAccountsDir = "/opt/kiro-gateway/accounts"
	defaultKiroGatewayStatePath   = "/opt/kiro-gateway/state.json"
	defaultKiroGatewayContainer   = "kiro-gateway-old"
	defaultKiroGatewayPort        = "18001"
)

type localServerConfig struct {
	Host        string `json:"host"`
	User        string `json:"user"`
	Password    string `json:"password"`
	BaseURL     string `json:"baseUrl"`
	GatewayPort string `json:"gatewayPort"`
	AccountsDir string `json:"accountsDir"`
	StatePath   string `json:"statePath"`
	Container   string `json:"container"`
}

type kiroGatewayAccountStatus struct {
	File               string `json:"file"`
	Email              string `json:"email"`
	Region             string `json:"region"`
	HasProfileArn      bool   `json:"hasProfileArn"`
	ProfileArnTail     string `json:"profileArnTail"`
	Failures           int    `json:"failures"`
	LastFailureTime    string `json:"lastFailureTime"`
	TotalRequests      int    `json:"totalRequests"`
	SuccessfulRequests int    `json:"successfulRequests"`
	FailedRequests     int    `json:"failedRequests"`
	Status             string `json:"status"`
}

type kiroGatewayStatus struct {
	Name          string                     `json:"name"`
	BaseURL       string                     `json:"baseUrl"`
	Host          string                     `json:"host"`
	Healthy       bool                       `json:"healthy"`
	HealthText    string                     `json:"healthText"`
	Container     string                     `json:"container"`
	ContainerText string                     `json:"containerText"`
	AccountCount  int                        `json:"accountCount"`
	Accounts      []kiroGatewayAccountStatus `json:"accounts"`
	RefreshedAt   string                     `json:"refreshedAt"`
	AccountsDir   string                     `json:"accountsDir"`
	StatePath     string                     `json:"statePath"`
}

func loadOldServerConfig() (localServerConfig, error) {
	path := filepath.Join(os.Getenv("USERPROFILE"), ".codex", "kiro_servers.local.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return localServerConfig{}, fmt.Errorf("读取服务器配置失败: %w", err)
	}
	var raw map[string]localServerConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return localServerConfig{}, fmt.Errorf("解析服务器配置失败: %w", err)
	}
	cfg := raw["old"]
	if cfg.Host == "" || cfg.User == "" || cfg.Password == "" {
		return localServerConfig{}, fmt.Errorf("服务器配置 old 缺少 host/user/password")
	}
	if cfg.GatewayPort == "" {
		cfg.GatewayPort = defaultKiroGatewayPort
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://" + cfg.Host + ":" + cfg.GatewayPort
	}
	if cfg.AccountsDir == "" {
		cfg.AccountsDir = defaultKiroGatewayAccountsDir
	}
	if cfg.StatePath == "" {
		cfg.StatePath = defaultKiroGatewayStatePath
	}
	if cfg.Container == "" {
		cfg.Container = defaultKiroGatewayContainer
	}
	return cfg, nil
}

func runOldServer(script string) (string, error) {
	cfg, err := loadOldServerConfig()
	if err != nil {
		return "", err
	}
	args := []string{"-ssh", cfg.User + "@" + cfg.Host, "-pw", cfg.Password, "-batch", script}
	cmd := exec.Command("plink", args...)
	hideCommandWindow(cmd)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("%w: %s", err, text)
	}
	return text, nil
}

func runPSCP(localPath, remotePath string) (string, error) {
	cfg, err := loadOldServerConfig()
	if err != nil {
		return "", err
	}
	args := []string{"-pw", cfg.Password, "-batch", localPath, cfg.User + "@" + cfg.Host + ":" + remotePath}
	cmd := exec.Command("pscp", args...)
	hideCommandWindow(cmd)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("%w: %s", err, text)
	}
	return text, nil
}

func remoteQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func parseGatewayState(raw string) map[string]map[string]interface{} {
	var state map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return map[string]map[string]interface{}{}
	}
	out := map[string]map[string]interface{}{}
	accounts, _ := state["accounts"].(map[string]interface{})
	for id, v := range accounts {
		if m, ok := v.(map[string]interface{}); ok {
			out[id] = m
		}
	}
	return out
}

func intFromMap(m map[string]interface{}, key string) int {
	v := m[key]
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func gatewayStatusFromFailures(failures int) string {
	if failures <= 0 {
		return "running"
	}
	if failures >= 5 {
		return "cooldown"
	}
	return "degraded"
}

func profileArnTail(arn string) string {
	if arn == "" {
		return ""
	}
	parts := strings.Split(arn, "/")
	return parts[len(parts)-1]
}

func safeGatewayFileName(file string) bool {
	if file == "" {
		return false
	}
	return regexp.MustCompile(`^[A-Za-z0-9._-]+\.json$`).MatchString(file)
}

func findGatewayStateByEmail(email string) (kiroAccountLifecycle, error) {
	states, err := loadKiroAccountStates()
	if err != nil {
		return kiroAccountLifecycle{}, err
	}
	state := states[email]
	if state.Email == "" {
		return kiroAccountLifecycle{}, fmt.Errorf("账号尚未生成 Gateway JSON")
	}
	if state.GatewayFile == "" {
		return kiroAccountLifecycle{}, fmt.Errorf("账号尚未生成 Gateway JSON")
	}
	return state, nil
}

func validateLocalGatewayFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("缺少本地 Gateway JSON 路径")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	base, err := filepath.Abs(kiroGatewayAccountDir())
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("Gateway JSON 不在 KiroX 输出目录内")
	}
	if !safeGatewayFileName(filepath.Base(abs)) {
		return "", fmt.Errorf("非法 Gateway JSON 文件名")
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("Gateway JSON 不存在: %w", err)
	}
	return abs, nil
}

// GetKiroGatewayStatus 读取 old 服务器上的 kiro-gateway 健康状态、账号文件和运行状态。
func (a *App) GetKiroGatewayStatus() map[string]interface{} {
	cfg, err := loadOldServerConfig()
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	script := strings.Join([]string{
		"set -e",
		"echo __HEALTH__",
		"curl -fsS --max-time 8 http://127.0.0.1:" + cfg.GatewayPort + "/health || true",
		"echo",
		"echo __CONTAINER__",
		"docker ps --filter name=" + remoteQuote(cfg.Container) + " --format '{{.Names}} {{.Status}}' || true",
		"echo __ACCOUNTS__",
		"python3 - <<'PY'",
		"import json, os, glob",
		"base=" + strconv.Quote(cfg.AccountsDir),
		"items=[]",
		"for p in sorted(glob.glob(base+'/*.json')):",
		"    try:",
		"        d=json.load(open(p, encoding='utf-8'))",
		"    except Exception as e:",
		"        d={'_error':str(e)}",
		"    arn=d.get('profileArn') or d.get('profile_arn') or ''",
		"    items.append({'file':os.path.basename(p),'email':d.get('email') or '','region':d.get('region') or '', 'hasProfileArn':bool(arn), 'profileArn':arn})",
		"print(json.dumps(items, ensure_ascii=False))",
		"PY",
		"echo __STATE__",
		"if [ -f " + remoteQuote(cfg.StatePath) + " ]; then",
		"  cat " + remoteQuote(cfg.StatePath),
		"else",
		"  docker exec " + remoteQuote(cfg.Container) + " sh -lc " + remoteQuote("cat /app/state.json 2>/dev/null || echo '{}'") + " 2>/dev/null || echo '{}'",
		"fi",
	}, "\n")
	out, err := runOldServer(script)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error(), "raw": out}
	}
	section := func(name string) string {
		start := "__" + name + "__"
		idx := strings.Index(out, start)
		if idx < 0 {
			return ""
		}
		rest := out[idx+len(start):]
		next := regexp.MustCompile(`(?m)^__[A-Z]+__$`).FindStringIndex(rest)
		if next != nil {
			rest = rest[:next[0]]
		}
		return strings.TrimSpace(rest)
	}
	healthText := section("HEALTH")
	containerText := section("CONTAINER")
	var accounts []kiroGatewayAccountStatus
	var rawAccounts []map[string]interface{}
	_ = json.Unmarshal([]byte(section("ACCOUNTS")), &rawAccounts)
	stateByID := parseGatewayState(section("STATE"))
	for _, raw := range rawAccounts {
		file, _ := raw["file"].(string)
		email, _ := raw["email"].(string)
		region, _ := raw["region"].(string)
		arn, _ := raw["profileArn"].(string)
		hasArn, _ := raw["hasProfileArn"].(bool)
		accountID := "/app/accounts/" + file
		st := stateByID[accountID]
		stats, _ := st["stats"].(map[string]interface{})
		failures := intFromMap(st, "failures")
		lastFailure := ""
		if ts, ok := st["last_failure_time"].(float64); ok && ts > 0 {
			lastFailure = time.Unix(int64(ts), 0).Format("2006-01-02 15:04:05")
		}
		accounts = append(accounts, kiroGatewayAccountStatus{
			File:               file,
			Email:              email,
			Region:             region,
			HasProfileArn:      hasArn,
			ProfileArnTail:     profileArnTail(arn),
			Failures:           failures,
			LastFailureTime:    lastFailure,
			TotalRequests:      intFromMap(stats, "total_requests"),
			SuccessfulRequests: intFromMap(stats, "successful_requests"),
			FailedRequests:     intFromMap(stats, "failed_requests"),
			Status:             gatewayStatusFromFailures(failures),
		})
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].File < accounts[j].File })
	status := kiroGatewayStatus{
		Name:          "old",
		BaseURL:       cfg.BaseURL,
		Host:          cfg.Host,
		Healthy:       strings.Contains(healthText, `"healthy"`) || strings.Contains(healthText, "healthy"),
		HealthText:    healthText,
		Container:     cfg.Container,
		ContainerText: containerText,
		AccountCount:  len(accounts),
		Accounts:      accounts,
		RefreshedAt:   nowLocalString(),
		AccountsDir:   cfg.AccountsDir,
		StatePath:     cfg.StatePath,
	}
	return map[string]interface{}{"success": true, "gateway": status}
}

// DeleteKiroGatewayAccount 删除 old 服务器上的指定账号 JSON。
func (a *App) DeleteKiroGatewayAccount(file string) map[string]interface{} {
	if !safeGatewayFileName(file) {
		return map[string]interface{}{"success": false, "error": "非法账号文件名"}
	}
	cfg, err := loadOldServerConfig()
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	path := cfg.AccountsDir + "/" + file
	out, err := runOldServer("rm -f -- " + remoteQuote(path) + "\n" + "echo deleted")
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error(), "raw": out}
	}
	return map[string]interface{}{"success": true, "file": file}
}

// UploadKiroGatewayAccount 上传某个本地 Gateway JSON 到 old 服务器并重启 gateway。
func (a *App) UploadKiroGatewayAccount(email string) map[string]interface{} {
	const action = "gateway-upload"
	kiroCliLog(action, email, "find-local-json", "查找本地 Gateway JSON")
	state, err := findGatewayStateByEmail(email)
	if err != nil {
		return kiroCliFail(action, email, "find-local-json", err)
	}
	localPath, err := validateLocalGatewayFile(state.GatewayFile)
	if err != nil {
		return kiroCliFail(action, email, "validate-local-json", err)
	}
	cfg, err := loadOldServerConfig()
	if err != nil {
		return kiroCliFail(action, email, "load-server-config", err)
	}
	fileName := filepath.Base(localPath)
	remotePath := cfg.AccountsDir + "/" + fileName
	kiroCliLog(action, email, "upload", "上传到 old 服务器: "+remotePath)
	if _, err := runOldServer("mkdir -p " + remoteQuote(cfg.AccountsDir)); err != nil {
		return kiroCliFail(action, email, "prepare-remote-dir", err)
	}
	if out, err := runPSCP(localPath, remotePath); err != nil {
		kiroCliLog(action, email, "upload", "pscp 输出: "+truncateKiroCliText(out, 500))
		return kiroCliFail(action, email, "upload", err)
	}
	kiroCliLog(action, email, "restart-gateway", "重启 gateway 容器")
	restart := a.RestartKiroGateway()
	if ok, _ := restart["success"].(bool); !ok {
		errText, _ := restart["error"].(string)
		if errText == "" {
			errText = "重启 gateway 失败"
		}
		return kiroCliFail(action, email, "restart-gateway", fmt.Errorf("%s", errText))
	}
	newState, _ := updateKiroAccountState(email, func(state *kiroAccountLifecycle) {
		state.Status = "gateway_uploaded"
		state.GatewayFile = localPath
		state.LastGatewayUploadAt = nowLocalString()
		state.LastError = ""
	})
	kiroCliLog(action, email, "done", "上传完成")
	return map[string]interface{}{
		"success":    true,
		"email":      email,
		"remotePath": remotePath,
		"state":      newState,
	}
}

// RestartKiroGateway 重启 old 服务器上的 gateway 容器。
func (a *App) RestartKiroGateway() map[string]interface{} {
	cfg, err := loadOldServerConfig()
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	out, err := runOldServer("docker restart " + remoteQuote(cfg.Container) + "\n" + "sleep 2\n" + "docker ps --filter name=" + remoteQuote(cfg.Container) + " --format '{{.Names}} {{.Status}}'")
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error(), "raw": out}
	}
	return map[string]interface{}{"success": true, "status": out}
}

// KiroGatewayChatSmokeTest 对 gateway 做一次最小 chat 测试。
func (a *App) KiroGatewayChatSmokeTest(model string) map[string]interface{} {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "claude-sonnet-4.5"
	}
	cfg, err := loadOldServerConfig()
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	body := fmt.Sprintf(`{"model":%s,"messages":[{"role":"user","content":"Say OK only."}],"max_tokens":20,"stream":false}`, strconv.Quote(model))
	script := "curl -fsS --max-time 60 -H 'Authorization: Bearer '\"$PROXY_API_KEY\" -H 'Content-Type: application/json' -d " + remoteQuote(body) + " http://127.0.0.1:" + cfg.GatewayPort + "/v1/chat/completions"
	out, err := runOldServer("PROXY_API_KEY=$(grep '^PROXY_API_KEY=' /opt/kiro-gateway/.env | cut -d= -f2- | tr -d '\"')\n" + script)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error(), "raw": out}
	}
	return map[string]interface{}{"success": true, "raw": out}
}
