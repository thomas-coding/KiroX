package email

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"reg_go/internal/storage"
)

const (
	defaultSmsbMailAPI = "https://smsbower.page/api/mail"

	defaultSmsbGmailService  = "aws"
	defaultSmsbGmailDomain   = "gmail.com"
	defaultSmsbGmailMaxPrice = "0.05"
)

var smsbMailAPIBase = defaultSmsbMailAPI

type SmsbGmailConfig struct {
	APIKey   string `json:"apiKey"`
	Service  string `json:"service"`
	Domain   string `json:"domain"`
	MaxPrice string `json:"maxPrice"`
	Ref      string `json:"ref"`
	Alias    bool   `json:"alias"`
}

type SmsbGmailProvider struct {
	client *http.Client
	ctx    context.Context
	cfg    SmsbGmailConfig

	mu      sync.Mutex
	email   string
	mailID  string
	closed  bool
	status  string
	lastErr string
}

type smsbMailResponse struct {
	Status int             `json:"status"`
	Error  string          `json:"error"`
	Mail   string          `json:"mail"`
	MailID json.RawMessage `json:"mailId"`
	ID     json.RawMessage `json:"id"`
	Code   string          `json:"code"`
}

func smsbLocalConfigPath() string {
	return storage.GetDefaultDataDir() + string(os.PathSeparator) + "smsb.local.json"
}

func loadLocalSmsbGmailConfig() SmsbGmailConfig {
	data, err := os.ReadFile(smsbLocalConfigPath())
	if err != nil {
		return SmsbGmailConfig{}
	}
	var cfg SmsbGmailConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return SmsbGmailConfig{}
	}
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Service = strings.TrimSpace(cfg.Service)
	cfg.Domain = strings.TrimSpace(cfg.Domain)
	cfg.MaxPrice = strings.TrimSpace(cfg.MaxPrice)
	cfg.Ref = strings.TrimSpace(cfg.Ref)
	return cfg
}

func resolveSmsbGmailConfig(cfg SmsbGmailConfig) (SmsbGmailConfig, error) {
	local := loadLocalSmsbGmailConfig()
	if cfg.APIKey == "" {
		cfg.APIKey = strings.TrimSpace(os.Getenv("KIROX_SMSB_API_KEY"))
		if cfg.APIKey == "" {
			cfg.APIKey = local.APIKey
		}
	}
	if cfg.Service == "" {
		cfg.Service = strings.TrimSpace(os.Getenv("KIROX_SMSB_SERVICE"))
		if cfg.Service == "" {
			cfg.Service = local.Service
		}
		if cfg.Service == "" {
			cfg.Service = defaultSmsbGmailService
		}
	}
	if cfg.Domain == "" {
		cfg.Domain = strings.TrimSpace(os.Getenv("KIROX_SMSB_DOMAIN"))
		if cfg.Domain == "" {
			cfg.Domain = local.Domain
		}
		if cfg.Domain == "" {
			cfg.Domain = defaultSmsbGmailDomain
		}
	}
	if cfg.MaxPrice == "" {
		cfg.MaxPrice = strings.TrimSpace(os.Getenv("KIROX_SMSB_MAX_PRICE"))
		if cfg.MaxPrice == "" {
			cfg.MaxPrice = local.MaxPrice
		}
		if cfg.MaxPrice == "" {
			cfg.MaxPrice = defaultSmsbGmailMaxPrice
		}
	}
	if cfg.Ref == "" {
		cfg.Ref = local.Ref
	}
	if !cfg.Alias {
		cfg.Alias = local.Alias
	}
	if cfg.APIKey == "" {
		return cfg, fmt.Errorf("SMSB API key 未配置，请设置 KIROX_SMSB_API_KEY 或 %s", smsbLocalConfigPath())
	}
	return cfg, nil
}

func NewSmsbGmailProvider(ctx context.Context, cfg SmsbGmailConfig) (*SmsbGmailProvider, error) {
	resolved, err := resolveSmsbGmailConfig(cfg)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p := &SmsbGmailProvider{
		client: &http.Client{},
		ctx:    ctx,
		cfg:    resolved,
	}
	if err := p.lease(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *SmsbGmailProvider) requestContext() context.Context {
	if p == nil || p.ctx == nil {
		return context.Background()
	}
	return p.ctx
}

func (p *SmsbGmailProvider) mailRequest(ctx context.Context, action string, params map[string]string) (smsbMailResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	values := url.Values{}
	values.Set("api_key", p.cfg.APIKey)
	for k, v := range params {
		if strings.TrimSpace(v) != "" {
			values.Set(k, v)
		}
	}
	reqURL := smsbMailAPIBase + "/" + action + "?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return smsbMailResponse{}, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return smsbMailResponse{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return smsbMailResponse{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var out smsbMailResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return smsbMailResponse{}, fmt.Errorf("解析 SMSB 响应失败: %w, body=%s", err, string(body))
	}
	return out, nil
}

