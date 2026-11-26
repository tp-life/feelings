package project

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"feelings-studio/internal/prompting"
	"feelings-studio/internal/psychology"
)

// WorkflowStage 表示情怀短片 6 步人机协同制作流水线的当前所处阶段。
type WorkflowStage string

const (
	StageStory        WorkflowStage = "story"         // ① 创作整体故事剧情（人工确认）
	StageStoryboard   WorkflowStage = "storyboard"    // ② 按剧情拆解故事板分镜（人工确认）
	StageAssetsPlan   WorkflowStage = "assets_plan"   // ③ 根据故事板决定资产清单
	StageAssetsRender WorkflowStage = "assets_render" // ④ 生成/上传资产定妆图
	StageKeyframes    WorkflowStage = "keyframes"     // ⑤ 生成分镜定妆图（首帧图，可跳过）
	StageVideo        WorkflowStage = "video"         // ⑥ 生成视频与声景成片
)

// StoryActBeat 描述整体故事在社会心理学 4 幕情绪弧线中的单幕推进大纲。
type StoryActBeat struct {
	Phase   psychology.ActPhase `yaml:"phase" json:"phase"`
	Title   string              `yaml:"title" json:"title"`
	Summary string              `yaml:"summary" json:"summary"`
}

// StoryScript 描述 Step 1 先行创作的完整电影感故事剧情（先定整体故事，再由剧情自然决定分镜数）。
type StoryScript struct {
	Logline             string         `yaml:"logline" json:"logline"`
	Synopsis            string         `yaml:"synopsis" json:"synopsis"`
	Acts                []StoryActBeat `yaml:"acts,omitempty" json:"acts,omitempty"`
	EmotionalTone       string         `yaml:"emotional_tone,omitempty" json:"emotional_tone,omitempty"`
	VisualAtmosphere    string         `yaml:"visual_atmosphere,omitempty" json:"visual_atmosphere,omitempty"`
	SuggestedShotReason string         `yaml:"suggested_shot_reason,omitempty" json:"suggested_shot_reason,omitempty"`
}

// AssetRef 描述一个角色、场景或关键道具的一致性锚点（如 {{Asset 1}} 主角 / 核心场景）。
type AssetRef struct {
	ID          string                  `yaml:"id" json:"id"`
	Role        string                  `yaml:"role" json:"role"`                               // character | scene | prop
	Name        string                  `yaml:"name" json:"name"`
	ImagePath   string                  `yaml:"image_path,omitempty" json:"image_path,omitempty"`
	Description string                  `yaml:"description" json:"description"`
	Prompt      prompting.BilingualText `yaml:"prompt,omitempty" json:"prompt,omitempty"`       // 资产定妆图 T2I 提示词
	SourceMode  string                  `yaml:"source_mode,omitempty" json:"source_mode,omitempty"` // generated | uploaded | imported | text_only
	Locked      bool                    `yaml:"locked,omitempty" json:"locked,omitempty"`
}

// AudioMixConfig 控制三轨感官混音参数、字幕压制与胶片质感强度。
type AudioMixConfig struct {
	VoiceVolume   float64 `yaml:"voice_volume" json:"voice_volume"`
	AmbientVolume float64 `yaml:"ambient_volume" json:"ambient_volume"`
	BGMVolume     float64 `yaml:"bgm_volume" json:"bgm_volume"`
	BGMPreset     string  `yaml:"bgm_preset,omitempty" json:"bgm_preset,omitempty"` // warm_pad | tape_piano | acoustic_breeze | music_box | custom | none
	BGMPath       string  `yaml:"bgm_path,omitempty" json:"bgm_path,omitempty"`
	GrainStrength int     `yaml:"grain_strength" json:"grain_strength"`
	CrossfadeSec  float64 `yaml:"crossfade_sec" json:"crossfade_sec"`
	BurnSubtitles *bool   `yaml:"burn_subtitles,omitempty" json:"burn_subtitles,omitempty"` // 默认 true：硬压暖白电影字幕
	GenerateCover *bool   `yaml:"generate_cover,omitempty" json:"generate_cover,omitempty"` // 默认 true：自动生成 cover.jpg 封面
}

