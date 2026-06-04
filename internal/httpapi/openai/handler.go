package openai

import (
	"context"
	"sync"

	"github.com/crmmc/grokpi/internal/config"
	"github.com/crmmc/grokpi/internal/flow"
	"github.com/crmmc/grokpi/internal/store"
)

// Handler holds dependencies for OpenAI-compatible API endpoints.
type Handler struct {
	ChatFlow  *flow.ChatFlow
	VideoFlow *flow.VideoFlow
	ImageFlow *flow.ImageFlow
	VideoJobs *store.VideoJobStore
	Cfg       *config.Config
	Runtime   *config.Runtime

	// cancelMu + cancels provide best-effort cancellation for in-flight video goroutines.
	cancelMu sync.Mutex
	cancels  map[string]context.CancelFunc // jobID -> cancel func (best effort, single instance)
}

func (h *Handler) currentConfig() *config.Config {
	if h == nil {
		return nil
	}
	if h.Runtime != nil {
		return h.Runtime.Get()
	}
	return h.Cfg
}

// registerCancel stores a cancel func for a job (called from job runner).
func (h *Handler) registerCancel(jobID string, cancel context.CancelFunc) {
	if h == nil {
		return
	}
	h.cancelMu.Lock()
	if h.cancels == nil {
		h.cancels = make(map[string]context.CancelFunc)
	}
	h.cancels[jobID] = cancel
	h.cancelMu.Unlock()
}

// cancelJob calls the cancel func if present and removes it.
func (h *Handler) cancelJob(jobID string) {
	if h == nil {
		return
	}
	h.cancelMu.Lock()
	if h.cancels != nil {
		if cancel, ok := h.cancels[jobID]; ok {
			cancel()
			delete(h.cancels, jobID)
		}
	}
	h.cancelMu.Unlock()
}

// unregisterCancel cleans up on job end.
func (h *Handler) unregisterCancel(jobID string) {
	if h == nil {
		return
	}
	h.cancelMu.Lock()
	if h.cancels != nil {
		delete(h.cancels, jobID)
	}
	h.cancelMu.Unlock()
}
