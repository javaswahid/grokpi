package token

import (
	"time"

	"github.com/crmmc/grokpi/internal/config"
	"github.com/crmmc/grokpi/internal/store"
)

// SelectionDiagnostics is safe to log and never includes token secrets.
type SelectionDiagnostics struct {
	Pool                string         `json:"pool"`
	Category            string         `json:"category"`
	CandidateTokenCount int            `json:"candidate_token_count"`
	RejectedTokenCount  int            `json:"rejected_token_count"`
	RejectedReasons     map[string]int `json:"rejected_token_reasons"`
	SelectedTokenID     uint           `json:"selected_token_id,omitempty"`
	SelectedTokenStatus string         `json:"selected_token_status,omitempty"`
	SelectedTokenQuota  int            `json:"selected_token_quota,omitempty"`
}

// TokenRejectionReason returns an operational reason when a token cannot serve
// a category. Empty means the token is eligible.
func TokenRejectionReason(t *store.Token, cat QuotaCategory, cfg *config.TokenConfig, now time.Time) string {
	if t == nil {
		return "nil_token"
	}
	if t.ExpiresAt != nil && !t.ExpiresAt.After(now) {
		return "token_expired"
	}
	switch Status(t.Status) {
	case "", StatusActive:
	case StatusQuotaExhausted:
		// Quota-exhausted tokens can recover when a specific category still has
		// quota after an admin reset or daily/manual quota update.
	case StatusCooling:
		if cat != CategoryVideo {
			return "cooling"
		}
		return "video_cooldown"
	case StatusExpired:
		return "token_expired"
	case StatusInvalid:
		return "token_invalid"
	case StatusRefreshRequired:
		return "token_refresh_required"
	case StatusRefreshFailed:
		return "token_refresh_failed"
	case StatusPolicyQuarantine:
		return "policy_quarantine"
	case StatusRateLimited:
		return "rate_limited"
	case StatusNeedsManualCheck:
		return "needs_manual_check"
	case StatusDisabled:
		return "token_disabled"
	default:
		return "token_status_" + t.Status
	}
	if cat == CategoryVideo && t.UpstreamVideoCapable != nil && !*t.UpstreamVideoCapable {
		switch t.LastVideoCheckStatus {
		case "video_cooldown", "upstream_429":
			return "video_cooldown"
		case "quota_exhausted", "upstream_quota_exhausted":
			return "upstream_quota_exhausted"
		case "upstream_401":
			return "token_expired"
		case "upstream_403", "no_video_access", "invalid_video_response":
			return "token_not_video_capable"
		default:
			return "token_not_video_capable"
		}
	}
	if !IsUnlimitedQuota(t, cfg) && GetQuota(t, cat) <= 0 {
		switch cat {
		case CategoryVideo:
			return "no_video_quota"
		case CategoryImage:
			return "no_image_quota"
		default:
			return "no_chat_quota"
		}
	}
	return ""
}

// SelectionDiagnostics summarizes why tokens in a pool are or are not usable.
func (m *TokenManager) SelectionDiagnostics(poolName string, cat QuotaCategory) SelectionDiagnostics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	diag := SelectionDiagnostics{
		Pool:            poolName,
		Category:        string(cat),
		RejectedReasons: map[string]int{},
	}
	now := time.Now()
	for _, t := range m.tokens {
		if t == nil || t.Pool != poolName {
			continue
		}
		diag.CandidateTokenCount++
		if reason := TokenRejectionReason(t, cat, m.cfg, now); reason != "" {
			diag.RejectedTokenCount++
			diag.RejectedReasons[reason]++
			continue
		}
		if diag.SelectedTokenID == 0 {
			diag.SelectedTokenID = t.ID
			diag.SelectedTokenStatus = t.Status
			diag.SelectedTokenQuota = GetQuota(t, cat)
		}
	}
	return diag
}