func (m AudioMixConfig) ShouldBurnSubtitles() bool {
	if m.BurnSubtitles == nil {
		return true
	}
	return *m.BurnSubtitles
}

func (m AudioMixConfig) ShouldGenerateCover() bool {
	if m.GenerateCover == nil {
		return true
	}
	return *m.GenerateCover
}

// ShotSpec 是单个分镜在 project.yaml 中的完整人机协同定义。
type ShotSpec struct {
	ShotID         string                      `yaml:"shot_id" json:"shot_id"`
	Order          int                         `yaml:"order" json:"order"`
	Phase          psychology.ActPhase         `yaml:"phase" json:"phase"`
	StorySummary   string                      `yaml:"story_summary,omitempty" json:"story_summary,omitempty"` // Step 1 人工确认的故事板画面概要
	PsychTrigger   string                      `yaml:"psych_trigger" json:"psych_trigger"`
	Symbols        []string                    `yaml:"symbols,omitempty" json:"symbols,omitempty"`
	FirstFramePath string                      `yaml:"first_frame_path,omitempty" json:"first_frame_path,omitempty"`
	LastFramePath  string                      `yaml:"last_frame_path,omitempty" json:"last_frame_path,omitempty"`
	Narration      string                      `yaml:"narration" json:"narration"`
	Emotion        string                      `yaml:"emotion" json:"emotion"`
	VoiceID        string                      `yaml:"voice_id" json:"voice_id"`
	VoiceSpeed     float64                     `yaml:"voice_speed" json:"voice_speed"`
	AmbientTag     string                      `yaml:"ambient_tag" json:"ambient_tag"`
	DurationSec    int                         `yaml:"duration_sec" json:"duration_sec"`
	Locked         bool                        `yaml:"locked" json:"locked"`
	T2ISpec        prompting.T2IPromptSpec     `yaml:"t2i_spec" json:"t2i_spec"`
	I2VSpec        prompting.I2VPromptSpec     `yaml:"i2v_spec" json:"i2v_spec"`
	DirectorSheet  prompting.DirectorSheetSpec `yaml:"director_sheet,omitempty" json:"director_sheet,omitempty"`
}

// Project 是人机协同工作台的唯一事实源（projects/<id>/project.yaml）。
type Project struct {
	ID                  string                   `yaml:"id" json:"id"`
	Title               string                   `yaml:"title" json:"title"`
	Theme               string                   `yaml:"theme" json:"theme"`
	Track               psychology.TrackID       `yaml:"track" json:"track"`
	StylePreset         string                   `yaml:"style_preset" json:"style_preset"`
	PromptLang          prompting.Lang           `yaml:"prompt_lang" json:"prompt_lang"`
	PromptMode          prompting.PromptMode     `yaml:"prompt_mode,omitempty" json:"prompt_mode,omitempty"`
	AspectRatio         string                   `yaml:"aspect_ratio" json:"aspect_ratio"`                         // 9:16 | 16:9 | 3:4
	NarratorVoice       string                   `yaml:"narrator_voice,omitempty" json:"narrator_voice,omitempty"` // warm-male | healing-female | childhood-boy | calm-senior
	NarratorSpeed       float64                  `yaml:"narrator_speed,omitempty" json:"narrator_speed,omitempty"` // 默认 0.92
	TargetPacing        string                   `yaml:"target_pacing,omitempty" json:"target_pacing,omitempty"`   // auto | short | standard
	ShotCount           int                      `yaml:"shot_count,omitempty" json:"shot_count,omitempty"`       // 实际拆镜后的分镜数（不再要求建项前硬填）
	Stage               WorkflowStage            `yaml:"stage,omitempty" json:"stage,omitempty"`
	Story               *StoryScript             `yaml:"story,omitempty" json:"story,omitempty"`
	StoryConfirmed      bool                     `yaml:"story_confirmed,omitempty" json:"story_confirmed,omitempty"`
	StoryboardConfirmed bool                     `yaml:"storyboard_confirmed,omitempty" json:"storyboard_confirmed,omitempty"`
	AssetsConfirmed     bool                     `yaml:"assets_confirmed,omitempty" json:"assets_confirmed,omitempty"`
	KeyframesSkipped    bool                     `yaml:"keyframes_skipped,omitempty" json:"keyframes_skipped,omitempty"`
	Assets              []AssetRef               `yaml:"assets,omitempty" json:"assets,omitempty"`
	Mix                 AudioMixConfig           `yaml:"mix" json:"mix"`
	Shots               []ShotSpec               `yaml:"shots,omitempty" json:"shots,omitempty"`
	CriticReport        *psychology.CriticReport `yaml:"critic_report,omitempty" json:"critic_report,omitempty"`
	CreatedAt           time.Time                `yaml:"created_at" json:"created_at"`
	UpdatedAt           time.Time                `yaml:"updated_at" json:"updated_at"`
}

