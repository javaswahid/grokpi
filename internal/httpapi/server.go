// Package httpapi provides HTTP routing and handlers.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/javaswahid/grokpi/internal/cache"
	"github.com/javaswahid/grokpi/internal/config"
	"github.com/javaswahid/grokpi/internal/store"
)

// ChatProvider abstracts downstream protocol handling for API routes.
type ChatProvider interface {
	SetupRoutes(r chi.Router)
}

// AudioProvider optionally exposes OpenAI-compatible audio routes outside /v1.
type AudioProvider interface {
	SetupAudioRoutes(r chi.Router)
}

// ModelProvider optionally exposes OpenAI-compatible model routes outside /v1.
type ModelProvider interface {
	SetupModelRoutes(r chi.Router)
}

type noopChatProvider struct{}

func (noopChatProvider) SetupRoutes(r chi.Router) {
	r.Get("/models", notImplementedHandler)
	r.Post("/chat/completions", notImplementedHandler)
}

func notImplementedHandler(w http.ResponseWriter, _ *http.Request) {
	WriteError(w, http.StatusNotImplemented, "server_error", "not_implemented",
		"Chat provider not configured")
}

// Server holds HTTP server dependencies.
type Server struct {
	router          chi.Router
	startTime       time.Time
	chatProvider    ChatProvider
	appKey          string
	version         string
	cfg             *config.Config
	runtime         *config.Runtime
	tokenStore      TokenStoreInterface
	tokenRefresher  TokenRefresher
	tokenPoolSyncer TokenPoolSyncer
	videoTester     TokenVideoCapabilityTester
	usageLogStore   UsageLogStoreInterface
	apiKeyStore     APIKeyStoreInterface
	cacheService    *cache.Service
	configStore     *store.ConfigStore
	dbPing          func(ctx context.Context) error // optional for real DB health in /ready
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	AppKey          string
	Version         string
	Config          *config.Config
	Runtime         *config.Runtime
	ChatProvider    ChatProvider
	TokenStore      TokenStoreInterface
	TokenRefresher  TokenRefresher
	TokenPoolSyncer TokenPoolSyncer
	VideoTester     TokenVideoCapabilityTester
	UsageLogStore   UsageLogStoreInterface
	APIKeyStore     APIKeyStoreInterface
	CacheService    *cache.Service
	ConfigStore     *store.ConfigStore
	DBPing          func(ctx context.Context) error
}

// NewServer creates a new HTTP server with configured routes.
func NewServer(cfg *ServerConfig) *Server {
	if cfg == nil {
		cfg = &ServerConfig{}
	}
	chatProvider := cfg.ChatProvider
	if chatProvider == nil {
		chatProvider = noopChatProvider{}
	}
	s := &Server{
		router:          chi.NewRouter(),
		startTime:       time.Now(),
		chatProvider:    chatProvider,
		appKey:          cfg.AppKey,
		version:         cfg.Version,
		cfg:             cfg.Config,
		runtime:         cfg.Runtime,
		tokenStore:      cfg.TokenStore,
		tokenRefresher:  cfg.TokenRefresher,
		tokenPoolSyncer: cfg.TokenPoolSyncer,
		dbPing:          cfg.DBPing,
		videoTester:     cfg.VideoTester,
		usageLogStore:   cfg.UsageLogStore,
		apiKeyStore:     cfg.APIKeyStore,
		cacheService:    cfg.CacheService,
		configStore:     cfg.ConfigStore,
	}
	s.setupMiddleware()
	s.setupRoutes()
	return s
}

// setupMiddleware configures global middleware.
func (s *Server) setupMiddleware() {
	s.router.Use(middleware.CleanPath)
	s.router.Use(securityHeaders)
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Recoverer)
	s.router.Use(debugRequestLogger)
	if s.runtime != nil {
		s.router.Use(requestTimeoutRuntimeMiddleware(s.runtime))
		s.router.Use(bodySizeLimitRuntimeMiddleware(s.runtime))
		return
	}
	s.router.Use(requestTimeoutMiddleware(s.cfg))
	s.router.Use(bodySizeLimitMiddleware(s.cfg))
}

// debugRequestLogger logs every incoming request at DEBUG level.
func debugRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		slog.Debug("http: incoming request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"content_length", r.ContentLength,
			"request_id", middleware.GetReqID(r.Context()))

		next.ServeHTTP(ww, r)

		slog.Debug("http: response sent",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"elapsed_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()))
	})
}