func (p *SmsbGmailProvider) lease() error {
	params := map[string]string{
		"service": p.cfg.Service,
		"domain":  p.cfg.Domain,
	}
	if p.cfg.MaxPrice != "" {
		params["maxPrice"] = p.cfg.MaxPrice
	}
	if p.cfg.Ref != "" {
		params["ref"] = p.cfg.Ref
	}
	if p.cfg.Alias {
		params["alias"] = "1"
	}
	ctx, cancel := context.WithTimeout(p.requestContext(), 30*time.Second)
	defer cancel()
	resp, err := p.mailRequest(ctx, "getActivation", params)
	if err != nil {
		return fmt.Errorf("SMSB Gmail 获取邮箱失败: %w", err)
	}
	if resp.Status != 1 {
		return fmt.Errorf("SMSB Gmail 获取邮箱失败: %s", smsbErrorText(resp))
	}
	email := strings.TrimSpace(resp.Mail)
	mailID := rawJSONValueString(resp.MailID)
	if mailID == "" {
		mailID = rawJSONValueString(resp.ID)
	}
	if email == "" || mailID == "" {
		return fmt.Errorf("SMSB Gmail 响应缺少邮箱或 mailId")
	}
	p.mu.Lock()
	p.email = email
	p.mailID = mailID
	p.closed = false
	p.status = "active"
	p.lastErr = ""
	p.mu.Unlock()
	log.Printf("[SMSB Gmail] 获取邮箱成功: %s id=%s service=%s", email, shortSMSBID(mailID), p.cfg.Service)
	return nil
}

func smsbErrorText(resp smsbMailResponse) string {
	if strings.TrimSpace(resp.Error) != "" {
		return strings.TrimSpace(resp.Error)
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func rawJSONValueString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return strings.TrimSpace(n.String())
	}
	return strings.Trim(strings.TrimSpace(string(raw)), `"`)
}

func shortSMSBID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:4] + "..." + id[len(id)-4:]
}

func (p *SmsbGmailProvider) Create() string {
	return p.GetAddress()
}

func (p *SmsbGmailProvider) GetAddress() string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.email
}

func (p *SmsbGmailProvider) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if p == nil {
		return "", fmt.Errorf("SMSB Gmail provider 未初始化")
	}
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	if intervalSec <= 0 {
		intervalSec = 3
	}
	p.mu.Lock()
	mailID := p.mailID
	email := p.email
	p.mu.Unlock()
	if mailID == "" {
		return "", fmt.Errorf("SMSB Gmail 缺少 mailId")
	}
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	log.Printf("[SMSB Gmail] 开始等待验证码: %s id=%s", email, shortSMSBID(mailID))
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(p.requestContext(), time.Duration(intervalSec+5)*time.Second)
		resp, err := p.mailRequest(ctx, "getCode", map[string]string{"mailId": mailID})
		cancel()
		if err != nil {
			p.Cancel("get_code_failed")
			return "", fmt.Errorf("SMSB Gmail 等待验证码失败: %w", err)
		}
		if resp.Status == 1 && strings.TrimSpace(resp.Code) != "" {
			code := strings.TrimSpace(resp.Code)
			p.mu.Lock()
			p.status = "code_received"
			p.mu.Unlock()
			_ = p.Complete()
			log.Printf("[SMSB Gmail] 获取到验证码: %s", code)
			return code, nil
		}
		errText := strings.ToLower(smsbErrorText(resp))
		if resp.Status != 1 && !strings.Contains(errText, "not been received") && !strings.Contains(errText, "try again later") {
			p.Cancel("get_code_failed")
			return "", fmt.Errorf("SMSB Gmail getCode 失败: %s", smsbErrorText(resp))
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	p.Cancel("code_timeout")
	return "", fmt.Errorf("SMSB Gmail 等待验证码超时 (%ds)", timeoutSec)
}

func (p *SmsbGmailProvider) setStatus(status int, reason string) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	mailID := p.mailID
	closed := p.closed
	p.mu.Unlock()
	if mailID == "" || closed {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := p.mailRequest(ctx, "setStatus", map[string]string{
		"id":     mailID,
		"status": fmt.Sprintf("%d", status),
	})
	if err != nil {
		p.rememberStatusError(reason, err)
		return err
	}
	if resp.Status != 1 {
		err := fmt.Errorf("%s", smsbErrorText(resp))
		p.rememberStatusError(reason, err)
		return err
	}
	p.mu.Lock()
	p.closed = true
	if status == 3 {
		p.status = "completed"
	} else if status == 2 {
		p.status = "cancelled"
	} else {
		p.status = fmt.Sprintf("status_%d", status)
	}
	p.mu.Unlock()
	return nil
}

func (p *SmsbGmailProvider) rememberStatusError(reason string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastErr = strings.TrimSpace(reason + ": " + err.Error())
}

func (p *SmsbGmailProvider) Complete() error {
	if p.isClosed() {
		return nil
	}
	if err := p.setStatus(3, "complete"); err != nil {
		log.Printf("[SMSB Gmail] 完成激活失败: %v", err)
		return err
	}
	log.Printf("[SMSB Gmail] 完成激活: %s", p.summary())
	return nil
}

func (p *SmsbGmailProvider) Cancel(reason string) error {
	if reason == "" {
		reason = "cancelled"
	}
	if p.isClosed() {
		return nil
	}
	if err := p.setStatus(2, reason); err != nil {
		log.Printf("[SMSB Gmail] 取消激活失败: %v", err)
		return err
	}
	log.Printf("[SMSB Gmail] 取消激活: %s reason=%s", p.summary(), reason)
	return nil
}

func (p *SmsbGmailProvider) isClosed() bool {
	if p == nil {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *SmsbGmailProvider) summary() string {
	if p == nil {
		return "-"
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return fmt.Sprintf("%s id=%s", p.email, shortSMSBID(p.mailID))
}