func DefaultMixConfig() AudioMixConfig {
	burn := true
	cover := true
	return AudioMixConfig{
		VoiceVolume:   1.0,
		AmbientVolume: 0.34,
		BGMVolume:     0.22,
		BGMPreset:     "warm_pad",
		GrainStrength: 5,
		CrossfadeSec:  0.40,
		BurnSubtitles: &burn,
		GenerateCover: &cover,
	}
}

// NormalizeAspectRatio 规范化画幅比例（支持 9:16 竖屏、16:9 横屏、3:4 小红书竖屏）。
func NormalizeAspectRatio(ar string) string {
	switch strings.TrimSpace(ar) {
	case "16:9":
		return "16:9"
	case "3:4":
		return "3:4"
	default:
		return "9:16"
	}
}

// ResolveDimensions 返回指定画幅对应的标准高清像素宽高。
func ResolveDimensions(aspectRatio string) (int, int) {
	switch NormalizeAspectRatio(aspectRatio) {
	case "16:9":
		return 1920, 1080
	case "3:4":
		return 1080, 1440
	default:
		return 1080, 1920
	}
}

type VoiceOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func BuiltInVoices() []VoiceOption {
	return []VoiceOption{
		{ID: "warm-male", Name: "温柔怀旧男声（默认）", Description: "低沉温暖、克制松弛的成年回望独白"},
		{ID: "healing-female", Name: "治愈叙事女声", Description: "轻柔细腻、适合都市避风港与散文式慢叙事"},
		{ID: "childhood-boy", Name: "清澈少年独白", Description: "干净质朴的第一人称少年回忆视角"},
		{ID: "calm-senior", Name: "沉稳岁月旁白", Description: "厚重平实、适合家庭牵挂与年代纪实"},
	}
}

type BGMPresetOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func BuiltInBGMPresets() []BGMPresetOption {
	return []BGMPresetOption{
		{ID: "warm_pad", Name: "温暖胶片弦垫（默认）", Description: "柔和 C大九和弦低通铺底，完全不抢人声"},
		{ID: "tape_piano", Name: "旧磁带电钢泛音", Description: "微带磁带温暖质感的四和弦慢速呼吸推进"},
		{ID: "acoustic_breeze", Name: "午后木吉他泛音", Description: "清透五度空弦泛音与微风律动，适合夏日与童年"},
		{ID: "music_box", Name: "童年八音盒余韵", Description: "晶莹轻柔的怀旧八音盒泛音与空间回响"},
		{ID: "none", Name: "无背景配乐（纯声景）", Description: "仅保留旁白与真实环境声景，极致纪实留白"},
		{ID: "custom", Name: "自定义上传配乐", Description: "使用本地上传或指定路径的 MP3/WAV/M4A 音乐"},
	}
}

