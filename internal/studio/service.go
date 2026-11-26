package studio

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"feelings-studio/internal/composer"
	"feelings-studio/internal/project"
	"feelings-studio/internal/prompting"
	"feelings-studio/internal/provider"
	providerfactory "feelings-studio/internal/provider/factory"
	"feelings-studio/internal/psychology"
	"feelings-studio/internal/scheduler"
	"feelings-studio/internal/settings"
	"feelings-studio/internal/store"
)

type Service struct {
	RootDir      string
	ConfigDir    string
	StrategyRepo *psychology.Repository
	Compiler     *prompting.Compiler
	Store        *store.Store

	provMu    sync.RWMutex
	Providers *providerfactory.Bundle
	Settings  *settings.ProvidersConfig

	subMu       sync.RWMutex
	subscribers map[chan scheduler.ProgressEvent]struct{}

	autoMu   sync.RWMutex
	autoRuns map[string]*autoRunEntry

	projLocksMu sync.Mutex
	projLocks   map[string]*sync.Mutex
}

func Open(rootDir, configDir string) (*Service, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}
	if configDir == "" {
		configDir = filepath.Join(absRoot, "configs")
	}

	repo, err := psychology.LoadRepository(configDir)
	if err != nil {
		return nil, err
	}
	comp, err := prompting.NewCompiler(configDir)
	if err != nil {
		return nil, err
	}
	dbDir := filepath.Join(absRoot, ".cache")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, err
	}
	st, err := store.Open(filepath.Join(dbDir, "state.db"))
	if err != nil {
		return nil, err
	}
	provCfg, err := settings.Load(absRoot)
	if err != nil {
		// 不要静默回退：否则用户以为在用真实模型，实际跑的是 Mock
		log.Printf("警告: 读取模型供应商配置 %s 失败，本次运行将使用默认 Mock 供应商: %v", settings.ConfigPath(absRoot), err)
		provCfg = settings.DefaultConfig()
	}

	return &Service{
		RootDir:      absRoot,
		ConfigDir:    configDir,
		StrategyRepo: repo,
		Compiler:     comp,
		Store:        st,
		Settings:     provCfg,
		Providers:    providerfactory.FromConfigWithEnv(absRoot, provCfg),
		subscribers:  make(map[chan scheduler.ProgressEvent]struct{}),
		autoRuns:     make(map[string]*autoRunEntry),
	}, nil
}

// GetProviders 安全获取当前热生效的 Provider Bundle。
func (s *Service) GetProviders() *providerfactory.Bundle {
	s.provMu.RLock()
	defer s.provMu.RUnlock()
	return s.Providers
}

// GetProviderSettings 返回当前供应商配置的深拷贝，调用方修改不会影响服务内部状态。
func (s *Service) GetProviderSettings() *settings.ProvidersConfig {
	s.provMu.RLock()
	defer s.provMu.RUnlock()
	return s.Settings.Clone()
}

// UpdateProviderSettings 持久化保存供应商配置并立即热重载所有 Provider（无需重启服务）。
func (s *Service) UpdateProviderSettings(incoming *settings.ProvidersConfig) (*settings.ProvidersConfig, error) {
	saved, err := settings.MergeAndSave(s.RootDir, incoming)
	if err != nil {
		return nil, err
	}
	bundle := providerfactory.FromConfigWithEnv(s.RootDir, saved)
	s.provMu.Lock()
	s.Settings = saved
	s.Providers = bundle
	s.provMu.Unlock()
	return saved, nil
}

func (s *Service) Close() error {
	if s.Store != nil {
		return s.Store.Close()
	}
	return nil
}

// DeleteProject 安全删除指定项目的全部本地文件（projects/<id>/）以及 SQLite 中的任务缓存记录。
func (s *Service) DeleteProject(ctx context.Context, projectID string) error {
	if err := project.ValidateID(projectID); err != nil {
		return err
	}
	unlock := s.lockProject(projectID)
	defer unlock()
	if err := project.Delete(s.RootDir, projectID); err != nil {
		return err
	}
	if s.Store != nil {
		_ = s.Store.DeleteProjectJobs(ctx, projectID)
	}
	return nil
}

func (s *Service) Subscribe() chan scheduler.ProgressEvent {
	ch := make(chan scheduler.ProgressEvent, 64)
	s.subMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subMu.Unlock()
	return ch
}

func (s *Service) Unsubscribe(ch chan scheduler.ProgressEvent) {
	s.subMu.Lock()
	delete(s.subscribers, ch)
	s.subMu.Unlock()
	close(ch)
}

func (s *Service) Broadcast(ev scheduler.ProgressEvent) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	s.subMu.RLock()
	defer s.subMu.RUnlock()
	for ch := range s.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}

type InitOptions struct {
	ID            string
	Title         string
	Theme         string
	Track         psychology.TrackID
	StylePreset   string
	PromptLang    prompting.Lang
	AspectRatio   string // 9:16 | 16:9 | 3:4
	NarratorVoice string // warm-male | healing-female | childhood-boy | calm-senior
	NarratorSpeed float64
	TargetPacing  string // auto | short | standard
	ShotCount     int    // 可选：0 表示不在建项时硬填，由 Step 1 整体故事剧情自然决定
	ImportDir     string
}

