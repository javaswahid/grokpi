package token

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/javaswahid/grokpi/internal/store"
)

var (
	// ErrNoQuota is returned when token has no remaining quota.
	ErrNoQuota = errors.New("no quota remaining")
	// ErrTokenNotFound is returned when token ID does not exist.
	ErrTokenNotFound = errors.New("token not found")
)

// RateLimitsRequest is the request body for rate-limits API.
type RateLimitsRequest struct {
	RequestKind string `json:"requestKind"`
	ModelName   string `json:"modelName"`
}

// RateLimitsResponse is the response from rate-limits API.
type RateLimitsResponse struct {
	RemainingQueries  int `json:"remainingQueries"`
	WindowSizeSeconds int `json:"windowSizeSeconds"`
}

const rateLimitsPath = "/rest/rate-limits"
const minCoolingDuration = 5 * time.Minute

type rateLimitStatusError struct {
	StatusCode int
}

func (e rateLimitStatusError) Error() string {
	return fmt.Sprintf("rate-limits API returned %d", e.StatusCode)
}

// Consume deducts quota from the token for the given category.
// cost allows variable deduction for different model types.
// Returns remaining quota after deduction.
func (m *TokenManager) Consume(tokenID uint, cat QuotaCategory, cost int) (remaining int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[tokenID]
	if !ok {
		return 0, ErrTokenNotFound
	}

	cur := GetQuota(token, cat)
	if IsUnlimitedQuota(token, m.cfg) {
		now := time.Now()
		token.LastUsed = &now
		if Status(token.Status) == StatusQuotaExhausted {
			token.Status = string(StatusActive)
			token.StatusReason = ""
		}
		m.dirty[tokenID] = struct{}{}
		return cur, nil
	}
	if cur <= 0 {
		token.Status = string(StatusQuotaExhausted)
		token.StatusReason = string(cat) + " quota exhausted"
		token.CoolUntil = nil
		m.dirty[tokenID] = struct{}{}
		return 0, ErrNoQuota
	}

	if cost <= 0 {
		cost = 1
	}
	newVal := cur - cost
	if newVal < 0 {
		newVal = 0
	}
	SetQuota(token, cat, newVal)

	now := time.Now()
	token.LastUsed = &now

	if newVal <= 0 {
		token.Status = string(StatusQuotaExhausted)
		token.StatusReason = string(cat) + " quota exhausted"
		token.CoolUntil = nil
	} else if Status(token.Status) == StatusQuotaExhausted {
		token.Status = string(StatusActive)
		token.StatusReason = ""
	}
	m.dirty[tokenID] = struct{}{}

	return newVal, nil
}

