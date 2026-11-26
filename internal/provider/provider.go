package provider

import (
	"context"
	"errors"
	"time"

	"feelings-studio/internal/prompting"
	"feelings-studio/internal/psychology"
)

// ProviderError 区分可重试错误（如 429/503）与致命错误（如鉴权失败/提示词违规）
type ProviderError struct {
	Provider  string
	Code      string
	Message   string
	Retryable bool
	Err       error
}

func (e *ProviderError) Error() string {
	if e.Code != "" {
		return e.Provider + " [" + e.Code + "]: " + e.Message
	}
	return e.Provider + ": " + e.Message
}

func (e *ProviderError) Unwrap() error { return e.Err }

func IsRetryable(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.Retryable
}

// --- 1. LLM：社会心理学整体故事创作 -> 剧情拆镜 -> 资产反推与导演详版 ---

type StoryActDraft struct {
	Phase   psychology.ActPhase `json:"phase"`
	Title   string              `json:"title"`
	Summary string              `json:"summary"`
}

type StoryDraft struct {
	Logline             string          `json:"logline"`
	Synopsis            string          `json:"synopsis"`
	Acts                []StoryActDraft `json:"acts,omitempty"`
	EmotionalTone       string          `json:"emotional_tone,omitempty"`
	VisualAtmosphere    string          `json:"visual_atmosphere,omitempty"`
	SuggestedShotReason string          `json:"suggested_shot_reason,omitempty"`
}

type StoryRequest struct {
	ProjectID    string
	Title        string
	Theme        string
	Track        psychology.TrackID
	StylePreset  string
	TargetPacing string // auto | short | standard
	SystemPrompt string
}

type StoryboardRequest struct {
	ProjectID         string
	Title             string
	Theme             string            // 例如：“离家前父母塞满的后备箱”
	Track             psychology.TrackID
	StylePreset       string
	TargetPacing      string            // auto | short | standard
	Story             *StoryDraft       // Step 1 人工确认后的整体故事（驱动拆镜与镜头数决策）
	SystemPrompt      string            // 编译好的导演级系统提示词
	RevisionFeedback  string            // 若上一轮未通过 PsychologyCritic，则携带修正指令
	Assets            map[string]string // 资产名 -> 描述或路径，用于约束角色一致性
	ExistingShotIDs   []string          // 若已导入现成首帧图（如 g1~g10），沿用其 ShotID
	HasImportedFrames bool              // 标记是否导入了现成首帧图
	ShotCount         int               // <=0 表示由整体故事剧情自然决定镜头数
	StyleHints        string
}

// AssetDraft 由剧本大脑规划出的项目级一致性资产（角色 / 场景 / 核心道具）。
type AssetDraft struct {
	ID          string                  `json:"id"`
	Role        string                  `json:"role"` // character | scene | prop
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Prompt      prompting.BilingualText `json:"prompt,omitempty"`
}

// StoryboardPlan 包含项目级一致性资产清单与结构化分镜草案。
type StoryboardPlan struct {
	Assets []AssetDraft `json:"assets,omitempty"`
	Shots  []ShotDraft  `json:"shots"`
}

type ShotDraft struct {
	ShotID        string                      `json:"shot_id"`
	Phase         psychology.ActPhase         `json:"phase"`
	StorySummary  string                      `json:"story_summary,omitempty"` // Step 1 故事板画面故事概述
	PsychTrigger  string                      `json:"psych_trigger"`
	Symbols       []string                    `json:"symbols,omitempty"`
	Narration     string                      `json:"narration"`     // 旁白文案
	MotionPrompt  string                      `json:"motion_prompt"` // 兼容简写：图生视频运镜/动作提示词
	Emotion       string                      `json:"emotion"`       // 供 TTS 参考的情绪标签
	AmbientTag    string                      `json:"ambient_tag"`   // 环境声景标签（普鲁斯特感官线索）
	DurationSec   int                         `json:"duration_sec"`
	T2ISpec       prompting.T2IPromptSpec     `json:"t2i_spec"`
	I2VSpec       prompting.I2VPromptSpec     `json:"i2v_spec"`
	DirectorSheet prompting.DirectorSheetSpec `json:"director_sheet,omitempty"`
}

// ConfirmedShotBrief 是 Step 1 人工确认后的单镜故事板摘要，作为 Step 2 反推资产与导演详版的输入。
type ConfirmedShotBrief struct {
	ShotID       string              `json:"shot_id"`
	Order        int                 `json:"order"`
	Phase        psychology.ActPhase `json:"phase"`
	StorySummary string              `json:"story_summary"`
	PsychTrigger string              `json:"psych_trigger"`
	Symbols      []string            `json:"symbols,omitempty"`
	Narration    string              `json:"narration"`
	Emotion      string              `json:"emotion"`
	AmbientTag   string              `json:"ambient_tag"`
	DurationSec  int                 `json:"duration_sec"`
}

