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

// VeoConfig Google Veo (Gemini API predictLongRunning) 异步图生视频适配器。
type VeoConfig struct {
	BaseURL string
	APIKey  string
	Model   string // 如 veo-3.0-generate-preview / veo-2.0-generate-001
	Timeout time.Duration
}

type VeoProvider struct {
	cfg    VeoConfig
	client *http.Client
}

func NewVeo(cfg VeoConfig) *VeoProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	if cfg.Model == "" {
		cfg.Model = "veo-2.0-generate-001"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &VeoProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (v *VeoProvider) Name() string { return "google-veo:" + v.cfg.Model }

func (v *VeoProvider) Submit(ctx context.Context, req provider.I2VSubmitRequest) (string, error) {
	if strings.TrimSpace(v.cfg.APIKey) == "" {
		return "", &provider.ProviderError{Provider: v.Name(), Code: "MISSING_API_KEY", Message: "Google Veo / Gemini API key is required", Retryable: false}
	}
	aspect := req.AspectRatio
	if aspect == "" || aspect == "3:4" {
		aspect = "9:16"
	}
	dur := req.DurationSec
	if dur < 5 {
		dur = 5
	}
	if dur > 8 {
		dur = 8
	}

	inst := map[string]any{
		"prompt": req.Prompt,
	}
	imgSource := strings.TrimSpace(req.FirstFramePath)
	if imgSource == "" && len(req.ReferenceImages) > 0 {
		imgSource = req.ReferenceImages[0]
	}
	if imgSource != "" {
		firstB64, err := encodeFileBase64(imgSource)
		if err != nil {
			return "", err
		}
		inst["image"] = map[string]string{
			"bytesBase64Encoded": firstB64,
			"mimeType":           "image/jpeg",
		}
	}

	payload := map[string]any{
		"instances": []map[string]any{inst},
		"parameters": map[string]any{
			"aspectRatio":      aspect,
			"durationSeconds":  dur,
			"personGeneration": "allow_adult",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("%s/models/%s:predictLongRunning", strings.TrimRight(v.cfg.BaseURL, "/"), v.cfg.Model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", v.cfg.APIKey)

	resp, err := v.client.Do(httpReq)
	if err != nil {
		return "", &provider.ProviderError{Provider: v.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", &provider.ProviderError{Provider: v.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}
	if resp.StatusCode != http.StatusOK {
		return "", &provider.ProviderError{Provider: v.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: false}
	}

	var res struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.Name == "" {
		return "", &provider.ProviderError{Provider: v.Name(), Code: "EMPTY_OP", Message: string(raw), Retryable: false}
	}
	return res.Name, nil
}

func (v *VeoProvider) Poll(ctx context.Context, remoteID string) (*provider.I2VStatus, error) {
	endpoint := strings.TrimRight(v.cfg.BaseURL, "/") + "/" + strings.TrimLeft(remoteID, "/")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("x-goog-api-key", v.cfg.APIKey)

	resp, err := v.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: v.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, &provider.ProviderError{Provider: v.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}

	var res struct {
		Done  bool `json:"done"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Response struct {
			GenerateVideoResponse struct {
				GeneratedSamples []struct {
					Video struct {
						URI string `json:"uri"`
					} `json:"video"`
				} `json:"generatedSamples"`
			} `json:"generateVideoResponse"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	st := &provider.I2VStatus{RemoteID: remoteID}
	if !res.Done {
		st.State = provider.TaskRunning
		return st, nil
	}
	if res.Error != nil {
		st.State = provider.TaskFailed
		st.ErrorMsg = res.Error.Message
		return st, nil
	}
	st.State = provider.TaskSucceeded
	samples := res.Response.GenerateVideoResponse.GeneratedSamples
	if len(samples) > 0 {
		uri := samples[0].Video.URI
		if uri != "" && !strings.Contains(uri, "key=") {
			sep := "?"
			if strings.Contains(uri, "?") {
				sep = "&"
			}
			uri = uri + sep + "key=" + v.cfg.APIKey
		}
		st.VideoURL = uri
	}
	return st, nil
}