func (s *Service) InitProject(opts InitOptions) (*project.Project, error) {
	if err := project.ValidateID(opts.ID); err != nil {
		return nil, err
	}
	if opts.Track == "" {
		opts.Track = psychology.TrackChildhood90s
	}
	pack, err := s.StrategyRepo.Get(opts.Track)
	if err != nil {
		return nil, err
	}
	styleID := opts.StylePreset
	if styleID == "" {
		styleID = pack.DefaultStyle
	}
	if _, err := s.Compiler.GetStyle(styleID); err != nil {
		return nil, err
	}
	if opts.PromptLang == "" {
		opts.PromptLang = prompting.LangZH
	}
	if opts.Title == "" {
		opts.Title = opts.ID
	}
	if strings.TrimSpace(opts.Theme) == "" {
		// 优先使用用户填写的项目标题作为创作主题，避免新建项目被硬编码成“小卖部”
		if strings.TrimSpace(opts.Title) != "" && opts.Title != opts.ID {
			opts.Theme = strings.TrimSpace(opts.Title)
		} else {
			switch opts.Track {
			case psychology.TrackFamilyBond:
				opts.Theme = "离家前父母塞满的后备箱与窗口亮着的灯"
			case psychology.TrackUrbanSanctuary:
				opts.Theme = "下班后吹着晚风的五分钟避风港"
			default:
				opts.Theme = "记忆里那个放学后的温暖午后"
			}
		}
	}
	if strings.TrimSpace(opts.TargetPacing) == "" {
		opts.TargetPacing = "auto"
	}
	aspectRatio := project.NormalizeAspectRatio(opts.AspectRatio)
	narratorVoice := strings.TrimSpace(opts.NarratorVoice)
	if narratorVoice == "" {
		narratorVoice = "warm-male"
	}
	narratorSpeed := opts.NarratorSpeed
	if narratorSpeed <= 0 {
		narratorSpeed = 0.92
	}

	p := &project.Project{
		ID:            opts.ID,
		Title:         opts.Title,
		Theme:         opts.Theme,
		Track:         opts.Track,
		StylePreset:   styleID,
		PromptLang:    opts.PromptLang,
		AspectRatio:   aspectRatio,
		NarratorVoice: narratorVoice,
		NarratorSpeed: narratorSpeed,
		TargetPacing:  opts.TargetPacing,
		ShotCount:     opts.ShotCount,
		Stage:         project.StageStory,
		Mix:           project.DefaultMixConfig(),
	}
	if st, err := s.Compiler.GetStyle(styleID); err == nil {
		p.Mix.GrainStrength = st.GrainStrength
	}
	if opts.ImportDir != "" {
		if err := project.ImportFromDir(p, opts.ImportDir); err != nil {
			return nil, err
		}
		if len(p.Shots) > 0 {
			p.StoryConfirmed = true
			p.StoryboardConfirmed = true
			if len(p.Assets) > 0 {
				p.AssetsConfirmed = true
			}
			p.Stage = project.StageVideo
		}
	}
	unlock := s.lockProject(p.ID)
	defer unlock()
	if _, err := os.Stat(project.ManifestPath(s.RootDir, p.ID)); err == nil {
		return nil, fmt.Errorf("project %q already exists", p.ID)
	}
	if err := project.Save(s.RootDir, p); err != nil {
		return nil, err
	}
	return p, nil
}

// CompileProjectPrompts 实时编译该项目的三层系统提示词（支持项目级独立模板覆盖）与所有分镜的中英双语提示词。
func (s *Service) CompileProjectPrompts(p *project.Project) (*prompting.SystemPromptBundle, []prompting.CompiledShotPrompts, error) {
	p.SyncShotAssetBindings()
	pack, err := s.StrategyRepo.Get(p.Track)
	if err != nil {
		return nil, nil, err
	}
	style, err := s.Compiler.GetStyle(p.StylePreset)
	if err != nil {
		return nil, nil, err
	}
	shotCount := p.ShotCount
	if len(p.Shots) > 0 {
		shotCount = len(p.Shots)
	}

	var storyLogline, storySynopsis, storyActsText string
	if p.Story != nil {
		storyLogline = p.Story.Logline
		storySynopsis = p.Story.Synopsis
		var actLines []string
		for i, act := range p.Story.Acts {
			actLines = append(actLines, fmt.Sprintf("  %d. [%s] %s：%s", i+1, act.Phase, act.Title, act.Summary))
		}
		storyActsText = strings.Join(actLines, "\n")
	}

	sysBundle, err := s.Compiler.RenderSystemPrompts(prompting.SystemTemplateContext{
		ProjectDir:        project.ProjectDir(s.RootDir, p.ID),
		Theme:             p.Theme,
		ShotCount:         shotCount,
		TargetPacingLabel: pacingLabelZH(p.TargetPacing),
		StoryLogline:      storyLogline,
		StorySynopsis:     storySynopsis,
		StoryActsText:     storyActsText,
		Strategy:          pack,
		Style:             style,
		Assets:            p.AssetDescriptionMap(),
	})
	if err != nil {
		return nil, nil, err
	}

	compiledShots := make([]prompting.CompiledShotPrompts, 0, len(p.Shots))
	for i, sh := range p.Shots {
		ord := sh.Order
		if ord <= 0 {
			ord = i + 1
		}
		compiledShots = append(compiledShots, s.Compiler.CompileShotFull(sh.ShotID, ord, sh.DurationSec, sh.T2ISpec, sh.I2VSpec, sh.DirectorSheet, style))
	}
	return sysBundle, compiledShots, nil
}

// EvaluateProject 对项目当前分镜执行社会心理学质检并更新 CriticReport。
func (s *Service) EvaluateProject(p *project.Project) (*psychology.CriticReport, error) {
	pack, err := s.StrategyRepo.Get(p.Track)
	if err != nil {
		return nil, err
	}
	rep := psychology.Evaluate(pack, p.ToShotsForReview())
	p.CriticReport = &rep
	return &rep, nil
}