// NormalizeAssets 为每个资产补全稳定的 ID、Role 与 SourceMode，便于前后端按唯一标识管理资产引用。
func (p *Project) NormalizeAssets() {
	used := make(map[string]struct{}, len(p.Assets))
	for i := range p.Assets {
		a := &p.Assets[i]
		if strings.TrimSpace(a.ID) == "" {
			seq := i + 1
			for {
				cand := fmt.Sprintf("a%d", seq)
				if _, exists := used[cand]; !exists {
					a.ID = cand
					break
				}
				seq++
			}
		}
		used[a.ID] = struct{}{}
		if strings.TrimSpace(a.Role) == "" {
			a.Role = inferAssetRole(a.Name)
		}
		if strings.TrimSpace(a.SourceMode) == "" {
			if strings.TrimSpace(a.ImagePath) != "" {
				a.SourceMode = "imported"
			} else {
				a.SourceMode = "generated"
			}
		}
	}
}

// AssetRefToken 返回第 idx 个资产的标准导演引用编号（如 {{Asset 1}}）。
func AssetRefToken(idx int) string {
	return fmt.Sprintf("{{Asset %d}}", idx+1)
}

// SyncShotAssetBindings 将所有分镜中的 asset_bindings 与项目资产库对齐：
// 1) 自动同步最新编号 {{Asset N}}、资产名称与类型；
// 2) 自动清理指向不存在资产的“幽灵引用”，防止旧项目或已删除资产串味。
func (p *Project) SyncShotAssetBindings() {
	p.NormalizeAssets()
	byID := make(map[string]int, len(p.Assets))
	byName := make(map[string]int, len(p.Assets))
	for i, a := range p.Assets {
		byID[a.ID] = i
		byName[strings.TrimSpace(a.Name)] = i
	}

	for si := range p.Shots {
		bindings := p.Shots[si].DirectorSheet.AssetBindings
		if len(bindings) == 0 {
			continue
		}
		cleaned := make([]prompting.AssetBinding, 0, len(bindings))
		for _, b := range bindings {
			idx := -1
			if b.AssetID != "" {
				if pos, ok := byID[b.AssetID]; ok {
					idx = pos
				}
			}
			if idx < 0 && strings.TrimSpace(b.AssetName) != "" {
				if pos, ok := byName[strings.TrimSpace(b.AssetName)]; ok {
					idx = pos
				}
			}
			if idx < 0 {
				// 明确声明不使用参考图的纯文字锚定予以保留；幽灵 {{Asset N}} 引用一律剔除
				if !strings.Contains(b.RefToken, "{{Asset") && strings.TrimSpace(b.AssetName) != "" {
					cleaned = append(cleaned, b)
				}
				continue
			}
			a := p.Assets[idx]
			b.AssetID = a.ID
			b.AssetName = a.Name
			if a.Role != "" {
				b.Role = a.Role
			}
			if a.SourceMode == "text_only" {
				b.RefToken = "不使用参考图（纯文字锚定）"
			} else {
				b.RefToken = AssetRefToken(idx)
			}
			cleaned = append(cleaned, b)
		}
		p.Shots[si].DirectorSheet.AssetBindings = cleaned
	}
}

// ResolveShotReferenceImages 严格返回当前单个分镜显式绑定且拥有本地图片文件的参考资产图路径，
// 避免把全项目所有无关资产图盲目传给每一镜。
func (p *Project) ResolveShotReferenceImages(sh ShotSpec) []string {
	p.NormalizeAssets()
	if len(sh.DirectorSheet.AssetBindings) == 0 {
		return nil
	}
	byID := make(map[string]AssetRef, len(p.Assets))
	byName := make(map[string]AssetRef, len(p.Assets))
	for _, a := range p.Assets {
		byID[a.ID] = a
		byName[strings.TrimSpace(a.Name)] = a
	}
	seen := make(map[string]struct{})
	var paths []string
	for _, b := range sh.DirectorSheet.AssetBindings {
		a, ok := byID[b.AssetID]
		if !ok {
			a, ok = byName[strings.TrimSpace(b.AssetName)]
		}
		if !ok || a.SourceMode == "text_only" || strings.TrimSpace(a.ImagePath) == "" {
			continue
		}
		if _, dup := seen[a.ImagePath]; dup {
			continue
		}
		if _, err := os.Stat(a.ImagePath); err == nil {
			seen[a.ImagePath] = struct{}{}
			paths = append(paths, a.ImagePath)
		}
	}
	return paths
}