// setupRoutes configures all routes.
func (s *Server) setupRoutes() {
	// Health check endpoints (no auth) - Kubernetes / ops friendly
	s.router.Get("/health", s.handleHealth)
	s.router.Get("/healthz", s.handleHealth)
	s.router.Get("/live", s.handleLive)
	s.router.Get("/ready", s.handleReady)

	// Public cached file serving (video only — images are now base64-inlined)
	if s.cacheService != nil {
		s.router.Get("/api/files/video/{name}", handleServeCacheFileByType(s.cacheService, "video"))
		s.router.Get("/api/files/{type}/{name}", handleServeCacheFile(s.cacheService))
	}

	// API routes (with auth)
	s.router.Route("/v1", func(r chi.Router) {
		s.applyAPIAuth(r)
		if s.tokenStore != nil {
			r.Post("/preflight", handleV1Preflight(s.tokenStore, func() *config.TokenConfig {
				if s.runtime != nil {
					current := s.runtime.Get()
					if current == nil {
						return nil
					}
					return &current.Token
				}
				if s.cfg == nil {
					return nil
				}
				return &s.cfg.Token
			}))
		}
		if s.chatProvider != nil {
			s.chatProvider.SetupRoutes(r)
		}
	})

	// OpenAI-compatible root aliases used by capability probes.
	if modelProvider, ok := s.chatProvider.(ModelProvider); ok {
		s.router.Group(func(r chi.Router) {
			s.applyAPIAuth(r)
			modelProvider.SetupModelRoutes(r)
		})
	}
	if audioProvider, ok := s.chatProvider.(AudioProvider); ok {
		s.router.Group(func(r chi.Router) {
			s.applyAPIAuth(r)
			audioProvider.SetupAudioRoutes(r)
		})
	}

	// Admin API routes (with AppKey auth)
	s.router.Route("/admin", func(r chi.Router) {
		if s.runtime != nil {
			r.Use(AdminRateLimitRuntime(s.runtime))
		} else {
			r.Use(AdminRateLimit(s.cfg))
		}

		// Public (no AppKeyAuth) — login endpoint
		if s.runtime != nil {
			r.Post("/login", handleAdminLoginRuntime(s.runtime))
		} else {
			r.Post("/login", handleAdminLogin(s.appKey))
		}

		// Protected — all other admin routes
		r.Group(func(r chi.Router) {
			if s.runtime != nil {
				r.Use(AppKeyAuthRuntime(s.runtime))
			} else {
				r.Use(AppKeyAuth(s.appKey))
			}

			// Verify endpoint (auth check - middleware handles validation)
			r.Get("/verify", func(w http.ResponseWriter, r *http.Request) {
				WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			})

			// Logout endpoint
			r.Post("/logout", handleAdminLogout())

			// System status endpoint
			r.Get("/system/status", handleSystemStatus(s.tokenStore, s.apiKeyStore, s.startTime, s.version))

			// Config endpoints
			if s.runtime != nil {
				r.Get("/config", handleGetConfigRuntime(s.runtime))
				r.Put("/config", handlePutConfigRuntime(s.runtime, s.configStore))
			} else if s.cfg != nil {
				r.Get("/config", handleGetConfig(s.cfg))
				r.Put("/config", handlePutConfig(s.cfg, s.configStore))
			}

			// Token endpoints
			if s.tokenStore != nil {
				r.Get("/tokens", handleListTokens(s.tokenStore))
				r.Get("/tokens/health", handleTokenHealth(s.tokenStore))
				r.Get("/tokens/ids", handleListTokenIDs(s.tokenStore))
				r.Get("/tokens/{id}", handleGetToken(s.tokenStore))
				r.Put("/tokens/{id}", handleUpdateToken(s.tokenStore, s.tokenPoolSyncer))
				r.Delete("/tokens/{id}", handleDeleteToken(s.tokenStore, s.tokenPoolSyncer))
				r.Post("/tokens/{id}/replace", handleReplaceToken(s.tokenStore, s.tokenPoolSyncer))
				r.Post("/tokens/{id}/reset-usage", handleResetTokenUsage(s.tokenStore, s.tokenPoolSyncer))
				r.Post("/tokens/{id}/test-video-capability", handleTestVideoCapability(s.videoTester))
				if s.runtime != nil {
					r.Post("/tokens/batch", handleBatchTokensFromProvider(s.tokenStore, s.tokenPoolSyncer, func() *config.TokenConfig {
						current := s.runtime.Get()
						if current == nil {
							return nil
						}
						return &current.Token
					}))
				} else {
					r.Post("/tokens/batch", handleBatchTokens(s.tokenStore, s.tokenPoolSyncer, s.cfg))
				}

				if s.tokenRefresher != nil {
					r.Post("/tokens/{id}/sync-quota", handleRefreshToken(s.tokenRefresher))
					if revalidator, ok := s.tokenRefresher.(TokenRevalidator); ok {
						r.Post("/tokens/{id}/revalidate", handleRevalidateToken(revalidator))
					}
					if officialRefresher, ok := s.tokenRefresher.(TokenOfficialRefresher); ok {
						r.Post("/tokens/{id}/refresh", handleOfficialRefreshToken(officialRefresher))
					}
				}

				// Stats endpoints (token-based)
				r.Get("/stats/tokens", handleTokenStats(s.tokenStore))
				if s.runtime != nil {
					r.Get("/stats/quota", handleQuotaStatsFromProvider(s.tokenStore, func() *config.TokenConfig {
						current := s.runtime.Get()
						if current == nil {
							return nil
						}
						return &current.Token
					}))
				} else {
					var tokenCfg *config.TokenConfig
					if s.cfg != nil {
						tokenCfg = &s.cfg.Token
					}
					r.Get("/stats/quota", handleQuotaStats(s.tokenStore, tokenCfg))
				}
			}

			// Stats endpoints (usage-based)
			if s.usageLogStore != nil {
				r.Get("/stats/usage", handleUsageStats(s.usageLogStore))
			}

			// System usage endpoint (period-based aggregation for /usage page)
			if s.usageLogStore != nil {
				r.Get("/system/usage", handleSystemUsage(s.usageLogStore))
				r.Get("/usage/logs", handleUsageLogs(s.usageLogStore))
			}

			// API Key management endpoints
			if s.apiKeyStore != nil {
				r.Route("/apikeys", func(r chi.Router) {
					r.Get("/", handleListAPIKeys(s.apiKeyStore))
					r.Get("/stats", handleAPIKeyStats(s.apiKeyStore))
					r.Post("/", handleCreateAPIKey(s.apiKeyStore))
					r.Get("/{id}", handleGetAPIKey(s.apiKeyStore))
					r.Patch("/{id}", handleUpdateAPIKey(s.apiKeyStore))
					r.Delete("/{id}", handleDeleteAPIKey(s.apiKeyStore))
					r.Post("/{id}/regenerate", handleRegenerateAPIKey(s.apiKeyStore))
				})
			}

			// Cache management endpoints
			if s.cacheService != nil {
				r.Get("/cache/stats", handleCacheStats(s.cacheService))
				r.Get("/cache/files", handleCacheFiles(s.cacheService))
				r.Post("/cache/delete", handleDeleteCacheFiles(s.cacheService))
				r.Post("/cache/clear", handleClearCache(s.cacheService))
				r.Get("/cache/files/{type}/{name}", handleServeCacheFile(s.cacheService))
			}
		})
	})

	// Frontend SPA (catch-all, must be last)
	s.router.Handle("/*", SPAHandler())
}

