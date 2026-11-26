package prompting

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"feelings-studio/configs"
	"feelings-studio/internal/psychology"
)

const (
	TemplateStory           = "prompts/story_outline.tmpl"
	TemplateDirector        = "prompts/director_storyboard.tmpl"
	TemplateAssetExtraction = "prompts/asset_extraction.tmpl"
	TemplateT2I             = "prompts/t2i_system.tmpl"
	TemplateI2V             = "prompts/i2v_system.tmpl"
)

type stylesFile struct {
	Styles []StylePreset `yaml:"styles"`
}

// Compiler 负责加载视觉风格预设、渲染三层系统提示词模板，以及双模式编译结构化双语分镜提示词。
type Compiler struct {
	overrideDir string
	styles      map[string]*StylePreset
}

func NewCompiler(overrideDir string) (*Compiler, error) {
	data, err := configs.ReadFile(overrideDir, "styles.yaml")
	if err != nil {
		return nil, fmt.Errorf("read styles.yaml: %w", err)
	}
	var sf stylesFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parse styles.yaml: %w", err)
	}
	styles := make(map[string]*StylePreset, len(sf.Styles))
	for i := range sf.Styles {
		st := sf.Styles[i]
		styles[st.ID] = &st
	}
	return &Compiler{
		overrideDir: overrideDir,
		styles:      styles,
	}, nil
}

func (c *Compiler) GetStyle(id string) (*StylePreset, error) {
	st, ok := c.styles[id]
	if !ok {
		return nil, fmt.Errorf("unknown visual style preset: %q", id)
	}
	return st, nil
}