func (p *Project) AssetDescriptionMap() map[string]string {
	p.NormalizeAssets()
	m := make(map[string]string, len(p.Assets))
	for i, a := range p.Assets {
		desc := strings.TrimSpace(a.Description)
		if desc == "" && a.ImagePath != "" {
			desc = "参考资产图：" + a.ImagePath
		}
		token := AssetRefToken(i)
		if a.SourceMode == "text_only" {
			token = "纯文字锚定"
		}
		key := fmt.Sprintf("%s %s（%s）", token, a.Name, a.Role)
		m[key] = desc
	}
	return m
}

func (p *Project) AssetImagePaths() []string {
	out := make([]string, 0, len(p.Assets))
	for _, a := range p.Assets {
		if a.SourceMode != "text_only" && a.ImagePath != "" {
			out = append(out, a.ImagePath)
		}
	}
	return out
}

func (p *Project) ToShotsForReview() []psychology.ShotForReview {
	out := make([]psychology.ShotForReview, 0, len(p.Shots))
	for _, s := range p.Shots {
		out = append(out, psychology.ShotForReview{
			ShotID:         s.ShotID,
			Phase:          s.Phase,
			PsychTrigger:   s.PsychTrigger,
			Symbols:        s.Symbols,
			Narration:      s.Narration,
			DurationSec:    s.DurationSec,
			AmbientTag:     s.AmbientTag,
			MotionActionZH: s.I2VSpec.SubjectMicroAction.ZH,
			EnvPhysicsZH:   s.I2VSpec.EnvironmentPhysics.ZH,
			CameraMovement: s.I2VSpec.CameraMovement,
		})
	}
	return out
}

func ProjectDir(rootDir, projectID string) string {
	return filepath.Join(rootDir, "projects", projectID)
}

func ManifestPath(rootDir, projectID string) string {
	return filepath.Join(ProjectDir(rootDir, projectID), "project.yaml")
}

func OutputDir(rootDir, projectID string) string {
	return filepath.Join(ProjectDir(rootDir, projectID), "output")
}