// PlanStory Step 1：先生成完整电影感整体故事剧情大纲（Logline / 完整正文 / 4幕推进），暂不拆镜、不预设分镜数。
func (s *Service) PlanStory(ctx context.Context, projectID string) (*project.Project, error) {
	p, err := project.Load(s.RootDir, projectID)
	if err != nil {
		return nil, err
	}
	sysBundle, _, err := s.CompileProjectPrompts(p)
	if err != nil {
		return nil, err
	}
	provs := s.GetProviders()
	draft, err := provs.LLM.GenerateStory(ctx, provider.StoryRequest{
		ProjectID:    p.ID,
		Title:        p.Title,
		Theme:        p.Theme,
		Track:        p.Track,
		StylePreset:  p.StylePreset,
		TargetPacing: p.TargetPacing,
		SystemPrompt: sysBundle.StoryOutline,
	})
	if err != nil {
		return nil, fmt.Errorf("llm generate story: %w", err)
	}

	acts := make([]project.StoryActBeat, 0, len(draft.Acts))
	for _, a := range draft.Acts {
		acts = append(acts, project.StoryActBeat{
			Phase:   a.Phase,
			Title:   a.Title,
			Summary: a.Summary,
		})
	}
	story := &project.StoryScript{
		Logline:             draft.Logline,
		Synopsis:            draft.Synopsis,
		Acts:                acts,
		EmotionalTone:       draft.EmotionalTone,
		VisualAtmosphere:    draft.VisualAtmosphere,
		SuggestedShotReason: draft.SuggestedShotReason,
	}
	return s.mutateProject(projectID, func(cur *project.Project) error {
		cur.Story = story
		cur.Stage = project.StageStory
		cur.StoryConfirmed = false
		return nil
	})
}

// PlanStoryboardOnly Step 1 → Step 2：确认整体故事剧情后，由剧情自然决定分镜数量并拆解出故事板草案（暂不提前建资产库）。
func (s *Service) PlanStoryboardOnly(ctx context.Context, projectID string) (*project.Project, error) {
	return s.planInternal(ctx, projectID, false)
}

// PlanProject 一步式编排（兼容 CLI 或带本地素材导入的项目）：生成故事板并在无资产时同步写入资产清单。
func (s *Service) PlanProject(ctx context.Context, projectID string) (*project.Project, error) {
	return s.planInternal(ctx, projectID, true)
}

func (s *Service) planInternal(ctx context.Context, projectID string, populateAssets bool) (*project.Project, error) {
	p, err := project.Load(s.RootDir, projectID)
	if err != nil {
		return nil, err
	}
	sysBundle, _, err := s.CompileProjectPrompts(p)
	if err != nil {
		return nil, err
	}

	// 保留已导入的首帧图片路径与顺序
	existingFrames := make(map[string]string, len(p.Shots))
	existingIDs := make([]string, 0, len(p.Shots))
	for _, sh := range p.Shots {
		existingIDs = append(existingIDs, sh.ShotID)
		if sh.FirstFramePath != "" {
			existingFrames[sh.ShotID] = sh.FirstFramePath
		}
	}
	// 若已导入现成首帧图，则严格沿用其数量；否则传 0 让整体故事剧情自然决定分镜数！
	shotCount := 0
	if len(existingFrames) > 0 {
		shotCount = len(existingIDs)
	} else if p.ShotCount > 0 && p.Story == nil {
		shotCount = p.ShotCount
	}

	var storyDraft *provider.StoryDraft
	if p.Story != nil {
		acts := make([]provider.StoryActDraft, 0, len(p.Story.Acts))
		for _, a := range p.Story.Acts {
			acts = append(acts, provider.StoryActDraft{Phase: a.Phase, Title: a.Title, Summary: a.Summary})
		}
		storyDraft = &provider.StoryDraft{
			Logline:             p.Story.Logline,
			Synopsis:            p.Story.Synopsis,
			Acts:                acts,
			EmotionalTone:       p.Story.EmotionalTone,
			VisualAtmosphere:    p.Story.VisualAtmosphere,
			SuggestedShotReason: p.Story.SuggestedShotReason,
		}
	}

	pack, err := s.StrategyRepo.Get(p.Track)
	if err != nil {
		return nil, err
	}

	var feedback string
	var plan *provider.StoryboardPlan
	var lastReport psychology.CriticReport

	provs := s.GetProviders()
	for attempt := 0; attempt < 2; attempt++ {
		var passExistingIDs []string
		if len(existingFrames) > 0 {
			passExistingIDs = existingIDs
		}
		plan, err = provs.LLM.GenerateStoryboard(ctx, provider.StoryboardRequest{
			ProjectID:         p.ID,
			Title:             p.Title,
			Theme:             p.Theme,
			Track:             p.Track,
			StylePreset:       p.StylePreset,
			TargetPacing:      p.TargetPacing,
			Story:             storyDraft,
			SystemPrompt:      sysBundle.DirectorStoryboard,
			RevisionFeedback:  feedback,
			Assets:            p.AssetDescriptionMap(),
			ExistingShotIDs:   passExistingIDs,
			HasImportedFrames: len(existingFrames) > 0,
			ShotCount:         shotCount,
		})
		if err != nil {
			return nil, fmt.Errorf("llm generate storyboard: %w", err)
		}
		drafts := plan.Shots

		reviewItems := make([]psychology.ShotForReview, 0, len(drafts))
		for _, d := range drafts {
			reviewItems = append(reviewItems, psychology.ShotForReview{
				ShotID:         d.ShotID,
				Phase:          d.Phase,
				PsychTrigger:   d.PsychTrigger,
				Symbols:        d.Symbols,
				Narration:      d.Narration,
				DurationSec:    d.DurationSec,
				AmbientTag:     d.AmbientTag,
				MotionActionZH: d.I2VSpec.SubjectMicroAction.ZH,
				EnvPhysicsZH:   d.I2VSpec.EnvironmentPhysics.ZH,
				CameraMovement: d.I2VSpec.CameraMovement,
			})
		}
		lastReport = psychology.Evaluate(pack, reviewItems)
		if lastReport.Passed {
			break
		}
		feedback = lastReport.RevisionFeedback()
	}

	var newAssets []project.AssetRef
	if populateAssets && len(plan.Assets) > 0 {
		newAssets = make([]project.AssetRef, 0, len(plan.Assets))
		for _, a := range plan.Assets {
			newAssets = append(newAssets, project.AssetRef{
				ID:          a.ID,
				Role:        a.Role,
				Name:        a.Name,
				Description: a.Description,
				Prompt:      a.Prompt,
				SourceMode:  "generated",
			})
		}
	}

	drafts := plan.Shots
	newShots := make([]project.ShotSpec, 0, len(drafts))
	for i, d := range drafts {
		dur := d.DurationSec
		if dur <= 0 {
			dur = 5
		}
		summary := strings.TrimSpace(d.StorySummary)
		if summary == "" {
			summary = strings.TrimSpace(d.T2ISpec.SubjectAnchor.ZH + "；" + d.T2ISpec.ActionRelation.ZH)
		}
		defVoice := p.NarratorVoice
		if defVoice == "" {
			defVoice = "warm-male"
		}
		defSpeed := p.NarratorSpeed
		if defSpeed <= 0 {
			defSpeed = 0.92
		}
		sh := project.ShotSpec{
			ShotID:         d.ShotID,
			Order:          i + 1,
			Phase:          d.Phase,
			StorySummary:   summary,
			PsychTrigger:   d.PsychTrigger,
			Symbols:        d.Symbols,
			FirstFramePath: existingFrames[d.ShotID],
			Narration:      d.Narration,
			Emotion:        d.Emotion,
			VoiceID:        defVoice,
			VoiceSpeed:     defSpeed,
			AmbientTag:     d.AmbientTag,
			DurationSec:    dur,
			T2ISpec:        d.T2ISpec,
			I2VSpec:        d.I2VSpec,
			DirectorSheet:  d.DirectorSheet,
		}
		if sh.I2VSpec.TransitionMode == "" {
			sh.I2VSpec.TransitionMode = prompting.TransitionSingleFrame
		}
		newShots = append(newShots, sh)
	}

	// 自动为声明了首尾帧过渡（first_last_frame）的镜头绑定下一镜首帧作为尾帧
	for i := range newShots {
		if newShots[i].I2VSpec.TransitionMode == prompting.TransitionFirstLastFrame && newShots[i].LastFramePath == "" && i+1 < len(newShots) {
			newShots[i].LastFramePath = newShots[i+1].FirstFramePath
		}
	}

	// LLM 调用可能耗时数十秒，这里在锁内基于最新清单回写结果，避免覆盖期间的其他修改
	return s.mutateProject(projectID, func(cur *project.Project) error {
		if len(newAssets) > 0 && len(cur.Assets) == 0 {
			cur.Assets = newAssets
			cur.NormalizeAssets()
		}
		cur.Shots = newShots
		cur.ShotCount = len(newShots)
		cur.CriticReport = &lastReport
		if !populateAssets {
			cur.StoryConfirmed = true
			cur.Stage = project.StageStoryboard
			cur.StoryboardConfirmed = false
		} else if cur.Stage == "" {
			cur.NormalizeWorkflow()
		}
		cur.SyncShotAssetBindings()
		return nil
	})
}

