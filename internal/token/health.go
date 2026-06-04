package token

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/javaswahid/grokpi/internal/config"
	"github.com/javaswahid/grokpi/internal/store"
)

const (
	defaultHealthCheckInterval = 15 * time.Minute
	maxConcurrentHealthChecks  = 3
	alertThrottleDuration      = 12 * time.Hour
)

// HealthChecker periodically revalidates tokens without resetting local usage counters.
type HealthChecker struct {
	manager    *TokenManager
	cfg        *config.TokenConfig
	configFunc func() *config.TokenConfig
	baseURL    string
	sem        chan struct{}
	alerts     *telegramAlert
}

// NewHealthChecker creates a token health checker.
func NewHealthChecker(manager *TokenManager, cfg *config.TokenConfig, baseURL string) *HealthChecker {
	return &HealthChecker{
		manager: manager,
		cfg:     cfg,
		baseURL: baseURL,
		sem:     make(chan struct{}, maxConcurrentHealthChecks),
		alerts:  newTelegramAlertFromEnv(),
	}
}

// SetConfigProvider sets a dynamic token config provider.
func (h *HealthChecker) SetConfigProvider(fn func() *config.TokenConfig) {
	h.configFunc = fn
}

// Start begins the periodic health loop.
func (h *HealthChecker) Start(ctx context.Context) {
	safeGo("token_health_checker", func() {
		h.run(ctx)
	})
}

func (h *HealthChecker) run(ctx context.Context) {
	h.checkNow(ctx)
	timer := time.NewTimer(h.interval())
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			h.checkNow(ctx)
			timer.Reset(h.interval())
		}
	}
}

func (h *HealthChecker) interval() time.Duration {
	cfg := h.currentConfig()
	if cfg != nil && cfg.TokenHealthCheckIntervalMinutes > 0 {
		return time.Duration(cfg.TokenHealthCheckIntervalMinutes) * time.Minute
	}
	return defaultHealthCheckInterval
}

func (h *HealthChecker) currentConfig() *config.TokenConfig {
	if h.configFunc != nil {
		return h.configFunc()
	}
	return h.cfg
}

func (h *HealthChecker) checkNow(ctx context.Context) {
	tokens := h.manager.GetHealthCheckTokens()
	if len(tokens) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, snapshot := range tokens {
		select {
		case <-ctx.Done():
			return
		case h.sem <- struct{}{}:
			s := snapshot
			wg.Add(1)
			safeGo("token_health_check_one", func() {
				defer wg.Done()
				defer func() { <-h.sem }()
				h.checkOne(ctx, s)
			})
		}
	}
	wg.Wait()
	h.alertIfVideoUnavailable(ctx)
}

func (h *HealthChecker) checkOne(ctx context.Context, snapshot TokenSnapshot) {
	storeToken := h.manager.GetToken(snapshot.ID)
	if storeToken == nil {
		return
	}

	now := time.Now()
	if storeToken.ExpiresAt != nil && !storeToken.ExpiresAt.After(now) {
		h.manager.MarkExpired(storeToken.ID, "token expired; replace token before continuing")
		h.alerts.Send(ctx, "token_expired_"+strconv.Itoa(int(storeToken.ID)), "GrokPi token expired. Replace token before continuing video phase.")
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := h.manager.ValidateToken(checkCtx, storeToken, h.baseURL); err != nil {
		slog.Warn("token health check failed", "token_id", snapshot.ID, "error", err)
	}

	updated := h.manager.GetToken(snapshot.ID)
	if updated == nil {
		return
	}
	switch Status(updated.Status) {
	case StatusExpired:
		h.alerts.Send(ctx, "token_expired_"+strconv.Itoa(int(updated.ID)), "GrokPi token expired. Replace token before continuing video phase.")
	case StatusInvalid:
		h.alerts.Send(ctx, "token_invalid_"+strconv.Itoa(int(updated.ID)), "GrokPi token invalid. Replace or revalidate token.")
	case StatusRefreshFailed:
		h.alerts.Send(ctx, "token_refresh_failed_"+strconv.Itoa(int(updated.ID)), "GrokPi token refresh failed. Use Replace Token.")
	}
	h.alertExpiryWindow(ctx, updated)
}

func (h *HealthChecker) alertExpiryWindow(ctx context.Context, token *store.Token) {
	if token == nil || token.ExpiresAt == nil {
		return
	}
	cfg := h.currentConfig()
	warningHours := 24
	criticalHours := 6
	if cfg != nil {
		if cfg.TokenExpiryWarningHours > 0 {
			warningHours = cfg.TokenExpiryWarningHours
		}
		if cfg.TokenCriticalExpiryHours > 0 {
			criticalHours = cfg.TokenCriticalExpiryHours
		}
	}
	remaining := time.Until(*token.ExpiresAt)
	if remaining <= 0 {
		return
	}
	switch {
	case remaining <= time.Duration(criticalHours)*time.Hour:
		h.alerts.Send(ctx, "token_expiry_critical_"+strconv.Itoa(int(token.ID)), "GrokPi token will expire in less than "+strconv.Itoa(criticalHours)+" hours.")
	case remaining <= time.Duration(warningHours)*time.Hour:
		h.alerts.Send(ctx, "token_expiry_warning_"+strconv.Itoa(int(token.ID)), "GrokPi token will expire in less than "+strconv.Itoa(warningHours)+" hours.")
	}
}

func (h *HealthChecker) alertIfVideoUnavailable(ctx context.Context) {
	tokens := h.manager.GetHealthCheckTokens()
	available := 0
	for _, t := range tokens {
		status := Status(t.Status)
		if (status == StatusActive || status == StatusQuotaExhausted) && t.VideoQuota > 0 {
			available++
		}
	}
	if available == 0 {
		h.alerts.Send(ctx, "all_video_tokens_unavailable", "All GrokPi video tokens are unavailable or out of local video quota.")
	}
}

type telegramAlert struct {
	botToken string
	chatID   string
	client   *http.Client
	mu       sync.Mutex
	lastSent map[string]time.Time
}

func newTelegramAlertFromEnv() *telegramAlert {
	return &telegramAlert{
		botToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		chatID:   os.Getenv("TELEGRAM_CHAT_ID"),
		client:   &http.Client{Timeout: 10 * time.Second},
		lastSent: make(map[string]time.Time),
	}
}

func (a *telegramAlert) Send(ctx context.Context, key, text string) {
	if a == nil || a.botToken == "" || a.chatID == "" || key == "" || text == "" {
		return
	}
	now := time.Now()
	a.mu.Lock()
	if last, ok := a.lastSent[key]; ok && now.Sub(last) < alertThrottleDuration {
		a.mu.Unlock()
		return
	}
	a.lastSent[key] = now
	a.mu.Unlock()

	payload := map[string]string{"chat_id": a.chatID, "text": text}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+a.botToken+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		slog.Warn("telegram token alert failed", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.Warn("telegram token alert returned non-2xx", "status", resp.StatusCode)
	}
}
