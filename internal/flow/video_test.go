package flow

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/javaswahid/grokpi/internal/store"
	"github.com/javaswahid/grokpi/internal/xai"
)

// mockVideoClient simulates xai video API calls.
type mockVideoClient struct {
	mu          sync.Mutex
	chatErr     error
	chatDelay   time.Duration
	videoURL    string
	pollCalls   int
	lastChatReq *xai.ChatRequest
}

func (m *mockVideoClient) Chat(ctx context.Context, req *xai.ChatRequest) (<-chan xai.StreamEvent, error) {
	m.mu.Lock()
	chatErr := m.chatErr
	chatDelay := m.chatDelay
	videoURL := m.videoURL
	m.lastChatReq = req
	m.mu.Unlock()
	if chatErr != nil {
		return nil, chatErr
	}

	if chatDelay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(chatDelay):
		}
	}

	// Simulate a chat response containing video URL
	payload := []byte(`{"result":{"response":{"post":{"id":"post-1"},"streamingVideoGenerationResponse":{"videoUrl":"` + videoURL + `"}}}}`)
	ch := make(chan xai.StreamEvent, 1)
	ch <- xai.StreamEvent{Data: payload}
	close(ch)
	return ch, nil
}

func (m *mockVideoClient) CreateImagePost(ctx context.Context, imageURL string) (string, error) {
	return "post-1", nil
}

func (m *mockVideoClient) CreateVideoPost(ctx context.Context, prompt string) (string, error) {
	return "post-1", nil
}

func (m *mockVideoClient) PollUpscale(ctx context.Context, jobID string, interval time.Duration) (string, error) {
	m.mu.Lock()
	m.pollCalls++
	videoURL := m.videoURL
	m.mu.Unlock()
	return videoURL, nil
}

func (m *mockVideoClient) DownloadURL(ctx context.Context, url string) ([]byte, error) {
	return nil, nil
}

func (m *mockVideoClient) DownloadTo(ctx context.Context, url string, w io.Writer) error {
	_, err := io.WriteString(w, "video-data")
	return err
}

func (m *mockVideoClient) UploadFile(ctx context.Context, fileName, fileMimeType, contentBase64 string) (string, string, error) {
	return "file-1", "generated/ref-image", nil
}

func TestVideoFlow_GenerateSync_Success(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/video.mp4",
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	url, err := vf.GenerateSync(context.Background(), &VideoRequest{
		Prompt: "A sunset over mountains",
		Model:  "grok-imagine-1.0-video",
		Size:   "1280x720",
	})
	if err != nil {
		t.Fatalf("GenerateSync() error = %v", err)
	}
	if url == "" {
		t.Error("GenerateSync() returned empty URL")
	}
}

func TestVideoFlow_GenerateSync_ChatError(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		chatErr: errors.New("connection refused"),
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	_, err := vf.GenerateSync(context.Background(), &VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-1.0-video",
	})
	if err == nil {
		t.Fatal("GenerateSync() expected error")
	}
}

func TestVideoFlow_GenerateSync_PresetAppended(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "ssoBasic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/generated/video.mp4",
	}

	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, &VideoFlowConfig{
		TimeoutSeconds:      5,
		PollIntervalSeconds: 1,
	})

	_, err := vf.GenerateSync(context.Background(), &VideoRequest{
		Prompt: "make it move",
		Model:  "grok-imagine-1.0-video",
		Size:   "1280x720",
		Preset: "normal",
	})
	if err != nil {
		t.Fatalf("GenerateSync() error = %v", err)
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.lastChatReq == nil {
		t.Fatal("expected captured chat request")
	}
	got := client.lastChatReq.Messages[0].Content
	if got != "make it move --mode=normal" {
		t.Errorf("chat prompt = %q, want %q", got, "make it move --mode=normal")
	}
}

func TestVideoFlow_GenerateSync_AspectRatioDirect(t *testing.T) {
	tests := []struct {
		name        string
		aspectRatio string
		size        string
		wantRatio   string
	}{
		{"explicit 3:2", "3:2", "720x480", "3:2"},
		{"explicit 16:9", "16:9", "853x480", "16:9"},
		{"explicit 1:1", "1:1", "480x480", "1:1"},
		{"explicit 9:16", "9:16", "270x480", "9:16"},
		{"fallback to ParseAspectRatio", "", "1280x720", "16:9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenSvc := &mockTokenService{
				tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
			}
			client := &mockVideoClient{
				videoURL: "https://example.com/video.mp4",
			}

			vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, &VideoFlowConfig{
				TimeoutSeconds: 5, PollIntervalSeconds: 1,
			})

			_, err := vf.GenerateSync(context.Background(), &VideoRequest{
				Prompt:      "test",
				Model:       "grok-imagine-1.0-video",
				Size:        tt.size,
				AspectRatio: tt.aspectRatio,
			})
			if err != nil {
				t.Fatalf("GenerateSync() error = %v", err)
			}

			client.mu.Lock()
			defer client.mu.Unlock()
			if client.lastChatReq == nil {
				t.Fatal("expected captured chat request")
			}

			// Extract aspectRatio from modelConfig
			mc, ok := client.lastChatReq.ModelConfig["modelMap"].(map[string]any)
			if !ok {
				t.Fatal("missing modelMap in ModelConfig")
			}
			vc, ok := mc["videoGenModelConfig"].(map[string]any)
			if !ok {
				t.Fatal("missing videoGenModelConfig")
			}
			gotRatio, _ := vc["aspectRatio"].(string)
			if gotRatio != tt.wantRatio {
				t.Errorf("aspectRatio = %q, want %q", gotRatio, tt.wantRatio)
			}
		})
	}
}

