package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"feelings-studio/internal/provider"
)

// ElevenLabsConfig ElevenLabs 海外情感语音合成适配器。
type ElevenLabsConfig struct {
	BaseURL string
	APIKey  string
	ModelID string // 如 eleven_multilingual_v2
	VoiceID string
	Timeout time.Duration
}

type ElevenLabsProvider struct {
	cfg    ElevenLabsConfig
	client *http.Client
}

func NewElevenLabs(cfg ElevenLabsConfig) *ElevenLabsProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.elevenlabs.io/v1"
	}
	if cfg.ModelID == "" {
		cfg.ModelID = "eleven_multilingual_v2"
	}
	if cfg.VoiceID == "" {
		cfg.VoiceID = "pNInz6obpgDQGcFmaJgB"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 45 * time.Second
	}
	return &ElevenLabsProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (e *ElevenLabsProvider) Name() string { return "elevenlabs:" + e.cfg.ModelID }

func (e *ElevenLabsProvider) Synthesize(ctx context.Context, req provider.TTSRequest) (*provider.TTSResult, error) {
	if strings.TrimSpace(e.cfg.APIKey) == "" {
		return nil, &provider.ProviderError{Provider: e.Name(), Code: "MISSING_API_KEY", Message: "ElevenLabs API key is required", Retryable: false}
	}
	voiceID := e.cfg.VoiceID
	if len(req.VoiceID) >= 16 && !strings.Contains(req.VoiceID, "-") {
		voiceID = req.VoiceID
	}
	speed := req.Speed
	if speed <= 0 {
		speed = 0.92
	}
	payload := map[string]any{
		"text":     req.Text,
		"model_id": e.cfg.ModelID,
		"voice_settings": map[string]any{
			"stability":        0.55,
			"similarity_boost": 0.78,
			"style":            0.25,
			"speed":            speed,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/text-to-speech/%s", strings.TrimRight(e.cfg.BaseURL, "/"), voiceID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("xi-api-key", e.cfg.APIKey)
	httpReq.Header.Set("Accept", "audio/mpeg")

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: e.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, &provider.ProviderError{Provider: e.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &provider.ProviderError{Provider: e.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: false}
	}

	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(req.OutputPath, raw, 0o644); err != nil {
		return nil, err
	}
	dur := probeDurationOrEstimate(ctx, req.OutputPath, req.Text, speed)
	return &provider.TTSResult{
		AudioPath: req.OutputPath,
		Duration:  dur,
		Cues:      alignCues(req.Text, dur),
	}, nil
}