func Save(rootDir string, p *Project) error {
	if p.ID == "" {
		return fmt.Errorf("project id is required")
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	p.AspectRatio = NormalizeAspectRatio(p.AspectRatio)
	if p.NarratorVoice == "" {
		p.NarratorVoice = "warm-male"
	}
	if p.NarratorSpeed <= 0 {
		p.NarratorSpeed = 0.92
	}
	if p.Mix.BGMPreset == "" {
		if strings.TrimSpace(p.Mix.BGMPath) != "" {
			p.Mix.BGMPreset = "custom"
		} else {
			p.Mix.BGMPreset = "warm_pad"
		}
	}
	if p.PromptLang == "" {
		p.PromptLang = prompting.LangZH
	}

	p.SyncShotAssetBindings()
	dir := ProjectDir(rootDir, p.ID)
	if err := os.MkdirAll(filepath.Join(dir, "output"), 0o755); err != nil {
		return fmt.Errorf("create project dir: %w", err)
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal project yaml: %w", err)
	}
	path := ManifestPath(rootDir, p.ID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Load(rootDir, projectID string) (*Project, error) {
	path := ManifestPath(rootDir, projectID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project manifest %s: %w", path, err)
	}
	var p Project
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse project manifest: %w", err)
	}
	if p.Mix.VoiceVolume == 0 && p.Mix.AmbientVolume == 0 {
		p.Mix = DefaultMixConfig()
	}
	if p.Mix.BGMPreset == "" {
		if strings.TrimSpace(p.Mix.BGMPath) != "" {
			p.Mix.BGMPreset = "custom"
		} else {
			p.Mix.BGMPreset = "warm_pad"
		}
	}
	p.AspectRatio = NormalizeAspectRatio(p.AspectRatio)
	if p.NarratorVoice == "" {
		p.NarratorVoice = "warm-male"
	}
	if p.NarratorSpeed <= 0 {
		p.NarratorSpeed = 0.92
	}
	p.NormalizeWorkflow()
	p.NormalizeAssets()
	return &p, nil
}

// NormalizeWorkflow 兼容历史项目（未显式保存 stage 字段时自动推断合理阶段，并补齐每镜 StorySummary）。
func (p *Project) NormalizeWorkflow() {
	for i := range p.Shots {
		if strings.TrimSpace(p.Shots[i].StorySummary) == "" {
			var parts []string
			if s := strings.TrimSpace(p.Shots[i].T2ISpec.SubjectAnchor.ZH); s != "" {
				parts = append(parts, s)
			}
			if a := strings.TrimSpace(p.Shots[i].T2ISpec.ActionRelation.ZH); a != "" {
				parts = append(parts, a)
			}
			p.Shots[i].StorySummary = strings.Join(parts, "；")
		}
	}
	if p.TargetPacing == "" {
		p.TargetPacing = "auto"
	}
	if p.Stage != "" {
		return
	}
	if len(p.Shots) == 0 {
		p.Stage = StageStory
		return
	}
	allFrames := true
	for _, sh := range p.Shots {
		if strings.TrimSpace(sh.FirstFramePath) == "" {
			allFrames = false
			break
		}
	}
	if allFrames {
		p.StoryConfirmed = true
		p.StoryboardConfirmed = true
		p.AssetsConfirmed = true
		p.Stage = StageVideo
		return
	}
	if len(p.Assets) > 0 {
		p.StoryConfirmed = true
		p.StoryboardConfirmed = true
		p.Stage = StageAssetsPlan
		return
	}
	p.StoryConfirmed = true
	p.Stage = StageStoryboard
}

// ValidateID 校验项目 ID 合法性，防止路径穿越误删工作区外目录。
func ValidateID(projectID string) error {
	id := strings.TrimSpace(projectID)
	if id == "" || id == "." || id == ".." {
		return fmt.Errorf("invalid project id %q", projectID)
	}
	if strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return fmt.Errorf("invalid project id %q: path separators are not allowed", projectID)
	}
	return nil
}

// Delete 安全删除指定项目目录（projects/<id>），严格限制在 projects 根目录下。
func Delete(rootDir, projectID string) error {
	if err := ValidateID(projectID); err != nil {
		return err
	}
	projectsRoot, err := filepath.Abs(filepath.Join(rootDir, "projects"))
	if err != nil {
		return err
	}
	targetDir, err := filepath.Abs(ProjectDir(rootDir, projectID))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(projectsRoot, targetDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "..") || strings.Contains(rel, string(filepath.Separator)) {
		return fmt.Errorf("refusing to delete path outside projects directory: %q", projectID)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "project.yaml")); err != nil {
		return fmt.Errorf("project %q not found", projectID)
	}
	return os.RemoveAll(targetDir)
}

