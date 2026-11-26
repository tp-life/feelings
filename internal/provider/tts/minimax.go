package tts

import (
	"bytes"
	"context"
	"encoding/hex"
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

// MiniMaxConfig MiniMax T2A v2 情感配音适配器配置。
type MiniMaxConfig struct {
	BaseURL string
	GroupID string
	APIKey  string
	Model   string
	VoiceID string
	Timeout time.Duration
}

type MiniMaxProvider struct {
	cfg    MiniMaxConfig
	client *http.Client
}

func NewMiniMax(cfg MiniMaxConfig) *MiniMaxProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.minimax.chat/v1/t2a_v2"
	}
	if cfg.Model == "" {
		cfg.Model = "speech-02-hd"
	}
	if cfg.VoiceID == "" {
		cfg.VoiceID = "male-qn-qingse"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 45 * time.Second
	}
	return &MiniMaxProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (m *MiniMaxProvider) Name() string { return "minimax-tts:" + m.cfg.Model }

func (m *MiniMaxProvider) Synthesize(ctx context.Context, req provider.TTSRequest) (*provider.TTSResult, error) {
	if m.cfg.APIKey == "" || m.cfg.GroupID == "" {
		return nil, &provider.ProviderError{Provider: m.Name(), Code: "MISSING_AUTH", Message: "MINIMAX_API_KEY and MINIMAX_GROUP_ID are required", Retryable: false}
	}
	speed := req.Speed
	if speed <= 0 {
		speed = 0.92
	}
	voice := m.cfg.VoiceID
	switch req.VoiceID {
	case "warm-male":
		voice = "male-qn-qingse"
	case "healing-female":
		voice = "female-shaonv"
	case "childhood-boy":
		voice = "male-qn-jingying"
	case "calm-senior":
		voice = "male-qn-badao"
	default:
		if req.VoiceID != "" && strings.Contains(req.VoiceID, "-") {
			voice = req.VoiceID
		}
	}
	payload := map[string]any{
		"model": m.cfg.Model,
		"text":  req.Text,
		"voice_setting": map[string]any{
			"voice_id": voice,
			"speed":    speed,
			"vol":      1.0,
			"pitch":    0,
		},
		"audio_setting": map[string]any{
			"sample_rate": 44100,
			"bitrate":     128000,
			"format":      "mp3",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s?GroupId=%s", strings.TrimRight(m.cfg.BaseURL, "/"), m.cfg.GroupID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: m.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, &provider.ProviderError{Provider: m.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}

	var res struct {
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
		Data struct {
			Audio string `json:"audio"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	if res.BaseResp.StatusCode != 0 || res.Data.Audio == "" {
		return nil, &provider.ProviderError{Provider: m.Name(), Code: fmt.Sprintf("CODE_%d", res.BaseResp.StatusCode), Message: res.BaseResp.StatusMsg, Retryable: false}
	}
	audioBytes, err := hex.DecodeString(res.Data.Audio)
	if err != nil {
		return nil, fmt.Errorf("decode minimax hex audio: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(req.OutputPath, audioBytes, 0o644); err != nil {
		return nil, err
	}
	dur := probeDurationOrEstimate(ctx, req.OutputPath, req.Text, speed)
	return &provider.TTSResult{
		AudioPath: req.OutputPath,
		Duration:  dur,
		Cues:      alignCues(req.Text, dur),
	}, nil
}
