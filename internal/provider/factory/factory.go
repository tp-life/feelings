package providerfactory

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"feelings-studio/internal/prompting"
	"feelings-studio/internal/provider"
	"feelings-studio/internal/provider/i2v"
	"feelings-studio/internal/provider/llm"
	"feelings-studio/internal/provider/mock"
	"feelings-studio/internal/provider/t2i"
	"feelings-studio/internal/provider/tts"
	"feelings-studio/internal/settings"
)

// Bundle 汇总全流水线所需的 5 类 Provider，默认全部走零成本本地 Mock + FFmpeg 引擎，
// 支持通过 Web 设置页（.feelings/providers.yaml）或环境变量随时切换任意国内/国外模型。
type Bundle struct {
	LLM       provider.LLMProvider
	T2I       provider.T2IProvider
	I2V       provider.I2VProvider
	TTS       provider.TTSProvider
	SFX       provider.SFXProvider
	ActiveIDs settings.StageSelection
}

func FromEnv(rootDir string) *Bundle {
	cfg, err := settings.Load(rootDir)
	if err != nil {
		log.Printf("警告: 读取模型供应商配置失败，回退到默认 Mock 供应商: %v", err)
		cfg = settings.DefaultConfig()
	}
	return FromConfigWithEnv(rootDir, cfg)
}

// FromConfigWithEnv 在给定配置的副本上叠加 FEELINGS_* 环境变量覆盖后构建 Provider（不修改传入的 cfg）。
func FromConfigWithEnv(rootDir string, cfg *settings.ProvidersConfig) *Bundle {
	if cfg == nil {
		cfg = settings.DefaultConfig()
	}
	cp := cfg.Clone()
	applyEnvOverrides(cp)
	return FromConfig(rootDir, cp)
}

