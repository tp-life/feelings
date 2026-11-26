package prompting

import "strings"

type Lang string

const (
	LangZH Lang = "zh"
	LangEN Lang = "en"
)

type PromptMode string

const (
	ModeDirector PromptMode = "director" // 工业级导演时序详版（适配即梦 Seedance / 可灵 2.x / 万相 Wan / Veo）
	ModeConcise  PromptMode = "concise"  // 精炼电影单段版（适配 Runway / Luma / MiniMax 海螺 / 轻量模型）
)

// BilingualText 保存中英双语文本，中文面向创作与国内模型，英文面向海外模型。
type BilingualText struct {
	ZH string `yaml:"zh" json:"zh"`
	EN string `yaml:"en" json:"en"`
}

func (b BilingualText) Get(lang Lang) string {
	if lang == LangEN {
		if strings.TrimSpace(b.EN) != "" {
			return strings.TrimSpace(b.EN)
		}
		return strings.TrimSpace(b.ZH)
	}
	if strings.TrimSpace(b.ZH) != "" {
		return strings.TrimSpace(b.ZH)
	}
	return strings.TrimSpace(b.EN)
}

type TransitionMode string

const (
	TransitionSingleFrame    TransitionMode = "single_frame"
	TransitionFirstLastFrame TransitionMode = "first_last_frame"
)

// T2IPromptSpec 结构化首帧生图提示词槽位。
type T2IPromptSpec struct {
	SubjectAnchor  BilingualText `yaml:"subject_anchor" json:"subject_anchor"`
	ActionRelation BilingualText `yaml:"action_relation" json:"action_relation"`
	EraEnvironment BilingualText `yaml:"era_environment" json:"era_environment"`
	ShotFraming    BilingualText `yaml:"shot_framing" json:"shot_framing"`
	LightingMood   BilingualText `yaml:"lighting_mood" json:"lighting_mood"`
	StyleOverride  BilingualText `yaml:"style_override,omitempty" json:"style_override,omitempty"`
	Negative       BilingualText `yaml:"negative" json:"negative"`
}

// I2VPromptSpec 结构化图生视频动态提示词槽位（精炼版核心数据源）。
type I2VPromptSpec struct {
	SubjectMicroAction BilingualText  `yaml:"subject_micro_action" json:"subject_micro_action"`
	EnvironmentPhysics BilingualText  `yaml:"environment_physics" json:"environment_physics"`
	CameraMovement     string         `yaml:"camera_movement" json:"camera_movement"`
	CameraDetail       BilingualText  `yaml:"camera_detail" json:"camera_detail"`
	LightingPacing     BilingualText  `yaml:"lighting_pacing" json:"lighting_pacing"`
	TransitionMode     TransitionMode `yaml:"transition_mode" json:"transition_mode"`
	TransitionHint     BilingualText  `yaml:"transition_hint,omitempty" json:"transition_hint,omitempty"`
	Negative           BilingualText  `yaml:"negative" json:"negative"`
}

// AssetBinding 显式声明某角色/场景/道具绑定的参考资产图及使用边界（防止多图模型串脸、照抄姿势或数量翻倍）。
type AssetBinding struct {
	AssetID     string        `yaml:"asset_id,omitempty" json:"asset_id,omitempty"`         // 对应 Project.Assets[].ID（如 a1, a2）
	Role        string        `yaml:"role" json:"role"`                                     // character | scene | prop
	AssetName   string        `yaml:"asset_name" json:"asset_name"`                         // 例如：主角 / 核心场景
	RefToken    string        `yaml:"ref_token" json:"ref_token"`                           // 例如：{{Asset 1}} 或 "不使用参考图（纯文字锚定）"
	UsageRule   BilingualText `yaml:"usage_rule" json:"usage_rule"`                         // 只参考外观/服装/材质，不参考角度、构图、站姿
	ExcludeHint BilingualText `yaml:"exclude_hint,omitempty" json:"exclude_hint,omitempty"` // 勿换成 {{Asset 2}}
}

// TimelineBeat 单镜内部的秒级微节拍（情怀片默认 2~3 段呼吸节拍，可自由增删至任意多段）。
type TimelineBeat struct {
	StartSec     float64       `yaml:"start_sec" json:"start_sec"`
	EndSec       float64       `yaml:"end_sec" json:"end_sec"`
	Title        BilingualText `yaml:"title" json:"title"`
	Visual       BilingualText `yaml:"visual" json:"visual"`
	Dialogue     BilingualText `yaml:"dialogue" json:"dialogue"`     // 默认 "无" / "None"，防止 AI 乱动嘴型
	Forbid       BilingualText `yaml:"forbid" json:"forbid"`         // 本段局部禁止项
	Constraint   BilingualText `yaml:"constraint" json:"constraint"` // 本段物理/构图/数量不变量
	Staging      BilingualText `yaml:"staging,omitempty" json:"staging,omitempty"`
	Camera       BilingualText `yaml:"camera" json:"camera"`
	SFX          BilingualText `yaml:"sfx" json:"sfx"`
	PsychMapping string        `yaml:"psych_mapping,omitempty" json:"psych_mapping,omitempty"` // 对应心理锚点（仅输出在后期参考区）
}

