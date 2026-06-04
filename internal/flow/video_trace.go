package flow

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

type videoTraceContextKey string

const videoTraceKey videoTraceContextKey = "videoTrace"

// VideoTrace captures provider-side milestones for one video request.
type VideoTrace struct {
	mu                sync.Mutex
	GrokPiRequestID   string
	ClientRequestID   string
	ProviderEventID   string
	ParentPostID      string
	VideoPostID       string
	UpstreamRequestID string
	FinalVideoURL     string
}

// WithVideoTrace attaches a mutable trace collector to a context.
func WithVideoTrace(ctx context.Context, trace *VideoTrace) context.Context {
	return context.WithValue(ctx, videoTraceKey, trace)
}

// VideoTraceFromContext returns the trace collector, if any.
func VideoTraceFromContext(ctx context.Context) *VideoTrace {
	trace, _ := ctx.Value(videoTraceKey).(*VideoTrace)
	return trace
}

func (t *VideoTrace) setParentPostID(value string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ParentPostID = strings.TrimSpace(value)
}

func (t *VideoTrace) setVideoPostID(value string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.VideoPostID = strings.TrimSpace(value)
}

func (t *VideoTrace) setFinalVideoURL(value string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.FinalVideoURL = strings.TrimSpace(value)
}

func (t *VideoTrace) snapshot() VideoTrace {
	t.mu.Lock()
	defer t.mu.Unlock()
	return VideoTrace{
		GrokPiRequestID:   t.GrokPiRequestID,
		ClientRequestID:   t.ClientRequestID,
		ProviderEventID:   t.ProviderEventID,
		ParentPostID:      t.ParentPostID,
		VideoPostID:       t.VideoPostID,
		UpstreamRequestID: t.UpstreamRequestID,
		FinalVideoURL:     t.FinalVideoURL,
	}
}

// Snapshot returns a safe copy of the current trace values.
func (t *VideoTrace) Snapshot() VideoTrace {
	if t == nil {
		return VideoTrace{}
	}
	return t.snapshot()
}

// LogVideoStage writes one structured lifecycle event for gateway diagnostics.
func LogVideoStage(ctx context.Context, stage string, attrs ...any) {
	trace := VideoTraceFromContext(ctx)
	if trace == nil {
		slog.Info("video: stage", append([]any{"stage", stage}, attrs...)...)
		return
	}
	s := trace.snapshot()
	base := []any{
		"stage", stage,
		"grokpi_request_id", s.GrokPiRequestID,
		"client_request_id", s.ClientRequestID,
		"provider_event_id", s.ProviderEventID,
		"parent_post_id", s.ParentPostID,
		"video_post_id", s.VideoPostID,
	}
	slog.Info("video: stage", append(base, attrs...)...)
}
