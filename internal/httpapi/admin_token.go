package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/crmmc/grokpi/internal/store"
	tokenpkg "github.com/crmmc/grokpi/internal/token"
	"github.com/go-chi/chi/v5"
)

// TokenStoreInterface defines the methods needed for token CRUD operations.
type TokenStoreInterface interface {
	ListTokens(ctx context.Context) ([]*store.Token, error)
	ListTokensFiltered(ctx context.Context, filter store.TokenFilter) ([]*store.Token, error)
	ListTokenIDs(ctx context.Context, filter store.TokenFilter) ([]uint, error)
	GetToken(ctx context.Context, id uint) (*store.Token, error)
	CreateToken(ctx context.Context, token *store.Token) error
	UpdateToken(ctx context.Context, token *store.Token) error
	DeleteToken(ctx context.Context, id uint) error
	BatchUpdateTokens(ctx context.Context, req store.BatchUpdateRequest) (int, error)
}

// TokenRefresher defines the interface for refreshing token quota.
type TokenRefresher interface {
	RefreshToken(ctx context.Context, id uint) (*store.Token, error)
}

// TokenRevalidator defines the interface for expiry-aware validation without quota reset.
type TokenRevalidator interface {
	RevalidateToken(ctx context.Context, id uint) (*store.Token, error)
}

// TokenOfficialRefresher defines safe official token refresh behavior.
type TokenOfficialRefresher interface {
	RefreshTokenOfficial(ctx context.Context, id uint) (*store.Token, error)
}

// TokenVideoCapabilityTester performs an explicit upstream video capability probe.
type TokenVideoCapabilityTester interface {
	TestVideoCapability(ctx context.Context, id uint) (*store.Token, error)
}

// TokenPoolSyncer syncs admin token changes to in-memory pools.
type TokenPoolSyncer interface {
	AddToPool(token *store.Token)
	RemoveFromPool(id uint)
	SyncToken(ctx context.Context, id uint) error
}

