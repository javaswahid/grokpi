// Package config provides configuration loading and management.
package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the root configuration structure.
type Config struct {
	App         AppConfig         `toml:"app"`
	Image       ImageConfig       `toml:"image"`
	ImagineFast ImagineFastConfig `toml:"imagine_fast"`
	Proxy       ProxyConfig       `toml:"proxy"`
	Retry       RetryConfig       `toml:"retry"`
	Token       TokenConfig       `toml:"token"`
}

// AppConfig contains application settings.
type AppConfig struct {
	AppKey                 string   `toml:"app_key"`
	MediaGenerationEnabled bool     `toml:"media_generation_enabled"`
	Temporary              bool     `toml:"temporary"`
	DisableMemory          bool     `toml:"disable_memory"`
	Stream                 bool     `toml:"stream"`
	Thinking               bool     `toml:"thinking"`
	DynamicStatsig         bool     `toml:"dynamic_statsig"`
	CustomInstruction      string   `toml:"custom_instruction"`
	FilterTags             []string `toml:"filter_tags"`
	// Server settings
	Host          string `toml:"host"`
	Port          int    `toml:"port"`
	LogJSON       bool   `toml:"log_json"`
	LogLevel      string `toml:"log_level"`
	LogFilePath   string `toml:"log_file_path"`
	LogMaxSizeMB  int    `toml:"log_max_size_mb"`
	LogMaxBackups int    `toml:"log_max_backups"`
	// Database settings
	DBDriver       string `toml:"db_driver"`
	DBPath         string `toml:"db_path"`
	DBDSN          string `toml:"db_dsn"`
	RequestTimeout int    `toml:"request_timeout"` // default request timeout in seconds (non-LLM routes)
	// Security settings
	ReadHeaderTimeout int   `toml:"read_header_timeout"` // seconds, max time to read request headers
	MaxHeaderBytes    int   `toml:"max_header_bytes"`    // max size of request headers in bytes
	BodyLimit         int64 `toml:"body_limit"`          // default max request body size in bytes
	ChatBodyLimit     int64 `toml:"chat_body_limit"`     // max body size for chat completions in bytes
	AdminMaxFails     int   `toml:"admin_max_fails"`     // max auth failures before temporary IP lockout
	AdminWindowSec    int   `toml:"admin_window_sec"`    // time window in seconds for counting admin auth failures
}

// ImageConfig contains image-generation behavior flags.
type ImageConfig struct {
	NSFW                    bool  `toml:"nsfw"`
	BlockedParallelAttempts int   `toml:"blocked_parallel_attempts"`
	BlockedParallelEnabled  *bool `toml:"blocked_parallel_enabled"`
}

// ImagineFastConfig contains server-side chat defaults for grok-imagine-1.0-fast.
type ImagineFastConfig struct {
	N    int    `toml:"n"`
	Size string `toml:"size"`
}

// ProxyConfig contains proxy settings.
type ProxyConfig struct {
	BaseProxyURL       string `toml:"base_proxy_url"`
	AssetProxyURL      string `toml:"asset_proxy_url"`
	CFCookies          string `toml:"cf_cookies"`
	SkipProxySSLVerify bool   `toml:"skip_proxy_ssl_verify"`
	Enabled            bool   `toml:"enabled"`
	FlareSolverrURL    string `toml:"flaresolverr_url"`
	RefreshInterval    int    `toml:"refresh_interval"`
	Timeout            int    `toml:"timeout"`
	CFClearance        string `toml:"cf_clearance"`
	Browser            string `toml:"browser"`
	UserAgent          string `toml:"user_agent"`
}

// RetryConfig contains retry policy settings.
type RetryConfig struct {
	MaxTokens               int     `toml:"max_tokens"`
	PerTokenRetries         int     `toml:"per_token_retries"`
	ResetSessionStatusCodes []int   `toml:"reset_session_status_codes"`
	CoolingStatusCodes      []int   `toml:"cooling_status_codes"`
	RetryBackoffBase        float64 `toml:"retry_backoff_base"`
	RetryBackoffFactor      float64 `toml:"retry_backoff_factor"`
	RetryBackoffMax         float64 `toml:"retry_backoff_max"`
	RetryBudget             float64 `toml:"retry_budget"`
}