// ExtractAssetsFromStoryboard Step 1 → Step 2：严格基于人工确认后的故事板反推项目资产清单，
// 并补全每个分镜的资产引用绑定、首帧生图槽位与工业级导演时序详版（绝不覆盖用户确认的旁白/剧情/已上传资产图）。
func (s *Service) ExtractAssetsFromStoryboard(ctx context.Context, projectID string) (*project.Project, error) {
	p, err := project.Load(s.RootDir, projectID)
	if err != nil {
		return nil, err
	}
	if len(p.Shots) == 0 {
		return nil, fmt.Errorf("project %s has no storyboard shots; create storyboard first", projectID)
	}
	sysBundle, _, err := s.CompileProjectPrompts(p)
	if err != nil {
		return nil, err
	}

	briefs := make([]provider.ConfirmedShotBrief, 0, len(p.Shots))
	for i, sh := range p.Shots {
		ord := sh.Order
		if ord <= 0 {
			ord = i + 1
		}
		briefs = append(briefs, provider.ConfirmedShotBrief{
			ShotID:       sh.ShotID,
			Order:        ord,
			Phase:        sh.Phase,
			StorySummary: sh.StorySummary,
			PsychTrigger: sh.PsychTrigger,
			Symbols:      sh.Symbols,
			Narration:    sh.Narration,
			Emotion:      sh.Emotion,
			AmbientTag:   sh.AmbientTag,
			DurationSec:  sh.DurationSec,
		})
	}

	existingDrafts := make([]provider.AssetDraft, 0, len(p.Assets))
	for _, a := range p.Assets {
		existingDrafts = append(existingDrafts, provider.AssetDraft{
			ID:          a.ID,
			Role:        a.Role,
			Name:        a.Name,
			Description: a.Description,
			Prompt:      a.Prompt,
		})
	}

	provs := s.GetProviders()
	res, err := provs.LLM.ExtractAssetsFromStoryboard(ctx, provider.AssetExtractionRequest{
		ProjectID:      p.ID,
		Title:          p.Title,
		Theme:          p.Theme,
		Track:          p.Track,
		StylePreset:    p.StylePreset,
		SystemPrompt:   sysBundle.AssetExtraction,
		ExistingAssets: existingDrafts,
		ConfirmedShots: briefs,
	})
	if err != nil {
		return nil, fmt.Errorf("extract assets from storyboard: %w", err)
	}

	return s.mutateProject(projectID, func(cur *project.Project) error {
		applyAssetExtraction(cur, res)
		cur.StoryboardConfirmed = true
		cur.Stage = project.StageAssetsPlan
		cur.SyncShotAssetBindings()
		_, _ = s.EvaluateProject(cur)
		return nil
	})
}