// applyAPIAuth adds authentication middleware to an API route group.
func (s *Server) applyAPIAuth(r chi.Router) {
	if s.apiKeyStore != nil {
		r.Use(APIKeyAuth(s.apiKeyStore))
	}
}

// Router returns the chi router for http.Server.
func (s *Server) Router() http.Handler {
	return s.router
}

// HealthResponse is the health check response (detailed).
type HealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version,omitempty"`
	Uptime    string `json:"uptime"`
	Timestamp string `json:"timestamp"`
	Database  string `json:"database,omitempty"`
	Queue     string `json:"queue,omitempty"`
	Storage   string `json:"storage,omitempty"`
	TTS       string `json:"tts,omitempty"`
	Video     string `json:"video,omitempty"`
	Details   string `json:"details,omitempty"`
}

// LiveResponse for /live (liveness probe - is process alive?)
type LiveResponse struct {
	Status    string `json:"status"`
	Uptime    string `json:"uptime"`
	Timestamp string `json:"timestamp"`
}

// ReadyResponse for /ready (readiness probe - can serve traffic?)
type ReadyResponse struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Checks    map[string]string `json:"checks"`
}

// handleHealth returns detailed server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	uptime := time.Since(s.startTime).Round(time.Second).String()

	resp := HealthResponse{
		Status:    "healthy",
		Version:   s.version,
		Uptime:    uptime,
		Timestamp: now.Format(time.RFC3339),
		Database:  "ok",
		Queue:     "ok",
		Storage:   "ok",
		TTS:       "ok",
		Video:     "ok",
	}

	// Perform lightweight dependency checks if providers available
	if s.cfg != nil {
		if !s.cfg.App.MediaGenerationEnabled {
			resp.TTS = "disabled"
			resp.Video = "disabled"
		}
	}
	if s.chatProvider == nil {
		resp.Status = "degraded"
		resp.Details = "chat provider not configured"
	}

	WriteJSON(w, http.StatusOK, resp)
}

// handleLive is Kubernetes-style liveness (process is up, no deadlock assumed).
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	resp := LiveResponse{
		Status:    "alive",
		Uptime:    time.Since(s.startTime).Round(time.Second).String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	WriteJSON(w, http.StatusOK, resp)
}

// handleReady is readiness: critical deps must be ready (db, basic services).
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	checks := map[string]string{
		"database": "ok",
		"config":   "ok",
		"token":    "ok",
		"video":    "ok",
	}

	status := "ready"
	if s.tokenStore == nil {
		checks["token"] = "not_configured"
		status = "not_ready"
	}

	// Real DB ping with short timeout if provided (wired from main)
	if s.dbPing != nil {
		pingCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := s.dbPing(pingCtx); err != nil {
			checks["database"] = "error: " + err.Error()
			status = "not_ready"
		}
	}

	// Video flow readiness (if handler not wired, mark degraded but don't fail ready for API keys only)
	if s.chatProvider == nil {
		checks["video"] = "degraded"
	}

	resp := ReadyResponse{
		Status:    status,
		Timestamp: now.Format(time.RFC3339),
		Checks:    checks,
	}
	code := http.StatusOK
	if status != "ready" {
		code = http.StatusServiceUnavailable
	}
	WriteJSON(w, code, resp)
}

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
