package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/crmmc/grokpi/internal/config"
	"github.com/crmmc/grokpi/internal/store"
	tokenpkg "github.com/crmmc/grokpi/internal/token"
)

// PreflightRequest checks whether a model category has usable token capacity.
type PreflightRequest struct {
	Model    string `json:"model"`
	Category string `json:"category,omitempty"`
}

// PreflightResponse is safe for BFF clients and never exposes token secrets.
type PreflightResponse struct {
	OK                  bool           `json:"ok"`
	Reason              string         `json:"reason,omitempty"`
	Model               string         `json:"model,omitempty"`
	Pool                string         `json:"pool,omitempty"`
	FallbackPool        string         `json:"fallback_pool,omitempty"`
	Category            string         `json:"category"`
	AvailableTokenCount int            `json:"available_token_count"`
	CandidateTokenCount int            `json:"candidate_token_count"`
	RejectedTokenCount  int            `json:"rejected_token_count"`
	RejectedReasons     map[string]int `json:"rejected_token_reasons,omitempty"`
	StatusSummary       map[string]int `json:"status_summary"`
	Warning             string         `json:"warning,omitempty"`
}

func handleV1Preflight(ts TokenStoreInterface, getCfg func() *config.TokenConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req PreflightRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}
		req.Model = strings.TrimSpace(req.Model)
		cat, ok := preflightCategory(req.Model, req.Category)
		if !ok {
			WriteJSON(w, http.StatusOK, PreflightResponse{
				OK:            false,
				Reason:        "invalid_category",
				Model:         req.Model,
				Category:      req.Category,
				StatusSummary: map[string]int{},
			})
			return
		}

		cfg := getCfg()
		if cfg == nil {
			cfg = &config.TokenConfig{}
		}
		primary, fallback, modelOK := tokenpkg.GetPoolsForModel(req.Model, cfg)
		if !modelOK {
			reason := "model_not_found"
			if cat == tokenpkg.CategoryVideo {
				reason = "video_route_not_configured"
			}
			WriteJSON(w, http.StatusOK, PreflightResponse{
				OK:            false,
				Reason:        reason,
				Model:         req.Model,
				Category:      string(cat),
				StatusSummary: map[string]int{},
			})
			return
		}
		if !CheckModelWhitelist(r.Context(), req.Model) {
			WriteJSON(w, http.StatusOK, PreflightResponse{
				OK:            false,
				Reason:        "api_key_model_forbidden",
				Model:         req.Model,
				Pool:          primary,
				FallbackPool:  fallback,
				Category:      string(cat),
				StatusSummary: map[string]int{},
			})
			return
		}

		tokens, err := ts.ListTokens(r.Context())
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "server_error", "preflight_failed", "Failed to read tokens")
			return
		}

		resp := evaluatePreflight(tokens, primary, fallback, cat, cfg)
		resp.Model = req.Model
		resp.Pool = primary
		resp.FallbackPool = fallback
		resp.Category = string(cat)
		WriteJSON(w, http.StatusOK, resp)
	}
}

func preflightCategory(model, requested string) (tokenpkg.QuotaCategory, bool) {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "", "auto":
	case "chat":
		return tokenpkg.CategoryChat, true
	case "image":
		return tokenpkg.CategoryImage, true
	case "video":
		return tokenpkg.CategoryVideo, true
	default:
		return "", false
	}

	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "video"):
		return tokenpkg.CategoryVideo, true
	case strings.Contains(lower, "imagine"), strings.Contains(lower, "image"):
		return tokenpkg.CategoryImage, true
	default:
		return tokenpkg.CategoryChat, true
	}
}

func evaluatePreflight(tokens []*store.Token, primary, fallback string, cat tokenpkg.QuotaCategory, cfg *config.TokenConfig) PreflightResponse {
	resp := PreflightResponse{StatusSummary: map[string]int{}, RejectedReasons: map[string]int{}}
	now := time.Now()

	for _, t := range tokens {
		if t == nil || (t.Pool != primary && (fallback == "" || t.Pool != fallback)) {
			continue
		}
		resp.CandidateTokenCount++
		status := t.Status
		if status == "" {
			status = store.TokenStatusActive
		}
		if t.ExpiresAt != nil && !t.ExpiresAt.After(now) {
			status = store.TokenStatusExpired
		}
		resp.StatusSummary[status]++

		if reason := tokenpkg.TokenRejectionReason(t, cat, cfg, now); reason != "" {
			resp.RejectedTokenCount++
			resp.RejectedReasons[reason]++
		} else {
			resp.AvailableTokenCount++
		}

		if resp.Warning == "" {
			resp.Warning = tokenExpiryWarning(t)
		}
	}

	if resp.AvailableTokenCount > 0 {
		resp.OK = true
		return resp
	}
	resp.OK = false
	if resp.CandidateTokenCount == 0 {
		resp.Reason = "no_token_available"
		return resp
	}
	for _, reason := range []string{
		"video_route_not_configured",
		"api_key_model_forbidden",
		"token_expired",
		"token_invalid",
		"token_refresh_required",
		"token_refresh_failed",
		"policy_quarantine",
		"needs_manual_check",
		"video_cooldown",
		"cooling",
		"rate_limited",
		"token_not_video_capable",
		"upstream_quota_exhausted",
		quotaReason(cat),
	} {
		if resp.RejectedReasons[reason] > 0 {
			resp.Reason = reason
			return resp
		}
	}
	resp.Reason = "no_token_available"
	return resp
}

func quotaReason(cat tokenpkg.QuotaCategory) string {
	switch cat {
	case tokenpkg.CategoryVideo:
		return "no_video_quota"
	case tokenpkg.CategoryImage:
		return "no_image_quota"
	default:
		return "no_chat_quota"
	}
}

func tokenExpiryWarning(t *store.Token) string {
	if t == nil || t.ExpiresAt == nil {
		return ""
	}
	remaining := time.Until(*t.ExpiresAt)
	switch {
	case remaining <= 0:
		return "Token expired. Replace token before continuing video phase."
	case remaining <= 6*time.Hour:
		return "Token will expire in less than 6 hours."
	case remaining <= 24*time.Hour:
		return "Token will expire in less than 24 hours."
	default:
		return ""
	}
}
