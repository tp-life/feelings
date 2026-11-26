package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"feelings-studio/internal/provider"
)

// DoubaoConfig 豆包/火山引擎情感语音合成适配器配置。
type DoubaoConfig struct {
	Endpoint  string
	AppID     string
	Token     string
	Cluster   string
	VoiceType string
	Timeout   time.Duration
}

type DoubaoProvider struct {
	cfg    DoubaoConfig
	client *http.Client
}

func NewDoubao(cfg DoubaoConfig) *DoubaoProvider {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://openspeech.bytedance.com/api/v1/tts"
	}
	if cfg.Cluster == "" {
		cfg.Cluster = "volcano_tts"
	}
	if cfg.VoiceType == "" {
		cfg.VoiceType = "zh_male_wenrouxiaoge_mars_bigtts"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 45 * time.Second
	}
	return &DoubaoProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (d *DoubaoProvider) Name() string { return "doubao-tts" }

func (d *DoubaoProvider) Synthesize(ctx context.Context, req provider.TTSRequest) (*provider.TTSResult, error) {
	if d.cfg.AppID == "" || d.cfg.Token == "" {
		return nil, &provider.ProviderError{Provider: d.Name(), Code: "MISSING_AUTH", Message: "DOUBAO_TTS_APP_ID and DOUBAO_TTS_TOKEN are required", Retryable: false}
	}
	voice := d.cfg.VoiceType
	switch req.VoiceID {
	case "warm-male":
		voice = "zh_male_wenrouxiaoge_mars_bigtts"
	case "healing-female":
		voice = "zh_female_cancan_mars_bigtts"
	case "childhood-boy":
		voice = "zh_male_shaonianzixin_moon_bigtts"
	case "calm-senior":
		voice = "zh_male_chunhou_mars_bigtts"
	default:
		if req.VoiceID != "" && strings.HasPrefix(req.VoiceID, "zh_") {
			voice = req.VoiceID
		}
	}
	speed := req.Speed
	if speed <= 0 {
		speed = 0.92
	}
	payload := map[string]any{
		"app": map[string]string{
			"appid":   d.cfg.AppID,
			"token":   d.cfg.Token,
			"cluster": d.cfg.Cluster,
		},
		"user": map[string]string{"uid": "feelings-studio"},
		"audio": map[string]any{
			"voice_type":  voice,
			"encoding":    "mp3",
			"speed_ratio": speed,
			"emotion":     req.Emotion,
		},
		"request": map[string]any{
			"reqid":     fmt.Sprintf("feelings-%d", time.Now().UnixNano()),
			"text":      req.Text,
			"operation": "query",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer;"+d.cfg.Token)

	resp, err := d.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: d.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, &provider.ProviderError{Provider: d.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}

	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	if res.Code != 3000 || res.Data == "" {
		return nil, &provider.ProviderError{Provider: d.Name(), Code: fmt.Sprintf("CODE_%d", res.Code), Message: res.Message, Retryable: false}
	}
	audioBytes, err := base64.StdEncoding.DecodeString(res.Data)
	if err != nil {
		return nil, err
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

func probeDurationOrEstimate(ctx context.Context, path, text string, speed float64) time.Duration {
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path,
	).Output()
	if err == nil {
		if sec, parseErr := strconv.ParseFloat(strings.TrimSpace(string(out)), 64); parseErr == nil && sec > 0 {
			return time.Duration(sec * float64(time.Second))
		}
	}
	if speed <= 0 {
		speed = 0.92
	}
	est := float64(utf8.RuneCountInString(text)) / (3.1 * speed)
	if est < 1.5 {
		est = 1.5
	}
	return time.Duration(est * float64(time.Second))
}

func alignCues(text string, total time.Duration) []provider.SubtitleCue {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return []provider.SubtitleCue{{Text: text, Start: 0, End: total}}
}
