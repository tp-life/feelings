package llm

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

// OpenAICompatConfig 兼容 DeepSeek / 通义千问 / 豆包 Ark / OpenAI 等 Chat Completions 协议。
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
		cfg.BaseURL = "https://api.deepseek.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-chat"
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
	return "openai-compat:" + p.cfg.Model
}

// GenerateStory Step 1：先生成整体电影感故事剧情大纲与 4 幕情绪弧线（暂不预设固定分镜数）。
func (p *OpenAICompatProvider) GenerateStory(ctx context.Context, req provider.StoryRequest) (*provider.StoryDraft, error) {
	if strings.TrimSpace(p.cfg.APIKey) == "" {
		return nil, &provider.ProviderError{
			Provider: p.Name(), Code: "MISSING_API_KEY", Message: "LLM API key is empty", Retryable: false,
		}
	}
	userMsg := fmt.Sprintf("请围绕主题「%s」（标题：%s）先创作一份完整、克制、极具电影画面感的整体故事剧情大纲（不要拆镜，专注于把故事讲透）。", req.Theme, req.Title)
	if req.TargetPacing != "" && req.TargetPacing != "auto" {
		userMsg += fmt.Sprintf("\n篇幅节奏倾向：%s。", req.TargetPacing)
	}
	content, err := p.chatJSON(ctx, req.SystemPrompt, userMsg, 0.72)
	if err != nil {
		return nil, err
	}
	obj := extractJSONBlock(cleanCodeFence(content), '{', '}')
	if obj == "" {
		return nil, fmt.Errorf("no valid story json object returned (content=%s)", content)
	}
	var story provider.StoryDraft
	if err := json.Unmarshal([]byte(obj), &story); err != nil {
		return nil, fmt.Errorf("parse story json: %w", err)
	}
	return &story, nil
}

func (p *OpenAICompatProvider) GenerateStoryboard(ctx context.Context, req provider.StoryboardRequest) (*provider.StoryboardPlan, error) {
	if strings.TrimSpace(p.cfg.APIKey) == "" {
		return nil, &provider.ProviderError{
			Provider: p.Name(), Code: "MISSING_API_KEY", Message: "LLM API key is empty", Retryable: false,
		}
	}

	var userMsg string
	if req.ShotCount > 0 {
		userMsg = fmt.Sprintf("请围绕主题「%s」将故事拆解为 %d 个 9:16 竖屏分镜。", req.Theme, req.ShotCount)
	} else {
		userMsg = fmt.Sprintf("请严格依据已确认的整体故事剧情，围绕主题「%s」自主决定最合理的分镜数量（通常 4~10 镜，由剧情节拍与留白需要自然决定，切勿机械凑数），拆解为 9:16 竖屏分镜。", req.Theme)
	}
	if req.Story != nil && strings.TrimSpace(req.Story.Synopsis) != "" {
		storyJSON, _ := json.MarshalIndent(req.Story, "", "  ")
		userMsg += "\n\n以下是创作者已确认的整体故事剧情，请忠实还原每一幕推进：\n" + string(storyJSON)
	}
	if len(req.ExistingShotIDs) > 0 {
		userMsg += fmt.Sprintf("\n请严格沿用以下分镜编号顺序：%s。", strings.Join(req.ExistingShotIDs, ", "))
	}
	if len(req.Assets) > 0 {
		userMsg += "\n当前项目已有固定资产，请在分镜的 asset_bindings 中严格沿用其编号与外观，不要虚构无关资产。"
	}
	if strings.TrimSpace(req.RevisionFeedback) != "" {
		userMsg += "\n\n" + req.RevisionFeedback
	}

	content, err := p.chatJSON(ctx, req.SystemPrompt, userMsg, 0.65)
	if err != nil {
		return nil, err
	}

	plan, err := parseStoryboardPlan(content)
	if err != nil {
		return nil, fmt.Errorf("parse storyboard json: %w", err)
	}
	return plan, nil
}

