package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"reg_go/internal/storage"
)

const (
	DefaultMailManagerURL = "http://43.162.94.131:8097"

	mailManagerProject  = "kiro"
	mailManagerConsumer = "kirox"
	mailManagerParser   = "kiro_otp"
)

// MailManagerConfig Mail Manage API 配置。
type MailManagerConfig struct {
	URL      string `json:"url"`
	APIKey   string `json:"apiKey"`
	Provider string `json:"provider"`
}

func loadLocalMailManagerConfig() MailManagerConfig {
	path := storage.GetDefaultDataDir() + string(os.PathSeparator) + "mail-manager.local.json"
	data, err := os.ReadFile(path)
	if err != nil {
		return MailManagerConfig{}
	}
	var cfg MailManagerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return MailManagerConfig{}
	}
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Provider = strings.TrimSpace(cfg.Provider)
	return cfg
}

// MailManagerProvider 通过 Mail Manage 租约邮箱并等待 Kiro OTP。
type MailManagerProvider struct {
	client   *http.Client
	ctx      context.Context
	baseURL  string
	apiKey   string
	provider string

	email     string
	mailboxID string
	leaseID   string

	mu            sync.Mutex
	sentAfterUnix int64
}

type mailManagerAPIResponse struct {
	OK                bool   `json:"ok"`
	Error             string `json:"error"`
	Message           string `json:"message"`
	MailboxID         string `json:"mailbox_id"`
	LeaseID           string `json:"lease_id"`
	Email             string `json:"email"`
	Provider          string `json:"provider"`
	Code              string `json:"code"`
	Status            string `json:"status"`
	RequestedEmail    string `json:"requested_email"`
	ResolvedEmail     string `json:"resolved_email"`
	MessageReceivedAt int64  `json:"message_received_at"`
}

// NewMailManagerProvider 创建 Mail Manage provider 并立即 lease 一个邮箱。
func NewMailManagerProvider(provider string, taskIndex int) (*MailManagerProvider, error) {
	return NewMailManagerProviderWithContext(context.Background(), provider, taskIndex)
}

// NewMailManagerProviderWithContext 创建 Mail Manage provider，并让长轮询请求跟随任务取消。
func NewMailManagerProviderWithContext(ctx context.Context, provider string, taskIndex int) (*MailManagerProvider, error) {
	cfg := MailManagerConfig{
		Provider: provider,
	}
	return NewMailManagerProviderWithConfig(ctx, cfg, taskIndex)
}