func (c *Compiler) ListStyles() []*StylePreset {
	out := make([]*StylePreset, 0, len(c.styles))
	for _, st := range c.styles {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *Compiler) ReadTemplate(relPath string) (string, error) {
	data, err := configs.ReadFile(c.overrideDir, relPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ReadTemplateForProject 优先读取项目级独立模板覆盖（projects/<id>/prompts/*.tmpl），若不存在则回退到全局模板。
func (c *Compiler) ReadTemplateForProject(projectDir, relPath string) (content string, isProjectOverride bool, err error) {
	if strings.TrimSpace(projectDir) != "" {
		candidate := filepath.Join(projectDir, filepath.FromSlash(relPath))
		if data, readErr := os.ReadFile(candidate); readErr == nil {
			return string(data), true, nil
		}
	}
	raw, err := c.ReadTemplate(relPath)
	return raw, false, err
}

func (c *Compiler) SaveTemplate(relPath, content string) error {
	if _, err := parsePromptTemplate(relPath, content); err != nil {
		return fmt.Errorf("template syntax error: %w", err)
	}
	return configs.WriteOverride(c.overrideDir, relPath, []byte(content))
}

// SaveProjectTemplate 保存为单个项目的独立系统提示词模板，避免串味到其他新项目。
func (c *Compiler) SaveProjectTemplate(projectDir, relPath, content string) error {
	if strings.TrimSpace(projectDir) == "" {
		return fmt.Errorf("project directory is required")
	}
	if _, err := parsePromptTemplate(relPath, content); err != nil {
		return fmt.Errorf("template syntax error: %w", err)
	}
	full := filepath.Join(projectDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(content), 0o644)
}

// ResetProjectTemplate 删除项目级独立模板覆盖，恢复继承全局默认模板。
func (c *Compiler) ResetProjectTemplate(projectDir, relPath string) (string, error) {
	if strings.TrimSpace(projectDir) != "" {
		full := filepath.Join(projectDir, filepath.FromSlash(relPath))
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return c.ReadTemplate(relPath)
}

func (c *Compiler) ResetTemplate(relPath string) (string, error) {
	data, err := configs.EmbeddedFS.ReadFile(relPath)
	if err != nil {
		return "", err
	}
	if err := configs.WriteOverride(c.overrideDir, relPath, data); err != nil {
		return "", err
	}
	return string(data), nil
}

type SystemTemplateContext struct {
	ProjectDir        string
	Theme             string
	ShotCount         int // <=0 表示不预设镜头数，由剧情自然决定
	TargetPacingLabel string
	StoryLogline      string
	StorySynopsis     string
	StoryActsText     string
	Strategy          *psychology.StrategyPack
	Style             *StylePreset
	Assets            map[string]string
}

func (c *Compiler) RenderSystemPrompts(ctx SystemTemplateContext) (*SystemPromptBundle, error) {
	if strings.TrimSpace(ctx.TargetPacingLabel) == "" {
		ctx.TargetPacingLabel = "由故事剧情自然决定（推荐）"
	}
	storyPrompt, err := c.renderOne(TemplateStory, ctx)
	if err != nil {
		return nil, fmt.Errorf("render story outline system prompt: %w", err)
	}
	dirPrompt, err := c.renderOne(TemplateDirector, ctx)
	if err != nil {
		return nil, fmt.Errorf("render director system prompt: %w", err)
	}
	assetPrompt, err := c.renderOne(TemplateAssetExtraction, ctx)
	if err != nil {
		return nil, fmt.Errorf("render asset extraction system prompt: %w", err)
	}
	t2iPrompt, err := c.renderOne(TemplateT2I, ctx)
	if err != nil {
		return nil, fmt.Errorf("render t2i system prompt: %w", err)
	}
	i2vPrompt, err := c.renderOne(TemplateI2V, ctx)
	if err != nil {
		return nil, fmt.Errorf("render i2v system prompt: %w", err)
	}
	return &SystemPromptBundle{
		StoryOutline:       storyPrompt,
		DirectorStoryboard: dirPrompt,
		AssetExtraction:    assetPrompt,
		T2ISystem:          t2iPrompt,
		I2VSystem:          i2vPrompt,
	}, nil
}

func (c *Compiler) renderOne(relPath string, data SystemTemplateContext) (string, error) {
	raw, _, err := c.ReadTemplateForProject(data.ProjectDir, relPath)
	if err != nil {
		return "", err
	}
	tmpl, err := parsePromptTemplate(relPath, raw)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func parsePromptTemplate(name, raw string) (*template.Template, error) {
	funcs := template.FuncMap{
		"join":   strings.Join,
		"mul100": func(v float64) float64 { return v * 100 },
	}
	return template.New(name).Funcs(funcs).Parse(raw)
}

// CompileAssetT2I 为项目资产库（角色定妆照/核心场景空镜/关键道具特写）编译干净统一的生图提示词。
func (c *Compiler) CompileAssetT2I(name, role, desc string, customPrompt BilingualText, style *StylePreset, lang Lang) CompiledPrompt {
	sep := "，"
	if lang == LangEN {
		sep = ", "
	}
	base := customPrompt.Get(lang)
	if base == "" {
		base = strings.TrimSpace(desc)
	}
	if base == "" {
		base = strings.TrimSpace(name)
	}

	var framing string
	if lang == LangEN {
		switch role {
		case "character":
			framing = "9:16 vertical character reference portrait, neutral natural posture, clean uncluttered background, sharp facial and clothing details for visual consistency"
		case "scene":
			framing = "9:16 vertical empty environment reference shot, no characters in frame, clear architectural material textures and consistent lighting"
		case "prop":
			framing = "9:16 vertical hero prop reference close-up, centered clean composition, authentic aged material texture, no readable text or logos"
		default:
			framing = "9:16 vertical visual consistency reference sheet, clean composition"
		}
	} else {
		switch role {
		case "character":
			framing = "9:16竖屏角色定妆参考图，自然平视站姿，干净背景，清晰呈现五官发型与服装材质细节，供全片人物一致性锚定"
		case "scene":
			framing = "9:16竖屏核心场景空镜参考图，画面无人物，清晰交代建筑结构、陈设年代质感与自然光影关系"
		case "prop":
			framing = "9:16竖屏核心道具定妆特写参考图，居中干净构图，突出真实年代材质细节，严禁可读文字与Logo"
		default:
			framing = "9:16竖屏视觉一致性参考图，画面干净克制"
		}
	}

	parts := nonEmpty(framing, base)
	if style != nil {
		if s := style.T2ISuffix.Get(lang); s != "" {
			parts = append(parts, s)
		}
	}
	var negParts []string
	if style != nil {
		negParts = appendUnique(negParts, style.NegativeStatic.Get(lang))
	}
	return CompiledPrompt{
		Prompt:         strings.Join(parts, sep),
		NegativePrompt: strings.Join(negParts, sep),
	}
}

// CompileT2I 将结构化首帧槽位与风格预设编译为指定语言的最终 T2I 提示词。
func (c *Compiler) CompileT2I(spec T2IPromptSpec, style *StylePreset, lang Lang) CompiledPrompt {
	sep := "，"
	if lang == LangEN {
		sep = ", "
	}

	parts := nonEmpty(
		spec.ShotFraming.Get(lang),
		spec.SubjectAnchor.Get(lang),
		spec.ActionRelation.Get(lang),
		spec.EraEnvironment.Get(lang),
		spec.LightingMood.Get(lang),
	)
	if override := spec.StyleOverride.Get(lang); override != "" {
		parts = append(parts, override)
	} else if style != nil {
		if s := style.T2ISuffix.Get(lang); s != "" {
			parts = append(parts, s)
		}
	}

	negParts := nonEmpty(spec.Negative.Get(lang))
	if style != nil {
		negParts = appendUnique(negParts, style.NegativeStatic.Get(lang))
	}

	return CompiledPrompt{
		Prompt:         strings.Join(parts, sep),
		NegativePrompt: strings.Join(negParts, sep),
	}
}

// CompileI2V 将结构化视频动态槽位编译为精炼电影单段版（适配 Runway / Luma / MiniMax 海螺 / 轻量模型）。
func (c *Compiler) CompileI2V(spec I2VPromptSpec, style *StylePreset, lang Lang) CompiledPrompt {
	sep := "；"
	if lang == LangEN {
		sep = ". "
	}

	camText := spec.CameraDetail.Get(lang)
	if camText == "" && spec.CameraMovement != "" {
		camText = defaultCameraLabel(spec.CameraMovement, lang)
	}

	parts := nonEmpty(
		spec.SubjectMicroAction.Get(lang),
		spec.EnvironmentPhysics.Get(lang),
		camText,
		spec.LightingPacing.Get(lang),
	)

	if spec.TransitionMode == TransitionFirstLastFrame {
		hint := spec.TransitionHint.Get(lang)
		if hint != "" {
			if lang == LangEN {
				parts = append(parts, "First-to-last frame transition: "+hint)
			} else {
				parts = append(parts, "首尾帧平滑过渡："+hint)
			}
		}
	}

	if style != nil {
		if s := style.I2VSuffix.Get(lang); s != "" {
			parts = append(parts, s)
		}
	}

	negSep := "，"
	if lang == LangEN {
		negSep = ", "
	}
	negParts := nonEmpty(spec.Negative.Get(lang))
	if style != nil {
		negParts = appendUnique(negParts, style.NegativeVideo.Get(lang))
	}

	return CompiledPrompt{
		Prompt:         strings.Join(parts, sep),
		NegativePrompt: strings.Join(negParts, negSep),
	}
}

// CompileDirectorSheet 将结构化分镜编译为“工业级导演时序详版”（适配即梦 Seedance / 可灵 2.x / 万相 Wan / Veo），
// 同时严格物理隔离「视频生成指令」与「后期心理字幕/转场元数据」。
func (c *Compiler) CompileDirectorSheet(
	shotID string,
	order int,
	durSec int,
	t2i T2IPromptSpec,
	i2v I2VPromptSpec,
	sheet DirectorSheetSpec,
	style *StylePreset,
	lang Lang,
) CompiledDirectorSheet {
	if durSec <= 0 {
		durSec = 5
	}
	concise := c.CompileI2V(i2v, style, lang)

	// 若未手动配置 TimelineBeats，自动从 I2VSpec 派生 2 段情怀呼吸节拍（确保任何分镜都能产出完整导演详版）
	beats := sheet.TimelineBeats
	if len(beats) == 0 {
		mid := float64(durSec) * 0.65
		beats = []TimelineBeat{
			{
				StartSec: 0,
				EndSec:   mid,
				Title:    BilingualText{ZH: "情绪建立与微动态", EN: "Atmosphere & Micro-Action"},
				Visual: BilingualText{
					ZH: joinNonEmpty("；", i2v.SubjectMicroAction.ZH, i2v.EnvironmentPhysics.ZH),
					EN: joinNonEmpty(". ", i2v.SubjectMicroAction.EN, i2v.EnvironmentPhysics.EN),
				},
				Dialogue:   BilingualText{ZH: "无", EN: "None"},
				Forbid:     BilingualText{ZH: "禁止人物突然看镜头，禁止画面中生成文字或现代物品", EN: "No sudden gaze at camera, no rendered text or modern props"},
				Constraint: BilingualText{ZH: "人物外观与光向全程锁定，动作缓慢克制", EN: "Lock subject appearance and light direction, slow restrained motion"},
				Camera:     BilingualText{ZH: defaultCameraLabel(i2v.CameraMovement, LangZH), EN: defaultCameraLabel(i2v.CameraMovement, LangEN)},
				SFX:        BilingualText{ZH: "轻柔自然环境底噪", EN: "Soft natural ambient room tone"},
			},
			{
				StartSec: mid,
				EndSec:   float64(durSec),
				Title:    BilingualText{ZH: "呼吸停顿与留白余韵", EN: "Breathing Pause & Lingering Stillness"},
				Visual: BilingualText{
					ZH: joinNonEmpty("；", i2v.EnvironmentPhysics.ZH, "动作缓缓收住，保留画面下方呼吸留白区"),
					EN: joinNonEmpty(". ", i2v.EnvironmentPhysics.EN, "Motion settles gently with clean lower breathing space"),
				},
				Dialogue:   BilingualText{ZH: "无", EN: "None"},
				Forbid:     BilingualText{ZH: "禁止镜头漂移、高光跳变或多余人物闯入", EN: "No camera drift, exposure flicker, or stray figures"},
				Constraint: BilingualText{ZH: "结尾平稳缓停，供后期极简字幕浮现", EN: "Smooth ease-out settle ready for post-production subtitles"},
				Camera:     BilingualText{ZH: "缓停定格", EN: "Gentle ease-out settle"},
				SFX:        BilingualText{ZH: "环境微风延续渐弱", EN: "Soft ambient air tapering off"},
			},
		}
	}

	var gen strings.Builder
	var post strings.Builder

	if lang == LangEN {
		title := sheet.ShotTitle.Get(LangEN)
		if title == "" {
			title = fmt.Sprintf("Shot %d (%s)", order, shotID)
		}
		fmt.Fprintf(&gen, "════════════════════════════════════════════════\nShot %d · %s (%ds · 9:16 Nostalgic Cinema)\n════════════════════════════════════════════════\n\n", order, title, durSec)

		if len(sheet.AssetBindings) > 0 {
			gen.WriteString("Asset Reference Binding:\n")
			for _, ab := range sheet.AssetBindings {
				line := fmt.Sprintf("- [%s] %s -> %s", ab.Role, ab.AssetName, ab.RefToken)
				if r := ab.UsageRule.Get(LangEN); r != "" {
					line += " (" + r + ")"
				}
				if ex := ab.ExcludeHint.Get(LangEN); ex != "" {
					line += " [" + ex + "]"
				}
				gen.WriteString(line + "\n")
			}
			gen.WriteString("\n")
		}
		if cast := sheet.CastSummary.Get(LangEN); cast != "" {
			fmt.Fprintf(&gen, "Cast:\n%s\n---\n\n", cast)
		}
		bg := sheet.BackgroundSetting.Get(LangEN)
		if bg == "" {
			bg = joinNonEmpty(", ", t2i.EraEnvironment.Get(LangEN), t2i.LightingMood.Get(LangEN))
		}
		if bg != "" {
			fmt.Fprintf(&gen, "Setting & Lighting:\n%s\n---\n\n", bg)
		}
		if len(sheet.AssetBindings) > 0 {
			gen.WriteString("Reference Image Usage Rules:\n")
			for _, ab := range sheet.AssetBindings {
				if r := ab.UsageRule.Get(LangEN); r != "" {
					fmt.Fprintf(&gen, "- %s (%s): %s\n", ab.AssetName, ab.RefToken, r)
				}
			}
			gen.WriteString("---\n\n")
		}
		if cont := sheet.ContinuitySummary.Get(LangEN); cont != "" {
			fmt.Fprintf(&gen, "Shot Continuity Context:\n%s\n---\n\n", cont)
		}
		if ff := sheet.FirstFrameLink.Get(LangEN); ff != "" {
			fmt.Fprintf(&gen, "First Frame Continuity:\n%s\n\n", ff)
		}
		if lf := sheet.LastFrameLink.Get(LangEN); lf != "" {
			fmt.Fprintf(&gen, "Last Frame Continuity:\n%s\n\n", lf)
		}

		for _, b := range beats {
			fmt.Fprintf(&gen, "Time Segment: %.1f-%.1fs, %s.\n", b.StartSec, b.EndSec, b.Title.Get(LangEN))
			fmt.Fprintf(&gen, "Visual: %s\n", b.Visual.Get(LangEN))
			dlg := b.Dialogue.Get(LangEN)
			if dlg == "" {
				dlg = "None."
			}
			fmt.Fprintf(&gen, "Dialogue: %s\n", dlg)
			if fb := b.Forbid.Get(LangEN); fb != "" {
				fmt.Fprintf(&gen, "Forbid: %s\n", fb)
			}
			if cs := b.Constraint.Get(LangEN); cs != "" {
				fmt.Fprintf(&gen, "Constraints: %s\n", cs)
			}
			if stg := b.Staging.Get(LangEN); stg != "" {
				fmt.Fprintf(&gen, "Staging & Facing: %s\n", stg)
			}
			if cam := b.Camera.Get(LangEN); cam != "" {
				fmt.Fprintf(&gen, "Camera: %s\n", cam)
			}
			if sfx := b.SFX.Get(LangEN); sfx != "" {
				fmt.Fprintf(&gen, "Sound Design: %s\n", sfx)
			}
			gen.WriteString("\n")
		}

		if len(sheet.OutputConstraints) > 0 {
			gen.WriteString("---\nOutput Constraints:\n")
			for idx, oc := range sheet.OutputConstraints {
				if txt := oc.Get(LangEN); txt != "" {
					fmt.Fprintf(&gen, "%d. %s\n", idx+1, txt)
				}
			}
			gen.WriteString("\n")
		}
		if style != nil {
			fmt.Fprintf(&gen, "[Visual Style: %s — %s]\n", style.Name, style.I2VSuffix.Get(LangEN))
		}
	} else {
		title := sheet.ShotTitle.Get(LangZH)
		if title == "" {
			title = fmt.Sprintf("分镜 %s", shotID)
		}
		fmt.Fprintf(&gen, "════════════════════════════════════════════════\n镜 %d · %s（%d秒 · 9:16 竖屏情怀短片）\n════════════════════════════════════════════════\n\n", order, title, durSec)

		if len(sheet.AssetBindings) > 0 {
			gen.WriteString("资产图锚定：\n")
			for _, ab := range sheet.AssetBindings {
				line := fmt.Sprintf("%s %s 的参考图是 %s", roleLabelZH(ab.Role), ab.AssetName, ab.RefToken)
				if ex := ab.ExcludeHint.Get(LangZH); ex != "" {
					line += "（" + ex + "）"
				}
				gen.WriteString(line + "\n")
			}
			gen.WriteString("\n")
		}
		if cast := sheet.CastSummary.Get(LangZH); cast != "" {
			fmt.Fprintf(&gen, "出场角色：\n%s\n---\n\n", cast)
		}
		bg := sheet.BackgroundSetting.Get(LangZH)
		if bg == "" {
			bg = joinNonEmpty("；", t2i.EraEnvironment.Get(LangZH), t2i.LightingMood.Get(LangZH))
		}
		if bg != "" {
			fmt.Fprintf(&gen, "背景场景：\n%s\n---\n\n", bg)
		}
		if len(sheet.AssetBindings) > 0 {
			gen.WriteString("参考图使用规则：\n")
			for _, ab := range sheet.AssetBindings {
				if r := ab.UsageRule.Get(LangZH); r != "" {
					fmt.Fprintf(&gen, "- %s（%s）：%s\n", ab.AssetName, ab.RefToken, r)
				}
			}
			gen.WriteString("---\n\n")
		}
		if cont := sheet.ContinuitySummary.Get(LangZH); cont != "" {
			fmt.Fprintf(&gen, "%s\n---\n\n", cont)
		}
		if ff := sheet.FirstFrameLink.Get(LangZH); ff != "" {
			fmt.Fprintf(&gen, "首帧衔接：\n%s\n\n", ff)
		}
		if lf := sheet.LastFrameLink.Get(LangZH); lf != "" {
			fmt.Fprintf(&gen, "尾帧衔接：\n%s\n\n", lf)
		}

		for _, b := range beats {
			fmt.Fprintf(&gen, "时间段：%.1f-%.1f秒，%s。\n", b.StartSec, b.EndSec, b.Title.Get(LangZH))
			fmt.Fprintf(&gen, "画面：%s\n", b.Visual.Get(LangZH))
			dlg := b.Dialogue.Get(LangZH)
			if dlg == "" {
				dlg = "无。"
			}
			fmt.Fprintf(&gen, "对白：%s\n", dlg)
			if fb := b.Forbid.Get(LangZH); fb != "" {
				fmt.Fprintf(&gen, "禁止：%s\n", fb)
			}
			if cs := b.Constraint.Get(LangZH); cs != "" {
				fmt.Fprintf(&gen, "约束：%s\n", cs)
			}
			if stg := b.Staging.Get(LangZH); stg != "" {
				fmt.Fprintf(&gen, "站位与朝向：%s\n", stg)
			}
			if cam := b.Camera.Get(LangZH); cam != "" {
				fmt.Fprintf(&gen, "运镜：%s\n", cam)
			}
			if sfx := b.SFX.Get(LangZH); sfx != "" {
				fmt.Fprintf(&gen, "音效：%s\n", sfx)
			}
			gen.WriteString("\n")
		}

		if len(sheet.OutputConstraints) > 0 {
			gen.WriteString("---\n输出约束：\n")
			for idx, oc := range sheet.OutputConstraints {
				if txt := oc.Get(LangZH); txt != "" {
					fmt.Fprintf(&gen, "%d. %s\n", idx+1, txt)
				}
			}
			gen.WriteString("\n")
		}
		if style != nil {
			fmt.Fprintf(&gen, "[视觉风格：%s，%s]\n", style.Name, style.I2VSuffix.Get(LangZH))
		}
	}

	// 严格物理隔离的后期元数据区（不传入视频生成 API，仅供人工剪辑与 FFmpeg 字幕/转场参考）
	hasPost := sheet.PostProduction.SubtitleStrategy != "" ||
		len(sheet.PostProduction.SubtitleRhythm) > 0 ||
		sheet.PostProduction.TransitionDesign.FromPrev != "" ||
		sheet.PostProduction.TransitionDesign.ToNext != ""
	for _, b := range beats {
		if b.PsychMapping != "" {
			hasPost = true
		}
	}
	if hasPost {
		post.WriteString("──── 心理锚点与字幕映射（后期压字用，非视频生成指令）────\n")
		if sheet.PostProduction.SubtitleStrategy != "" {
			post.WriteString(sheet.PostProduction.SubtitleStrategy + "\n")
		}
		for _, b := range beats {
			if b.PsychMapping != "" {
				fmt.Fprintf(&post, "· %.1f-%.1fs　%s → %s\n", b.StartSec, b.EndSec, b.Title.Get(LangZH), b.PsychMapping)
			}
		}
		if len(sheet.PostProduction.SubtitleRhythm) > 0 {
			post.WriteString("\n建议字幕节奏：\n")
			for i, sr := range sheet.PostProduction.SubtitleRhythm {
				fmt.Fprintf(&post, "%d. %s\n", i+1, sr)
			}
		}
		td := sheet.PostProduction.TransitionDesign
		if td.FromPrev != "" || td.ToNext != "" || td.ToOutro != "" {
			post.WriteString("\n──── 转场设计（后期剪辑/FFmpeg 参考，非画面生成指令）────\n")
			if td.FromPrev != "" {
				fmt.Fprintf(&post, "承接上一镜：%s\n", td.FromPrev)
			}
			if td.ToNext != "" {
				fmt.Fprintf(&post, "衔接下一镜：%s\n", td.ToNext)
			}
			if td.ToOutro != "" {
				fmt.Fprintf(&post, "尾镜收束：%s\n", td.ToOutro)
			}
		}
	}

	genText := strings.TrimSpace(gen.String())
	postText := strings.TrimSpace(post.String())
	full := genText
	if postText != "" {
		full = genText + "\n\n" + postText
	}

	return CompiledDirectorSheet{
		GenerationPrompt: genText,
		PostNotes:        postText,
		FullSheet:        full,
		NegativePrompt:   concise.NegativePrompt,
	}
}

// CompileShot 一次性产出单镜的完整双模式中英双语编译结果（精炼电影版 + 导演时序详版）。
func (c *Compiler) CompileShot(shotID string, t2i T2IPromptSpec, i2v I2VPromptSpec, style *StylePreset) CompiledShotPrompts {
	return c.CompileShotFull(shotID, 1, 5, t2i, i2v, DirectorSheetSpec{}, style)
}

func (c *Compiler) CompileShotFull(
	shotID string,
	order int,
	durSec int,
	t2i T2IPromptSpec,
	i2v I2VPromptSpec,
	sheet DirectorSheetSpec,
	style *StylePreset,
) CompiledShotPrompts {
	return CompiledShotPrompts{
		ShotID:        shotID,
		T2IZH:         c.CompileT2I(t2i, style, LangZH),
		T2IEN:         c.CompileT2I(t2i, style, LangEN),
		I2VZH:         c.CompileI2V(i2v, style, LangZH),
		I2VEN:         c.CompileI2V(i2v, style, LangEN),
		I2VDirectorZH: c.CompileDirectorSheet(shotID, order, durSec, t2i, i2v, sheet, style, LangZH),
		I2VDirectorEN: c.CompileDirectorSheet(shotID, order, durSec, t2i, i2v, sheet, style, LangEN),
	}
}

func roleLabelZH(role string) string {
	switch role {
	case "character":
		return "角色"
	case "scene":
		return "场景"
	case "prop":
		return "道具"
	default:
		return "资产"
	}
}

func joinNonEmpty(sep string, items ...string) string {
	return strings.Join(nonEmpty(items...), sep)
}

func defaultCameraLabel(move string, lang Lang) string {
	zhMap := map[string]string{
		"slow_push_in":     "摄影机极缓向前推进",
		"slow_pull_out":    "摄影机极缓向后拉远",
		"gentle_pan_right": "摄影机极缓向右平移",
		"gentle_pan_left":  "摄影机极缓向左平移",
		"static_gaze":      "固定机位静谧凝视，画面极度平稳",
		"handheld_breath":  "极轻柔手持呼吸感稳定微动",
	}
	enMap := map[string]string{
		"slow_push_in":     "ultra-slow gentle camera push-in",
		"slow_pull_out":    "ultra-slow gentle camera pull-out",
		"gentle_pan_right": "subtle slow camera pan to the right",
		"gentle_pan_left":  "subtle slow camera pan to the left",
		"static_gaze":      "static locked-off contemplative camera",
		"handheld_breath":  "subtle organic breathing handheld stability",
	}
	if lang == LangEN {
		if v, ok := enMap[move]; ok {
			return v
		}
		return move
	}
	if v, ok := zhMap[move]; ok {
		return v
	}
	return move
}

func nonEmpty(items ...string) []string {
	var out []string
	for _, item := range items {
		if s := strings.TrimSpace(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func appendUnique(base []string, extra string) []string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return base
	}
	for _, b := range base {
		if b == extra || strings.Contains(b, extra) {
			return base
		}
	}
	return append(base, extra)
}