func ListIDs(rootDir string) ([]string, error) {
	dir := filepath.Join(rootDir, "projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "project.yaml")); err == nil {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

var shotFileRe = regexp.MustCompile(`^(?i)([a-z]+)(\d+)\.(jpe?g|png|webp)$`)

// ImportFromDir 自动扫描目录中的角色/场景参考图（资产*.jpeg）和分镜首帧图（g1.jpeg~g10.jpeg）。
func ImportFromDir(p *Project, scanDir string) error {
	absDir, err := filepath.Abs(scanDir)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return fmt.Errorf("scan dir %s: %w", absDir, err)
	}

	type frameItem struct {
		shotID string
		num    int
		path   string
	}
	var assets []AssetRef
	var frames []frameItem

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".jpeg") && !strings.HasSuffix(lower, ".jpg") && !strings.HasSuffix(lower, ".png") && !strings.HasSuffix(lower, ".webp") {
			continue
		}
		fullPath := filepath.Join(absDir, name)
		if strings.HasPrefix(name, "资产") {
			assetName := strings.TrimSuffix(name, filepath.Ext(name))
			assetName = strings.TrimLeft(strings.TrimPrefix(assetName, "资产"), "-_ ")
			if assetName == "" {
				assetName = name
			}
			assets = append(assets, AssetRef{
				Role:        inferAssetRole(assetName),
				Name:        assetName,
				ImagePath:   fullPath,
				Description: defaultAssetDescription(assetName),
				SourceMode:  "imported",
			})
			continue
		}
		if m := shotFileRe.FindStringSubmatch(name); len(m) == 4 {
			num, _ := strconv.Atoi(m[2])
			frames = append(frames, frameItem{
				shotID: strings.ToLower(m[1]) + m[2],
				num:    num,
				path:   fullPath,
			})
		}
	}

	sort.Slice(assets, func(i, j int) bool { return assets[i].Name < assets[j].Name })
	sort.Slice(frames, func(i, j int) bool { return frames[i].num < frames[j].num })

	if len(assets) > 0 {
		p.Assets = assets
		p.NormalizeAssets()
	}
	if len(frames) > 0 {
		shots := make([]ShotSpec, 0, len(frames))
		for i, f := range frames {
			shots = append(shots, ShotSpec{
				ShotID:         f.shotID,
				Order:          i + 1,
				Phase:          inferPhaseByOrder(i, len(frames)),
				FirstFramePath: f.path,
				VoiceID:        "warm-male",
				VoiceSpeed:     0.92,
				DurationSec:    5,
				Emotion:        "nostalgic",
				I2VSpec: prompting.I2VPromptSpec{
					CameraMovement: "slow_push_in",
					TransitionMode: prompting.TransitionSingleFrame,
				},
			})
		}
		p.Shots = shots
		p.ShotCount = len(shots)
	}
	return nil
}

func inferPhaseByOrder(idx, total int) psychology.ActPhase {
	if total <= 0 {
		return psychology.PhaseSensoryHook
	}
	ratio := float64(idx) / float64(total)
	switch {
	case ratio < 0.2:
		return psychology.PhaseSensoryHook
	case ratio < 0.4:
		return psychology.PhaseConcreteRitual
	case ratio < 0.7:
		return psychology.PhaseRelationalWarmth
	default:
		return psychology.PhaseLiminalClosure
	}
}

func inferAssetRole(name string) string {
	switch {
	case strings.Contains(name, "场景") || strings.Contains(name, "小卖部") || strings.Contains(name, "厨房") ||
		strings.Contains(name, "阳台") || strings.Contains(name, "客厅") || strings.Contains(name, "巷") ||
		strings.Contains(name, "街道") || strings.Contains(name, "车站") || strings.Contains(name, "公寓") ||
		strings.Contains(name, "小屋") || strings.Contains(name, "天台"):
		return "scene"
	case strings.Contains(name, "道具") || strings.Contains(name, "袋") || strings.Contains(name, "冰棍") ||
		strings.Contains(name, "相册") || strings.Contains(name, "电扇") || strings.Contains(name, "糖") ||
		strings.Contains(name, "杯") || strings.Contains(name, "后备箱"):
		return "prop"
	default:
		return "character"
	}
}

func defaultAssetDescription(assetName string) string {
	switch {
	case strings.Contains(assetName, "男孩"):
		return "8岁中国小男孩，寸头，微旧纯白短袖T恤，深蓝色运动短裤，黑色布鞋，神情纯真腼腆"
	case strings.Contains(assetName, "老板"):
		return "40多岁小卖部老板，短黑发，浅灰色棉T恤，慈祥温和的笑容，眼角有细纹"
	case strings.Contains(assetName, "小伙伴") || strings.Contains(assetName, "蓝色"):
		return "8岁发小男孩，寸头，浅蓝色棉T恤，黑色短裤，深色帆布鞋，活泼阳光"
	case strings.Contains(assetName, "小卖部"):
		return "90年代老旧胡同小卖部内部，陈旧木柜台、成排玻璃糖罐、算盘、铁皮零钱盒、老式白色卧式冰柜、满架复古零食与玻璃汽水瓶"
	default:
		return "保持与参考资产图外观、服装、年代材质高度一致"
	}
}
