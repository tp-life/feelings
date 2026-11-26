package t2i

import (
	"bytes"
	"context"
	"encoding/base64"
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

// OpenAICompatConfig 兼容火山方舟/即梦/OpenAI Images 协议的文生图适配器。
type OpenAICompatConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type OpenAICompatProvider struct {
	cfg    OpenAICompatConfig
	client *http.Client
}

func NewOpenAICompat(cfg OpenAICompatConfig) *OpenAICompatProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://ark.cn-beijing.volces.com/api/v3"
	}
	if cfg.Model == "" {
		cfg.Model = "doubao-seedream-3-0-t2i"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	return &OpenAICompatProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (p *OpenAICompatProvider) Name() string {
	return "t2i:" + p.cfg.Model
}

func (p *OpenAICompatProvider) Generate(ctx context.Context, req provider.T2IRequest) (string, error) {
	if strings.TrimSpace(p.cfg.APIKey) == "" {
		return "", &provider.ProviderError{Provider: p.Name(), Code: "MISSING_API_KEY", Message: "T2I API key is empty", Retryable: false}
	}
	size := "1080x1920"
	switch req.AspectRatio {
	case "16:9":
		size = "1920x1080"
	case "3:4":
		size = "1080x1440"
	}
	payload := map[string]any{
		"model":           p.cfg.Model,
		"prompt":          req.Prompt,
		"negative_prompt": req.NegativePrompt,
		"size":            size,
		"response_format": "b64_json",
	}
	if req.Seed != 0 {
		payload["seed"] = req.Seed
	}
	// 注入当前分镜显式绑定的角色/场景/道具参考图（兼容即梦 Seedream 3.0/4.0 image / reference_images 垫图一致性协议）
	if len(req.ReferenceImages) > 0 {
		var refURIs []string
		for _, imgPath := range req.ReferenceImages {
			if rawImg, readErr := os.ReadFile(imgPath); readErr == nil && len(rawImg) > 0 {
				mime := "image/jpeg"
				lower := strings.ToLower(imgPath)
				if strings.HasSuffix(lower, ".png") {
					mime = "image/png"
				} else if strings.HasSuffix(lower, ".webp") {
					mime = "image/webp"
				}
				refURIs = append(refURIs, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(rawImg))
			}
		}
		if len(refURIs) == 1 {
			payload["image"] = refURIs[0]
		} else if len(refURIs) > 1 {
			payload["image"] = refURIs
			payload["reference_images"] = refURIs
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") + "/images/generations"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", &provider.ProviderError{Provider: p.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", &provider.ProviderError{Provider: p.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(respBody), Retryable: true}
	}
	if resp.StatusCode != http.StatusOK {
		return "", &provider.ProviderError{Provider: p.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(respBody), Retryable: false}
	}

	var parsed struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Data) == 0 {
		return "", &provider.ProviderError{Provider: p.Name(), Code: "EMPTY_DATA", Message: "no image returned", Retryable: true}
	}

	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return "", err
	}
	if parsed.Data[0].B64JSON != "" {
		raw, err := base64.StdEncoding.DecodeString(parsed.Data[0].B64JSON)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(req.OutputPath, raw, 0o644); err != nil {
			return "", err
		}
		return req.OutputPath, nil
	}
	if parsed.Data[0].URL != "" {
		dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.Data[0].URL, nil)
		if err != nil {
			return "", err
		}
		dlResp, err := p.client.Do(dlReq)
		if err != nil {
			return "", err
		}
		defer dlResp.Body.Close()
		imgBytes, err := io.ReadAll(dlResp.Body)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(req.OutputPath, imgBytes, 0o644); err != nil {
			return "", err
		}
		return req.OutputPath, nil
	}
	return "", &provider.ProviderError{Provider: p.Name(), Code: "EMPTY_IMAGE", Message: "neither b64_json nor url returned", Retryable: false}
}
