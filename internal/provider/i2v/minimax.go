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

	"feelings-studio/internal/prompting"
	"feelings-studio/internal/provider"
)

// MiniMaxVideoConfig MiniMax 海螺视频（Hailuo / MiniMax-Hailuo-02）异步图生视频适配器。
type MiniMaxVideoConfig struct {
	BaseURL string
	APIKey  string
	Model   string // 如 MiniMax-Hailuo-02 / I2V-01-Director
	Timeout time.Duration
}

type MiniMaxVideoProvider struct {
	cfg    MiniMaxVideoConfig
	client *http.Client
}

func NewMiniMaxVideo(cfg MiniMaxVideoConfig) *MiniMaxVideoProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.minimaxi.com"
	}
	if cfg.Model == "" {
		cfg.Model = "MiniMax-Hailuo-02"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &MiniMaxVideoProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (m *MiniMaxVideoProvider) Name() string { return "minimax-hailuo:" + m.cfg.Model }

func (m *MiniMaxVideoProvider) Submit(ctx context.Context, req provider.I2VSubmitRequest) (string, error) {
	if strings.TrimSpace(m.cfg.APIKey) == "" {
		return "", &provider.ProviderError{Provider: m.Name(), Code: "MISSING_API_KEY", Message: "MiniMax API key is required", Retryable: false}
	}
	dur := 6
	if req.DurationSec >= 9 {
		dur = 10
	}
	payload := map[string]any{
		"model":            m.cfg.Model,
		"prompt":           req.Prompt,
		"duration":         dur,
		"prompt_optimizer": false,
	}
	if strings.TrimSpace(req.FirstFramePath) != "" {
		firstB64, err := encodeFileBase64(req.FirstFramePath)
		if err != nil {
			return "", err
		}
		payload["first_frame_image"] = encodeDataURI(req.FirstFramePath, firstB64)
		if req.TransitionMode == prompting.TransitionFirstLastFrame && req.LastFramePath != "" {
			if tailB64, tailErr := encodeFileBase64(req.LastFramePath); tailErr == nil {
				payload["last_frame_image"] = encodeDataURI(req.LastFramePath, tailB64)
			}
		}
	} else if len(req.ReferenceImages) > 0 {
		// 跳过首帧时，传入主体参考图（MiniMax S2V subject_reference 协议）
		if refB64, refErr := encodeFileBase64(req.ReferenceImages[0]); refErr == nil {
			payload["subject_reference"] = []map[string]any{
				{
					"type":  "character",
					"image": []string{encodeDataURI(req.ReferenceImages[0], refB64)},
				},
			}
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(m.cfg.BaseURL, "/") + "/v1/video_generation"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return "", &provider.ProviderError{Provider: m.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", &provider.ProviderError{Provider: m.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}
	if resp.StatusCode != http.StatusOK {
		return "", &provider.ProviderError{Provider: m.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: false}
	}

	var res struct {
		TaskID   string `json:"task_id"`
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.BaseResp.StatusCode != 0 || res.TaskID == "" {
		return "", &provider.ProviderError{Provider: m.Name(), Code: fmt.Sprintf("CODE_%d", res.BaseResp.StatusCode), Message: res.BaseResp.StatusMsg, Retryable: false}
	}
	return res.TaskID, nil
}

func (m *MiniMaxVideoProvider) Poll(ctx context.Context, remoteID string) (*provider.I2VStatus, error) {
	endpoint := strings.TrimRight(m.cfg.BaseURL, "/") + "/v1/query/video_generation?task_id=" + remoteID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
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
		Status string `json:"status"`
		FileID string `json:"file_id"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}

	st := &provider.I2VStatus{RemoteID: remoteID}
	switch res.Status {
	case "Queueing", "Preparing":
		st.State = provider.TaskQueued
	case "Processing":
		st.State = provider.TaskRunning
	case "Success":
		dlURL, err := m.retrieveFileURL(ctx, res.FileID)
		if err != nil {
			return nil, err
		}
		st.State = provider.TaskSucceeded
		st.VideoURL = dlURL
	case "Fail":
		st.State = provider.TaskFailed
		st.ErrorMsg = "minimax video generation failed"
	default:
		st.State = provider.TaskRunning
	}
	return st, nil
}

func (m *MiniMaxVideoProvider) retrieveFileURL(ctx context.Context, fileID string) (string, error) {
	endpoint := strings.TrimRight(m.cfg.BaseURL, "/") + "/v1/files/retrieve?file_id=" + fileID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)
	resp, err := m.client.Do(httpReq)
	if err != nil {
		return "", &provider.ProviderError{Provider: m.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var res struct {
		File struct {
			DownloadURL string `json:"download_url"`
		} `json:"file"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.File.DownloadURL == "" {
		return "", fmt.Errorf("empty download_url for file_id=%s", fileID)
	}
	return res.File.DownloadURL, nil
}