// applyAssetExtraction 把 LLM 提取的资产与每镜导演详版合并进项目：
// 保留已有资产的上传/导入图片、锁定状态与来源模式，绝不覆盖用户确认的旁白/剧情。
func applyAssetExtraction(p *project.Project, res *provider.AssetExtractionResult) {
	preservedByID := make(map[string]project.AssetRef, len(p.Assets))
	preservedByName := make(map[string]project.AssetRef, len(p.Assets))
	for _, a := range p.Assets {
		preservedByID[a.ID] = a
		preservedByName[strings.TrimSpace(a.Name)] = a
	}

	// 合并提取出的资产：保留已有资产的上传/导入图片、锁定状态与来源模式
	if len(res.Assets) > 0 {
		merged := make([]project.AssetRef, 0, len(res.Assets))
		seenIDs := make(map[string]struct{}, len(res.Assets))
		for _, a := range res.Assets {
			ref := project.AssetRef{
				ID:          a.ID,
				Role:        a.Role,
				Name:        a.Name,
				Description: a.Description,
				Prompt:      a.Prompt,
				SourceMode:  "generated",
			}
			if prev, ok := preservedByID[a.ID]; ok {
				if prev.Locked {
					ref = prev
				} else {
					ref.ImagePath = prev.ImagePath
					if prev.SourceMode != "" {
						ref.SourceMode = prev.SourceMode
					}
				}
			} else if prev, ok := preservedByName[strings.TrimSpace(a.Name)]; ok {
				ref.ID = prev.ID
				ref.ImagePath = prev.ImagePath
				ref.Locked = prev.Locked
				if prev.SourceMode != "" {
					ref.SourceMode = prev.SourceMode
				}
			}
			merged = append(merged, ref)
			seenIDs[ref.ID] = struct{}{}
		}
		// 保留所有已被人工锁定的既有资产，防止被覆盖丢失
		for _, prev := range p.Assets {
			if prev.Locked {
				if _, exists := seenIDs[prev.ID]; !exists {
					merged = append(merged, prev)
				}
			}
		}
		p.Assets = merged
		p.NormalizeAssets()
	}

	// 将每镜反推的资产绑定、T2I/I2V 与导演时序详版回填到已确认的故事板分镜上
	updateByID := make(map[string]provider.ShotDirectorEnrichment, len(res.Shots))
	for _, u := range res.Shots {
		updateByID[u.ShotID] = u
	}
	for i := range p.Shots {
		if u, ok := updateByID[p.Shots[i].ShotID]; ok {
			if u.T2ISpec.SubjectAnchor.ZH != "" || u.T2ISpec.SubjectAnchor.EN != "" {
				p.Shots[i].T2ISpec = u.T2ISpec
			}
			if u.I2VSpec.SubjectMicroAction.ZH != "" || u.I2VSpec.SubjectMicroAction.EN != "" {
				if u.I2VSpec.TransitionMode == "" {
					u.I2VSpec.TransitionMode = prompting.TransitionSingleFrame
				}
				p.Shots[i].I2VSpec = u.I2VSpec
			}
			if len(u.DirectorSheet.TimelineBeats) > 0 || len(u.DirectorSheet.AssetBindings) > 0 {
				p.Shots[i].DirectorSheet = u.DirectorSheet
			}
		}
	}
}

// UpdateWorkflowStage 更新项目的 6 步流水线阶段与确认标记（支持柔性前后切换与按需跳过分镜定妆图）。
func (s *Service) UpdateWorkflowStage(projectID string, stage project.WorkflowStage, storyConfirmed, storyboardConfirmed, assetsConfirmed, keyframesSkipped *bool) (*project.Project, error) {
	return s.mutateProject(projectID, func(p *project.Project) error {
		if stage != "" {
			p.Stage = stage
		}
		if storyConfirmed != nil {
			p.StoryConfirmed = *storyConfirmed
		}
		if storyboardConfirmed != nil {
			p.StoryboardConfirmed = *storyboardConfirmed
		}
		if assetsConfirmed != nil {
			p.AssetsConfirmed = *assetsConfirmed
		}
		if keyframesSkipped != nil {
			p.KeyframesSkipped = *keyframesSkipped
		}
		return nil
	})
}

func pacingLabelZH(pacing string) string {
	switch pacing {
	case "short":
		return "精炼短片（约 20~30 秒，紧凑凝练）"
	case "standard":
		return "完整叙事（约 40~60 秒，舒缓留白）"
	default:
		return "由故事剧情自然决定（推荐，不预设固定分镜数）"
	}
}

