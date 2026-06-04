package openai

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"github.com/crmmc/grokpi/internal/flow"
	"github.com/crmmc/grokpi/internal/httpapi"
	"github.com/crmmc/grokpi/internal/store"
)

const (
	videoJobStatusQueued     = "queued"
	videoJobStatusProcessing = "processing"
	videoJobStatusCompleted  = "completed"
	videoJobStatusFailed     = "failed"
	videoJobStatusTimeout    = "timeout"
	videoJobStatusCancelled  = "cancelled"
)

type videoGenerationResponse struct {
	JobID       string     `json:"jobId"`
	Status      string     `json:"status"`
	Model       string     `json:"model"`
	VideoURL    string     `json:"videoUrl,omitempty"`
	ErrorCode   string     `json:"errorCode,omitempty"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

func (h *Handler) handleCreateVideoGeneration(w http.ResponseWriter, r *http.Request) {
	if h.VideoFlow == nil || h.VideoJobs == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, "server_error", "not_implemented", "async video flow not configured")
		return
	}

	req, ok := h.decodeChatRequest(w, r)
	if !ok {
		return
	}
	normalized, valErr := normalizeChatRequest(req, h.currentConfig())
	if valErr != nil {
		httpapi.WriteError(w, valErr.status, valErr.errType, valErr.code, valErr.message)
		return
	}
	if apiErr := h.validateModel(r, normalized.Model); apiErr != nil {
		httpapi.WriteJSON(w, apiErr.Status, apiErr)
		return
	}
	if !isVideoModel(normalized.Model) {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_video_model", "video generation endpoint requires a video model")
		return
	}
	if cfg := h.currentConfig(); cfg != nil && !cfg.App.MediaGenerationEnabled {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "media_generation_disabled", "Image and video generation is disabled by the administrator")
		return
	}

	prompt, images, err := extractChatPromptAndImages(r.Context(), normalized.Messages)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_messages", err.Error())
		return
	}
	if strings.TrimSpace(prompt) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt is required")
		return
	}
	videoCfg, err := h.resolveChatVideoConfig(normalized.VideoConfig)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_video_config", err.Error())
		return
	}

	videoReq := &flow.VideoRequest{
		Prompt:      prompt,
		Model:       normalized.Model,
		Size:        videoCfg.size,
		AspectRatio: videoCfg.aspectRatio,
		Seconds:     videoCfg.seconds,
		Quality:     videoCfg.quality,
		Preset:      videoCfg.preset,
	}
	if len(images) > 0 {
		videoReq.ReferenceImage = images[0]
	}

	payload, _ := json.Marshal(normalized)
	sum := sha256.Sum256(payload)
	now := time.Now().UTC()
	apiKeyID, _ := httpapi.APIKeyIDFromContext(r.Context())
	job := &store.VideoGenerationJob{
		JobID:           newVideoJobID(),
		APIKeyID:        apiKeyID,
		ClientRequestID: strings.TrimSpace(r.Header.Get("X-Client-Request-Id")),
		ProviderEventID: strings.TrimSpace(r.Header.Get("X-Provider-Event-Id")),
		GrokPiRequestID:  chimiddleware.GetReqID(r.Context()),
		Model:            normalized.Model,
		Status:           videoJobStatusQueued,
		PayloadHash:      hex.EncodeToString(sum[:]),
		PayloadSize:      int64(len(payload)),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := h.VideoJobs.Create(r.Context(), job); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "server_error", "video_job_create_failed", err.Error())
		return
	}

	trace := &flow.VideoTrace{
		GrokPiRequestID: job.GrokPiRequestID,
		ClientRequestID: job.ClientRequestID,
		ProviderEventID: job.ProviderEventID,
	}
	flow.LogVideoStage(flow.WithVideoTrace(r.Context(), trace), "request_received",
		"job_id", job.JobID,
		"model", job.Model,
		"payload_size", job.PayloadSize,
	)
	// Create cancellable context for this job (supports /cancel)
	ctx, cancel := context.WithCancel(context.Background())
	h.registerCancel(job.JobID, cancel)

	flow.SafeGo("video_generation_"+job.JobID, func() {
		defer h.unregisterCancel(job.JobID)
		h.runVideoGenerationJob(job.JobID, apiKeyID, videoReq, trace, ctx, cancel)
	})

	httpapi.WriteJSON(w, http.StatusAccepted, toVideoGenerationResponse(r, job))
}

func (h *Handler) handleGetVideoGeneration(w http.ResponseWriter, r *http.Request) {
	job, ok := h.loadVideoJobForRequest(w, r)
	if !ok {
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, toVideoGenerationResponse(r, job))
}

func (h *Handler) handleGetVideoGenerationResult(w http.ResponseWriter, r *http.Request) {
	job, ok := h.loadVideoJobForRequest(w, r)
	if !ok {
		return
	}
	if job.Status != videoJobStatusCompleted || strings.TrimSpace(job.VideoURL) == "" {
		httpapi.WriteError(w, http.StatusConflict, "invalid_request_error", "video_result_not_ready", "video result is not ready")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{
		"jobId":    job.JobID,
		"status":   job.Status,
		"videoUrl": videoURLForRequest(r, job.VideoURL),
	})
}

func (h *Handler) loadVideoJobForRequest(w http.ResponseWriter, r *http.Request) (*store.VideoGenerationJob, bool) {
	if h.VideoJobs == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, "server_error", "not_implemented", "async video flow not configured")
		return nil, false
	}
	jobID := strings.TrimSpace(chi.URLParam(r, "jobId"))
	job, err := h.VideoJobs.GetByID(r.Context(), jobID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "video_job_not_found", "video job not found")
		return nil, false
	}
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "server_error", "video_job_lookup_failed", err.Error())
		return nil, false
	}
	if apiKeyID, ok := httpapi.APIKeyIDFromContext(r.Context()); ok && job.APIKeyID != 0 && job.APIKeyID != apiKeyID {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "video_job_not_found", "video job not found")
		return nil, false
	}
	return job, true
}

func (h *Handler) runVideoGenerationJob(jobID string, apiKeyID uint, videoReq *flow.VideoRequest, trace *flow.VideoTrace, jobCtx context.Context, cancel context.CancelFunc) {
	ctx := context.WithValue(jobCtx, flow.FlowAPIKeyIDKey, apiKeyID)
	ctx = flow.WithVideoTrace(ctx, trace)

	job, err := h.VideoJobs.GetByID(ctx, jobID)
	if err != nil {
		flow.LogVideoStage(ctx, "failed", "job_id", jobID, "error_code", "video_job_lookup_failed", "error", err)
		return
	}

	// Check for early cancel (e.g. user called /cancel immediately)
	select {
	case <-ctx.Done():
		job.Status = videoJobStatusCancelled
		job.ErrorCode = "cancelled"
		job.ErrorMessage = "job cancelled before processing"
		job.UpdatedAt = time.Now().UTC()
		_ = h.VideoJobs.Save(ctx, job)
		flow.LogVideoStage(ctx, "cancelled", "job_id", job.JobID)
		return
	default:
	}

	job.Status = videoJobStatusProcessing
	job.UpdatedAt = time.Now().UTC()
	_ = h.VideoJobs.Save(ctx, job)
	flow.LogVideoStage(ctx, "auth_passed", "job_id", job.JobID, "model", job.Model)

	// Simple retry (1 retry on transient non-permanent errors)
	var videoURL string
	var genErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			flow.LogVideoStage(ctx, "retry", "job_id", jobID, "attempt", attempt+1)
			// small backoff
			select {
			case <-ctx.Done():
				genErr = context.Canceled
				break
			case <-time.After(2 * time.Second):
			}
		}
		videoURL, genErr = h.VideoFlow.GenerateSync(ctx, videoReq)
		if genErr == nil {
			break
		}
		// Only retry on certain transient errors
		var vErr *flow.VideoError
		if errors.As(genErr, &vErr) {
			if vErr.Code == "timeout" || vErr.Code == "upstream_429" || vErr.Code == "rate_limited" {
				continue
			}
		}
		break // non-retryable
	}

	now := time.Now().UTC()
	snapshot := trace.Snapshot()
	job.ParentPostID = snapshot.ParentPostID
	job.VideoPostID = snapshot.VideoPostID
	job.UpstreamRequestID = snapshot.UpstreamRequestID
	job.UpdatedAt = now
	job.CompletedAt = &now

	if ctx.Err() == context.Canceled {
		job.Status = videoJobStatusCancelled
		job.ErrorCode = "cancelled"
		job.ErrorMessage = "job was cancelled"
		_ = h.VideoJobs.Save(ctx, job)
		flow.LogVideoStage(ctx, "cancelled", "job_id", job.JobID)
		return
	}

	if genErr != nil {
		var videoErr *flow.VideoError
		if errors.As(genErr, &videoErr) {
			job.ErrorCode = videoErr.Code
			job.ErrorMessage = videoErr.Error()
		} else {
			job.ErrorCode = "video_generation_failed"
			job.ErrorMessage = genErr.Error()
		}
		if job.ErrorCode == "timeout" {
			job.Status = videoJobStatusTimeout
		} else {
			job.Status = videoJobStatusFailed
		}
		_ = h.VideoJobs.Save(ctx, job)
		flow.LogVideoStage(ctx, "failed", "job_id", job.JobID, "error_code", job.ErrorCode, "error", job.ErrorMessage)
		return
	}

	job.Status = videoJobStatusCompleted
	job.VideoURL = videoURL
	job.ErrorCode = ""
	job.ErrorMessage = ""
	_ = h.VideoJobs.Save(ctx, job)
	flow.LogVideoStage(ctx, "response_sent", "job_id", job.JobID, "video_url", videoURL)
}

func toVideoGenerationResponse(r *http.Request, job *store.VideoGenerationJob) videoGenerationResponse {
	return videoGenerationResponse{
		JobID:        job.JobID,
		Status:       job.Status,
		Model:        job.Model,
		VideoURL:     videoURLForRequest(r, job.VideoURL),
		ErrorCode:    job.ErrorCode,
		ErrorMessage: job.ErrorMessage,
		CreatedAt:    job.CreatedAt,
		UpdatedAt:    job.UpdatedAt,
		CompletedAt:  job.CompletedAt,
	}
}

func videoURLForRequest(r *http.Request, videoURL string) string {
	if strings.HasPrefix(videoURL, "/api/files/video/") {
		filename := strings.TrimPrefix(videoURL, "/api/files/video/")
		return buildFileURL(r, "video", filename)
	}
	return videoURL
}

func newVideoJobID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "vid_job_" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return "vid_job_" + hex.EncodeToString(buf)
}

// handleCancelVideoGeneration marks job cancelled (best-effort) and triggers context cancel if running.
func (h *Handler) handleCancelVideoGeneration(w http.ResponseWriter, r *http.Request) {
	job, ok := h.loadVideoJobForRequest(w, r)
	if !ok {
		return
	}

	if job.Status == videoJobStatusCompleted || job.Status == videoJobStatusFailed ||
		job.Status == videoJobStatusTimeout || job.Status == videoJobStatusCancelled {
		httpapi.WriteError(w, http.StatusConflict, "invalid_request_error", "video_job_not_cancellable",
			"job is already in terminal state: "+job.Status)
		return
	}

	// Trigger cancel
	h.cancelJob(job.JobID)

	// Update DB immediately
	now := time.Now().UTC()
	job.Status = videoJobStatusCancelled
	job.ErrorCode = "cancelled"
	job.ErrorMessage = "cancelled by user"
	job.UpdatedAt = now
	if err := h.VideoJobs.Save(r.Context(), job); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "server_error", "video_job_cancel_failed", err.Error())
		return
	}

	flow.LogVideoStage(r.Context(), "user_cancelled", "job_id", job.JobID)

	httpapi.WriteJSON(w, http.StatusOK, toVideoGenerationResponse(r, job))
}
