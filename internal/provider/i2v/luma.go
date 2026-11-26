package i2v

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"feelings-studio/internal/provider"
)

// LumaConfig Luma Dream Machine (Ray 2) 异步视频生成适配器。
type LumaConfig struct {
	BaseURL string
	APIKey  string
	Model   string // 如 ray-2 / ray-flash-2
	Timeout time.Duration
}

type LumaProvider struct {
	cfg    LumaConfig
	client *http.Client
}

func NewLuma(cfg LumaConfig) *LumaProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.lumalabs.ai/dream-machine/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "ray-2"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &LumaProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (l *LumaProvider) Name() string { return "luma:" + l.cfg.Model }

func (l *LumaProvider) Submit(ctx context.Context, req provider.I2VSubmitRequest) (string, error) {
	if strings.TrimSpace(l.cfg.APIKey) == "" {
		return "", &provider.ProviderError{Provider: l.Name(), Code: "MISSING_API_KEY", Message: "Luma API key is required", Retryable: false}
	}
	aspect := req.AspectRatio
	if aspect == "" {
		aspect = "9:16"
	}
	dur := "5s"
	if req.DurationSec >= 8 {
		dur = "9s"
	}
	payload := map[string]any{
		"model":        l.cfg.Model,
		"prompt":       req.Prompt,
		"aspect_ratio": aspect,
		"duration":     dur,
	}
	// 若首帧为公网 URL 则直接注入 keyframes；本地文件则提示需公网图床或优先使用支持 Base64 的模型
	if strings.HasPrefix(req.FirstFramePath, "http://") || strings.HasPrefix(req.FirstFramePath, "https://") {
		keyframes := map[string]any{
			"frame0": map[string]string{"type": "image", "url": req.FirstFramePath},
		}
		if strings.HasPrefix(req.LastFramePath, "http://") || strings.HasPrefix(req.LastFramePath, "https://") {
			keyframes["frame1"] = map[string]string{"type": "image", "url": req.LastFramePath}
		}
		payload["keyframes"] = keyframes
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(l.cfg.BaseURL, "/") + "/generations"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+l.cfg.APIKey)

	resp, err := l.client.Do(httpReq)
	if err != nil {
		return "", &provider.ProviderError{Provider: l.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", &provider.ProviderError{Provider: l.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", &provider.ProviderError{Provider: l.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: false}
	}

	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.ID == "" {
		return "", &provider.ProviderError{Provider: l.Name(), Code: "EMPTY_ID", Message: string(raw), Retryable: false}
	}
	return res.ID, nil
}

func (l *LumaProvider) Poll(ctx context.Context, remoteID string) (*provider.I2VStatus, error) {
	endpoint := strings.TrimRight(l.cfg.BaseURL, "/") + "/generations/" + remoteID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+l.cfg.APIKey)

	resp, err := l.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: l.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, &provider.ProviderError{Provider: l.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}

	var res struct {
		State         string `json:"state"`
		FailureReason string `json:"failure_reason"`
		Assets        struct {
			Video string `json:"video"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	st := &provider.I2VStatus{RemoteID: remoteID}
	switch strings.ToLower(res.State) {
	case "queued":
		st.State = provider.TaskQueued
	case "dreaming":
		st.State = provider.TaskRunning
	case "completed":
		st.State = provider.TaskSucceeded
		st.VideoURL = res.Assets.Video
	case "failed":
		st.State = provider.TaskFailed
		st.ErrorMsg = res.FailureReason
	default:
		st.State = provider.TaskRunning
	}
	return st, nil
}