func (p *OpenAICompatProvider) chatJSON(ctx context.Context, sysPrompt, userMsg string, temp float64) (string, error) {
	payload := map[string]any{
		"model":       p.cfg.Model,
		"temperature": temp,
		"messages": []map[string]string{
			{"role": "system", "content": sysPrompt},
			{"role": "user", "content": userMsg},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") + "/chat/completions"
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
		return "", &provider.ProviderError{
			Provider: p.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(respBody), Retryable: true,
		}
	}
	if resp.StatusCode != http.StatusOK {
		return "", &provider.ProviderError{
			Provider: p.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(respBody), Retryable: false,
		}
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", &provider.ProviderError{Provider: p.Name(), Code: "EMPTY_CHOICES", Message: "no choices returned", Retryable: true}
	}
	return parsed.Choices[0].Message.Content, nil
}

func cleanCodeFence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// ExtractAssetsFromStoryboard Step 2：把人工确认后的完整故事板交给 LLM，反推项目资产清单并补全每镜资产绑定与导演时序详版。
func (p *OpenAICompatProvider) ExtractAssetsFromStoryboard(ctx context.Context, req provider.AssetExtractionRequest) (*provider.AssetExtractionResult, error) {
	if strings.TrimSpace(p.cfg.APIKey) == "" {
		return nil, &provider.ProviderError{
			Provider: p.Name(), Code: "MISSING_API_KEY", Message: "LLM API key is empty", Retryable: false,
		}
	}

	shotsJSON, _ := json.MarshalIndent(req.ConfirmedShots, "", "  ")
	userMsg := fmt.Sprintf("以下是创作者已确认的主题「%s」完整故事板（共 %d 镜），请严格依据故事板内容反推本片必须锁定的核心资产清单（assets），并为每一镜补全 t2i_spec、i2v_spec 与含 asset_bindings / timeline_beats 的 director_sheet：\n\n%s", req.Theme, len(req.ConfirmedShots), string(shotsJSON))
	if len(req.ExistingAssets) > 0 {
		existJSON, _ := json.MarshalIndent(req.ExistingAssets, "", "  ")
		userMsg += "\n\n当前项目已有以下既有资产，请保留其 ID 与核心定义并可按需增补：\n" + string(existJSON)
	}

	payload := map[string]any{
		"model":       p.cfg.Model,
		"temperature": 0.45,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemPrompt},
			{"role": "user", "content": userMsg},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{Provider: p.Name(), Code: "NETWORK", Message: err.Error(), Retryable: true, Err: err}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, &provider.ProviderError{
			Provider: p.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(respBody), Retryable: true,
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &provider.ProviderError{
			Provider: p.Name(), Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: string(respBody), Retryable: false,
		}
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode llm asset extraction response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, &provider.ProviderError{Provider: p.Name(), Code: "EMPTY_CHOICES", Message: "no choices returned", Retryable: true}
	}

	cleaned := strings.TrimSpace(parsed.Choices[0].Message.Content)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	obj := extractJSONBlock(strings.TrimSpace(cleaned), '{', '}')
	if obj == "" {
		return nil, fmt.Errorf("no json object returned for asset extraction")
	}
	var out provider.AssetExtractionResult
	if err := json.Unmarshal([]byte(obj), &out); err != nil {
		return nil, fmt.Errorf("parse asset extraction json: %w", err)
	}
	return &out, nil
}

func parseStoryboardPlan(content string) (*provider.StoryboardPlan, error) {
	cleaned := strings.TrimSpace(content)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// 优先解析新结构 {"assets": [...], "shots": [...]}
	if obj := extractJSONBlock(cleaned, '{', '}'); obj != "" {
		var plan provider.StoryboardPlan
		if err := json.Unmarshal([]byte(obj), &plan); err == nil && len(plan.Shots) > 0 {
			return &plan, nil
		}
	}
	// 向下兼容仅返回分镜数组 [...] 的模型输出
	if arr := extractJSONBlock(cleaned, '[', ']'); arr != "" {
		var drafts []provider.ShotDraft
		if err := json.Unmarshal([]byte(arr), &drafts); err != nil {
			return nil, fmt.Errorf("%w (content=%s)", err, arr)
		}
		return &provider.StoryboardPlan{Shots: drafts}, nil
	}
	return nil, fmt.Errorf("no valid json object or array found (content=%s)", cleaned)
}

func extractJSONBlock(s string, openCh, closeCh byte) string {
	start := strings.IndexByte(s, openCh)
	end := strings.LastIndexByte(s, closeCh)
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}