// TransitionDesignSpec 后期剪辑转场参考（与视频生成指令物理隔离）。
type TransitionDesignSpec struct {
	FromPrev string `yaml:"from_prev,omitempty" json:"from_prev,omitempty"`
	ToNext   string `yaml:"to_next,omitempty" json:"to_next,omitempty"`
	ToOutro  string `yaml:"to_outro,omitempty" json:"to_outro,omitempty"`
}

// PostProductionNotes 严格隔离于视频生成指令之外的后期文字与剪辑指导（避免模型在画面里生成乱码汉字）。
type PostProductionNotes struct {
	SubtitleStrategy string               `yaml:"subtitle_strategy,omitempty" json:"subtitle_strategy,omitempty"`
	SubtitleRhythm   []string             `yaml:"subtitle_rhythm,omitempty" json:"subtitle_rhythm,omitempty"`
	TransitionDesign TransitionDesignSpec `yaml:"transition_design,omitempty" json:"transition_design,omitempty"`
}

// DirectorSheetSpec 工业级导演时序分镜结构（融合资产锚定、参考图规则、首尾帧衔接、秒级微节拍、硬性输出约束与隔离的后期元数据）。
type DirectorSheetSpec struct {
	ShotTitle         BilingualText       `yaml:"shot_title,omitempty" json:"shot_title,omitempty"`
	AssetBindings     []AssetBinding      `yaml:"asset_bindings,omitempty" json:"asset_bindings,omitempty"`
	CastSummary       BilingualText       `yaml:"cast_summary,omitempty" json:"cast_summary,omitempty"`
	BackgroundSetting BilingualText       `yaml:"background_setting,omitempty" json:"background_setting,omitempty"`
	ContinuitySummary BilingualText       `yaml:"continuity_summary,omitempty" json:"continuity_summary,omitempty"`
	FirstFrameLink    BilingualText       `yaml:"first_frame_link,omitempty" json:"first_frame_link,omitempty"`
	LastFrameLink     BilingualText       `yaml:"last_frame_link,omitempty" json:"last_frame_link,omitempty"`
	TimelineBeats     []TimelineBeat      `yaml:"timeline_beats,omitempty" json:"timeline_beats,omitempty"`
	OutputConstraints []BilingualText     `yaml:"output_constraints,omitempty" json:"output_constraints,omitempty"`
	PostProduction    PostProductionNotes `yaml:"post_production,omitempty" json:"post_production,omitempty"`
}

// StylePreset 视觉风格预设（同时驱动提示词美学词缀与 FFmpeg 滤镜）。
type StylePreset struct {
	ID             string        `yaml:"id" json:"id"`
	Name           string        `yaml:"name" json:"name"`
	Description    string        `yaml:"description" json:"description"`
	T2ISuffix      BilingualText `yaml:"t2i_suffix" json:"t2i_suffix"`
	I2VSuffix      BilingualText `yaml:"i2v_suffix" json:"i2v_suffix"`
	NegativeStatic BilingualText `yaml:"negative_static" json:"negative_static"`
	NegativeVideo  BilingualText `yaml:"negative_video" json:"negative_video"`
	FFmpegFilter   string        `yaml:"ffmpeg_filter" json:"ffmpeg_filter"`
	GrainStrength  int           `yaml:"grain_strength" json:"grain_strength"`
}

type CompiledPrompt struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt"`
}

// CompiledDirectorSheet 同时提供“纯净生成指令（直接喂给 API，不含后期压字说明）”和“完整导演稿（含后期字幕与转场参考）”。
type CompiledDirectorSheet struct {
	GenerationPrompt string `json:"generation_prompt"`
	PostNotes        string `json:"post_notes"`
	FullSheet        string `json:"full_sheet"`
	NegativePrompt   string `json:"negative_prompt"`
}

type CompiledShotPrompts struct {
	ShotID        string                `json:"shot_id"`
	T2IZH         CompiledPrompt        `json:"t2i_zh"`
	T2IEN         CompiledPrompt        `json:"t2i_en"`
	I2VZH         CompiledPrompt        `json:"i2v_zh"`
	I2VEN         CompiledPrompt        `json:"i2v_en"`
	I2VDirectorZH CompiledDirectorSheet `json:"i2v_director_zh"`
	I2VDirectorEN CompiledDirectorSheet `json:"i2v_director_en"`
}

type SystemPromptBundle struct {
	StoryOutline       string `json:"story_outline"`
	DirectorStoryboard string `json:"director_storyboard"`
	AssetExtraction    string `json:"asset_extraction"`
	T2ISystem          string `json:"t2i_system"`
	I2VSystem          string `json:"i2v_system"`
}
