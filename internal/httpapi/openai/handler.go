package openai

import (
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