// AssetExtractionRequest Step 2 请求：基于人工确认后的故事板决定所需资产，并补全每镜的绑定与导演详版。
type AssetExtractionRequest struct {
	ProjectID      string
	Title          string
	Theme          string
	Track          psychology.TrackID
	StylePreset    string
	SystemPrompt   string
	ExistingAssets []AssetDraft
	ConfirmedShots []ConfirmedShotBrief
}

// ShotDirectorEnrichment Step 2 为单个已确认分镜补全的资产绑定、首帧生图槽位与导演时序详版。
type ShotDirectorEnrichment struct {
	ShotID        string                      `json:"shot_id"`
	T2ISpec       prompting.T2IPromptSpec     `json:"t2i_spec"`
	I2VSpec       prompting.I2VPromptSpec     `json:"i2v_spec"`
	DirectorSheet prompting.DirectorSheetSpec `json:"director_sheet"`
}

type AssetExtractionResult struct {
	Assets []AssetDraft             `json:"assets"`
	Shots  []ShotDirectorEnrichment `json:"shots"`
}

type LLMProvider interface {
	Name() string
	GenerateStory(ctx context.Context, req StoryRequest) (*StoryDraft, error)
	GenerateStoryboard(ctx context.Context, req StoryboardRequest) (*StoryboardPlan, error)
	ExtractAssetsFromStoryboard(ctx context.Context, req AssetExtractionRequest) (*AssetExtractionResult, error)
}

// --- 2. T2I：角色一致性分镜首帧生图接口（全链路可选阶段） ---

type T2IRequest struct {
	Prompt          string
	NegativePrompt  string
	ReferenceImages []string // 绑定角色/场景资产图以保持外观一致
	AspectRatio     string   // 例如 "9:16"
	OutputPath      string
	Seed            int64
}

type T2IProvider interface {
	Name() string
	Generate(ctx context.Context, req T2IRequest) (outputPath string, err error)
}

// --- 3. I2V：异步图生视频接口（支持单首帧与首尾帧过渡） ---

type I2VSubmitRequest struct {
	FirstFramePath  string                   // 首帧分镜图（如 g1.jpeg）
	LastFramePath   string                   // 可选：尾帧图，用于首尾帧平滑过渡
	ReferenceImages []string                 // 可选：该分镜显式绑定的角色/场景/道具资产参考图（供多参考图模型使用）
	Prompt          string                   // 编译后的动态运镜与微动作提示词
	NegativePrompt  string                   // 动态负向提示词
	TransitionMode  prompting.TransitionMode // single_frame 或 first_last_frame
	CameraMovement  string                   // 结构化运镜标识（如 slow_push_in）
	PromptLang      prompting.Lang           // zh 或 en
	AspectRatio     string                   // 例如 "9:16"
	DurationSec     int                      // 例如 5 或 10
	Seed            int64
}

type TaskState string

const (
	TaskQueued    TaskState = "queued"
	TaskRunning   TaskState = "running"
	TaskSucceeded TaskState = "succeeded"
	TaskFailed    TaskState = "failed"
)

type I2VStatus struct {
	RemoteID string
	State    TaskState
	VideoURL string // 支持 http(s):// 或本地 file:// 路径
	ErrorMsg string
}

type I2VProvider interface {
	Name() string
	Submit(ctx context.Context, req I2VSubmitRequest) (remoteID string, err error)
	Poll(ctx context.Context, remoteID string) (*I2VStatus, error)
}

// --- 4. TTS：情感旁白合成接口 ---

type TTSRequest struct {
	Text       string
	VoiceID    string  // 例如温柔怀旧男声/女声
	Emotion    string  // 例如 "nostalgic", "warm", "calm"
	Speed      float64 // 语速倍率，情怀短片常用 0.88 - 0.95
	OutputPath string
}

type SubtitleCue struct {
	Text  string        `json:"text"`
	Start time.Duration `json:"start"`
	End   time.Duration `json:"end"`
}

type TTSResult struct {
	AudioPath string
	Duration  time.Duration
	Cues      []SubtitleCue // 词/句级时间戳，直接驱动 ASS 字幕对齐
}

type TTSProvider interface {
	Name() string
	Synthesize(ctx context.Context, req TTSRequest) (*TTSResult, error)
}

// --- 5. SFX：环境声景合成/检索接口 ---

type SFXRequest struct {
	Tag         string // 例如 summer_cicada / old_freezer_hum / alley_breeze / evening_crickets
	DurationSec int
	OutputPath  string
}

type SFXProvider interface {
	Name() string
	Generate(ctx context.Context, req SFXRequest) (audioPath string, err error)
}