func FromConfig(rootDir string, cfg *settings.ProvidersConfig) *Bundle {
	if cfg == nil {
		cfg = settings.DefaultConfig()
	}
	b := &Bundle{ActiveIDs: cfg.Active}

	// 1. LLM
	switch strings.ToLower(cfg.Active.LLM) {
	case "deepseek", "qwen", "doubao_ark", "openai", "gemini", "openai_compat":
		prof := cfg.Profiles[strings.ToLower(cfg.Active.LLM)]
		b.LLM = llm.NewOpenAICompat(llm.OpenAICompatConfig{
			BaseURL: firstNonEmpty(prof.BaseURL, os.Getenv("FEELINGS_LLM_BASE_URL")),
			APIKey:  firstNonEmpty(prof.APIKey, os.Getenv("FEELINGS_LLM_API_KEY")),
			Model:   firstNonEmpty(prof.Model, os.Getenv("FEELINGS_LLM_MODEL")),
		})
	default:
		b.LLM = mock.NewMockLLM()
		b.ActiveIDs.LLM = "mock"
	}

	// 2. T2I
	switch strings.ToLower(cfg.Active.T2I) {
	case "seedream", "openai_t2i", "openai_compat", "volcengine":
		prof := cfg.Profiles[strings.ToLower(cfg.Active.T2I)]
		b.T2I = t2i.NewOpenAICompat(t2i.OpenAICompatConfig{
			BaseURL: firstNonEmpty(prof.BaseURL, os.Getenv("FEELINGS_T2I_BASE_URL")),
			APIKey:  firstNonEmpty(prof.APIKey, os.Getenv("FEELINGS_T2I_API_KEY")),
			Model:   firstNonEmpty(prof.Model, os.Getenv("FEELINGS_T2I_MODEL")),
		})
	default:
		b.T2I = mock.NewLocalT2I()
		b.ActiveIDs.T2I = "mock"
	}

	// 3. I2V（支持国内：即梦 / 万相Wan / MiniMax海螺 / 可灵；国外：Veo / Runway / Luma；本地：Mock）
	switch strings.ToLower(cfg.Active.I2V) {
	case "jimeng", "seedance":
		prof := cfg.Profiles["jimeng"]
		b.I2V = i2v.NewJimeng(i2v.JimengConfig{
			BaseURL: firstNonEmpty(prof.BaseURL, os.Getenv("JIMENG_BASE_URL")),
			APIKey:  firstNonEmpty(prof.APIKey, os.Getenv("JIMENG_API_KEY")),
			Model:   firstNonEmpty(prof.Model, os.Getenv("JIMENG_MODEL")),
		})
	case "wan":
		prof := cfg.Profiles["wan"]
		b.I2V = i2v.NewWan(i2v.WanConfig{
			BaseURL: firstNonEmpty(prof.BaseURL, os.Getenv("WAN_BASE_URL")),
			APIKey:  firstNonEmpty(prof.APIKey, os.Getenv("DASHSCOPE_API_KEY")),
			Model:   firstNonEmpty(prof.Model, os.Getenv("WAN_MODEL")),
		})
	case "minimax_hailuo", "hailuo", "minimax":
		prof := cfg.Profiles["minimax_hailuo"]
		b.I2V = i2v.NewMiniMaxVideo(i2v.MiniMaxVideoConfig{
			BaseURL: firstNonEmpty(prof.BaseURL, os.Getenv("MINIMAX_VIDEO_BASE_URL")),
			APIKey:  firstNonEmpty(prof.APIKey, os.Getenv("MINIMAX_API_KEY")),
			Model:   firstNonEmpty(prof.Model, os.Getenv("MINIMAX_VIDEO_MODEL")),
		})
	case "kling":
		prof := cfg.Profiles["kling"]
		b.I2V = i2v.NewKling(i2v.KlingConfig{
			BaseURL:   firstNonEmpty(prof.BaseURL, os.Getenv("KLING_BASE_URL")),
			AccessKey: firstNonEmpty(prof.AccessKey, os.Getenv("KLING_ACCESS_KEY")),
			SecretKey: firstNonEmpty(prof.SecretKey, os.Getenv("KLING_SECRET_KEY")),
			ModelName: firstNonEmpty(prof.Model, os.Getenv("KLING_MODEL")),
			Mode:      firstNonEmpty(prof.Mode, os.Getenv("KLING_MODE")),
		})
	case "veo":
		prof := cfg.Profiles["veo"]
		b.I2V = i2v.NewVeo(i2v.VeoConfig{
			BaseURL: firstNonEmpty(prof.BaseURL, os.Getenv("VEO_BASE_URL")),
			APIKey:  firstNonEmpty(prof.APIKey, os.Getenv("GEMINI_API_KEY")),
			Model:   firstNonEmpty(prof.Model, os.Getenv("VEO_MODEL")),
		})
	case "runway":
		prof := cfg.Profiles["runway"]
		b.I2V = i2v.NewRunway(i2v.RunwayConfig{
			BaseURL: firstNonEmpty(prof.BaseURL, os.Getenv("RUNWAY_BASE_URL")),
			APIKey:  firstNonEmpty(prof.APIKey, os.Getenv("RUNWAY_API_KEY")),
			Model:   firstNonEmpty(prof.Model, os.Getenv("RUNWAY_MODEL")),
		})
	case "luma":
		prof := cfg.Profiles["luma"]
		b.I2V = i2v.NewLuma(i2v.LumaConfig{
			BaseURL: firstNonEmpty(prof.BaseURL, os.Getenv("LUMA_BASE_URL")),
			APIKey:  firstNonEmpty(prof.APIKey, os.Getenv("LUMA_API_KEY")),
			Model:   firstNonEmpty(prof.Model, os.Getenv("LUMA_MODEL")),
		})
	default:
		b.I2V = mock.NewLocalMotionI2V(filepath.Join(rootDir, ".cache", "local_i2v"))
		b.ActiveIDs.I2V = "mock"
	}

	// 4. TTS
	switch strings.ToLower(cfg.Active.TTS) {
	case "doubao", "doubao_tts":
		prof := cfg.Profiles["doubao_tts"]
		b.TTS = tts.NewDoubao(tts.DoubaoConfig{
			Endpoint:  firstNonEmpty(prof.BaseURL, os.Getenv("DOUBAO_TTS_ENDPOINT")),
			AppID:     firstNonEmpty(prof.AppID, os.Getenv("DOUBAO_TTS_APP_ID")),
			Token:     firstNonEmpty(prof.APIKey, os.Getenv("DOUBAO_TTS_TOKEN")),
			VoiceType: firstNonEmpty(prof.VoiceID, os.Getenv("DOUBAO_TTS_VOICE")),
		})
	case "minimax", "minimax_tts":
		prof := cfg.Profiles["minimax_tts"]
		b.TTS = tts.NewMiniMax(tts.MiniMaxConfig{
			BaseURL: firstNonEmpty(prof.BaseURL, os.Getenv("MINIMAX_TTS_URL")),
			GroupID: firstNonEmpty(prof.GroupID, os.Getenv("MINIMAX_GROUP_ID")),
			APIKey:  firstNonEmpty(prof.APIKey, os.Getenv("MINIMAX_API_KEY")),
			Model:   firstNonEmpty(prof.Model, os.Getenv("MINIMAX_TTS_MODEL")),
			VoiceID: firstNonEmpty(prof.VoiceID, os.Getenv("MINIMAX_VOICE_ID")),
		})
	case "elevenlabs":
		prof := cfg.Profiles["elevenlabs"]
		b.TTS = tts.NewElevenLabs(tts.ElevenLabsConfig{
			BaseURL: firstNonEmpty(prof.BaseURL, os.Getenv("ELEVENLABS_BASE_URL")),
			APIKey:  firstNonEmpty(prof.APIKey, os.Getenv("ELEVENLABS_API_KEY")),
			ModelID: firstNonEmpty(prof.Model, os.Getenv("ELEVENLABS_MODEL")),
			VoiceID: firstNonEmpty(prof.VoiceID, os.Getenv("ELEVENLABS_VOICE_ID")),
		})
	case "macos_say":
		b.TTS = mock.NewLocalTTS(true)
	default:
		b.TTS = mock.NewLocalTTS(false)
		b.ActiveIDs.TTS = "mock"
	}

	// 5. SFX
	b.SFX = mock.NewLocalSFX(filepath.Join(rootDir, "assets", "sfx"))
	b.ActiveIDs.SFX = "mock"
	return b
}

// RecommendedPromptMode 根据当前激活的 I2V 供应商推荐最契合的提示词编译模式。
func RecommendedPromptMode(i2vID string) prompting.PromptMode {
	switch strings.ToLower(i2vID) {
	case "runway", "luma", "minimax_hailuo":
		return prompting.ModeConcise
	default:
		return prompting.ModeDirector
	}
}

func applyEnvOverrides(cfg *settings.ProvidersConfig) {
	if v := strings.TrimSpace(os.Getenv("FEELINGS_LLM_PROVIDER")); v != "" {
		cfg.Active.LLM = v
	}
	if v := strings.TrimSpace(os.Getenv("FEELINGS_T2I_PROVIDER")); v != "" {
		cfg.Active.T2I = v
	}
	if v := strings.TrimSpace(os.Getenv("FEELINGS_I2V_PROVIDER")); v != "" {
		cfg.Active.I2V = v
	}
	if v := strings.TrimSpace(os.Getenv("FEELINGS_TTS_PROVIDER")); v != "" {
		cfg.Active.TTS = v
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
