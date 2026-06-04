// Package token provides token lifecycle management with state machine and pool selection.
package token

import "time"

// Status represents the state of a token in the pool.
type Status string

const (
	// StatusActive indicates the token is available for use.
	StatusActive Status = "active"
	// StatusCooling indicates the token is temporarily unavailable (rate limited).
	StatusCooling Status = "cooling"
	// StatusDisabled indicates the token is manually disabled by the user.
	StatusDisabled Status = "disabled"
	// StatusExpired indicates the token was auto-detected as invalid (e.g. 401).
	StatusExpired Status = "expired"
	// StatusQuotaExhausted indicates local quota accounting has reached zero.
	StatusQuotaExhausted Status = "quota_exhausted"
	// StatusInvalid indicates the token format or upstream authentication is invalid.
	StatusInvalid Status = "invalid"
	// StatusRefreshRequired indicates the token needs manual replacement or official refresh before use.
	StatusRefreshRequired Status = "refresh_required"
	// StatusRefreshFailed indicates an official refresh attempt failed.
	StatusRefreshFailed Status = "refresh_failed"
	// StatusPolicyQuarantine indicates a policy/safety failure blocked reuse until prompt repair.
	StatusPolicyQuarantine Status = "policy_quarantine"
	// StatusRateLimited is an explicit rate-limit lifecycle state for admin/reporting compatibility.
	StatusRateLimited Status = "rate_limited"
	// StatusNeedsManualCheck indicates the token needs operator review.
	StatusNeedsManualCheck Status = "needs_manual_check"
)

// Refresh status constants.
const (
	RefreshStatusIdle        = "idle"
	RefreshStatusValidated   = "validated"
	RefreshStatusRequired    = "required"
	RefreshStatusUnavailable = "unavailable"
	RefreshStatusFailed      = "failed"
)

// Quota mode constants.
const (
	QuotaModeLimited    = "limited"
	QuotaModeDailyReset = "daily_reset"
	QuotaModeUnlimited  = "unlimited"
	QuotaModeManual     = "manual"
)

// Pool tier constants.
const (
	PoolBasic = "ssoBasic"
	PoolSuper = "ssoSuper"
)

// Default cooling configuration.
const (
	DefaultCoolDuration   = 5 * time.Minute
	DefaultCoolCycleLimit = 3
)