// TokenConfig contains token pool settings.
type TokenConfig struct {
	FailThreshold         int `toml:"fail_threshold"`
	UsageFlushIntervalSec int `toml:"usage_flush_interval_sec"`
	CoolCheckIntervalSec  int `toml:"cool_check_interval_sec"`
	// Model group fields
	BasicModels                     []string `toml:"basic_models"`
	SuperModels                     []string `toml:"super_models"`
	PreferredPool                   string   `toml:"preferred_pool"`
	BasicCoolDurationMin            int      `toml:"basic_cool_duration_min"`
	SuperCoolDurationMin            int      `toml:"super_cool_duration_min"`
	DefaultChatQuota                int      `toml:"default_chat_quota"`
	DefaultImageQuota               int      `toml:"default_image_quota"`
	DefaultVideoQuota               int      `toml:"default_video_quota"`
	MaxChatQuota                    int      `toml:"max_chat_quota"`
	MaxImageQuota                   int      `toml:"max_image_quota"`
	MaxVideoQuota                   int      `toml:"max_video_quota"`
	AllowUnlimitedQuota             bool     `toml:"allow_unlimited_quota"`
	TokenHealthCheckIntervalMinutes int      `toml:"token_health_check_interval_minutes"`
	TokenExpiryWarningHours         int      `toml:"token_expiry_warning_hours"`
	TokenCriticalExpiryHours        int      `toml:"token_critical_expiry_hours"`
	AllowTokenAutoRefresh           bool     `toml:"allow_token_auto_refresh"`
	RequireOfficialRefreshFlow      bool     `toml:"require_official_refresh_flow"`
	QuotaRecoveryMode               string   `toml:"quota_recovery_mode"`
	SelectionAlgorithm              string   `toml:"selection_algorithm" json:"selection_algorithm"`
}