func TestVideoFlow_GenerateSync_Timeout(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		chatDelay: 10 * time.Second, // longer than timeout
	}

	cfg := &VideoFlowConfig{
		TimeoutSeconds:      1,
		PollIntervalSeconds: 1,
	}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	_, err := vf.GenerateSync(context.Background(), &VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-1.0-video",
	})
	if err == nil {
		t.Fatal("GenerateSync() expected timeout error")
	}
}

func TestVideoFlow_GenerateSync_NoConsumeOnFailure(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		chatErr: errors.New("generation failed"),
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	_, err := vf.GenerateSync(context.Background(), &VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-1.0-video",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	tokenSvc.mu.Lock()
	defer tokenSvc.mu.Unlock()
	if len(tokenSvc.consumeCalls) != 0 {
		t.Errorf("Consume should not be called on failure, got %d calls", len(tokenSvc.consumeCalls))
	}
}

func TestVideoFlow_ClassifiesPolicyBlock(t *testing.T) {
	err := classifyVideoGenerationError(errors.New("provider rejected request: policy violation unsafe prompt"))
	videoErr, ok := err.(*VideoError)
	if !ok {
		t.Fatalf("expected VideoError, got %T", err)
	}
	if videoErr.Code != "policy_violation" {
		t.Fatalf("code = %s, want policy_violation", videoErr.Code)
	}
	if videoErr.Status != 422 {
		t.Fatalf("status = %d, want 422", videoErr.Status)
	}
}

func TestVideoFlow_ReportTokenError_DoesNotPenalizeMediaFailures(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	vf := NewVideoFlow(tokenSvc, nil, &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1})

	cases := []error{
		&VideoError{Code: "invalid_video_response", Message: "missing final url"},
		&VideoError{Code: "timeout", Message: "provider timeout"},
		&VideoError{Code: "download_failed", Message: "cache save failed"},
		&VideoError{Code: "unknown_upstream_error", Message: "empty final media"},
	}
	for _, err := range cases {
		vf.reportTokenError(1, err)
	}

	tokenSvc.mu.Lock()
	defer tokenSvc.mu.Unlock()
	if len(tokenSvc.errorCalls) != 0 {
		t.Fatalf("media failures must not call ReportError, got %d", len(tokenSvc.errorCalls))
	}
	if len(tokenSvc.rateLimitCalls) != 0 {
		t.Fatalf("media failures must not call ReportRateLimit, got %d", len(tokenSvc.rateLimitCalls))
	}
	if len(tokenSvc.expiredCalls) != 0 {
		t.Fatalf("media failures must not call MarkExpired, got %d", len(tokenSvc.expiredCalls))
	}
}

func TestVideoFlow_ReportTokenError_KeepsPolicyAndCooldownTokenDecisions(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	vf := NewVideoFlow(tokenSvc, nil, &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1})

	vf.reportTokenError(1, &VideoError{Code: "policy_violation", Message: "policy violation unsafe prompt"})
	vf.reportTokenError(1, &VideoError{Code: "video_cooldown", Message: "cooldown"})
	vf.reportTokenError(1, &VideoError{Code: "token_expired", Message: "401"})

	tokenSvc.mu.Lock()
	defer tokenSvc.mu.Unlock()
	if len(tokenSvc.errorCalls) != 1 {
		t.Fatalf("policy violation should call ReportError once, got %d", len(tokenSvc.errorCalls))
	}
	if len(tokenSvc.rateLimitCalls) != 1 {
		t.Fatalf("cooldown should call ReportRateLimit once, got %d", len(tokenSvc.rateLimitCalls))
	}
	if len(tokenSvc.expiredCalls) != 1 {
		t.Fatalf("expired token should call MarkExpired once, got %d", len(tokenSvc.expiredCalls))
	}
}

func TestVideoFlow_GenerateSync_ConsumeOnSuccess(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockVideoClient{
		videoURL: "https://example.com/video.mp4",
	}

	cfg := &VideoFlowConfig{TimeoutSeconds: 5, PollIntervalSeconds: 1}
	vf := NewVideoFlow(tokenSvc, func(token string) VideoClient { return client }, cfg)

	_, err := vf.GenerateSync(context.Background(), &VideoRequest{
		Prompt: "Test",
		Model:  "grok-imagine-1.0-video",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tokenSvc.mu.Lock()
	defer tokenSvc.mu.Unlock()
	if len(tokenSvc.consumeCalls) != 1 {
		t.Errorf("Consume should be called once on success, got %d calls", len(tokenSvc.consumeCalls))
	}
}
