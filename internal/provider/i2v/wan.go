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

// WanConfig 阿里云百炼 / 通义万相（Wan2.1 / Wan2.2）异步图生视频适配器。
type WanConfig struct {
	BaseURL string
	APIKey  string
	Model   string // 如 wan2.1-i2v-plus / wan2.1-kf2v-plus / wanx2.1-i2v-turbo
	Timeout time.Duration
}

type WanProvider struct {
	cfg    WanConfig
	client *http.Client
}

func NewWan(cfg WanConfig) *WanProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://dashscope.aliyuncs.com"
	}
	if cfg.Model == "" {
		cfg.Model = "wan2.1-i2v-plus"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &WanProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (w *WanProvider) Name() string { return "wan:" + w.cfg.Model }

func (w *WanProvider) Submit(ctx context.Context, req provider.I2VSubmitRequest) (string, error) {
	if strings.TrimSpace(w.cfg.APIKey) == "" {
		return "", &provider.ProviderError{Provider: w.Name(), Code: "MISSING_API_KEY", Message: "DashScope Wan API key is required", Retryable: false}
	}
	input := map[string]any{
		"prompt": req.Prompt,
	}
	if req.NegativePrompt != "" {
		input["negative_prompt"] = req.NegativePrompt
	}
	imgSource := strings.TrimSpace(req.FirstFramePath)
	if imgSource == "" && len(req.ReferenceImages) > 0 {
		imgSource = req.ReferenceImages[0]
	}
	modelName := w.cfg.Model
	if imgSource != "" {
		firstB64, err := encodeFileBase64(imgSource)
		if err != nil {
			return "", err
		}
		firstURI := encodeDataURI(imgSource, firstB64)
		input["img_url"] = firstURI
		if req.TransitionMode == prompting.TransitionFirstLastFrame && req.LastFramePath != "" {
			if tailB64, tailErr := encodeFileBase64(req.LastFramePath); tailErr == nil {
				input["first_frame_url"] = firstURI
				input["last_frame_url"] = encodeDataURI(req.LastFramePath, tailB64)
			}
		}
	} else if strings.Contains(modelName, "i2v") {
		// 跳过首帧且无参考图时，自动切换到万相文生视频模型兜底
		modelName = strings.Replace(modelName, "i2v", "t2v", 1)
	}

	size := "720*1280"
	switch req.AspectRatio {
	case "16:9":
		size = "1280*720"
	case "3:4":
		size = "960*1280"
	}
	payload := map[string]any{
		"model": modelName,
		"input": input,
		"parameters": map[string]any{
			"resolution":    "720P",
			"size":          size,
			"prompt_extend": false, // 我们已经由导演提示词编译器精细控制，关闭二次改写防漂移
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(w.cfg.BaseURL, "/") + "/api/v1/services/aigc/video-generation/video-synthesis"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+w.cfg.APIKey)
	httpReq.Header.Set("X-DashScope-Async", "enable")

	resp, err := w.client.Do(httpReq)
	if err != nil {
		return "", &provider.ProviderError{Provider: w.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", &provider.ProviderError{Provider: w.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}
	if resp.StatusCode != http.StatusOK {
		return "", &provider.ProviderError{Provider: w.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: false}
	}

	var res struct {
		Output struct {
			TaskID string `json:"task_id"`
		} `json:"output"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.Output.TaskID == "" {
		return "", &provider.ProviderError{Provider: w.Name(), Code: res.Code, Message: res.Message, Retryable: false}
	}
	return res.Output.TaskID, nil
}

func (w *WanProvider) Poll(ctx context.Context, remoteID string) (*provider.I2VStatus, error) {
	endpoint := strings.TrimRight(w.cfg.BaseURL, "/") + "/api/v1/tasks/" + remoteID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+w.cfg.APIKey)

	resp, err := w.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: w.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, &provider.ProviderError{Provider: w.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}

	var res struct {
		Output struct {
			TaskStatus string `json:"task_status"`
			VideoURL   string `json:"video_url"`
			Message    string `json:"message"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	st := &provider.I2VStatus{RemoteID: remoteID}
	switch strings.ToUpper(res.Output.TaskStatus) {
	case "PENDING":
		st.State = provider.TaskQueued
	case "RUNNING":
		st.State = provider.TaskRunning
	case "SUCCEEDED":
		st.State = provider.TaskSucceeded
		st.VideoURL = res.Output.VideoURL
	case "FAILED", "CANCELED":
		st.State = provider.TaskFailed
		st.ErrorMsg = res.Output.Message
	default:
		st.State = provider.TaskRunning
	}
	return st, nil
}