// SyncQuota fetches quota from upstream API and updates token state.
// The upstream rate-limits API returns a single remainingQueries value
// which maps to ChatQuota (the primary category).
// If quota recovered and token is cooling, restores to active.
func (m *TokenManager) SyncQuota(ctx context.Context, token *store.Token, baseURL string) error {
	if Status(token.Status) == StatusPolicyQuarantine {
		return ErrPolicyQuarantine
	}
	startedAt := time.Now()

	resp, err := m.fetchRateLimits(ctx, token.Token, baseURL)
	if err != nil {
		var statusErr rateLimitStatusError
		if errors.As(err, &statusErr) {
			m.mu.Lock()
			defer m.mu.Unlock()
			token.LastRefreshAttemptAt = &startedAt
			token.RefreshCount++
			switch statusErr.StatusCode {
			case http.StatusUnauthorized, http.StatusForbidden:
				token.Status = string(StatusExpired)
				token.StatusReason = "upstream authentication expired; replace token before continuing"
				token.ExpiresAt = &startedAt
				token.RefreshStatus = RefreshStatusRequired
				token.RefreshError = token.StatusReason
			case http.StatusBadRequest:
				token.Status = string(StatusInvalid)
				token.StatusReason = "upstream rejected token as invalid"
				token.RefreshStatus = RefreshStatusFailed
				token.RefreshError = token.StatusReason
			default:
				token.RefreshStatus = RefreshStatusFailed
				token.RefreshError = fmt.Sprintf("rate-limits API returned %d", statusErr.StatusCode)
			}
			m.dirty[token.ID] = struct{}{}
		}
		return fmt.Errorf("fetch rate limits: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	quotaMode := NormalizeQuotaMode(token.QuotaMode)
	token.LastRefreshAttemptAt = &startedAt
	token.LastValidatedAt = &startedAt
	token.RefreshStatus = RefreshStatusValidated
	token.RefreshError = ""
	if token.TokenVersion <= 0 {
		token.TokenVersion = 1
	}
	token.ChatQuota = ClampQuota(CategoryChat, resp.RemainingQueries, m.cfg, quotaMode)
	token.InitialChatQuota = token.ChatQuota

	// Restore image/video quotas to configured defaults on sync
	if m.cfg.DefaultImageQuota > 0 {
		token.ImageQuota = ClampQuota(CategoryImage, m.cfg.DefaultImageQuota, m.cfg, quotaMode)
		token.InitialImageQuota = token.ImageQuota
	}
	if m.cfg.DefaultVideoQuota > 0 {
		token.VideoQuota = ClampQuota(CategoryVideo, m.cfg.DefaultVideoQuota, m.cfg, quotaMode)
		token.InitialVideoQuota = token.VideoQuota
	}

	switch {
	case resp.RemainingQueries > 0 && (Status(token.Status) == StatusCooling || Status(token.Status) == StatusQuotaExhausted || Status(token.Status) == StatusExpired || Status(token.Status) == StatusInvalid || Status(token.Status) == StatusRefreshRequired || Status(token.Status) == StatusRefreshFailed):
		// Restore cooling token to active if quota recovered
		token.Status = string(StatusActive)
		token.StatusReason = ""
		token.CoolUntil = nil
		token.FailCount = 0
		token.ExpiresAt = nil
	case resp.RemainingQueries <= 0 && Status(token.Status) == StatusActive:
		// Prevent zombie: active token with no quota must enter cooling
		token.Status = string(StatusQuotaExhausted)
		token.StatusReason = "chat quota exhausted"
		token.CoolUntil = nil
	}

	m.dirty[token.ID] = struct{}{}
	return nil
}

// ValidateToken verifies an upstream token without changing local usage quota.
// Use this for expiry-aware lifecycle checks and admin revalidation.
func (m *TokenManager) ValidateToken(ctx context.Context, token *store.Token, baseURL string) error {
	if Status(token.Status) == StatusPolicyQuarantine {
		return ErrPolicyQuarantine
	}
	startedAt := time.Now()

	_, err := m.fetchRateLimits(ctx, token.Token, baseURL)
	if err != nil {
		var statusErr rateLimitStatusError
		if errors.As(err, &statusErr) {
			m.mu.Lock()
			defer m.mu.Unlock()
			token.LastRefreshAttemptAt = &startedAt
			token.RefreshCount++
			switch statusErr.StatusCode {
			case http.StatusUnauthorized, http.StatusForbidden:
				token.Status = string(StatusExpired)
				token.StatusReason = "upstream authentication expired; replace token before continuing"
				token.ExpiresAt = &startedAt
				token.RefreshStatus = RefreshStatusRequired
				token.RefreshError = token.StatusReason
			case http.StatusBadRequest:
				token.Status = string(StatusInvalid)
				token.StatusReason = "upstream rejected token as invalid"
				token.RefreshStatus = RefreshStatusFailed
				token.RefreshError = token.StatusReason
			default:
				token.RefreshStatus = RefreshStatusFailed
				token.RefreshError = fmt.Sprintf("rate-limits API returned %d", statusErr.StatusCode)
				if token.Status == string(StatusRefreshRequired) || token.Status == string(StatusRefreshFailed) {
					token.Status = string(StatusRefreshFailed)
					token.StatusReason = token.RefreshError
				}
			}
			m.dirty[token.ID] = struct{}{}
		}
		return fmt.Errorf("validate token: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	token.LastRefreshAttemptAt = &startedAt
	token.LastValidatedAt = &startedAt
	token.RefreshStatus = RefreshStatusValidated
	token.RefreshError = ""
	if token.TokenVersion <= 0 {
		token.TokenVersion = 1
	}

	switch Status(token.Status) {
	case StatusExpired, StatusInvalid, StatusRefreshRequired, StatusRefreshFailed:
		if token.ChatQuota > 0 || token.ImageQuota > 0 || token.VideoQuota > 0 || IsUnlimitedQuota(token, m.cfg) {
			token.Status = string(StatusActive)
			token.StatusReason = ""
			token.CoolUntil = nil
			token.FailCount = 0
			token.ExpiresAt = nil
		} else {
			token.Status = string(StatusQuotaExhausted)
			token.StatusReason = "local quota exhausted"
			token.CoolUntil = nil
		}
	}

	m.dirty[token.ID] = struct{}{}
	return nil
}

// fetchRateLimits calls the rate-limits API.
func (m *TokenManager) fetchRateLimits(ctx context.Context, authToken, baseURL string) (*RateLimitsResponse, error) {
	reqBody := RateLimitsRequest{
		RequestKind: "DEFAULT",
		ModelName:   "grok-3",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := baseURL + rateLimitsPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	// Get proxy/cookie settings if provider is set
	var proxyURL string
	var userAgent string
	var browserProfile string
	var cfCookies string
	var cfClearance string
	var skipProxySSLVerify bool

	m.mu.RLock()
	proxyFunc := m.proxyFunc
	m.mu.RUnlock()

	if proxyFunc != nil {
		if pcfg := proxyFunc(); pcfg != nil {
			proxyURL = pcfg.BaseProxyURL
			userAgent = pcfg.UserAgent
			browserProfile = pcfg.Browser
			cfCookies = pcfg.CFCookies
			cfClearance = pcfg.CFClearance
			skipProxySSLVerify = pcfg.SkipProxySSLVerify
		}
	}

	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	}

	// Format cookies: sso, sso-rw, and Cloudflare cookies if any
	cookieVal := "sso=" + authToken + "; sso-rw=" + authToken
	if cfClearance != "" {
		if cfCookies == "" {
			cfCookies = "cf_clearance=" + cfClearance
		} else if strings.Contains(cfCookies, "cf_clearance=") {
			// replace cf_clearance in cfCookies
			re := regexp.MustCompile(`(^|;\s*)cf_clearance=[^;]*`)
			cfCookies = re.ReplaceAllString(cfCookies, "${1}cf_clearance="+cfClearance)
		} else {
			cfCookies = strings.TrimRight(cfCookies, "; ") + "; cf_clearance=" + cfClearance
		}
	}
	if cfCookies != "" {
		cookieVal += "; " + cfCookies
	}

	req.Header.Set("Cookie", cookieVal)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", "https://grok.com")
	req.Header.Set("Referer", "https://grok.com/")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	// Set client hints if applicable
	majorVer := ""
	if browserProfile != "" {
		re := regexp.MustCompile(`\d+`)
		majorVer = re.FindString(browserProfile)
	}
	if majorVer == "" {
		majorVer = "146"
	}
	req.Header.Set("Sec-Ch-Ua", fmt.Sprintf(`"Google Chrome";v="%s", "Chromium";v="%s", "Not(A:Brand";v="24"`, majorVer, majorVer))
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)

	// Create tls-client with options
	tlsOpts := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(15),
	}

	// Resolve profile (use profiles subpackage for tls-client v1.14+)
	var profile profiles.ClientProfile
	switch strings.ToLower(browserProfile) {
	case "firefox_102":
		profile = profiles.Firefox_102
	case "firefox_117":
		profile = profiles.Firefox_117
	case "chrome_103":
		profile = profiles.Chrome_103
	case "chrome_111":
		profile = profiles.Chrome_111
	case "chrome_112":
		profile = profiles.Chrome_112
	case "chrome_116":
		profile = profiles.Chrome_116
	case "chrome_117":
		profile = profiles.Chrome_117
	case "chrome_118":
		profile = profiles.Chrome_118
	case "chrome_119":
		profile = profiles.Chrome_119
	case "chrome_120":
		profile = profiles.Chrome_120
	default:
		profile = profiles.Chrome_120
	}
	tlsOpts = append(tlsOpts, tls_client.WithClientProfile(profile))

	if skipProxySSLVerify {
		tlsOpts = append(tlsOpts, tls_client.WithInsecureSkipVerify())
	}
	if proxyURL != "" {
		tlsOpts = append(tlsOpts, tls_client.WithProxyUrl(proxyURL))
	}

	client, err := tls_client.NewHttpClient(nil, tlsOpts...)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, rateLimitStatusError{StatusCode: resp.StatusCode}
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var result RateLimitsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (m *TokenManager) coolingDurationForToken(token *store.Token) time.Duration {
	if token == nil || m.cfg == nil {
		return minCoolingDuration
	}
	var duration time.Duration
	switch token.Pool {
	case PoolSuper:
		duration = time.Duration(m.cfg.SuperCoolDurationMin) * time.Minute
	default:
		duration = time.Duration(m.cfg.BasicCoolDurationMin) * time.Minute
	}
	if duration < minCoolingDuration {
		return minCoolingDuration
	}
	return duration
}