// RenderAssets 为项目资产库（角色/场景/关键道具）批量或单张生成定妆参考图（T2I）。
func (s *Service) RenderAssets(ctx context.Context, projectID, assetID string, force bool) (*project.Project, error) {
	p, err := project.Load(s.RootDir, projectID)
	if err != nil {
		return nil, err
	}
	p.NormalizeAssets()
	if len(p.Assets) == 0 {
		return nil, fmt.Errorf("project %s has no assets defined", projectID)
	}
	style, err := s.Compiler.GetStyle(p.StylePreset)
	if err != nil {
		return nil, err
	}

	provs := s.GetProviders()
	assetsDir := filepath.Join(project.ProjectDir(s.RootDir, projectID), "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return nil, err
	}

	generated := make(map[string]string)
	applyGenerated := func() (*project.Project, error) {
		return s.mutateProject(projectID, func(cur *project.Project) error {
			for i := range cur.Assets {
				if path, ok := generated[cur.Assets[i].ID]; ok {
					cur.Assets[i].ImagePath = path
					cur.Assets[i].SourceMode = "generated"
				}
			}
			return nil
		})
	}

	matched := 0
	for i := range p.Assets {
		a := &p.Assets[i]
		if assetID != "" && a.ID != assetID {
			continue
		}
		matched++
		if a.SourceMode == "text_only" {
			continue
		}
		if a.Locked && !force {
			continue
		}
		if !force && assetID == "" && a.ImagePath != "" {
			if _, statErr := os.Stat(a.ImagePath); statErr == nil {
				continue
			}
		}

		compiled := s.Compiler.CompileAssetT2I(a.Name, a.Role, a.Description, a.Prompt, style, p.PromptLang)
		outPath := filepath.Join(assetsDir, fmt.Sprintf("%s.jpeg", a.ID))

		s.Broadcast(scheduler.ProgressEvent{
			ProjectID: projectID,
			ShotID:    a.ID,
			Kind:      "asset_t2i",
			Stage:     "running",
			Message:   fmt.Sprintf("正在生成资产定妆图 %s（%s）...", a.Name, project.AssetRefToken(i)),
		})

		genPath, err := provs.T2I.Generate(ctx, provider.T2IRequest{
			Prompt:         compiled.Prompt,
			NegativePrompt: compiled.NegativePrompt,
			AspectRatio:    p.AspectRatio,
			OutputPath:     outPath,
			Seed:           time.Now().UnixNano()%100000 + int64(i*17+1),
		})
		if err != nil {
			s.Broadcast(scheduler.ProgressEvent{
				ProjectID: projectID,
				ShotID:    a.ID,
				Kind:      "asset_t2i",
				Stage:     "failed",
				Message:   err.Error(),
			})
			// 已成功生成的资产图仍然回填，避免重跑时重复扣费
			if len(generated) > 0 {
				if _, saveErr := applyGenerated(); saveErr != nil {
					return nil, fmt.Errorf("generate asset %s (%s): %w (另外保存已生成资产失败: %v)", a.ID, a.Name, err, saveErr)
				}
			}
			return nil, fmt.Errorf("generate asset %s (%s): %w", a.ID, a.Name, err)
		}
		generated[a.ID] = genPath

		s.Broadcast(scheduler.ProgressEvent{
			ProjectID:  projectID,
			ShotID:     a.ID,
			Kind:       "asset_t2i",
			Stage:      "succeeded",
			Message:    fmt.Sprintf("资产图 %s 生成完成", a.Name),
			OutputPath: genPath,
		})
	}
	if assetID != "" && matched == 0 {
		return nil, fmt.Errorf("asset %q not found in project %s", assetID, projectID)
	}
	return applyGenerated()
}

// SaveUploadedAssetImage 保存通过浏览器上传的资产参考图并回填到项目资产库。
func (s *Service) SaveUploadedAssetImage(projectID, assetID, ext string, data []byte) (*project.Project, error) {
	ext = normalizeImageExt(ext)
	if ext == "" {
		return nil, fmt.Errorf("unsupported image format; use jpg/jpeg/png/webp")
	}
	return s.mutateProject(projectID, func(p *project.Project) error {
		p.NormalizeAssets()
		for i := range p.Assets {
			if p.Assets[i].ID != assetID {
				continue
			}
			assetsDir := filepath.Join(project.ProjectDir(s.RootDir, projectID), "assets")
			if err := os.MkdirAll(assetsDir, 0o755); err != nil {
				return err
			}
			outPath := filepath.Join(assetsDir, fmt.Sprintf("%s_upload%s", p.Assets[i].ID, ext))
			if err := os.WriteFile(outPath, data, 0o644); err != nil {
				return err
			}
			p.Assets[i].ImagePath = outPath
			p.Assets[i].SourceMode = "uploaded"
			return nil
		}
		return fmt.Errorf("asset %q not found in project %s", assetID, projectID)
	})
}

// SaveUploadedShotFrame 保存通过浏览器上传的分镜首帧图并回填到对应分镜。
func (s *Service) SaveUploadedShotFrame(projectID, shotID, ext string, data []byte) (*project.Project, error) {
	ext = normalizeImageExt(ext)
	if ext == "" {
		return nil, fmt.Errorf("unsupported image format; use jpg/jpeg/png/webp")
	}
	return s.mutateProject(projectID, func(p *project.Project) error {
		for i := range p.Shots {
			if p.Shots[i].ShotID != shotID {
				continue
			}
			framesDir := filepath.Join(project.OutputDir(s.RootDir, projectID), "frames")
			if err := os.MkdirAll(framesDir, 0o755); err != nil {
				return err
			}
			outPath := filepath.Join(framesDir, fmt.Sprintf("%s_upload%s", p.Shots[i].ShotID, ext))
			if err := os.WriteFile(outPath, data, 0o644); err != nil {
				return err
			}
			p.Shots[i].FirstFramePath = outPath
			return nil
		}
		return fmt.Errorf("shot %q not found in project %s", shotID, projectID)
	})
}

func normalizeImageExt(ext string) string {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".jpg", ".jpeg":
		return ".jpeg"
	case ".png":
		return ".png"
	case ".webp":
		return ".webp"
	default:
		return ""
	}
}

func normalizeAudioExt(ext string) string {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".mp3":
		return ".mp3"
	case ".wav":
		return ".wav"
	case ".m4a":
		return ".m4a"
	case ".aac":
		return ".aac"
	case ".ogg":
		return ".ogg"
	default:
		return ""
	}
}

// SaveUploadedBGM 保存用户通过浏览器上传的自定义背景音乐文件并绑定到项目混音配置。
func (s *Service) SaveUploadedBGM(projectID, ext string, data []byte) (*project.Project, error) {
	ext = normalizeAudioExt(ext)
	if ext == "" {
		return nil, fmt.Errorf("unsupported audio format; use mp3/wav/m4a/aac/ogg")
	}
	return s.mutateProject(projectID, func(p *project.Project) error {
		assetsDir := filepath.Join(project.ProjectDir(s.RootDir, projectID), "assets")
		if err := os.MkdirAll(assetsDir, 0o755); err != nil {
			return err
		}
		outPath := filepath.Join(assetsDir, "bgm_custom"+ext)
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return err
		}
		p.Mix.BGMPath = outPath
		p.Mix.BGMPreset = "custom"
		return nil
	})
}

