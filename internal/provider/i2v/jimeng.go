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

// JimengConfig 即梦 / 火山方舟 Seedance 图生视频适配器配置。
type JimengConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type JimengProvider struct {
	cfg    JimengConfig
	client *http.Client
}

func NewJimeng(cfg JimengConfig) *JimengProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://ark.cn-beijing.volces.com/api/v3"
	}
	if cfg.Model == "" {
		cfg.Model = "doubao-seedance-1-0-pro"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &JimengProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (j *JimengProvider) Name() string { return "jimeng:" + j.cfg.Model }

func (j *JimengProvider) Submit(ctx context.Context, req provider.I2VSubmitRequest) (string, error) {
	if strings.TrimSpace(j.cfg.APIKey) == "" {
		return "", &provider.ProviderError{Provider: j.Name(), Code: "MISSING_API_KEY", Message: "JIMENG_API_KEY is required", Retryable: false}
	}
	promptWithParams := req.Prompt
	if req.AspectRatio != "" && !strings.Contains(promptWithParams, "--ratio") {
		promptWithParams = fmt.Sprintf("%s --ratio %s", promptWithParams, req.AspectRatio)
	}
	if req.DurationSec > 0 && !strings.Contains(promptWithParams, "--dur") {
		dur := 5
		if req.DurationSec >= 8 {
			dur = 10
		}
		promptWithParams = fmt.Sprintf("%s --dur %d", promptWithParams, dur)
	}

	content := []map[string]any{
		{"type": "text", "text": promptWithParams},
	}

	// 模式 1：存在首帧图（稳控图生视频 + 可选尾帧过渡）
	if strings.TrimSpace(req.FirstFramePath) != "" {
		firstB64, err := encodeFileBase64(req.FirstFramePath)
		if err != nil {
			return "", err
		}
		content = append(content, map[string]any{
			"type":      "image_url",
			"image_url": map[string]string{"url": encodeDataURI(req.FirstFramePath, firstB64)},
			"role":      "first_frame",
		})
		if req.TransitionMode == prompting.TransitionFirstLastFrame && req.LastFramePath != "" {
			if tailB64, tailErr := encodeFileBase64(req.LastFramePath); tailErr == nil {
				content = append(content, map[string]any{
					"type":      "image_url",
					"image_url": map[string]string{"url": encodeDataURI(req.LastFramePath, tailB64)},
					"role":      "last_frame",
				})
			}
		}
	} else if len(req.ReferenceImages) > 0 {
		// 模式 2：跳过分镜首帧图，直接传入本镜绑定的多张角色/场景/道具资产图（Seedance 多图参考生视频 role=reference_image）
		for _, refPath := range req.ReferenceImages {
			if len(content) >= 5 { // Seedance 多参考图上限保护（最多 4 张参考图）
				break
			}
			if refB64, refErr := encodeFileBase64(refPath); refErr == nil {
				content = append(content, map[string]any{
					"type":      "image_url",
					"image_url": map[string]string{"url": encodeDataURI(refPath, refB64)},
					"role":      "reference_image",
				})
			}
		}
	}
	// 模式 3：无首帧也无参考图时，自动以纯文本 content 走 Seedance 文生视频（T2V）兜底

	payload := map[string]any{
		"model":   j.cfg.Model,
		"content": content,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(j.cfg.BaseURL, "/") + "/contents/generations/tasks"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+j.cfg.APIKey)

	resp, err := j.client.Do(httpReq)
	if err != nil {
		return "", &provider.ProviderError{Provider: j.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", &provider.ProviderError{Provider: j.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", &provider.ProviderError{Provider: j.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: false}
	}

	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.ID == "" {
		return "", &provider.ProviderError{Provider: j.Name(), Code: "EMPTY_ID", Message: string(raw), Retryable: false}
	}
	return res.ID, nil
}

func (j *JimengProvider) Poll(ctx context.Context, remoteID string) (*provider.I2VStatus, error) {
	endpoint := strings.TrimRight(j.cfg.BaseURL, "/") + "/contents/generations/tasks/" + remoteID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+j.cfg.APIKey)

	resp, err := j.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: j.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, &provider.ProviderError{Provider: j.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}

	var res struct {
		Status  string `json:"status"`
		Content struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	st := &provider.I2VStatus{RemoteID: remoteID}
	switch res.Status {
	case "queued":
		st.State = provider.TaskQueued
	case "running":
		st.State = provider.TaskRunning
	case "succeeded":
		st.State = provider.TaskSucceeded
		st.VideoURL = res.Content.VideoURL
	case "failed", "cancelled":
		st.State = provider.TaskFailed
		if res.Error != nil {
			st.ErrorMsg = res.Error.Message
		}
	default:
		st.State = provider.TaskRunning
	}
	return st, nil
}
