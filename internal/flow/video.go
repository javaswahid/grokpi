package flow

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/javaswahid/grokpi/internal/cache"
	"github.com/javaswahid/grokpi/internal/config"
	"github.com/javaswahid/grokpi/internal/store"
	tkn "github.com/javaswahid/grokpi/internal/token"
	"github.com/javaswahid/grokpi/internal/xai"
)

type videoSelectionDiagnosticsProvider interface {
	SelectionDiagnostics(pool string, cat tkn.QuotaCategory) tkn.SelectionDiagnostics
}

// VideoError is an operational video failure with a stable public code.
type VideoError struct {
	Code    string
	Status  int
	Message string
}

func (e *VideoError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// VideoClient defines the interface for video generation API calls.
type VideoClient interface {
	Chat(ctx context.Context, req *xai.ChatRequest) (<-chan xai.StreamEvent, error)
	CreateImagePost(ctx context.Context, imageURL string) (string, error)
	CreateVideoPost(ctx context.Context, prompt string) (string, error)
	PollUpscale(ctx context.Context, videoID string, interval time.Duration) (string, error)
	DownloadTo(ctx context.Context, url string, w io.Writer) error
	DownloadURL(ctx context.Context, url string) ([]byte, error)
	UploadFile(ctx context.Context, fileName, fileMimeType, contentBase64 string) (string, string, error)
}

// VideoFlowConfig holds configuration for video processing.
type VideoFlowConfig struct {
	TimeoutSeconds      int
	PollIntervalSeconds int
	TokenConfig         *config.TokenConfig
}

// VideoRequest represents a video generation request.
type VideoRequest struct {
	Prompt         string
	Model          string
	Size           string
	AspectRatio    string // e.g. "16:9", "3:2" — passed directly to xAI
	Seconds        int
	Quality        string
	Preset         string
	ReferenceImage []byte
}

// VideoFlow handles async video generation.
type VideoFlow struct {
	tokenSvc      TokenServicer
	clientFactory func(token string) VideoClient
	cfg           *VideoFlowConfig
	usageLog      UsageRecorder
	cacheSvc      *cache.Service
	appConfigFn   func() *config.AppConfig
}

// NewVideoFlow creates a new VideoFlow.
func NewVideoFlow(
	tokenSvc TokenServicer,
	clientFactory func(token string) VideoClient,
	cfg *VideoFlowConfig,
) *VideoFlow {
	if cfg == nil {
		cfg = &VideoFlowConfig{
			TimeoutSeconds:      300,
			PollIntervalSeconds: 5,
		}
	}
	return &VideoFlow{
		tokenSvc:      tokenSvc,
		clientFactory: clientFactory,
		cfg:           cfg,
	}
}

// SetUsageRecorder sets the usage recorder for logging API usage.
func (f *VideoFlow) SetUsageRecorder(ur UsageRecorder) {
	f.usageLog = ur
}

// SetCacheService sets the cache service for video download proxy.
func (f *VideoFlow) SetCacheService(svc *cache.Service) {
	f.cacheSvc = svc
}

// SetAppConfig sets app-level defaults for app-chat based video generation.
func (f *VideoFlow) SetAppConfig(cfg *config.AppConfig) {
	f.appConfigFn = func() *config.AppConfig { return cfg }
}

// SetAppConfigProvider sets a dynamic app config provider.
func (f *VideoFlow) SetAppConfigProvider(fn func() *config.AppConfig) {
	f.appConfigFn = fn
}

func (f *VideoFlow) appConfig() *config.AppConfig {
	if f.appConfigFn == nil {
		return nil
	}
	return f.appConfigFn()
}

// GenerateSync runs video generation synchronously and returns the final URL.
func (f *VideoFlow) GenerateSync(ctx context.Context, req *VideoRequest) (string, error) {
	apiKeyID := FlowAPIKeyIDFromContext(ctx)
	LogVideoStage(ctx, "routed_to_video", "model", req.Model)
	tok, err := f.pickTokenForModel(req.Model)
	if err != nil {
		LogVideoStage(ctx, "failed", "error_code", "token_selection_failed", "error", err)
		return "", err
	}

	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(f.cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	LogVideoStage(timeoutCtx, "upstream_submit_started", "model", req.Model)
	videoURL, err := f.generateVideoViaChat(timeoutCtx, tok, req)
	if err != nil {
		err = classifyVideoGenerationError(err)
		LogVideoStage(timeoutCtx, "failed", "error_code", videoErrorCode(err), "error", err, "duration_ms", time.Since(start).Milliseconds())
		f.reportTokenError(tok.ID, err)
		f.recordUsage(apiKeyID, tok.ID, req.Model, 500, time.Since(start))
		return "", err
	}

	if _, err := f.tokenSvc.Consume(tok.ID, tkn.CategoryVideo, 1); err != nil {
		return "", err
	}
	f.tokenSvc.ReportSuccess(tok.ID)
	f.recordUsage(apiKeyID, tok.ID, req.Model, 200, time.Since(start))
	LogVideoStage(timeoutCtx, "upstream_completed", "duration_ms", time.Since(start).Milliseconds(), "video_url", videoURL)
	return videoURL, nil
}

func videoErrorCode(err error) string {
	var videoErr *VideoError
	if errors.As(err, &videoErr) {
		return videoErr.Code
	}
	return "video_generation_failed"
}

// reportTokenError reports the appropriate token error based on error type.
func (f *VideoFlow) reportTokenError(tokenID uint, err error) {
	if isTransportError(err) {
		return
	}
	reason := truncateReason(err.Error())
	var videoErr *VideoError
	if errors.As(err, &videoErr) {
		switch videoErr.Code {
		case "token_expired":
			f.tokenSvc.MarkExpired(tokenID, "token expired for video: "+reason)
			return
		case "token_invalid":
			f.tokenSvc.ReportError(tokenID, "token invalid for video: "+reason)
			return
		case "video_cooldown", "upstream_quota_exhausted":
			f.tokenSvc.ReportRateLimit(tokenID, reason)
			return
		case "policy_violation":
			f.tokenSvc.ReportError(tokenID, reason)
			return
		case "timeout", "invalid_video_response", "download_failed", "unknown_upstream_error":
			return
		}
	}
	if errors.Is(err, xai.ErrInvalidToken) || errors.Is(err, xai.ErrForbidden) {
		f.tokenSvc.MarkExpired(tokenID, "token expired or invalid for video: "+reason)
		return
	}
	if ShouldCoolToken(err, nil) {
		f.tokenSvc.ReportRateLimit(tokenID, reason)
	} else {
		f.tokenSvc.ReportError(tokenID, reason)
	}
}

// recordUsage records a video API usage log entry via the buffer (non-blocking).
func (f *VideoFlow) recordUsage(apiKeyID, tokenID uint, model string, status int, latency time.Duration) {
	if f.usageLog == nil {
		return
	}
	_ = f.usageLog.Record(context.Background(), &store.UsageLog{
		APIKeyID:    apiKeyID,
		TokenID:     tokenID,
		Model:       model,
		Endpoint:    "video",
		Status:      status,
		DurationMs:  latency.Milliseconds(),
		TTFTMs:      0,
		CacheTokens: 0,
		CreatedAt:   time.Now(),
	})
}

func (f *VideoFlow) pickTokenForModel(model string) (*store.Token, error) {
	cfg := f.cfg
	if cfg != nil && cfg.TokenConfig != nil {
		primary, fallback, ok := tkn.GetPoolsForModel(model, cfg.TokenConfig)
		if ok {
			tok, err := f.tokenSvc.Pick(primary, tkn.CategoryVideo)
			if err == nil {
				f.logVideoSelection(model, primary, tok, "")
				return tok, nil
			}
			if fallback != "" {
				f.logVideoSelection(model, primary, nil, err.Error())
				tok, fallbackErr := f.tokenSvc.Pick(fallback, tkn.CategoryVideo)
				if fallbackErr == nil {
					f.logVideoSelection(model, fallback, tok, "")
					return tok, nil
				}
				f.logVideoSelection(model, fallback, nil, fallbackErr.Error())
				return nil, f.videoTokenSelectionError(model, err, primary, fallback)
			}
			f.logVideoSelection(model, primary, nil, err.Error())
			return nil, f.videoTokenSelectionError(model, err, primary)
		}
		return nil, &VideoError{Code: "video_route_not_configured", Status: 404, Message: "video model is not routed to any token pool"}
	}
	tok, err := f.tokenSvc.Pick(tkn.PoolBasic, tkn.CategoryVideo)
	if err == nil {
		f.logVideoSelection(model, tkn.PoolBasic, tok, "")
		return tok, nil
	}
	f.logVideoSelection(model, tkn.PoolBasic, nil, err.Error())
	return nil, f.videoTokenSelectionError(model, err, tkn.PoolBasic)
}

func (f *VideoFlow) videoTokenSelectionError(model string, err error, pools ...string) error {
	code := "no_token_available"
	if diagProvider, ok := f.tokenSvc.(videoSelectionDiagnosticsProvider); ok {
		for _, pool := range pools {
			diag := diagProvider.SelectionDiagnostics(pool, tkn.CategoryVideo)
			poolCode := selectionErrorCode(diag)
			if poolCode != "no_token_available" {
				code = poolCode
				break
			}
		}
	}
	return &VideoError{Code: code, Status: videoErrorStatus(code), Message: code + ": no eligible video token for " + model}
}

func (f *VideoFlow) logVideoSelection(model, pool string, tok *store.Token, finalError string) {
	attrs := []any{
		"requested_model", model,
		"requested_capability", "video",
		"selected_pool", pool,
		"final_error_code", finalError,
	}
	if diagProvider, ok := f.tokenSvc.(videoSelectionDiagnosticsProvider); ok {
		diag := diagProvider.SelectionDiagnostics(pool, tkn.CategoryVideo)
		attrs = append(attrs,
			"candidate_token_count", diag.CandidateTokenCount,
			"rejected_token_count", diag.RejectedTokenCount,
			"rejected_token_reasons", diag.RejectedReasons,
		)
	}
	if tok != nil {
		attrs = append(attrs,
			"selected_token_id", tok.ID,
			"selected_token_status", tok.Status,
			"selected_token_video_quota", tok.VideoQuota,
			"selected_token_video_used", max(tok.InitialVideoQuota-tok.VideoQuota, 0),
		)
	}
	slog.Info("video: token selector", attrs...)
}

func selectionErrorCode(diag tkn.SelectionDiagnostics) string {
	if diag.CandidateTokenCount == 0 {
		return "no_token_available"
	}
	for _, code := range []string{
		"api_key_model_forbidden",
		"video_route_not_configured",
		"token_expired",
		"token_invalid",
		"token_refresh_required",
		"policy_quarantine",
		"needs_manual_check",
		"video_cooldown",
		"rate_limited",
		"token_not_video_capable",
		"upstream_quota_exhausted",
		"no_video_quota",
	} {
		if diag.RejectedReasons[code] > 0 {
			return code
		}
	}
	return "no_token_available"
}

func classifyVideoGenerationError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	lower := strings.ToLower(message)
	code := ""
	switch {
	case errors.Is(err, xai.ErrInvalidToken):
		code = "token_expired"
	case tkn.IsPolicyViolationReason(lower):
		code = "policy_violation"
	case errors.Is(err, xai.ErrForbidden):
		code = "token_invalid"
	case errors.Is(err, xai.ErrRateLimited), strings.Contains(lower, "status 429"), strings.Contains(lower, "cooldown"):
		code = "video_cooldown"
	case strings.Contains(lower, "status 401"):
		code = "token_expired"
	case strings.Contains(lower, "status 403"):
		code = "token_not_video_capable"
	case strings.Contains(lower, "quota"):
		code = "upstream_quota_exhausted"
	case strings.Contains(lower, "context deadline"), strings.Contains(lower, "timeout"), strings.Contains(lower, "timed out"):
		code = "timeout"
	case strings.Contains(lower, "missing final url"), strings.Contains(lower, "no video url"), strings.Contains(lower, "returned no video url"):
		code = "invalid_video_response"
	case strings.Contains(lower, "download failed"), strings.Contains(lower, "cache save failed"):
		code = "download_failed"
	default:
		code = "unknown_upstream_error"
	}
	return &VideoError{Code: code, Status: videoErrorStatus(code), Message: code + ": " + message}
}

func videoErrorStatus(code string) int {
	switch code {
	case "api_key_model_forbidden", "model_not_allowed", "token_not_video_capable":
		return 403
	case "video_route_not_configured":
		return 404
	case "video_cooldown", "upstream_quota_exhausted", "no_video_quota":
		return 429
	case "token_expired", "token_invalid", "token_refresh_required":
		return 401
	case "policy_violation":
		return 422
	case "timeout":
		return 504
	default:
		return 503
	}
}

func (f *VideoFlow) cacheVideo(ctx context.Context, client VideoClient, videoURL string) string {
	if f.cacheSvc == nil {
		return videoURL
	}
	reader, writer := io.Pipe()
	doneCh := make(chan error, 1)
	SafeGo("video_cache_download", func() {
		err := client.DownloadTo(ctx, videoURL, writer)
		doneCh <- err
		writer.CloseWithError(err)
	})
	filename, saveErr := f.cacheSvc.SaveStream("video", reader, ".mp4")
	if dlErr := <-doneCh; dlErr != nil {
		slog.Warn("video: stream download failed, using original URL", "error", dlErr)
		return videoURL
	}
	if saveErr != nil {
		slog.Warn("video: cache save failed, using original URL", "error", saveErr)
		return videoURL
	}
	return "/api/files/video/" + filename
}
