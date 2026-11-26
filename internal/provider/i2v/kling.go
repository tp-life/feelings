package i2v

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"feelings-studio/internal/prompting"
	"feelings-studio/internal/provider"
)

// KlingConfig 可灵（Kling AI）图生视频适配器配置。
type KlingConfig struct {
	BaseURL   string
	AccessKey string
	SecretKey string
	ModelName string // 例如 kling-v1-6 / kling-v2-master
	Mode      string // std 或 pro
	Timeout   time.Duration
}

type KlingProvider struct {
	cfg    KlingConfig
	client *http.Client
}

func NewKling(cfg KlingConfig) *KlingProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.klingai.com"
	}
	if cfg.ModelName == "" {
		cfg.ModelName = "kling-v1-6"
	}
	if cfg.Mode == "" {
		cfg.Mode = "pro"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &KlingProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (k *KlingProvider) Name() string { return "kling:" + k.cfg.ModelName }

func (k *KlingProvider) Submit(ctx context.Context, req provider.I2VSubmitRequest) (string, error) {
	token, err := k.signJWT()
	if err != nil {
		return "", &provider.ProviderError{Provider: k.Name(), Code: "AUTH", Message: err.Error(), Retryable: false}
	}
	dur := "5"
	if req.DurationSec >= 8 {
		dur = "10"
	}
	aspect := req.AspectRatio
	if aspect == "" {
		aspect = "9:16"
	}

	routeKind := "i2v"
	subPath := "/v1/videos/image2video"
	payload := map[string]any{
		"model_name":      k.cfg.ModelName,
		"mode":            k.cfg.Mode,
		"prompt":          req.Prompt,
		"negative_prompt": req.NegativePrompt,
		"duration":        dur,
		"aspect_ratio":    aspect,
		"cfg_scale":       0.5,
	}

	if strings.TrimSpace(req.FirstFramePath) != "" {
		firstB64, readErr := encodeFileBase64(req.FirstFramePath)
		if readErr != nil {
			return "", readErr
		}
		payload["image"] = firstB64
		if req.TransitionMode == prompting.TransitionFirstLastFrame && req.LastFramePath != "" {
			if tailB64, tailErr := encodeFileBase64(req.LastFramePath); tailErr == nil {
				payload["image_tail"] = tailB64
			}
		}
	} else if len(req.ReferenceImages) > 0 {
		// 跳过分镜首帧图时，优先走可灵多图参考生视频（multi-image2video，最多 4 张资产图）
		var imgList []map[string]string
		for _, refPath := range req.ReferenceImages {
			if len(imgList) >= 4 {
				break
			}
			if b64, refErr := encodeFileBase64(refPath); refErr == nil {
				imgList = append(imgList, map[string]string{"image": b64})
			}
		}
		if len(imgList) > 0 {
			routeKind = "m2v"
			subPath = "/v1/videos/multi-image2video"
			payload["image_list"] = imgList
		} else {
			routeKind = "t2v"
			subPath = "/v1/videos/text2video"
		}
	} else {
		routeKind = "t2v"
		subPath = "/v1/videos/text2video"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(k.cfg.BaseURL, "/") + subPath
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := k.client.Do(httpReq)
	if err != nil {
		return "", &provider.ProviderError{Provider: k.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", &provider.ProviderError{Provider: k.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}
	if resp.StatusCode != http.StatusOK {
		return "", &provider.ProviderError{Provider: k.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: false}
	}

	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.Code != 0 || res.Data.TaskID == "" {
		return "", &provider.ProviderError{Provider: k.Name(), Code: fmt.Sprintf("CODE_%d", res.Code), Message: res.Message, Retryable: false}
	}
	if routeKind != "i2v" {
		return routeKind + ":" + res.Data.TaskID, nil
	}
	return res.Data.TaskID, nil
}

func (k *KlingProvider) Poll(ctx context.Context, remoteID string) (*provider.I2VStatus, error) {
	token, err := k.signJWT()
	if err != nil {
		return nil, &provider.ProviderError{Provider: k.Name(), Code: "AUTH", Message: err.Error(), Retryable: false}
	}
	subPath := "/v1/videos/image2video/"
	taskID := remoteID
	if strings.HasPrefix(remoteID, "m2v:") {
		subPath = "/v1/videos/multi-image2video/"
		taskID = strings.TrimPrefix(remoteID, "m2v:")
	} else if strings.HasPrefix(remoteID, "t2v:") {
		subPath = "/v1/videos/text2video/"
		taskID = strings.TrimPrefix(remoteID, "t2v:")
	}
	endpoint := strings.TrimRight(k.cfg.BaseURL, "/") + subPath + taskID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := k.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: k.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, &provider.ProviderError{Provider: k.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(raw), Retryable: true}
	}

	var res struct {
		Data struct {
			TaskID        string `json:"task_id"`
			TaskStatus    string `json:"task_status"`
			TaskStatusMsg string `json:"task_status_msg"`
			TaskResult    struct {
				Videos []struct {
					URL string `json:"url"`
				} `json:"videos"`
			} `json:"task_result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	st := &provider.I2VStatus{RemoteID: remoteID}
	switch res.Data.TaskStatus {
	case "submitted":
		st.State = provider.TaskQueued
	case "processing":
		st.State = provider.TaskRunning
	case "succeed":
		st.State = provider.TaskSucceeded
		if len(res.Data.TaskResult.Videos) > 0 {
			st.VideoURL = res.Data.TaskResult.Videos[0].URL
		}
	case "failed":
		st.State = provider.TaskFailed
		st.ErrorMsg = res.Data.TaskStatusMsg
	default:
		st.State = provider.TaskRunning
	}
	return st, nil
}

func (k *KlingProvider) signJWT() (string, error) {
	if k.cfg.AccessKey == "" || k.cfg.SecretKey == "" {
		return "", fmt.Errorf("KLING_ACCESS_KEY and KLING_SECRET_KEY are required")
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	now := time.Now().Unix()
	claimsBytes, _ := json.Marshal(map[string]any{
		"iss": k.cfg.AccessKey,
		"exp": now + 1800,
		"nbf": now - 5,
	})
	claims := base64.RawURLEncoding.EncodeToString(claimsBytes)
	signingInput := header + "." + claims
	mac := hmac.New(sha256.New, []byte(k.cfg.SecretKey))
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig, nil
}

func encodeFileBase64(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func encodeDataURI(path, b64 string) string {
	mime := "image/jpeg"
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".png") {
		mime = "image/png"
	} else if strings.HasSuffix(lower, ".webp") {
		mime = "image/webp"
	}
	return "data:" + mime + ";base64," + b64
}
