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

// RunwayConfig Runway Gen-3 / Gen-4 异步图生视频适配器。
type RunwayConfig struct {
	BaseURL string
	APIKey  string
	Model   string // 如 gen4_turbo / gen3a_turbo
	Timeout time.Duration
}

type RunwayProvider struct {
	cfg    RunwayConfig
	client *http.Client
}

func NewRunway(cfg RunwayConfig) *RunwayProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.dev.runwayml.com"
	}
	if cfg.Model == "" {
		cfg.Model = "gen4_turbo"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &RunwayProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (r *RunwayProvider) Name() string { return "runway:" + r.cfg.Model }

func (r *RunwayProvider) Submit(ctx context.Context, req provider.I2VSubmitRequest) (string, error) {
	if strings.TrimSpace(r.cfg.APIKey) == "" {
		return "", &provider.ProviderError{Provider: r.Name(), Code: "MISSING_API_KEY", Message: "Runway API key is required", Retryable: false}
	}
	imgSource := strings.TrimSpace(req.FirstFramePath)
	if imgSource == "" && len(req.ReferenceImages) > 0 {
		imgSource = req.ReferenceImages[0]
	}
	if imgSource == "" {
		return "", &provider.ProviderError{Provider: r.Name(), Code: "MISSING_FRAME", Message: "Runway requires a first frame or bound asset reference image", Retryable: false}
	}
	firstB64, err := encodeFileBase64(imgSource)
	if err != nil {
		return "", err
	}
	dur := 5
	if req.DurationSec >= 8 {
		dur = 10
	}
	ratio := "720:1280"
	switch req.AspectRatio {
	case "16:9":
		ratio = "1280:720"
	case "3:4":
		ratio = "832:1104"
	}
	promptText := req.Prompt
	if len([]rune(promptText)) > 950 {
		promptText = string([]rune(promptText)[:950])
	}
	payload := map[string]any{
		"model":       r.cfg.Model,
		"promptImage": encodeDataURI(imgSource, firstB64),
		"promptText":  promptText,
		"duration":    dur,
		"ratio":       ratio,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(r.cfg.BaseURL, "/") + "/v1/image_to_video"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+r.cfg.APIKey)
	httpReq.Header.Set("X-Runway-Version", "2024-11-06")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return "", &provider.ProviderError{Provider: r.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", &provider.ProviderError{Provider: r.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", &provider.ProviderError{Provider: r.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: false}
	}

	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.ID == "" {
		return "", &provider.ProviderError{Provider: r.Name(), Code: "EMPTY_ID", Message: string(raw), Retryable: false}
	}
	return res.ID, nil
}

func (r *RunwayProvider) Poll(ctx context.Context, remoteID string) (*provider.I2VStatus, error) {
	endpoint := strings.TrimRight(r.cfg.BaseURL, "/") + "/v1/tasks/" + remoteID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+r.cfg.APIKey)
	httpReq.Header.Set("X-Runway-Version", "2024-11-06")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: r.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, &provider.ProviderError{Provider: r.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}

	var res struct {
		Status  string   `json:"status"`
		Output  []string `json:"output"`
		Failure string   `json:"failure"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	st := &provider.I2VStatus{RemoteID: remoteID}
	switch strings.ToUpper(res.Status) {
	case "PENDING", "THROTTLED":
		st.State = provider.TaskQueued
	case "RUNNING":
		st.State = provider.TaskRunning
	case "SUCCEEDED":
		st.State = provider.TaskSucceeded
		if len(res.Output) > 0 {
			st.VideoURL = res.Output[0]
		}
	case "FAILED", "CANCELLED":
		st.State = provider.TaskFailed
		st.ErrorMsg = res.Failure
	default:
		st.State = provider.TaskRunning
	}
	return st, nil
}