// TokenResponse is the API response for a token (with masked sensitive data).
type TokenResponse struct {
	ID                   uint       `json:"id"`
	Token                string     `json:"token"`
	Pool                 string     `json:"pool"`
	Status               string     `json:"status"`
	StatusReason         string     `json:"status_reason,omitempty"`
	QuotaMode            string     `json:"quota_mode"`
	ChatQuota            int        `json:"chat_quota"`
	TotalChatQuota       int        `json:"total_chat_quota"`
	ImageQuota           int        `json:"image_quota"`
	TotalImageQuota      int        `json:"total_image_quota"`
	VideoQuota           int        `json:"video_quota"`
	TotalVideoQuota      int        `json:"total_video_quota"`
	FailCount            int        `json:"fail_count"`
	CoolUntil            *time.Time `json:"cool_until,omitempty"`
	LastUsed             *time.Time `json:"last_used,omitempty"`
	Remark               string     `json:"remark,omitempty"`
	NsfwEnabled          bool       `json:"nsfw_enabled"`
	Priority             int        `json:"priority"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	LastValidatedAt      *time.Time `json:"last_validated_at,omitempty"`
	LastRefreshAttemptAt *time.Time `json:"last_refresh_attempt_at,omitempty"`
	RefreshStatus        string     `json:"refresh_status,omitempty"`
	RefreshError         string     `json:"refresh_error,omitempty"`
	RefreshCount         int        `json:"refresh_count"`
	TokenVersion         int        `json:"token_version"`
	ReplacedAt           *time.Time `json:"replaced_at,omitempty"`
	WarningMessage       string     `json:"warning_message,omitempty"`
	ConfiguredVideoQuota int        `json:"configured_video_quota"`
	LocalVideoUsed       int        `json:"local_video_used"`
	LocalVideoRemaining  int        `json:"local_video_remaining"`
	UpstreamVideoCapable *bool      `json:"upstream_video_capable,omitempty"`
	LastVideoCheckAt     *time.Time `json:"last_video_check_at,omitempty"`
	LastVideoCheckStatus string     `json:"last_video_check_status,omitempty"`
	LastVideoError       string     `json:"last_video_error,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// tokenToResponse converts a store.Token to TokenResponse with masked token.
func tokenToResponse(t *store.Token) TokenResponse {
	totalChat, totalImage, totalVideo := resolveTokenQuotaTotals(t, nil)

	return TokenResponse{
		ID:                   t.ID,
		Token:                maskSecret(t.Token),
		Pool:                 t.Pool,
		Status:               t.Status,
		StatusReason:         t.StatusReason,
		QuotaMode:            tokenpkg.NormalizeQuotaMode(t.QuotaMode),
		ChatQuota:            t.ChatQuota,
		TotalChatQuota:       totalChat,
		ImageQuota:           t.ImageQuota,
		TotalImageQuota:      totalImage,
		VideoQuota:           t.VideoQuota,
		TotalVideoQuota:      totalVideo,
		FailCount:            t.FailCount,
		CoolUntil:            t.CoolUntil,
		LastUsed:             t.LastUsed,
		Remark:               t.Remark,
		NsfwEnabled:          t.NsfwEnabled,
		Priority:             t.Priority,
		ExpiresAt:            t.ExpiresAt,
		LastValidatedAt:      t.LastValidatedAt,
		LastRefreshAttemptAt: t.LastRefreshAttemptAt,
		RefreshStatus:        t.RefreshStatus,
		RefreshError:         t.RefreshError,
		RefreshCount:         t.RefreshCount,
		TokenVersion:         max(t.TokenVersion, 1),
		ReplacedAt:           t.ReplacedAt,
		WarningMessage:       tokenWarningMessage(t),
		ConfiguredVideoQuota: totalVideo,
		LocalVideoUsed:       max(totalVideo-t.VideoQuota, 0),
		LocalVideoRemaining:  t.VideoQuota,
		UpstreamVideoCapable: t.UpstreamVideoCapable,
		LastVideoCheckAt:     t.LastVideoCheckAt,
		LastVideoCheckStatus: t.LastVideoCheckStatus,
		LastVideoError:       t.LastVideoError,
		CreatedAt:            t.CreatedAt,
		UpdatedAt:            t.UpdatedAt,
	}
}

// PaginatedTokenResponse wraps tokens with pagination metadata.
type PaginatedTokenResponse struct {
	Data       []TokenResponse `json:"data"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

type TokenHealthResponse struct {
	StatusSummary           map[string]int `json:"status_summary"`
	Total                   int            `json:"total"`
	Active                  int            `json:"active"`
	RefreshEligible         int            `json:"refresh_eligible"`
	PolicyQuarantine        int            `json:"policy_quarantine"`
	PolicyAutoEnableBlocked bool           `json:"policy_auto_enable_blocked"`
	Message                 string         `json:"message"`
}

func handleTokenHealth(ts TokenStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokens, err := ts.ListTokens(r.Context())
		if err != nil {
			WriteError(w, 500, "server_error", "token_health_failed", "Failed to read token health")
			return
		}
		resp := TokenHealthResponse{
			StatusSummary:           map[string]int{},
			PolicyAutoEnableBlocked: true,
			Message:                 "policy_quarantine tokens require safe prompt repair before reuse",
		}
		for _, t := range tokens {
			status := t.Status
			if status == "" {
				status = store.TokenStatusActive
			}
			resp.Total++
			resp.StatusSummary[status]++
			if status == store.TokenStatusActive {
				resp.Active++
			}
			switch status {
			case store.TokenStatusPolicyQuarantine:
				resp.PolicyQuarantine++
			case store.TokenStatusExpired, store.TokenStatusCooling, store.TokenStatusRateLimited, store.TokenStatusQuotaExhausted, store.TokenStatusRefreshRequired, store.TokenStatusRefreshFailed:
				resp.RefreshEligible++
			}
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}

// handleListTokens returns a handler that lists all tokens with pagination.
func handleListTokens(ts TokenStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := store.TokenFilter{}

		// Parse status filter
		if status := r.URL.Query().Get("status"); status != "" {
			filter.Status = &status
		}

		// Parse nsfw filter
		if nsfw := r.URL.Query().Get("nsfw"); nsfw != "" {
			val, err := strconv.ParseBool(nsfw)
			if err != nil {
				WriteError(w, 400, "invalid_request", "invalid_nsfw",
					"nsfw must be true or false")
				return
			}
			filter.NsfwEnabled = &val
		}

		// Parse pagination params
		page := 1
		pageSize := 20
		if p := r.URL.Query().Get("page"); p != "" {
			if v, err := strconv.Atoi(p); err == nil && v > 0 {
				page = v
			}
		}
		if ps := r.URL.Query().Get("page_size"); ps != "" {
			if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 100 {
				pageSize = v
			}
		}

		var tokens []*store.Token
		var err error
		if filter.Status != nil || filter.NsfwEnabled != nil {
			tokens, err = ts.ListTokensFiltered(r.Context(), filter)
		} else {
			tokens, err = ts.ListTokens(r.Context())
		}

		if err != nil {
			WriteError(w, 500, "server_error", "list_failed",
				"Failed to list tokens")
			return
		}

		total := len(tokens)
		totalPages := 0
		if total > 0 {
			totalPages = (total + pageSize - 1) / pageSize
		}

		// Apply pagination
		offset := (page - 1) * pageSize
		end := offset + pageSize
		if offset > total {
			offset = total
		}
		if end > total {
			end = total
		}
		paged := tokens[offset:end]

		data := make([]TokenResponse, len(paged))
		for i, t := range paged {
			data[i] = tokenToResponse(t)
		}

		resp := PaginatedTokenResponse{
			Data:       data,
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: totalPages,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// handleGetToken returns a handler that gets a single token by ID.
func handleGetToken(ts TokenStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id",
				"Invalid token ID")
			return
		}

		token, err := ts.GetToken(r.Context(), uint(id))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "token_not_found",
					"Token not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed",
				"Failed to get token")
			return
		}

		resp := tokenToResponse(token)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// TokenUpdateRequest is the request body for updating a token.
type TokenUpdateRequest struct {
	Status      *string `json:"status,omitempty"`
	Pool        *string `json:"pool,omitempty"`
	ChatQuota   *int    `json:"chat_quota,omitempty"`
	ImageQuota  *int    `json:"image_quota,omitempty"`
	VideoQuota  *int    `json:"video_quota,omitempty"`
	QuotaMode   *string `json:"quota_mode,omitempty"`
	Priority    *int    `json:"priority,omitempty"`
	Remark      *string `json:"remark,omitempty"`
	NsfwEnabled *bool   `json:"nsfw_enabled,omitempty"`
}

// handleUpdateToken returns a handler that updates an existing token.
func handleUpdateToken(ts TokenStoreInterface, syncer TokenPoolSyncer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id",
				"Invalid token ID")
			return
		}

		// First get the existing token
		token, err := ts.GetToken(r.Context(), uint(id))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "token_not_found",
					"Token not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed",
				"Failed to get token")
			return
		}

		var req TokenUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json",
				"Invalid JSON in request body")
			return
		}

		// Validate remark max length
		if req.Remark != nil && len(*req.Remark) > 500 {
			WriteError(w, 400, "invalid_request", "remark_too_long",
				"Remark must be 500 characters or less")
			return
		}

		// Validate status if provided
		if req.Status != nil {
			validStatuses := map[string]bool{
				store.TokenStatusActive:           true,
				store.TokenStatusDisabled:         true,
				store.TokenStatusExpired:          true,
				store.TokenStatusCooling:          true,
				store.TokenStatusQuotaExhausted:   true,
				store.TokenStatusInvalid:          true,
				store.TokenStatusRefreshRequired:  true,
				store.TokenStatusRefreshFailed:    true,
				store.TokenStatusPolicyQuarantine: true,
				store.TokenStatusRateLimited:      true,
				store.TokenStatusNeedsManualCheck: true,
			}
			if !validStatuses[*req.Status] {
				WriteError(w, 400, "invalid_request", "invalid_status",
					"Invalid status. Must be: active, disabled, expired, cooling, quota_exhausted, invalid, refresh_required, refresh_failed, policy_quarantine, rate_limited, or needs_manual_check")
				return
			}
		}
		if req.QuotaMode != nil && !tokenpkg.ValidQuotaMode(*req.QuotaMode) {
			WriteError(w, 400, "invalid_request", "invalid_quota_mode",
				"Invalid quota_mode. Must be: limited, daily_reset, unlimited, or manual")
			return
		}

		// Apply updates
		if req.Status != nil {
			token.Status = *req.Status
			// Record status reason for manual changes
			switch *req.Status {
			case store.TokenStatusDisabled:
				token.StatusReason = "manual disable"
			case store.TokenStatusActive:
				token.StatusReason = ""
				token.ExpiresAt = nil
			case store.TokenStatusQuotaExhausted:
				token.StatusReason = "manual quota exhausted"
			case store.TokenStatusInvalid:
				token.StatusReason = "manual invalid"
			case store.TokenStatusRefreshRequired:
				token.StatusReason = "manual refresh required"
				token.RefreshStatus = tokenpkg.RefreshStatusRequired
			case store.TokenStatusRefreshFailed:
				token.StatusReason = "manual refresh failed"
				token.RefreshStatus = tokenpkg.RefreshStatusFailed
			case store.TokenStatusPolicyQuarantine:
				token.StatusReason = "manual policy quarantine"
				token.RefreshStatus = tokenpkg.RefreshStatusRequired
				token.RefreshError = "policy quarantine: safe prompt repair required"
			case store.TokenStatusRateLimited:
				token.StatusReason = "manual rate limit"
			case store.TokenStatusNeedsManualCheck:
				token.StatusReason = "manual check required"
			}
		}
		if req.Pool != nil {
			token.Pool = *req.Pool
		}
		if req.QuotaMode != nil {
			token.QuotaMode = tokenpkg.NormalizeQuotaMode(*req.QuotaMode)
		}
		if req.ChatQuota != nil {
			token.ChatQuota = *req.ChatQuota
			token.InitialChatQuota = *req.ChatQuota
		}
		if req.ImageQuota != nil {
			token.ImageQuota = *req.ImageQuota
			token.InitialImageQuota = *req.ImageQuota
		}
		if req.VideoQuota != nil {
			token.VideoQuota = *req.VideoQuota
			token.InitialVideoQuota = *req.VideoQuota
		}
		if req.Priority != nil {
			token.Priority = *req.Priority
		}
		if req.Remark != nil {
			token.Remark = *req.Remark
		}
		if req.NsfwEnabled != nil {
			token.NsfwEnabled = *req.NsfwEnabled
		}

		if err := ts.UpdateToken(r.Context(), token); err != nil {
			WriteError(w, 500, "server_error", "update_failed",
				"Failed to update token")
			return
		}

		// Sync to in-memory pool
		if syncer != nil {
			if err := syncer.SyncToken(r.Context(), uint(id)); err != nil {
				slog.Warn("failed to sync token to pool", "token_id", id, "error", err)
			}
		}

		resp := tokenToResponse(token)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// TokenReplaceRequest replaces the secret token while preserving token record history.
type TokenReplaceRequest struct {
	Token      string  `json:"token"`
	Status     *string `json:"status,omitempty"`
	QuotaMode  *string `json:"quota_mode,omitempty"`
	ChatQuota  *int    `json:"chat_quota,omitempty"`
	ImageQuota *int    `json:"image_quota,omitempty"`
	VideoQuota *int    `json:"video_quota,omitempty"`
}

// handleReplaceToken returns a handler that replaces a token secret in-place.
func handleReplaceToken(ts TokenStoreInterface, syncer TokenPoolSyncer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseTokenID(w, r)
		if !ok {
			return
		}

		token, err := ts.GetToken(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "token_not_found", "Token not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed", "Failed to get token")
			return
		}

		var req TokenReplaceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}
		if len(req.Token) < 20 {
			WriteError(w, 400, "invalid_request", "token_too_short", "Token must be at least 20 characters")
			return
		}
		if req.Status != nil && !validTokenStatus(*req.Status) {
			WriteError(w, 400, "invalid_request", "invalid_status",
				"Invalid status. Must be: active, disabled, expired, cooling, quota_exhausted, invalid, refresh_required, refresh_failed, policy_quarantine, rate_limited, or needs_manual_check")
			return
		}
		if req.QuotaMode != nil && !tokenpkg.ValidQuotaMode(*req.QuotaMode) {
			WriteError(w, 400, "invalid_request", "invalid_quota_mode",
				"Invalid quota_mode. Must be: limited, daily_reset, unlimited, or manual")
			return
		}

		token.Token = req.Token
		token.Status = store.TokenStatusActive
		if req.Status != nil {
			token.Status = *req.Status
		}
		token.StatusReason = ""
		token.FailCount = 0
		token.CoolUntil = nil
		now := time.Now()
		token.ReplacedAt = &now
		token.ExpiresAt = nil
		token.LastValidatedAt = nil
		token.LastRefreshAttemptAt = nil
		token.RefreshStatus = tokenpkg.RefreshStatusIdle
		token.RefreshError = ""
		token.TokenVersion = max(token.TokenVersion, 1) + 1
		token.UpstreamVideoCapable = nil
		token.LastVideoCheckAt = nil
		token.LastVideoCheckStatus = ""
		token.LastVideoError = ""
		if req.QuotaMode != nil {
			token.QuotaMode = tokenpkg.NormalizeQuotaMode(*req.QuotaMode)
		} else {
			token.QuotaMode = tokenpkg.NormalizeQuotaMode(token.QuotaMode)
		}
		if req.ChatQuota != nil {
			token.ChatQuota = *req.ChatQuota
			token.InitialChatQuota = *req.ChatQuota
		}
		if req.ImageQuota != nil {
			token.ImageQuota = *req.ImageQuota
			token.InitialImageQuota = *req.ImageQuota
		}
		if req.VideoQuota != nil {
			token.VideoQuota = *req.VideoQuota
			token.InitialVideoQuota = *req.VideoQuota
		}

		if err := ts.UpdateToken(r.Context(), token); err != nil {
			WriteError(w, 500, "server_error", "replace_failed", "Failed to replace token")
			return
		}
		syncTokenAfterAdminChange(r.Context(), syncer, id)

		WriteJSON(w, http.StatusOK, tokenToResponse(token))
	}
}

// handleResetTokenUsage resets local quota counters to initial quotas.
func handleResetTokenUsage(ts TokenStoreInterface, syncer TokenPoolSyncer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseTokenID(w, r)
		if !ok {
			return
		}

		token, err := ts.GetToken(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "token_not_found", "Token not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed", "Failed to get token")
			return
		}

		if token.InitialChatQuota > 0 {
			token.ChatQuota = token.InitialChatQuota
		}
		if token.InitialImageQuota > 0 {
			token.ImageQuota = token.InitialImageQuota
		}
		if token.InitialVideoQuota > 0 {
			token.VideoQuota = token.InitialVideoQuota
		}
		if token.Status == store.TokenStatusCooling || token.Status == store.TokenStatusQuotaExhausted || token.Status == store.TokenStatusRefreshFailed {
			token.Status = store.TokenStatusActive
			token.StatusReason = ""
			token.CoolUntil = nil
		}
		token.FailCount = 0
		token.QuotaMode = tokenpkg.NormalizeQuotaMode(token.QuotaMode)

		if err := ts.UpdateToken(r.Context(), token); err != nil {
			WriteError(w, 500, "server_error", "reset_failed", "Failed to reset token usage")
			return
		}
		syncTokenAfterAdminChange(r.Context(), syncer, id)

		WriteJSON(w, http.StatusOK, tokenToResponse(token))
	}
}

func handleTestVideoCapability(tester TokenVideoCapabilityTester) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if tester == nil {
			WriteError(w, http.StatusNotImplemented, "server_error", "video_capability_test_unavailable", "Video capability test is not configured")
			return
		}
		id, ok := parseTokenID(w, r)
		if !ok {
			return
		}
		token, err := tester.TestVideoCapability(r.Context(), id)
		if err != nil {
			WriteError(w, http.StatusBadGateway, "server_error", "video_capability_test_failed", err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, tokenToResponse(token))
	}
}

func parseTokenID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		WriteError(w, 400, "invalid_request", "invalid_id", "Invalid token ID")
		return 0, false
	}
	return uint(id), true
}

func validTokenStatus(status string) bool {
	switch status {
	case store.TokenStatusActive, store.TokenStatusDisabled, store.TokenStatusExpired,
		store.TokenStatusCooling, store.TokenStatusQuotaExhausted, store.TokenStatusInvalid,
		store.TokenStatusRefreshRequired, store.TokenStatusRefreshFailed,
		store.TokenStatusPolicyQuarantine, store.TokenStatusRateLimited, store.TokenStatusNeedsManualCheck:
		return true
	default:
		return false
	}
}

func tokenWarningMessage(t *store.Token) string {
	if t == nil {
		return ""
	}
	switch t.Status {
	case store.TokenStatusExpired:
		return "Token expired. Replace token before continuing video phase."
	case store.TokenStatusInvalid:
		return "Token invalid. Revalidate or replace token before continuing."
	case store.TokenStatusRefreshRequired:
		return "Token refresh required. Use Replace Token if official refresh is unavailable."
	case store.TokenStatusRefreshFailed:
		return "Token refresh failed. Use Replace Token."
	case store.TokenStatusPolicyQuarantine:
		return "Token in policy quarantine. Repair and validate the prompt before enabling."
	case store.TokenStatusRateLimited:
		return "Token is rate limited. Wait for recovery before using."
	case store.TokenStatusNeedsManualCheck:
		return "Token needs manual review before use."
	}
	if t.ExpiresAt == nil {
		return ""
	}
	now := time.Now()
	if t.ExpiresAt.Before(now) {
		return "Token expired. Replace token before continuing video phase."
	}
	until := time.Until(*t.ExpiresAt)
	if until <= 6*time.Hour {
		return "Token expires in less than 6 hours. Replace token soon."
	}
	if until <= 24*time.Hour {
		return "Token expires in less than 24 hours."
	}
	return ""
}

func syncTokenAfterAdminChange(ctx context.Context, syncer TokenPoolSyncer, id uint) {
	if syncer == nil {
		return
	}
	if err := syncer.SyncToken(ctx, id); err != nil {
		slog.Warn("failed to sync token to pool", "token_id", id, "error", err)
	}
}

// handleListTokenIDs returns a handler that lists token IDs matching a status filter.
func handleListTokenIDs(ts TokenStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := store.TokenFilter{}
		if status := r.URL.Query().Get("status"); status != "" {
			filter.Status = &status
		}
		ids, err := ts.ListTokenIDs(r.Context(), filter)
		if err != nil {
			WriteError(w, 500, "server_error", "list_failed",
				"Failed to list token IDs")
			return
		}
		WriteJSON(w, http.StatusOK, map[string][]uint{"ids": ids})
	}
}

// handleDeleteToken returns a handler that deletes a token.
func handleDeleteToken(ts TokenStoreInterface, syncer TokenPoolSyncer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id",
				"Invalid token ID")
			return
		}

		if err := ts.DeleteToken(r.Context(), uint(id)); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "token_not_found",
					"Token not found")
				return
			}
			WriteError(w, 500, "server_error", "delete_failed",
				"Failed to delete token")
			return
		}

		// Sync to in-memory pool
		if syncer != nil {
			syncer.RemoveFromPool(uint(id))
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