// EnsureBGMPresetPreview 生成或返回内置怀旧配乐预设的 8 秒试听音频路径。
func (s *Service) EnsureBGMPresetPreview(ctx context.Context, preset string) (string, error) {
	if preset == "" || preset == "custom" || preset == "none" {
		preset = "warm_pad"
	}
	if !isBuiltInBGMPreset(preset) {
		return "", fmt.Errorf("unknown bgm preset %q", preset)
	}
	prevDir := filepath.Join(s.RootDir, ".cache", "bgm_previews")
	if err := os.MkdirAll(prevDir, 0o755); err != nil {
		return "", err
	}
	outPath := filepath.Join(prevDir, "preview_"+preset+".wav")
	if _, err := os.Stat(outPath); err == nil {
		return outPath, nil
	}
	if _, err := composer.SynthesizeBGMPreset(ctx, preset, 8.0, outPath); err != nil {
		return "", err
	}
	return outPath, nil
}

func isBuiltInBGMPreset(id string) bool {
	for _, b := range project.BuiltInBGMPresets() {
		if b.ID == id {
			return true
		}
	}
	return false
}

type RenderOptions struct {
	ShotID     string
	OnlyKind   store.JobKind
	ForceRetry bool
	Workers    int
}

// RenderProject 执行全项目或单镜头的 T2I（可选）/ I2V / TTS / SFX 并发渲染。
func (s *Service) RenderProject(ctx context.Context, projectID string, opts RenderOptions) error {
	p, err := project.Load(s.RootDir, projectID)
	if err != nil {
		return err
	}
	if len(p.Shots) == 0 {
		return fmt.Errorf("project %s has no shots; run plan first", projectID)
	}
	style, err := s.Compiler.GetStyle(p.StylePreset)
	if err != nil {
		return err
	}

	outDir := project.OutputDir(s.RootDir, projectID)
	framesDir := filepath.Join(outDir, "frames")
	clipsDir := filepath.Join(outDir, "clips")
	audioDir := filepath.Join(outDir, "audio")
	sfxDir := filepath.Join(outDir, "sfx")

	provs := s.GetProviders()
	effectiveMode := p.PromptMode
	if effectiveMode == "" {
		effectiveMode = providerfactory.RecommendedPromptMode(provs.ActiveIDs.I2V)
	}

	p.SyncShotAssetBindings()
	var tasks []scheduler.ShotTask
	for i, sh := range p.Shots {
		if opts.ShotID != "" && sh.ShotID != opts.ShotID {
			continue
		}
		ord := sh.Order
		if ord <= 0 {
			ord = i + 1
		}
		compiled := s.Compiler.CompileShotFull(sh.ShotID, ord, sh.DurationSec, sh.T2ISpec, sh.I2VSpec, sh.DirectorSheet, style)

		t2iPrompt := compiled.T2IZH
		i2vText := compiled.I2VZH.Prompt
		i2vNeg := compiled.I2VZH.NegativePrompt
		if p.PromptLang == prompting.LangEN {
			t2iPrompt = compiled.T2IEN
			if effectiveMode == prompting.ModeDirector {
				// 仅喂给视频 API 纯净生成指令（自动剔除后期字幕与剪辑转场说明）
				i2vText = compiled.I2VDirectorEN.GenerationPrompt
				i2vNeg = compiled.I2VDirectorEN.NegativePrompt
			} else {
				i2vText = compiled.I2VEN.Prompt
				i2vNeg = compiled.I2VEN.NegativePrompt
			}
		} else if effectiveMode == prompting.ModeDirector {
			i2vText = compiled.I2VDirectorZH.GenerationPrompt
			i2vNeg = compiled.I2VDirectorZH.NegativePrompt
		}

		// 严格只取当前分镜显式绑定的参考资产图，杜绝无关资产串镜
		shotAssetRefs := p.ResolveShotReferenceImages(sh)

		framePath := sh.FirstFramePath
		var t2iReq *provider.T2IRequest
		// 默认生成分镜首帧定妆图后再图生视频；当用户显式开启跳过首帧（keyframes_skipped）且非定向单跑 t2i 时，直接走多参考图/文生视频！
		needT2I := opts.OnlyKind == store.KindT2I || (framePath == "" && !p.KeyframesSkipped)
		if needT2I {
			generatedPath := filepath.Join(framesDir, sh.ShotID+".jpeg")
			seed := int64(1000 + i)
			if opts.ForceRetry {
				seed = time.Now().UnixNano()%100000 + int64(i)
			}
			t2iReq = &provider.T2IRequest{
				Prompt:          t2iPrompt.Prompt,
				NegativePrompt:  t2iPrompt.NegativePrompt,
				ReferenceImages: shotAssetRefs,
				AspectRatio:     p.AspectRatio,
				OutputPath:      generatedPath,
				Seed:            seed,
			}
		}

		lastFrame := sh.LastFramePath
		if sh.I2VSpec.TransitionMode == prompting.TransitionFirstLastFrame && lastFrame == "" && i+1 < len(p.Shots) {
			lastFrame = p.Shots[i+1].FirstFramePath
		}

		var ttsReq *provider.TTSRequest
		if strings.TrimSpace(sh.Narration) != "" {
			voiceID := strings.TrimSpace(sh.VoiceID)
			if voiceID == "" || voiceID == "warm-male" && p.NarratorVoice != "" {
				voiceID = p.NarratorVoice
			}
			if voiceID == "" {
				voiceID = "warm-male"
			}
			voiceSpeed := sh.VoiceSpeed
			if voiceSpeed <= 0 {
				voiceSpeed = p.NarratorSpeed
			}
			if voiceSpeed <= 0 {
				voiceSpeed = 0.92
			}
			ttsReq = &provider.TTSRequest{
				Text:       sh.Narration,
				VoiceID:    voiceID,
				Emotion:    sh.Emotion,
				Speed:      voiceSpeed,
				OutputPath: filepath.Join(audioDir, sh.ShotID+".wav"),
			}
		}

		var sfxReq *provider.SFXRequest
		if strings.TrimSpace(sh.AmbientTag) != "" {
			sfxReq = &provider.SFXRequest{
				Tag:         sh.AmbientTag,
				DurationSec: sh.DurationSec,
				OutputPath:  filepath.Join(sfxDir, sh.ShotID+".wav"),
			}
		}

		tasks = append(tasks, scheduler.ShotTask{
			ProjectID: p.ID,
			ShotID:    sh.ShotID,
			T2IReq:    t2iReq,
			I2VReq: provider.I2VSubmitRequest{
				FirstFramePath:  framePath,
				LastFramePath:   lastFrame,
				ReferenceImages: shotAssetRefs,
				Prompt:          i2vText,
				NegativePrompt:  i2vNeg,
				TransitionMode:  sh.I2VSpec.TransitionMode,
				CameraMovement:  sh.I2VSpec.CameraMovement,
				PromptLang:      p.PromptLang,
				AspectRatio:     p.AspectRatio,
				DurationSec:     sh.DurationSec,
				Seed:            int64(2000 + i),
			},
			TTSReq:     ttsReq,
			SFXReq:     sfxReq,
			OutputDir:  clipsDir,
			ForceRetry: opts.ForceRetry,
			OnlyKind:   opts.OnlyKind,
			Locked:     sh.Locked,
		})
	}
	if len(tasks) == 0 {
		return fmt.Errorf("shot %q not found in project %s", opts.ShotID, projectID)
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = 3
	}
	pollInterval := 2 * time.Second
	if strings.HasPrefix(provs.I2V.Name(), "mock-") {
		pollInterval = 50 * time.Millisecond
	}

	sched := scheduler.New(
		s.Store,
		provs.I2V,
		provs.TTS,
		workers,
		20,
		scheduler.WithT2I(provs.T2I),
		scheduler.WithSFX(provs.SFX),
		scheduler.WithPollInterval(pollInterval),
		scheduler.WithContinueOnError(true),
		scheduler.WithProgressHook(s.Broadcast),
	)
	if err := sched.RunBatch(ctx, tasks); err != nil {
		return err
	}

	// 若有新通过 T2I 生成/重绘的首帧图，自动回填至 project.yaml 便于 Web 界面展示。
	// 渲染可能持续数分钟，这里在锁内基于最新清单只回填首帧路径，不覆盖期间用户的其他修改。
	_, err = s.mutateProject(projectID, func(cur *project.Project) error {
		for i := range cur.Shots {
			if opts.ShotID != "" && cur.Shots[i].ShotID != opts.ShotID {
				continue
			}
			if cur.Shots[i].FirstFramePath == "" || opts.OnlyKind == store.KindT2I {
				candidate := filepath.Join(framesDir, cur.Shots[i].ShotID+".jpeg")
				if _, err := os.Stat(candidate); err == nil {
					cur.Shots[i].FirstFramePath = candidate
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("save generated keyframe paths: %w", err)
	}
	return nil
}

// ComposeProject 将已渲染的分镜视频、情感旁白与环境声景通过 FFmpeg 精编为最终成片 final.mp4。
func (s *Service) ComposeProject(ctx context.Context, projectID string) (*composer.VideoInfo, error) {
	p, err := project.Load(s.RootDir, projectID)
	if err != nil {
		return nil, err
	}
	style, err := s.Compiler.GetStyle(p.StylePreset)
	if err != nil {
		return nil, err
	}

	outDir := project.OutputDir(s.RootDir, projectID)
	clipsDir := filepath.Join(outDir, "clips")
	audioDir := filepath.Join(outDir, "audio")
	sfxDir := filepath.Join(outDir, "sfx")

	var clips []composer.ShotClip
	for _, sh := range p.Shots {
		videoPath := filepath.Join(clipsDir, sh.ShotID+".mp4")
		if _, err := os.Stat(videoPath); err != nil {
			return nil, fmt.Errorf("missing rendered video clip for shot %s (%s); run render first", sh.ShotID, videoPath)
		}
		voicePath := filepath.Join(audioDir, sh.ShotID+".wav")
		if _, err := os.Stat(voicePath); err != nil {
			voicePath = ""
		}
		ambientPath := filepath.Join(sfxDir, sh.ShotID+".wav")
		if _, err := os.Stat(ambientPath); err != nil {
			ambientPath = ""
		}
		clips = append(clips, composer.ShotClip{
			ShotID:      sh.ShotID,
			VideoPath:   videoPath,
			VoicePath:   voicePath,
			AmbientPath: ambientPath,
			Narration:   sh.Narration,
			DurationSec: float64(sh.DurationSec),
		})
	}

	s.Broadcast(scheduler.ProgressEvent{
		ProjectID: projectID,
		Kind:      "compose",
		Stage:     "running",
		Message:   "正在执行 FFmpeg 胶片调色、旁白时长对齐、三轨混音与电影硬字幕/封面压制...",
	})

	subtitleLine := p.Theme
	if p.Story != nil && strings.TrimSpace(p.Story.Logline) != "" {
		subtitleLine = p.Story.Logline
	}
	finalPath := filepath.Join(outDir, "final.mp4")
	coverPath := filepath.Join(outDir, "cover.jpg")
	info, err := composer.Compose(ctx, composer.ComposeRequest{
		Title:           p.Title,
		Subtitle:        subtitleLine,
		AspectRatio:     p.AspectRatio,
		Clips:           clips,
		Style:           style,
		Mix:             p.Mix,
		OutputPath:      finalPath,
		CoverOutputPath: coverPath,
		WorkDir:         filepath.Join(outDir, ".compose_work"),
	})
	if err != nil {
		s.Broadcast(scheduler.ProgressEvent{
			ProjectID: projectID,
			Kind:      "compose",
			Stage:     "failed",
			Message:   err.Error(),
		})
		return nil, err
	}

	s.Broadcast(scheduler.ProgressEvent{
		ProjectID:  projectID,
		Kind:       "compose",
		Stage:      "succeeded",
		Message:    fmt.Sprintf("成片合成完成（%.1fs，%dx%d）", info.DurationSec, info.Width, info.Height),
		OutputPath: finalPath,
	})
	return info, nil
}
