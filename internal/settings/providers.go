package settings

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type StageSelection struct {
	LLM string `yaml:"llm" json:"llm"`
	T2I string `yaml:"t2i" json:"t2i"`
	I2V string `yaml:"i2v" json:"i2v"`
	TTS string `yaml:"tts" json:"tts"`
	SFX string `yaml:"sfx" json:"sfx"`
}

type ProviderProfile struct {
	BaseURL   string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	Model     string `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey    string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	AccessKey string `yaml:"access_key,omitempty" json:"access_key,omitempty"`
	SecretKey string `yaml:"secret_key,omitempty" json:"secret_key,omitempty"`
	AppID     string `yaml:"app_id,omitempty" json:"app_id,omitempty"`
	GroupID   string `yaml:"group_id,omitempty" json:"group_id,omitempty"`
	VoiceID   string `yaml:"voice_id,omitempty" json:"voice_id,omitempty"`
	Mode      string `yaml:"mode,omitempty" json:"mode,omitempty"`
}

type ProvidersConfig struct {
	Active   StageSelection             `yaml:"active" json:"active"`
	Profiles map[string]ProviderProfile `yaml:"profiles" json:"profiles"`
}

// ProviderSpec 描述模型供应商目录元数据，用于前端设置页动态渲染表单与默认预设。
type ProviderSpec struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Region          string   `json:"region"` // local | domestic | overseas
	Stage           string   `json:"stage"`  // llm | t2i | i2v | tts | sfx
	Description     string   `json:"description"`
	DefaultBaseURL  string   `json:"default_base_url,omitempty"`
	PresetModels    []string `json:"preset_models,omitempty"`
	PromptProfile   string   `json:"prompt_profile,omitempty"` // director | concise
	RequiredFields  []string `json:"required_fields,omitempty"`
	OptionalFields  []string `json:"optional_fields,omitempty"`
	SupportsTailRef bool     `json:"supports_tail_ref,omitempty"`
}

func Catalog() []ProviderSpec {
	return []ProviderSpec{
		// --- LLM ---
		{ID: "mock", Name: "本地 Mock 心理学导演", Region: "local", Stage: "llm", Description: "零费用内置三大赛道高质量双语结构化分镜与导演时序详版"},
		{ID: "deepseek", Name: "DeepSeek（国内）", Region: "domestic", Stage: "llm", Description: "性价比极高，中文文学与心理细节表达出色", DefaultBaseURL: "https://api.deepseek.com/v1", PresetModels: []string{"deepseek-chat", "deepseek-reasoner"}, RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}},
		{ID: "qwen", Name: "通义千问 Qwen（阿里云百炼）", Region: "domestic", Stage: "llm", Description: "阿里云 DashScope OpenAI 兼容协议", DefaultBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", PresetModels: []string{"qwen-plus", "qwen-max", "qwen-turbo"}, RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}},
		{ID: "doubao_ark", Name: "豆包 Ark（火山方舟）", Region: "domestic", Stage: "llm", Description: "字节火山方舟大模型接口（填接入点或模型名）", DefaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3", PresetModels: []string{"doubao-1-5-pro-32k-250115", "doubao-seed-1-6-250615"}, RequiredFields: []string{"api_key", "model"}, OptionalFields: []string{"base_url"}},
		{ID: "openai", Name: "OpenAI（国外）", Region: "overseas", Stage: "llm", Description: "GPT-4o / GPT-4.1 系列，支持中英双语结构化输出", DefaultBaseURL: "https://api.openai.com/v1", PresetModels: []string{"gpt-4o", "gpt-4.1", "gpt-4o-mini"}, RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}},
		{ID: "gemini", Name: "Google Gemini（国外）", Region: "overseas", Stage: "llm", Description: "Gemini OpenAI 兼容接口，长上下文与电影感文案强", DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", PresetModels: []string{"gemini-2.5-pro", "gemini-2.5-flash"}, RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}},

		// --- T2I ---
		{ID: "mock", Name: "本地暖调占位分镜图", Region: "local", Stage: "t2i", Description: "无现成首帧图时零费用生成 9:16 暖调分镜占位图"},
		{ID: "seedream", Name: "即梦 / 火山 Seedream（国内）", Region: "domestic", Stage: "t2i", Description: "年代纪实与中文提示词还原度极高", DefaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3", PresetModels: []string{"doubao-seedream-3-0-t2i", "doubao-seedream-4-0-t2i"}, RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}},
		{ID: "openai_t2i", Name: "OpenAI Images（国外）", Region: "overseas", Stage: "t2i", Description: "GPT-Image-1 / DALL·E 兼容生图接口", DefaultBaseURL: "https://api.openai.com/v1", PresetModels: []string{"gpt-image-1", "dall-e-3"}, RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}},

		// --- I2V（核心视频生成供应商）---
		{ID: "mock", Name: "本地 FFmpeg 运镜引擎", Region: "local", Stage: "i2v", Description: "零费用基于首帧/首尾帧图生成真实 1080×1920 电影缓推/平移 MP4", PromptProfile: "director", SupportsTailRef: true},
		{ID: "jimeng", Name: "即梦 / 火山 Seedance（国内）", Region: "domestic", Stage: "i2v", Description: "强烈推荐导演时序详版，支持首尾帧过渡与多资产约束", DefaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3", PresetModels: []string{"doubao-seedance-1-0-pro", "doubao-seedance-1-0-lite-i2v"}, PromptProfile: "director", RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}, SupportsTailRef: true},
		{ID: "wan", Name: "通义万相 Wan（阿里云百炼）", Region: "domestic", Stage: "i2v", Description: "支持 Wan2.1/Wan2.2 图生视频及 kf2v 首尾帧过渡", DefaultBaseURL: "https://dashscope.aliyuncs.com", PresetModels: []string{"wan2.1-i2v-plus", "wan2.1-kf2v-plus", "wan2.1-i2v-turbo"}, PromptProfile: "director", RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}, SupportsTailRef: true},
		{ID: "minimax_hailuo", Name: "MiniMax 海螺视频 Hailuo（国内）", Region: "domestic", Stage: "i2v", Description: "人物表情与运镜表现自然，支持首尾帧与导演运镜", DefaultBaseURL: "https://api.minimaxi.com", PresetModels: []string{"MiniMax-Hailuo-02", "I2V-01-Director", "S2V-01"}, PromptProfile: "concise", RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}, SupportsTailRef: true},
		{ID: "kling", Name: "可灵 Kling AI（快手国内）", Region: "domestic", Stage: "i2v", Description: "中文人物一致性与微动态极稳，支持首尾帧（AccessKey+SecretKey JWT）", DefaultBaseURL: "https://api.klingai.com", PresetModels: []string{"kling-v2-master", "kling-v1-6", "kling-v1-5"}, PromptProfile: "director", RequiredFields: []string{"access_key", "secret_key"}, OptionalFields: []string{"base_url", "model", "mode"}, SupportsTailRef: true},
		{ID: "veo", Name: "Google Veo（国外）", Region: "overseas", Stage: "i2v", Description: "电影级光影、空气感与自然物理细节，强烈推荐搭配英文导演详版", DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta", PresetModels: []string{"veo-3.0-generate-preview", "veo-2.0-generate-001"}, PromptProfile: "director", RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}},
		{ID: "runway", Name: "Runway Gen-3 / Gen-4（国外）", Region: "overseas", Stage: "i2v", Description: "高稳定性镜头运镜，推荐使用英文精炼电影版提示词", DefaultBaseURL: "https://api.dev.runwayml.com", PresetModels: []string{"gen4_turbo", "gen3a_turbo"}, PromptProfile: "concise", RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}},
		{ID: "luma", Name: "Luma Dream Machine（国外）", Region: "overseas", Stage: "i2v", Description: "Ray 2 丝滑光影过渡，推荐精炼电影版提示词", DefaultBaseURL: "https://api.lumalabs.ai/dream-machine/v1", PresetModels: []string{"ray-2", "ray-flash-2"}, PromptProfile: "concise", RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model"}, SupportsTailRef: true},

		// --- TTS ---
		{ID: "mock", Name: "本地静音时间轴对齐", Region: "local", Stage: "tts", Description: "零费用生成精确时长空白音轨与句级字幕时间轴"},
		{ID: "macos_say", Name: "macOS 本机中文朗读", Region: "local", Stage: "tts", Description: "零费用调用 macOS 系统中文语音（Tingting）"},
		{ID: "doubao_tts", Name: "豆包语音（火山引擎）", Region: "domestic", Stage: "tts", Description: "自然温柔中文情感旁白，支持语速与情绪控制", DefaultBaseURL: "https://openspeech.bytedance.com/api/v1/tts", RequiredFields: []string{"app_id", "api_key"}, OptionalFields: []string{"base_url", "voice_id"}},
		{ID: "minimax_tts", Name: "MiniMax T2A 情感语音（国内）", Region: "domestic", Stage: "tts", Description: "细腻低语速叙事声线，非常适合情怀独白", DefaultBaseURL: "https://api.minimax.chat/v1/t2a_v2", PresetModels: []string{"speech-02-hd", "speech-02-turbo"}, RequiredFields: []string{"group_id", "api_key"}, OptionalFields: []string{"base_url", "model", "voice_id"}},
		{ID: "elevenlabs", Name: "ElevenLabs（国外）", Region: "overseas", Stage: "tts", Description: "顶级电影感多语言情感配音", DefaultBaseURL: "https://api.elevenlabs.io/v1", PresetModels: []string{"eleven_multilingual_v2", "eleven_flash_v2_5"}, RequiredFields: []string{"api_key"}, OptionalFields: []string{"base_url", "model", "voice_id"}},

		// --- SFX ---
		{ID: "mock", Name: "本地声景库 + FFmpeg 声学合成", Region: "local", Stage: "sfx", Description: "优先读取 assets/sfx/<tag>.wav，缺失时用 FFmpeg 柔和合成蝉鸣/微风/老风扇"},
	}
}

func DefaultConfig() *ProvidersConfig {
	profiles := make(map[string]ProviderProfile)
	for _, spec := range Catalog() {
		if spec.ID == "mock" || spec.ID == "macos_say" {
			continue
		}
		p := ProviderProfile{BaseURL: spec.DefaultBaseURL}
		if len(spec.PresetModels) > 0 {
			p.Model = spec.PresetModels[0]
		}
		if spec.ID == "kling" {
			p.Mode = "pro"
		}
		profiles[spec.ID] = p
	}
	return &ProvidersConfig{
		Active: StageSelection{
			LLM: "mock",
			T2I: "mock",
			I2V: "mock",
			TTS: "mock",
			SFX: "mock",
		},
		Profiles: profiles,
	}
}

func ConfigPath(rootDir string) string {
	return filepath.Join(rootDir, ".feelings", "providers.yaml")
}

func Load(rootDir string) (*ProvidersConfig, error) {
	cfg := DefaultConfig()
	path := ConfigPath(rootDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read providers config: %w", err)
	}
	var disk ProvidersConfig
	if err := yaml.Unmarshal(data, &disk); err != nil {
		return nil, fmt.Errorf("parse providers config: %w", err)
	}
	if disk.Active.LLM != "" {
		cfg.Active.LLM = disk.Active.LLM
	}
	if disk.Active.T2I != "" {
		cfg.Active.T2I = disk.Active.T2I
	}
	if disk.Active.I2V != "" {
		cfg.Active.I2V = disk.Active.I2V
	}
	if disk.Active.TTS != "" {
		cfg.Active.TTS = disk.Active.TTS
	}
	if disk.Active.SFX != "" {
		cfg.Active.SFX = disk.Active.SFX
	}
	for k, v := range disk.Profiles {
		base := cfg.Profiles[k]
		if v.BaseURL != "" {
			base.BaseURL = v.BaseURL
		}
		if v.Model != "" {
			base.Model = v.Model
		}
		if v.APIKey != "" {
			base.APIKey = v.APIKey
		}
		if v.AccessKey != "" {
			base.AccessKey = v.AccessKey
		}
		if v.SecretKey != "" {
			base.SecretKey = v.SecretKey
		}
		if v.AppID != "" {
			base.AppID = v.AppID
		}
		if v.GroupID != "" {
			base.GroupID = v.GroupID
		}
		if v.VoiceID != "" {
			base.VoiceID = v.VoiceID
		}
		if v.Mode != "" {
			base.Mode = v.Mode
		}
		cfg.Profiles[k] = base
	}
	return cfg, nil
}

// MergeAndSave 将前端提交的配置与磁盘现有真实密钥安全合并（若提交的是脱敏串或留空，则保留原有密钥），并以 0600 权限落盘。
func MergeAndSave(rootDir string, incoming *ProvidersConfig) (*ProvidersConfig, error) {
	existing, err := Load(rootDir)
	if err != nil {
		return nil, err
	}
	if incoming.Active.LLM != "" {
		existing.Active.LLM = incoming.Active.LLM
	}
	if incoming.Active.T2I != "" {
		existing.Active.T2I = incoming.Active.T2I
	}
	if incoming.Active.I2V != "" {
		existing.Active.I2V = incoming.Active.I2V
	}
	if incoming.Active.TTS != "" {
		existing.Active.TTS = incoming.Active.TTS
	}
	if incoming.Active.SFX != "" {
		existing.Active.SFX = incoming.Active.SFX
	}
	for id, inc := range incoming.Profiles {
		cur := existing.Profiles[id]
		cur.BaseURL = strings.TrimSpace(inc.BaseURL)
		cur.Model = strings.TrimSpace(inc.Model)
		cur.AppID = strings.TrimSpace(inc.AppID)
		cur.GroupID = strings.TrimSpace(inc.GroupID)
		cur.VoiceID = strings.TrimSpace(inc.VoiceID)
		cur.Mode = strings.TrimSpace(inc.Mode)

		if !isMaskedOrEmpty(inc.APIKey) {
			cur.APIKey = strings.TrimSpace(inc.APIKey)
		} else if inc.APIKey == "__CLEAR__" {
			cur.APIKey = ""
		}
		if !isMaskedOrEmpty(inc.AccessKey) {
			cur.AccessKey = strings.TrimSpace(inc.AccessKey)
		} else if inc.AccessKey == "__CLEAR__" {
			cur.AccessKey = ""
		}
		if !isMaskedOrEmpty(inc.SecretKey) {
			cur.SecretKey = strings.TrimSpace(inc.SecretKey)
		} else if inc.SecretKey == "__CLEAR__" {
			cur.SecretKey = ""
		}
		existing.Profiles[id] = cur
	}

	path := ConfigPath(rootDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(existing)
	if err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, err
	}
	return existing, nil
}

// MaskedCopy 返回适合发送到前端展示的脱敏配置副本（不暴露完整 API Key）。
func (c *ProvidersConfig) MaskedCopy() *ProvidersConfig {
	out := &ProvidersConfig{
		Active:   c.Active,
		Profiles: make(map[string]ProviderProfile, len(c.Profiles)),
	}
	for k, v := range c.Profiles {
		v.APIKey = maskSecret(v.APIKey)
		v.AccessKey = maskSecret(v.AccessKey)
		v.SecretKey = maskSecret(v.SecretKey)
		out.Profiles[k] = v
	}
	return out
}

func maskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func isMaskedOrEmpty(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || s == "__CLEAR__" || strings.Contains(s, "****")
}

type TestResult struct {
	ProviderID string `json:"provider_id"`
	OK         bool   `json:"ok"`
	Message    string `json:"message"`
	LatencyMS  int64  `json:"latency_ms"`
}

// TestProvider 校验必填凭据并执行非扣费的轻量连通性检查（绝不提交扣费视频任务）。
func TestProvider(ctx context.Context, cfg *ProvidersConfig, providerID string) TestResult {
	start := time.Now()
	if providerID == "mock" || providerID == "macos_say" {
		return TestResult{ProviderID: providerID, OK: true, Message: "本地零费用引擎已就绪", LatencyMS: time.Since(start).Milliseconds()}
	}

	var spec *ProviderSpec
	for _, s := range Catalog() {
		if s.ID == providerID {
			cp := s
			spec = &cp
			break
		}
	}
	if spec == nil {
		return TestResult{ProviderID: providerID, OK: false, Message: "未知供应商 ID: " + providerID}
	}

	prof := cfg.Profiles[providerID]
	for _, reqField := range spec.RequiredFields {
		switch reqField {
		case "api_key":
			if strings.TrimSpace(prof.APIKey) == "" {
				return TestResult{ProviderID: providerID, OK: false, Message: "缺少必填凭据：API Key"}
			}
		case "access_key":
			if strings.TrimSpace(prof.AccessKey) == "" {
				return TestResult{ProviderID: providerID, OK: false, Message: "缺少必填凭据：Access Key"}
			}
		case "secret_key":
			if strings.TrimSpace(prof.SecretKey) == "" {
				return TestResult{ProviderID: providerID, OK: false, Message: "缺少必填凭据：Secret Key"}
			}
		case "app_id":
			if strings.TrimSpace(prof.AppID) == "" {
				return TestResult{ProviderID: providerID, OK: false, Message: "缺少必填凭据：App ID"}
			}
		case "group_id":
			if strings.TrimSpace(prof.GroupID) == "" {
				return TestResult{ProviderID: providerID, OK: false, Message: "缺少必填凭据：Group ID"}
			}
		case "model":
			if strings.TrimSpace(prof.Model) == "" {
				return TestResult{ProviderID: providerID, OK: false, Message: "缺少必填项：Model / 接入点 ID"}
			}
		}
	}

	baseURL := strings.TrimSpace(prof.BaseURL)
	if baseURL == "" {
		baseURL = spec.DefaultBaseURL
	}
	if baseURL == "" {
		return TestResult{ProviderID: providerID, OK: true, Message: "凭据格式校验通过", LatencyMS: time.Since(start).Milliseconds()}
	}

	// 轻量非扣费探测：对支持 OpenAI /models 的 LLM 查 /models，其他仅探测端点 TLS/HTTP 可达性
	client := &http.Client{Timeout: 6 * time.Second}
	probeURL := baseURL
	if spec.Stage == "llm" {
		probeURL = strings.TrimRight(baseURL, "/") + "/models"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return TestResult{ProviderID: providerID, OK: false, Message: "BaseURL 格式非法: " + err.Error()}
	}
	if prof.APIKey != "" {
		if providerID == "veo" {
			req.Header.Set("x-goog-api-key", prof.APIKey)
		} else if providerID == "elevenlabs" {
			req.Header.Set("xi-api-key", prof.APIKey)
		} else {
			req.Header.Set("Authorization", "Bearer "+prof.APIKey)
		}
	}
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return TestResult{ProviderID: providerID, OK: false, Message: fmt.Sprintf("凭据已保存，但端点网络探测失败（请检查网络/代理）：%v", err), LatencyMS: latency}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return TestResult{ProviderID: providerID, OK: false, Message: fmt.Sprintf("端点可达但鉴权被拒 (HTTP %d)，请核对 API Key", resp.StatusCode), LatencyMS: latency}
	}
	return TestResult{
		ProviderID: providerID,
		OK:         true,
		Message:    fmt.Sprintf("凭据完整且端点可达 (HTTP %d)", resp.StatusCode),
		LatencyMS:  latency,
	}
}