// NewMailManagerProviderWithConfig 创建 Mail Manage provider 并立即 lease 一个邮箱。
func NewMailManagerProviderWithConfig(ctx context.Context, config MailManagerConfig, taskIndex int) (*MailManagerProvider, error) {
	localCfg := loadLocalMailManagerConfig()
	if config.URL == "" {
		config.URL = os.Getenv("KIROX_MAIL_MANAGER_URL")
		if config.URL == "" {
			config.URL = localCfg.URL
			if config.URL == "" {
				config.URL = DefaultMailManagerURL
			}
		}
	}
	if config.APIKey == "" {
		config.APIKey = os.Getenv("KIROX_MAIL_MANAGER_API_KEY")
		if config.APIKey == "" {
			config.APIKey = localCfg.APIKey
		}
	}
	if config.Provider == "" {
		config.Provider = localCfg.Provider
		if config.Provider == "" {
			config.Provider = "hotmail"
		}
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("Mail Manager API key 未配置，请设置 KIROX_MAIL_MANAGER_API_KEY 或 %s", storage.GetDefaultDataDir()+string(os.PathSeparator)+"mail-manager.local.json")
	}
	if !IsValidMailManagerProvider(config.Provider) {
		return nil, fmt.Errorf("不支持的 Mail Manager 邮箱类型: %s", config.Provider)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	p := &MailManagerProvider{
		client:   &http.Client{},
		ctx:      ctx,
		baseURL:  strings.TrimRight(config.URL, "/"),
		apiKey:   config.APIKey,
		provider: config.Provider,
	}
	if err := p.lease(taskIndex); err != nil {
		return nil, err
	}
	return p, nil
}

// IsValidMailManagerProvider 校验 Mail Manage 支持的邮箱来源类型。
func IsValidMailManagerProvider(provider string) bool {
	switch provider {
	case "hotmail", "icloud", "cf_gmail", "manual":
		return true
	default:
		return false
	}
}

func (p *MailManagerProvider) lease(taskIndex int) error {
	now := time.Now()
	p.MarkSentAfter(now.Add(-5 * time.Second).Unix())

	payload := map[string]interface{}{
		"project":         mailManagerProject,
		"provider":        p.provider,
		"consumer":        mailManagerConsumer,
		"idempotency_key": fmt.Sprintf("kirox-%d-%d-%s", os.Getpid(), taskIndex, uuid.NewString()),
		"ttl_seconds":     900,
		"metadata": map[string]interface{}{
			"run_id":     fmt.Sprintf("kirox-%s", now.UTC().Format("20060102T150405Z")),
			"task_index": taskIndex,
		},
	}

	var resp mailManagerAPIResponse
	ctx, cancel := context.WithTimeout(p.requestContext(), 30*time.Second)
	defer cancel()
	if err := p.postJSON(ctx, "/v1/mailboxes/lease", payload, &resp); err != nil {
		return fmt.Errorf("Mail Manager lease 失败: %w", err)
	}
	if resp.Email == "" || resp.LeaseID == "" {
		return fmt.Errorf("Mail Manager lease 响应缺少邮箱或租约")
	}

	p.email = resp.Email
	p.mailboxID = resp.MailboxID
	p.leaseID = resp.LeaseID
	if resp.Provider != "" {
		p.provider = resp.Provider
	}
	return nil
}

func (p *MailManagerProvider) requestContext() context.Context {
	if p == nil || p.ctx == nil {
		return context.Background()
	}
	return p.ctx
}

func (p *MailManagerProvider) postJSON(ctx context.Context, path string, payload interface{}, out interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	if len(respBody) == 0 {
		return fmt.Errorf("空响应")
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("解析响应失败: %w, body=%s", err, string(respBody))
	}
	if apiResp, ok := out.(*mailManagerAPIResponse); ok && !apiResp.OK {
		msg := apiResp.Message
		if msg == "" {
			msg = apiResp.Error
		}
		if msg == "" {
			msg = "unknown error"
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// Create 已在构造时 lease 好邮箱，直接返回。
func (p *MailManagerProvider) Create() string {
	return p.GetAddress()
}

// GetAddress 获取当前邮箱地址。
func (p *MailManagerProvider) GetAddress() string {
	if p == nil {
		return ""
	}
	return p.email
}

// MarkSentAfter 记录触发目标站点发信的时间，用于过滤旧验证码。
func (p *MailManagerProvider) MarkSentAfter(unixSec int64) {
	if p == nil || unixSec <= 0 {
		return
	}
	p.mu.Lock()
	p.sentAfterUnix = unixSec
	p.mu.Unlock()
}

// WaitForCode 等待 Kiro OTP。
func (p *MailManagerProvider) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if p == nil || p.email == "" {
		return "", fmt.Errorf("Mail Manager 邮箱未初始化")
	}
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	if intervalSec <= 0 {
		intervalSec = 3
	}

	p.mu.Lock()
	sentAfter := p.sentAfterUnix
	p.mu.Unlock()
	if sentAfter <= 0 {
		sentAfter = time.Now().Add(-5 * time.Second).Unix()
	}

	payload := map[string]interface{}{
		"project":               mailManagerProject,
		"email":                 p.email,
		"parser":                mailManagerParser,
		"sent_after_unix":       sentAfter,
		"timeout_seconds":       timeoutSec,
		"poll_interval_seconds": intervalSec,
		"exclude_codes":         []string{},
	}

	var resp mailManagerAPIResponse
	ctx, cancel := context.WithTimeout(p.requestContext(), time.Duration(timeoutSec+15)*time.Second)
	defer cancel()
	if err := p.postJSON(ctx, "/v1/otp/wait", payload, &resp); err != nil {
		return "", fmt.Errorf("Mail Manager 等待验证码失败: %w", err)
	}
	if resp.Code == "" {
		return "", fmt.Errorf("Mail Manager 未返回验证码")
	}
	return resp.Code, nil
}

// Fail 标记本次租约失败。
func (p *MailManagerProvider) Fail(reason string) error {
	if p == nil || p.leaseID == "" {
		return nil
	}
	if reason == "" {
		reason = "kiro_registration_failed"
	}
	var resp mailManagerAPIResponse
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return p.postJSON(ctx, "/v1/leases/"+p.leaseID+"/fail", map[string]string{"reason": reason}, &resp)
}

// Release 释放尚未提交到目标站点的租约。
func (p *MailManagerProvider) Release(reason string) error {
	if p == nil || p.leaseID == "" {
		return nil
	}
	if reason == "" {
		reason = "kiro_registration_cancelled_before_email_submit"
	}
	var resp mailManagerAPIResponse
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return p.postJSON(ctx, "/v1/leases/"+p.leaseID+"/release", map[string]string{"reason": reason}, &resp)
}
