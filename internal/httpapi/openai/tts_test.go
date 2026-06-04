package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestHandleSpeechProxiesOpenAIShapeToXAITTS(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/tts", r.URL.Path)
		require.Equal(t, "Bearer test-upstream-key", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "Ini adalah uji narasi MASJAVAS.", body["text"])
		require.Equal(t, "ara", body["voice_id"])
		require.Equal(t, "id", body["language"])
		outputFormat := body["output_format"].(map[string]any)
		require.Equal(t, "mp3", outputFormat["codec"])
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("fake-mp3-bytes"))
	}))
	defer upstream.Close()
	t.Setenv("GROKPI_TTS_API_KEY", "test-upstream-key")
	t.Setenv("GROKPI_TTS_UPSTREAM_URL", upstream.URL+"/v1/tts")

	router := chi.NewRouter()
	(&Handler{}).SetupRoutes(router)
	req := httptest.NewRequest(http.MethodPost, "/audio/speech", strings.NewReader(`{
		"model": "grok-tts",
		"voice": "narrator",
		"input": "Ini adalah uji narasi MASJAVAS.",
		"language": "id",
		"response_format": "mp3"
	}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "audio/mpeg", rec.Header().Get("Content-Type"))
	require.Equal(t, "fake-mp3-bytes", rec.Body.String())
}

func TestHandleSpeechRequiresServerSideUpstreamKey(t *testing.T) {
	t.Setenv("GROKPI_TTS_API_KEY", "")
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROKPI_XAI_API_KEY", "")

	router := chi.NewRouter()
	(&Handler{}).SetupRoutes(router)
	req := httptest.NewRequest(http.MethodPost, "/audio/speech", strings.NewReader(`{"model":"grok-tts","input":"hello"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Contains(t, rec.Body.String(), "tts_upstream_not_configured")
}
