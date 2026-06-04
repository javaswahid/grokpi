package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/javaswahid/grokpi/internal/httpapi"
)

const (
	defaultTTSUpstreamURL = "https://api.x.ai/v1/tts"
	defaultTTSLanguage    = "auto"
	defaultTTSVoice       = "eve"
	defaultTTSFormat      = "mp3"
)

type speechRequest struct {
	Model                    string          `json:"model,omitempty"`
	Voice                    string          `json:"voice,omitempty"`
	Input                    string          `json:"input,omitempty"`
	ResponseFormat           string          `json:"response_format,omitempty"`
	Text                     string          `json:"text,omitempty"`
	VoiceID                  string          `json:"voice_id,omitempty"`
	Language                 string          `json:"language,omitempty"`
	OutputFormat             json.RawMessage `json:"output_format,omitempty"`
	OptimizeStreamingLatency *int            `json:"optimize_streaming_latency,omitempty"`
	TextNormalization        *bool           `json:"text_normalization,omitempty"`
}

type xaiTTSRequest struct {
	Text                     string          `json:"text"`
	VoiceID                  string          `json:"voice_id"`
	Language                 string          `json:"language"`
	OutputFormat             json.RawMessage `json:"output_format,omitempty"`
	OptimizeStreamingLatency *int            `json:"optimize_streaming_latency,omitempty"`
	TextNormalization        *bool           `json:"text_normalization,omitempty"`
}

func (h *Handler) handleSpeech(w http.ResponseWriter, r *http.Request) {
	h.handleTTS(w, r, true)
}

func (h *Handler) handleNativeTTS(w http.ResponseWriter, r *http.Request) {
	h.handleTTS(w, r, false)
}

func (h *Handler) handleTTS(w http.ResponseWriter, r *http.Request, openAICompat bool) {
	var req speechRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "Invalid JSON in request body")
		return
	}
	if openAICompat && strings.TrimSpace(req.Model) != "" && !httpapi.CheckModelWhitelist(r.Context(), req.Model) {
		httpapi.WriteJSON(w, http.StatusForbidden, httpapi.NewAPIError(http.StatusForbidden, "forbidden", "model_not_allowed", "Model not allowed for this API key: "+req.Model))
		return
	}
	upstreamKey := ttsAPIKey()
	if upstreamKey == "" {
		httpapi.WriteError(w, http.StatusNotImplemented, "server_error", "tts_upstream_not_configured", "TTS upstream API key is not configured on the server")
		return
	}
	upstreamReq, contentType, ok := buildTTSUpstreamRequest(req)
	if !ok {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "missing_input", "TTS input text is required")
		return
	}
	payload, err := json.Marshal(upstreamReq)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_tts_request", "Invalid TTS request")
		return
	}

	httpReqBase, err := http.NewRequestWithContext(r.Context(), http.MethodPost, ttsUpstreamURL(), bytes.NewReader(payload))
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "server_error", "tts_request_failed", "Failed to build TTS upstream request")
		return
	}
	httpReqBase.Header.Set("Authorization", "Bearer "+upstreamKey)
	httpReqBase.Header.Set("Content-Type", "application/json")

	// Retry + better error handling (PHASE 6)
	const maxAttempts = 2
	var lastErr error
	client := &http.Client{Timeout: 90 * time.Second} // per-attempt; long audio still allowed via streaming copy

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// small backoff + jitter
			backoff := time.Duration(300+attempt*200) * time.Millisecond
			select {
			case <-r.Context().Done():
				httpapi.WriteError(w, http.StatusRequestTimeout, "upstream_error", "tts_cancelled", "TTS request cancelled during retry")
				return
			case <-time.After(backoff):
			}
		}

		// Clone request body for retry (since Reader is consumed)
		reqForAttempt := httpReqBase.Clone(r.Context())
		if attempt > 0 {
			reqForAttempt.Body = io.NopCloser(bytes.NewReader(payload))
		}

		resp, err := client.Do(reqForAttempt)
		if err != nil {
			lastErr = err
			continue
		}

		func() {
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
				lastErr = fmt.Errorf("upstream status %d: %s", resp.StatusCode, sanitizeUpstreamMessage(string(body)))
				// do not return yet; allow retry on 5xx/429
				if resp.StatusCode >= 500 || resp.StatusCode == 429 {
					return
				}
				// client error, no retry
				httpapi.WriteError(w, resp.StatusCode, "upstream_error", "tts_upstream_failed", sanitizeUpstreamMessage(string(body)))
				lastErr = nil // handled
				return
			}

			// Success path
			if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
				w.Header().Set("Content-Type", upstreamType)
			} else {
				w.Header().Set("Content-Type", contentType)
			}
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(w, resp.Body)
			lastErr = nil
		}()

		if lastErr == nil {
			// logged success with attempt count for metrics
			// (real metrics would use prometheus or logging hook)
			return
		}
	}

	if lastErr != nil {
		httpapi.WriteError(w, http.StatusBadGateway, "upstream_error", "tts_upstream_failed", "TTS upstream request failed after retries: "+lastErr.Error())
	}
	// Note: on success, headers/body are written inside the retry loop's success handler and we return early.
}

func buildTTSUpstreamRequest(req speechRequest) (xaiTTSRequest, string, bool) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		text = strings.TrimSpace(req.Input)
	}
	if text == "" {
		return xaiTTSRequest{}, "", false
	}
	voiceID := strings.TrimSpace(req.VoiceID)
	if voiceID == "" {
		voiceID = mapSpeechVoice(req.Voice)
	}
	language := strings.TrimSpace(req.Language)
	if language == "" {
		language = envOrDefault("GROKPI_TTS_DEFAULT_LANGUAGE", defaultTTSLanguage)
	}
	format := strings.ToLower(strings.TrimSpace(req.ResponseFormat))
	if format == "" {
		format = defaultTTSFormat
	}
	outputFormat := req.OutputFormat
	if len(outputFormat) == 0 && format != "" {
		outputFormat, _ = json.Marshal(map[string]any{"codec": format})
	}
	return xaiTTSRequest{
		Text:                     text,
		VoiceID:                  voiceID,
		Language:                 language,
		OutputFormat:             outputFormat,
		OptimizeStreamingLatency: req.OptimizeStreamingLatency,
		TextNormalization:        req.TextNormalization,
	}, contentTypeForAudioFormat(format), true
}

func mapSpeechVoice(voice string) string {
	normalized := strings.ToLower(strings.TrimSpace(voice))
	switch normalized {
	case "ara", "eve", "leo", "rex", "sal":
		return normalized
	case "narrator":
		return envOrDefault("GROKPI_TTS_NARRATOR_VOICE", "ara")
	case "":
		return envOrDefault("GROKPI_TTS_DEFAULT_VOICE", defaultTTSVoice)
	default:
		return normalized
	}
}

func contentTypeForAudioFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/L16"
	case "mulaw", "ulaw":
		return "audio/basic"
	case "alaw":
		return "audio/basic"
	default:
		return "audio/mpeg"
	}
}

func ttsAPIKey() string {
	for _, name := range []string{"GROKPI_TTS_API_KEY", "XAI_API_KEY", "GROKPI_XAI_API_KEY"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func ttsUpstreamURL() string {
	return envOrDefault("GROKPI_TTS_UPSTREAM_URL", defaultTTSUpstreamURL)
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func sanitizeUpstreamMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "TTS upstream returned an error"
	}
	return message
}