// ApplyDBOverrides applies database config entries on top of file-based config.
// Priority: DB > config file > defaults.
func (c *Config) ApplyDBOverrides(kvs map[string]string) {
	// Pre-check: browser and UA must be paired from the same source.
	// If browser has no DB override, don't apply DB UA override either.
	browserInDB := kvs["proxy.browser"]
	if browserInDB == "" {
		// Browser not overridden by DB → remove UA override so both use defaults.
		delete(kvs, "proxy.user_agent")
	}

	for k, v := range kvs {
		switch k {
		case "app.app_key":
			c.App.AppKey = v
		case "app.media_generation_enabled":
			c.App.MediaGenerationEnabled = v == "true"
		case "app.temporary":
			c.App.Temporary = v == "true"
		case "app.stream":
			c.App.Stream = v == "true"
		case "app.thinking":
			c.App.Thinking = v == "true"
		case "app.dynamic_statsig":
			c.App.DynamicStatsig = v == "true"
		case "app.custom_instruction":
			c.App.CustomInstruction = v
		case "app.filter_tags":
			if v != "" {
				c.App.FilterTags = strings.Split(v, ",")
			} else {
				c.App.FilterTags = []string{}
			}
		case "app.disable_memory":
			c.App.DisableMemory = v == "true"
		case "app.request_timeout":
			if n, err := strconv.Atoi(v); err == nil {
				c.App.RequestTimeout = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "app.read_header_timeout":
			if n, err := strconv.Atoi(v); err == nil {
				c.App.ReadHeaderTimeout = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "app.max_header_bytes":
			if n, err := strconv.Atoi(v); err == nil {
				c.App.MaxHeaderBytes = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "app.body_limit":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				c.App.BodyLimit = n
			} else {
				slog.Warn("config: invalid int64 override ignored", "key", k, "value", v, "error", err)
			}
		case "app.chat_body_limit":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				c.App.ChatBodyLimit = n
			} else {
				slog.Warn("config: invalid int64 override ignored", "key", k, "value", v, "error", err)
			}
		case "app.admin_max_fails":
			if n, err := strconv.Atoi(v); err == nil {
				c.App.AdminMaxFails = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "app.admin_window_sec":
			if n, err := strconv.Atoi(v); err == nil {
				c.App.AdminWindowSec = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		// Proxy overrides
		case "proxy.base_proxy_url":
			c.Proxy.BaseProxyURL = v
		case "proxy.asset_proxy_url":
			c.Proxy.AssetProxyURL = v
		case "proxy.cf_cookies":
			c.Proxy.CFCookies = v
		case "proxy.skip_proxy_ssl_verify":
			c.Proxy.SkipProxySSLVerify = v == "true"
		case "proxy.enabled":
			c.Proxy.Enabled = v == "true"
		case "proxy.flaresolverr_url":
			c.Proxy.FlareSolverrURL = v
		case "proxy.refresh_interval":
			if n, err := strconv.Atoi(v); err == nil {
				c.Proxy.RefreshInterval = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "proxy.timeout":
			if n, err := strconv.Atoi(v); err == nil {
				c.Proxy.Timeout = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "proxy.cf_clearance":
			c.Proxy.CFClearance = v
		case "proxy.browser":
			if v != "" {
				c.Proxy.Browser = v
			}
		case "proxy.user_agent":
			if v != "" {
				c.Proxy.UserAgent = v
			}
		// Retry overrides
		case "retry.max_tokens":
			if n, err := strconv.Atoi(v); err == nil {
				c.Retry.MaxTokens = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "retry.per_token_retries":
			if n, err := strconv.Atoi(v); err == nil {
				c.Retry.PerTokenRetries = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "retry.reset_session_status_codes":
			if v != "" {
				parts := strings.Split(v, ",")
				codes := make([]int, 0, len(parts))
				for _, p := range parts {
					if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
						codes = append(codes, n)
					}
				}
				c.Retry.ResetSessionStatusCodes = codes
			} else {
				c.Retry.ResetSessionStatusCodes = []int{}
			}
		case "retry.cooling_status_codes":
			if v != "" {
				parts := strings.Split(v, ",")
				codes := make([]int, 0, len(parts))
				for _, p := range parts {
					if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
						codes = append(codes, n)
					}
				}
				c.Retry.CoolingStatusCodes = codes
			} else {
				c.Retry.CoolingStatusCodes = []int{}
			}
		case "retry.retry_backoff_base":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				c.Retry.RetryBackoffBase = f
			} else {
				slog.Warn("config: invalid float override ignored", "key", k, "value", v, "error", err)
			}
		case "retry.retry_backoff_factor":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				c.Retry.RetryBackoffFactor = f
			} else {
				slog.Warn("config: invalid float override ignored", "key", k, "value", v, "error", err)
			}
		case "retry.retry_backoff_max":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				c.Retry.RetryBackoffMax = f
			} else {
				slog.Warn("config: invalid float override ignored", "key", k, "value", v, "error", err)
			}
		case "retry.retry_budget":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				c.Retry.RetryBudget = f
			} else {
				slog.Warn("config: invalid float override ignored", "key", k, "value", v, "error", err)
			}
		// Image overrides
		case "image.nsfw":
			c.Image.NSFW = v == "true"
		case "image.blocked_parallel_attempts":
			if n, err := strconv.Atoi(v); err == nil {
				c.Image.BlockedParallelAttempts = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "image.blocked_parallel_enabled":
			enabled := v == "true"
			c.Image.BlockedParallelEnabled = &enabled
		// ImagineFast overrides
		case "imagine_fast.n":
			if n, err := strconv.Atoi(v); err == nil {
				c.ImagineFast.N = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "imagine_fast.size":
			c.ImagineFast.Size = v
		// Token overrides
		case "token.fail_threshold":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.FailThreshold = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.cool_check_interval_sec":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.CoolCheckIntervalSec = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.usage_flush_interval_sec":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.UsageFlushIntervalSec = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.basic_models":
			if v != "" {
				c.Token.BasicModels = splitTrimmed(v)
			}
		case "token.super_models":
			if v != "" {
				c.Token.SuperModels = splitTrimmed(v)
			}
		case "token.preferred_pool":
			c.Token.PreferredPool = v
		case "token.basic_cool_duration_min":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.BasicCoolDurationMin = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.super_cool_duration_min":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.SuperCoolDurationMin = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.default_chat_quota":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.DefaultChatQuota = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.default_image_quota":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.DefaultImageQuota = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.default_video_quota":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.DefaultVideoQuota = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.max_chat_quota":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.MaxChatQuota = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.max_image_quota":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.MaxImageQuota = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.max_video_quota":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.MaxVideoQuota = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.allow_unlimited_quota":
			c.Token.AllowUnlimitedQuota = v == "true"
		case "token.token_health_check_interval_minutes":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.TokenHealthCheckIntervalMinutes = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.token_expiry_warning_hours":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.TokenExpiryWarningHours = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.token_critical_expiry_hours":
			if n, err := strconv.Atoi(v); err == nil {
				c.Token.TokenCriticalExpiryHours = n
			} else {
				slog.Warn("config: invalid int override ignored", "key", k, "value", v, "error", err)
			}
		case "token.allow_token_auto_refresh":
			c.Token.AllowTokenAutoRefresh = v == "true"
		case "token.require_official_refresh_flow":
			c.Token.RequireOfficialRefreshFlow = v == "true"
		case "token.quota_recovery_mode":
			c.Token.QuotaRecoveryMode = v
		case "token.selection_algorithm":
			if v != "" {
				c.Token.SelectionAlgorithm = v
			}
		}
	}
}

func splitTrimmed(v string) []string {
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// ApplyEnvOverrides applies environment-based token quota overrides.
// Priority: env > DB > config file > defaults.
func (c *Config) ApplyEnvOverrides() {
	applyIntEnv("GROKPI_DEFAULT_CHAT_QUOTA", &c.Token.DefaultChatQuota)
	applyIntEnv("GROKPI_DEFAULT_IMAGE_QUOTA", &c.Token.DefaultImageQuota)
	applyIntEnv("GROKPI_DEFAULT_VIDEO_QUOTA", &c.Token.DefaultVideoQuota)
	applyIntEnv("GROKPI_MAX_CHAT_QUOTA", &c.Token.MaxChatQuota)
	applyIntEnv("GROKPI_MAX_IMAGE_QUOTA", &c.Token.MaxImageQuota)
	applyIntEnv("GROKPI_MAX_VIDEO_QUOTA", &c.Token.MaxVideoQuota)
	applyBoolEnv("GROKPI_ALLOW_UNLIMITED_QUOTA", &c.Token.AllowUnlimitedQuota)
	applyIntEnv("GROKPI_TOKEN_HEALTH_CHECK_INTERVAL_MINUTES", &c.Token.TokenHealthCheckIntervalMinutes)
	applyIntEnv("GROKPI_TOKEN_EXPIRY_WARNING_HOURS", &c.Token.TokenExpiryWarningHours)
	applyIntEnv("GROKPI_TOKEN_CRITICAL_EXPIRY_HOURS", &c.Token.TokenCriticalExpiryHours)
	applyBoolEnv("GROKPI_ALLOW_TOKEN_AUTO_REFRESH", &c.Token.AllowTokenAutoRefresh)
	applyBoolEnv("GROKPI_REQUIRE_OFFICIAL_REFRESH_FLOW", &c.Token.RequireOfficialRefreshFlow)
}

func applyIntEnv(name string, target *int) {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*target = n
		} else {
			slog.Warn("config: invalid int env ignored", "env", name, "value", v, "error", err)
		}
	}
}

func applyBoolEnv(name string, target *bool) {
	if v := os.Getenv(name); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*target = b
		} else {
			slog.Warn("config: invalid bool env ignored", "env", name, "value", v, "error", err)
		}
	}
}

// Load loads configuration from the given path.
// If the file does not exist, returns default configuration.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		cfg.ApplyEnvOverrides()
		return cfg, nil
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg.ApplyEnvOverrides()
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}

	cfg.ApplyEnvOverrides()
	return cfg, nil
}
