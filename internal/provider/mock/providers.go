package mock

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"feelings-studio/internal/prompting"
	"feelings-studio/internal/provider"
	"feelings-studio/internal/psychology"
)

// --- 1. MockLLM：基于社会心理学策略输出高完成度中英双语结构化分镜 ---

type MockLLM struct{}

func NewMockLLM() *MockLLM { return &MockLLM{} }

func (m *MockLLM) Name() string { return "mock-psychology-director" }

// GenerateStory Step 1（Mock）：围绕当前主题与社会心理学赛道创作一份完整的电影感故事剧情大纲，
// 并依据剧情复杂度与篇幅倾向给出合理的拆镜建议（不提前锁死镜头数）。
func (m *MockLLM) GenerateStory(_ context.Context, req provider.StoryRequest) (*provider.StoryDraft, error) {
	theme := strings.TrimSpace(req.Theme)
	if theme == "" {
		theme = strings.TrimSpace(req.Title)
	}
	if theme == "" {
		theme = "记忆里那个慢下来的午后"
	}

	pacingNote := "剧情包含感官入场、日常仪式、人物微光与时空余韵四段完整推进，建议拆解为 6 个分镜（约 30~35 秒），为每段情绪留足无旁白呼吸空间。"
	switch req.TargetPacing {
	case "short":
		pacingNote = "偏精炼短片节奏，建议拆解为 4~5 个凝练分镜（约 20~26 秒），每幕精准落点一个高共鸣画面。"
	case "standard":
		pacingNote = "偏完整电影叙事节奏，建议拆解为 7~8 个舒缓分镜（约 40~48 秒），给足人物互动细节与前后时空对照余韵。"
	}

	switch req.Track {
	case psychology.TrackFamilyBond:
		return &provider.StoryDraft{
			Logline:  fmt.Sprintf("围绕「%s」，记录中国式父母从不说出口却藏在凌晨灯光与鼓囊行囊里的笨拙牵挂。", theme),
			Synopsis: fmt.Sprintf("天还没亮透，老家厨房那盏暖黄白炽灯已经亮起。围绕「%s」，母亲系着洗旧的棉围裙安静忙活，父亲戴着老花镜在旧木饭桌旁把自家晒干的吃食一层层装袋、仔细打上双结。嘴上总念叨着别带太多，手里却还是不断往里塞。车子驶离老小区路口时，后视镜里并肩站着的两个微驼背影一直目送到拐角。入夜后，老居民楼三楼那扇温暖发亮的窗口，成了远行者心里永远有处可归的安全基地。", theme),
			Acts: []provider.StoryActDraft{
				{Phase: psychology.PhaseSensoryHook, Title: "清晨暖灯与烟火声", Summary: fmt.Sprintf("天色微蓝未亮，围绕「%s」展开老家厨房亮起的暖黄灯光、升腾白气与克制环境音。", theme)},
				{Phase: psychology.PhaseConcreteRitual, Title: "反复塞满的具象照料", Summary: "磨亮木桌旁，父母略显粗糙的双手仔细打包家乡吃食并系紧双结，用具体动作替代直白抒情。"},
				{Phase: psychology.PhaseRelationalWarmth, Title: "路口目送与无声挥别", Summary: "清晨红砖老楼路口，父母并肩静静站立，母亲缓缓抬手轻挥，直到离去的身影拐过街角。"},
				{Phase: psychology.PhaseLiminalClosure, Title: "始终亮着的那扇窗", Summary: "深蓝暮色中老楼三楼亮着琥珀暖灯的一扇窗，晚风轻拂纱帘，留下“总有人等你回家”的安心余韵。"},
			},
			EmotionalTone:       "含蓄克制、平实温暖、略带微酸而落点笃定安心",
			VisualAtmosphere:    "清晨微蓝冷光与室内暖黄白炽灯对照，家用 DV / 纪实胶片柔光质感，伴随沸水声、老挂钟与微风",
			SuggestedShotReason: pacingNote,
		}, nil

	case psychology.TrackUrbanSanctuary:
		return &provider.StoryDraft{
			Logline:  fmt.Sprintf("围绕「%s」，在高压都市的间隙里，为疲惫的成年人切出一块允许停下来的五分钟心理避风港。", theme),
			Synopsis: fmt.Sprintf("晚高峰下起细雨，关掉最后一个工作窗口，车窗玻璃上缓缓滑落的水珠映出城市模糊的尾灯与年轻人微倦的剪影。回到安静的租住小屋，围绕「%s」，先按下薄荷绿老电扇的开关，看透明玻璃杯里的冰块与细小气泡在夕阳水纹中慢慢升起。阳台上刚晒过太阳的棉被边，橘猫在琥珀暖光里安稳打盹，没有人催促下一步。暮色降临，晚风轻托起白纱窗帘，允许此刻什么都不想、先慢慢呼吸。", theme),
			Acts: []provider.StoryActDraft{
				{Phase: psychology.PhaseSensoryHook, Title: "雨夜车窗与都市疲惫切口", Summary: "傍晚细雨轻敲车窗，冷青色城市车流光斑缓慢流淌，建立高压都市人的真实当下代入。"},
				{Phase: psychology.PhaseConcreteRitual, Title: "回到小屋的感官降噪仪式", Summary: fmt.Sprintf("围绕「%s」，老电扇慢摇出温柔风声，玻璃杯里的气泡与夕阳水纹光斑迅速卸下认知负荷。", theme)},
				{Phase: psychology.PhaseRelationalWarmth, Title: "无目的的松弛片刻", Summary: "洒满落日余晖的老阳台上，晒过太阳的棉被与打盹橘猫在晚风中呼吸，构建安全松弛体感。"},
				{Phase: psychology.PhaseLiminalClosure, Title: "晚风纱帘与心理许可", Summary: "暮色暖青与落日余晖交融，白纱帘随晚风轻起轻落，温柔给予观众“今晚不用急着赶路”的许可。"},
			},
			EmotionalTone:       "轻缓治愈、低饱和松弛、不强行打鸡血的温柔庇护感",
			VisualAtmosphere:    "由克制冷青晚高峰自然过渡到低饱和琥珀夕阳与暖青暮色，细雨、老电扇低鸣与晚风铺底",
			SuggestedShotReason: pacingNote,
		}, nil

	default:
		return &provider.StoryDraft{
			Logline:  fmt.Sprintf("借由「%s」这枚记忆信物，带忙碌的成年人悄悄回到那个阳光很慢、陪伴很近的旧日午后。", theme),
			Synopsis: fmt.Sprintf("记忆里的夏天总是从一道斜照进老屋与街巷的暖黄午后阳光开始。围绕「%s」，斑驳墙面、旧木门窗与带着使用磨损痕迹的老物件静静停在时光里。那时候日子简单而笃定，指尖触碰熟悉物件的温度、身旁长辈或挚友含蓄却真诚的一个微笑与转身，构成了成年后反复汲取力量的温暖底色。多年以后环境几经变迁，唯有记忆深处那束琥珀色黄昏光影与静候在原地的旧物，依然温柔提醒着我们：那些被好好善待过的瞬间，从未真正离开。", theme),
			Acts: []provider.StoryActDraft{
				{Phase: psychology.PhaseSensoryHook, Title: "午后光影与感官时空坐标", Summary: fmt.Sprintf("以斑驳老建筑、斜阳树影与微风蝉鸣建立「%s」所处的怀旧时空入口。", theme)},
				{Phase: psychology.PhaseConcreteRitual, Title: "旧物触感与日常仪式", Summary: fmt.Sprintf("聚焦「%s」最具代表性的年代老物件与手部细微动作，用触觉与光影唤醒集体记忆。", theme)},
				{Phase: psychology.PhaseRelationalWarmth, Title: "含蓄善意与陪伴微光", Summary: "在温暖自然光中定格人物克制而真诚的眼神交汇或并肩背影，不煽情却直抵内心柔软处。"},
				{Phase: psychology.PhaseLiminalClosure, Title: "黄昏空镜与自我连续性留白", Summary: "回到琥珀金黄昏下的静谧空镜，留出大片呼吸感与开放式回忆提问，让观众投射自己的人生记忆。"},
			},
			EmotionalTone:       "怀旧温润、克制抒情、从淡淡感怀落回治愈与自我和解",
			VisualAtmosphere:    "1990年代 35mm 柯达暖黄胶片质感，低角度午后与黄昏斜阳，轻柔微尘、蝉鸣与晚风虫鸣",
			SuggestedShotReason: pacingNote,
		}, nil
	}
}

func (m *MockLLM) GenerateStoryboard(_ context.Context, req provider.StoryboardRequest) (*provider.StoryboardPlan, error) {
	theme := strings.TrimSpace(req.Theme)
	if theme == "" {
		theme = strings.TrimSpace(req.Title)
	}

	// 仅当明确导入了 g1~g10 小卖部素材或主题明确指向“小卖部/冰棍”且为 summer-1998 时，才使用专属 10 镜示例；
	// 其余所有新建项目均严格围绕当前项目主题与已确认故事动态构建干净分镜，绝不串味旧项目！
	isSummerDemo := req.Track == psychology.TrackChildhood90s && (strings.Contains(theme, "小卖部") ||
		strings.Contains(theme, "冰棍") ||
		strings.Contains(strings.ToLower(req.ProjectID), "summer-1998") ||
		(req.HasImportedFrames && len(req.ExistingShotIDs) == 10 && req.ExistingShotIDs[0] == "g1"))

	// 若未指定固定镜头数（ShotCount <= 0），完全由整体故事剧情与节奏倾向自然决定拆镜数量！
	decidedCount := req.ShotCount
	if decidedCount <= 0 && len(req.ExistingShotIDs) == 0 && !isSummerDemo {
		decidedCount = decideShotCountFromStory(req.Story, req.TargetPacing)
	}

	var assets []provider.AssetDraft
	var drafts []provider.ShotDraft

	switch {
	case isSummerDemo:
		drafts = summer1998Drafts()
		assets = summer1998Assets()
	case req.Track == psychology.TrackFamilyBond:
		assets, drafts = buildThemeAwareFamilyPlan(theme, decidedCount)
	case req.Track == psychology.TrackUrbanSanctuary:
		assets, drafts = buildThemeAwareUrbanPlan(theme, decidedCount)
	default:
		assets, drafts = buildThemeAwareChildhoodPlan(theme, decidedCount)
	}

	// 若已有 Step 1 人工确认后的整体故事（Story），将用户确认的各幕剧情自然映射到拆解出的分镜上
	if !isSummerDemo && req.Story != nil && len(req.Story.Acts) > 0 {
		actByPhase := make(map[psychology.ActPhase]provider.StoryActDraft, len(req.Story.Acts))
		for _, act := range req.Story.Acts {
			actByPhase[act.Phase] = act
		}
		phaseSeen := map[psychology.ActPhase]int{}
		for i := range drafts {
			act, ok := actByPhase[drafts[i].Phase]
			if !ok || strings.TrimSpace(act.Summary) == "" {
				continue
			}
			phaseSeen[drafts[i].Phase]++
			if phaseSeen[drafts[i].Phase] == 1 {
				drafts[i].StorySummary = fmt.Sprintf("【%s】%s", act.Title, act.Summary)
			} else {
				drafts[i].StorySummary = fmt.Sprintf("【%s · 细节延展】%s（聚焦细微动作与情绪呼吸）", act.Title, act.Summary)
			}
			drafts[i].T2ISpec.SubjectAnchor.ZH = drafts[i].StorySummary
		}
	}

	drafts = enrichDirectorSheets(drafts, assets, isSummerDemo)
	for i := range drafts {
		if strings.TrimSpace(drafts[i].StorySummary) == "" {
			drafts[i].StorySummary = strings.TrimSpace(drafts[i].T2ISpec.SubjectAnchor.ZH + "；" + drafts[i].T2ISpec.ActionRelation.ZH)
		}
	}

	// 若调用方导入了现有图片（如 g1~g10 或 s1~s6），严格对齐 ShotID 与数量
	if len(req.ExistingShotIDs) > 0 {
		out := make([]provider.ShotDraft, 0, len(req.ExistingShotIDs))
		for i, id := range req.ExistingShotIDs {
			d := drafts[i%len(drafts)]
			d.ShotID = id
			out = append(out, d)
		}
		return &provider.StoryboardPlan{Assets: assets, Shots: out}, nil
	}
	if decidedCount > 0 && decidedCount < len(drafts) {
		drafts = drafts[:decidedCount]
	}
	return &provider.StoryboardPlan{Assets: assets, Shots: drafts}, nil
}

// decideShotCountFromStory 根据确认后的整体故事体量与节奏倾向自然决定分镜数量。
func decideShotCountFromStory(story *provider.StoryDraft, pacing string) int {
	switch pacing {
	case "short":
		return 4
	case "standard":
		return 7
	default:
		// auto：根据故事正文丰富度与幕次自然决定（通常 5~6 镜）
		if story != nil {
			runes := utf8.RuneCountInString(strings.TrimSpace(story.Synopsis))
			if runes >= 180 {
				return 6
			}
			if runes >= 100 {
				return 5
			}
		}
		return 5
	}
}

// ExtractAssetsFromStoryboard Step 2（Mock）：读取人工确认后的真实故事板，反推契合的资产清单，
// 并按每镜确认的 StorySummary / Narration / Phase 动态生成精确的资产绑定与导演时序详版。
func (m *MockLLM) ExtractAssetsFromStoryboard(_ context.Context, req provider.AssetExtractionRequest) (*provider.AssetExtractionResult, error) {
	theme := strings.TrimSpace(req.Theme)
	if theme == "" {
		theme = strings.TrimSpace(req.Title)
	}
	if theme == "" {
		theme = "温情记忆短片"
	}

	// 1. 保留用户已有资产；若尚未创建，则结合赛道、主题与故事板中出现的符号提炼核心资产
	assets := make([]provider.AssetDraft, 0, len(req.ExistingAssets)+3)
	assets = append(assets, req.ExistingAssets...)
	if len(assets) == 0 {
		var symbolHints []string
		seenSym := map[string]struct{}{}
		for _, sh := range req.ConfirmedShots {
			for _, sym := range sh.Symbols {
				s := strings.TrimSpace(sym)
				if s != "" {
					if _, ok := seenSym[s]; !ok {
						seenSym[s] = struct{}{}
						symbolHints = append(symbolHints, s)
					}
				}
			}
		}
		switch req.Track {
		case psychology.TrackFamilyBond:
			assets, _ = buildThemeAwareFamilyPlan(theme, len(req.ConfirmedShots))
		case psychology.TrackUrbanSanctuary:
			assets, _ = buildThemeAwareUrbanPlan(theme, len(req.ConfirmedShots))
		default:
			if strings.Contains(theme, "小卖部") || strings.Contains(theme, "冰棍") {
				assets = summer1998Assets()
			} else {
				assets, _ = buildThemeAwareChildhoodPlan(theme, len(req.ConfirmedShots))
			}
		}
		// 将人工在故事板里编辑/确认的具体符号注入核心道具描述，避免脱节
		if len(symbolHints) > 0 && len(assets) >= 3 {
			propHint := strings.Join(symbolHints[:minInt(len(symbolHints), 2)], "、")
			assets[2].Description = fmt.Sprintf("围绕「%s」与故事板关键符号（%s）的真实年代物件，保留自然使用痕迹，无现代商标文字", theme, propHint)
			assets[2].Prompt.ZH = fmt.Sprintf("主题「%s」的核心年代道具（%s）定妆特写，自然窗光，真实磨损质感，无可读文字", theme, propHint)
		}
	}

	// 2. 基于用户确认后的每一镜 StorySummary 与 Narration，反推每镜的 T2I/I2V 与导演时序详版
	updates := make([]provider.ShotDirectorEnrichment, 0, len(req.ConfirmedShots))
	for i, sh := range req.ConfirmedShots {
		dur := sh.DurationSec
		if dur <= 0 {
			dur = 5
		}
		summary := strings.TrimSpace(sh.StorySummary)
		if summary == "" {
			summary = fmt.Sprintf("围绕「%s」的第%d镜画面", theme, i+1)
		}
		cam := "slow_push_in"
		switch sh.Phase {
		case psychology.PhaseConcreteRitual:
			cam = "static_gaze"
		case psychology.PhaseRelationalWarmth:
			cam = "slow_push_in"
		case psychology.PhaseLiminalClosure:
			cam = "slow_pull_out"
		}
		draft := provider.ShotDraft{
			ShotID:       sh.ShotID,
			Phase:        sh.Phase,
			StorySummary: summary,
			PsychTrigger: sh.PsychTrigger,
			Symbols:      sh.Symbols,
			Narration:    sh.Narration,
			Emotion:      sh.Emotion,
			AmbientTag:   sh.AmbientTag,
			DurationSec:  dur,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: summary, EN: fmt.Sprintf("Nostalgic vertical scene for shot %s (%s)", sh.ShotID, sh.Phase)},
				ActionRelation: prompting.BilingualText{ZH: "人物与核心年代物件保持自然克制的纪实状态，画面干净有呼吸感", EN: "Natural restrained documentary action with authentic emotional breathing room"},
				EraEnvironment: prompting.BilingualText{ZH: fmt.Sprintf("契合主题「%s」的真实年代空间与柔和自然光影，严禁现代穿帮物", theme), EN: "Authentic era environment bathed in soft natural light, zero modern props"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏电影构图，底部保留约18%干净字幕安全区", EN: "9:16 vertical cinematic framing with clean lower 18% subtitle safe area"},
				LightingMood:   prompting.BilingualText{ZH: "温暖柔和自然光影，低饱和电影胶片层次", EN: "Soft warm natural lighting with restrained cinematic film tones"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "紧扣故事板画面内容「" + summary + "」，做极缓慢克制的微动作", EN: "Subtle unhurried micro-action matching the confirmed storyboard frame"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "柔和自然光尘缓缓飘动，微风轻拂衣角或窗边枝叶", EN: "Delicate sunlit dust motes drifting gently with subtle ambient air movement"},
				CameraMovement:     cam,
				CameraDetail:       prompting.BilingualText{ZH: "摄影机极缓平稳运镜，结尾0.5秒柔和缓停", EN: "Ultra-slow steady camera motion easing gently to rest in final 0.5s"},
				LightingPacing:     prompting.BilingualText{ZH: "24fps 胶片慢拍，情绪克制稳定不变形", EN: "24fps unhurried film pacing with high temporal stability"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		}
		sheet := buildDefaultDirectorSheet(i+1, draft, assets)
		// 根据故事板文本智能强化资产绑定（若故事板里明确提到人物/空镜，进一步贴合）
		if strings.Contains(summary, "空镜") || strings.Contains(summary, "无人") {
			var onlyNonChar []prompting.AssetBinding
			for _, b := range sheet.AssetBindings {
				if b.Role != "character" {
					onlyNonChar = append(onlyNonChar, b)
				}
			}
			sheet.AssetBindings = onlyNonChar
			sheet.CastSummary = prompting.BilingualText{ZH: "无人物出镜（纯空镜留白）。", EN: "No characters (pure contemplative empty frame)."}
		}
		updates = append(updates, provider.ShotDirectorEnrichment{
			ShotID:        sh.ShotID,
			T2ISpec:       draft.T2ISpec,
			I2VSpec:       draft.I2VSpec,
			DirectorSheet: sheet,
		})
	}

	return &provider.AssetExtractionResult{
		Assets: assets,
		Shots:  updates,
	}, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func summer1998Assets() []provider.AssetDraft {
	return []provider.AssetDraft{
		{ID: "a1", Role: "character", Name: "小卖店老板", Description: "40多岁小卖部老板，短黑发，浅灰色棉T恤，慈祥温和的微笑", Prompt: prompting.BilingualText{ZH: "40多岁小卖部老板，短黑发，浅灰色棉T恤，慈祥温和的微笑，90年代胶片纪实人像", EN: "kind 40s Chinese corner store owner, short black hair, light grey cotton t-shirt, gentle warm smile, 1990s documentary portrait"}},
		{ID: "a2", Role: "scene", Name: "小卖部内部", Description: "90年代老旧胡同小卖部内部，陈旧木柜台、玻璃糖罐、算盘、铁皮零钱盒与老式白色卧式冰柜", Prompt: prompting.BilingualText{ZH: "90年代老旧胡同小卖部内部空镜，木柜台、玻璃糖罐、老式白色卧式冰柜、满架复古零食", EN: "1990s Chinese alley corner store interior empty shot, worn wooden counter, glass candy jars, white chest freezer"}},
		{ID: "a3", Role: "character", Name: "男孩", Description: "8岁中国小男孩，寸头，微旧纯白短袖T恤，深蓝色短裤，黑色布鞋", Prompt: prompting.BilingualText{ZH: "8岁中国小男孩，寸头，微旧纯白短袖T恤，深蓝色短裤，黑色布鞋，90年代胶片质感", EN: "8-year-old Chinese boy with buzz cut, plain white cotton tee, navy shorts, black cloth shoes, 1990s film portrait"}},
		{ID: "a4", Role: "character", Name: "蓝色衣服小伙伴", Description: "8岁发小男孩，寸头，浅蓝色棉T恤，黑色短裤，深色帆布鞋", Prompt: prompting.BilingualText{ZH: "8岁中国发小男孩，寸头，浅蓝色棉T恤，黑色短裤，深色帆布鞋，阳光自然", EN: "8-year-old Chinese childhood buddy, buzz cut, light blue cotton tee, black shorts, dark canvas shoes"}},
	}
}

// buildThemeAwareChildhoodPlan 为任意非小卖部题材的童年怀旧主题生成干净、无旧资产串味的资产库与分镜。
func buildThemeAwareChildhoodPlan(theme string, shotCount int) ([]provider.AssetDraft, []provider.ShotDraft) {
	if theme == "" {
		theme = "记忆里的旧时光"
	}
	assets := []provider.AssetDraft{
		{
			ID: "a1", Role: "character", Name: "主角（童年记忆视角）",
			Description: fmt.Sprintf("围绕「%s」的90年代质朴人物，衣着洗得微旧的自然棉布便服，神情真实克制，无现代配饰", theme),
			Prompt: prompting.BilingualText{
				ZH: fmt.Sprintf("围绕主题「%s」的1990年代中国人物定妆参考图，素净旧棉布日常衣着，真实柔和神情，胶片纪实质感", theme),
				EN: "1990s Chinese nostalgic protagonist reference portrait in simple worn cotton everyday clothing, authentic restrained expression, 35mm film documentary",
			},
		},
		{
			ID: "a2", Role: "scene", Name: "核心回忆场景",
			Description: fmt.Sprintf("承载「%s」的90年代老建筑与日常空间，斑驳墙面、旧木门窗与午后温暖斜阳，无现代招牌或电子设备", theme),
			Prompt: prompting.BilingualText{
				ZH: fmt.Sprintf("主题「%s」对应的1990年代怀旧老场景空镜，斑驳墙面与老旧木质陈设，午后琥珀色自然斜阳，无人物", theme),
				EN: "1990s nostalgic Chinese neighborhood environment empty reference shot, weathered walls, vintage wood textures, warm afternoon sunlight, no people",
			},
		},
		{
			ID: "a3", Role: "prop", Name: "核心时代信物",
			Description: fmt.Sprintf("与「%s」紧密相关的90年代怀旧老物件，保留真实使用磨损痕迹与年代质感，无可读商标文字", theme),
			Prompt: prompting.BilingualText{
				ZH: fmt.Sprintf("主题「%s」的核心怀旧老物件定妆特写，真实岁月磨损纹理，柔和自然窗光，干净背景无文字", theme),
				EN: "hero nostalgic 1990s everyday object reference close-up with authentic aged wear patina, soft natural window light, clean background, no readable text",
			},
		},
	}

	base := []provider.ShotDraft{
		{
			ShotID: "s1", Phase: psychology.PhaseSensoryHook,
			PsychTrigger: fmt.Sprintf("普鲁斯特感官唤起：以光影与环境声直接切入「%s」的时空坐标", theme),
			Symbols:      []string{"斑驳水泥墙与木门", "二八大杠自行车"},
			Narration:    "那年的午后，光影总是很慢。",
			Emotion:      "calm", AmbientTag: "summer_cicada", DurationSec: 5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: fmt.Sprintf("围绕「%s」展开的90年代老场景外景空镜，安静无现代杂物", theme), EN: "1990s nostalgic establishing empty scene with zero modern clutter"},
				ActionRelation: prompting.BilingualText{ZH: "老木门窗半敞，午后光斑静静落在斑驳墙面与地面上", EN: "weathered wooden doorway half-open with warm afternoon sunlight resting on textured walls"},
				EraEnvironment: prompting.BilingualText{ZH: fmt.Sprintf("与「%s」契合的90年代老旧建筑、绿荫树影与旧日生活痕迹", theme), EN: "authentic 1990s residential architecture, dappled summer tree shadows, lived-in everyday details"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏环境远景，底部保留约18%干净字幕呼吸留白", EN: "9:16 vertical establishing wide shot with clean lower 18% breathing space"},
				LightingMood:   prompting.BilingualText{ZH: "午后暖黄斜阳，阴影柔和绵长", EN: "warm afternoon golden sunlight casting long soft shadows"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "空镜保持宁静凝视感，画面无突发动作", EN: "quiet serene establishing stillness with no abrupt movement"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "午后微风轻拂树影与窗帘边角，空气中细微浮尘缓缓漂动", EN: "warm breeze sways tree shadows and curtain edge while sunlit dust motes drift slowly"},
				CameraMovement:     "slow_push_in",
				CameraDetail:       prompting.BilingualText{ZH: "摄影机极缓向前推进，仿佛慢慢走回记忆深处", EN: "ultra-slow steady forward push-in"},
				LightingPacing:     prompting.BilingualText{ZH: "克制舒缓24fps胶片慢拍", EN: "unhurried 24fps film cadence"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID: "s2", Phase: psychology.PhaseConcreteRitual,
			PsychTrigger: fmt.Sprintf("代际具象仪式：通过「%s」中的细微日常动作唤醒共同回忆", theme),
			Symbols:      []string{"五毛硬币", "发黄旧相册"},
			Narration:    "熟悉的老物件，握在手里还带着温度。",
			Emotion:      "nostalgic", AmbientTag: "alley_breeze", DurationSec: 5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: fmt.Sprintf("「%s」中的主角手部与核心时代老物件近景特写", theme), EN: "intimate close-up of the protagonist's hands holding the nostalgic era keepsake"},
				ActionRelation: prompting.BilingualText{ZH: "指尖轻触带着岁月磨损痕迹的老物件，动作专注而珍惜", EN: "fingertips gently resting on the worn vintage keepsake with quiet care"},
				EraEnvironment: prompting.BilingualText{ZH: "虚化背景为旧木桌与90年代居家陈设，光线温润自然", EN: "softly blurred vintage wooden table and 1990s home details in warm diffused light"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏浅景深特写，焦点锁定指尖与物件材质", EN: "9:16 vertical shallow-DOF close-up focused on hands and tactile patina"},
				LightingMood:   prompting.BilingualText{ZH: "侧逆窗光勾勒物件边缘柔和暖光", EN: "soft side window light rim-lighting the aged texture"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "指尖极其轻柔地摩挲老物件边缘，停顿自然克制", EN: "fingertips softly brush the worn edge with unhurried restraint"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "窗光微尘缓缓升浮，物件表面泛过一丝温润自然反光", EN: "dust motes drift through the window beam as soft light glints across the surface"},
				CameraMovement:     "static_gaze",
				CameraDetail:       prompting.BilingualText{ZH: "近乎静止的凝视镜头，带极轻微有机呼吸感", EN: "nearly static contemplative gaze with minimal organic breath"},
				LightingPacing:     prompting.BilingualText{ZH: "沉静细腻的慢拍节奏，高时间稳定性", EN: "quiet intimate slow rhythm with high temporal stability"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID: "s3", Phase: psychology.PhaseRelationalWarmth,
			PsychTrigger: fmt.Sprintf("关系微光：围绕「%s」呈现含蓄克制的陪伴与善意瞬间", theme),
			Symbols:      []string{"老冰棍", "斑驳水泥墙与木门"},
			Narration:    "没说出口的惦记，都藏在那个转身里。",
			Emotion:      "warm", AmbientTag: "alley_breeze", DurationSec: 5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: fmt.Sprintf("围绕「%s」展开的主角侧面中景，神情温和松弛", theme), EN: "side medium shot of the protagonist in a quiet warm moment"},
				ActionRelation: prompting.BilingualText{ZH: "人物微微低头整理手中的旧物，嘴角带着极克制的暖意", EN: "character gently tends to the keepsake with a subtle restrained warm smile"},
				EraEnvironment: prompting.BilingualText{ZH: "老场景中自然漫射的傍晚暖光、旧木窗与远处微暗街景", EN: "warm late-afternoon light inside the familiar vintage space"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏纪实侧面中景，留出人物视线方向空间", EN: "9:16 vertical candid documentary side medium shot with generous look-space"},
				LightingMood:   prompting.BilingualText{ZH: "柔和琥珀暖光包裹人物侧脸，温柔不煽情", EN: "soft amber sunlight wrapping the profile tenderly"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "人物轻轻抬头望向光来处，眼角浮现克制温和的微光", EN: "character slowly lifts gaze toward the light with a gentle restrained expression"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "微风轻拂衣角与窗边枝叶，光斑在墙面极缓晃动", EN: "soft breeze moves clothing hem and leaves outside while light dapples sway gently"},
				CameraMovement:     "slow_push_in",
				CameraDetail:       prompting.BilingualText{ZH: "摄影机极缓推近人物侧脸与手部微动，最后0.5秒缓停", EN: "ultra-slow push-in easing gently to rest in final 0.5s"},
				LightingPacing:     prompting.BilingualText{ZH: "温暖克制的抒情胶片节奏，面部自然不变形", EN: "tender restrained cinematic pacing keeping facial features natural"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID: "s4", Phase: psychology.PhaseLiminalClosure,
			PsychTrigger: fmt.Sprintf("时空余韵与开放式留白：回到「%s」的静谧空镜，留给观众投射个人记忆", theme),
			Symbols:      []string{"发黄旧相册", "斑驳水泥墙与木门"},
			Narration:    "你记忆里最想回去的那个瞬间，是什么时候？",
			Emotion:      "calm", AmbientTag: "evening_crickets", DurationSec: 6,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: fmt.Sprintf("黄昏琥珀光下的「%s」主题静谧空镜，无人物出镜", theme), EN: "peaceful golden-hour empty closing scene with no people"},
				ActionRelation: prompting.BilingualText{ZH: "核心时代物件静静留在原地，沐浴在逐渐沉淀的夕阳余晖中", EN: "the nostalgic keepsake rests quietly in the fading amber dusk glow"},
				EraEnvironment: prompting.BilingualText{ZH: "老场景长影拉长，远方晚霞温柔柔和，空气宁静安稳", EN: "long tranquil dusk shadows stretching across the familiar old space"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏中远景空镜，下方三分之一干净留白", EN: "9:16 vertical contemplative medium-wide empty shot with clean lower third"},
				LightingMood:   prompting.BilingualText{ZH: "浓郁温柔的黄昏低角度琥珀金光，治愈收束", EN: "rich tender low-angle amber dusk sunlight for emotional closure"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "纯空镜静谧凝视，物件完全静止", EN: "pure contemplative empty frame with objects held still"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "晚风轻拂枝叶，墙面夕阳树影缓慢轻晃，金色空气颗粒飘浮", EN: "evening breeze gently sways leaves and sunset shadows as golden dust floats"},
				CameraMovement:     "slow_pull_out",
				CameraDetail:       prompting.BilingualText{ZH: "摄影机极缓向后拉远后缓停定格", EN: "ultra-slow pull-out easing into peaceful stillness"},
				LightingPacing:     prompting.BilingualText{ZH: "黄昏暖光缓缓沉淀，留足呼吸余韵", EN: "warm golden-hour light settling into tranquil closure"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
	}

	return assets, expandShotDraftsToCount(base, shotCount)
}

func buildThemeAwareFamilyPlan(theme string, shotCount int) ([]provider.AssetDraft, []provider.ShotDraft) {
	if theme == "" {
		theme = "离家前父母塞满的后备箱"
	}
	assets := []provider.AssetDraft{
		{
			ID: "a1", Role: "character", Name: "父母（母亲与父亲）",
			Description: "60岁左右中国普通父母，母亲微卷短发系洗旧棉围裙，父亲戴老花镜穿旧藏蓝夹克，神情质朴含蓄",
			Prompt: prompting.BilingualText{
				ZH: fmt.Sprintf("围绕「%s」的60岁中国父母定妆参考图，素净日常旧衣，眉眼温和含蓄，真实家用纪实质感", theme),
				EN: "warm 60s Chinese parents reference portrait in simple everyday clothes, worn cotton apron and vintage navy jacket, understated tender demeanor",
			},
		},
		{
			ID: "a2", Role: "scene", Name: "老家厨房与门口",
			Description: "90年代老式瓷砖厨房与老居民楼红砖路口，暖黄白炽灯、磨亮木饭桌、老式暖水壶，充满烟火气",
			Prompt: prompting.BilingualText{
				ZH: fmt.Sprintf("主题「%s」对应的老家厨房与旧居民楼空镜参考图，暖黄白炽灯与清晨微光，无人物", theme),
				EN: "vintage Chinese family home kitchen and old red-brick residential lane empty reference shot, warm tungsten bulb and cool dawn light, no people",
			},
		},
		{
			ID: "a3", Role: "prop", Name: "打包好的土特产与行李",
			Description: "装满自家花生、腊肉与报纸包裹玻璃罐的厚实塑料编织袋，系着双结，无现代快递单或醒目商标",
			Prompt: prompting.BilingualText{
				ZH: fmt.Sprintf("围绕「%s」的家乡土特产袋与旧木桌道具参考特写，系紧绳结的塑料袋与报纸裹罐子，自然晨光，无可读文字", theme),
				EN: "homemade family food packages tied carefully in bags and newspaper-wrapped jars on a worn wooden table, soft morning light, no readable text",
			},
		},
	}
	drafts := familyBondDrafts(theme)
	return assets, expandShotDraftsToCount(drafts, shotCount)
}

func buildThemeAwareUrbanPlan(theme string, shotCount int) ([]provider.AssetDraft, []provider.ShotDraft) {
	if theme == "" {
		theme = "下班后吹着晚风的五分钟避风港"
	}
	assets := []provider.AssetDraft{
		{
			ID: "a1", Role: "character", Name: "下班后的年轻人（含蓄剪影）",
			Description: "20多岁都市年轻人，宽松浅灰纯棉居家T恤，安静松弛的背影与侧影，无夸张表情",
			Prompt: prompting.BilingualText{
				ZH: fmt.Sprintf("围绕「%s」的都市年轻人侧影参考图，素色浅灰棉T恤，安静疲惫后逐渐舒缓的神态，自然柔和电影光", theme),
				EN: "young urban adult in relaxed light grey cotton t-shirt, quiet restful side profile, soft natural cinematic twilight lighting",
			},
		},
		{
			ID: "a2", Role: "scene", Name: "租住小屋与落日阳台",
			Description: "简洁温馨的小屋一角与老式阳台，白纱窗帘、原木小桌、窗台绿植与柔和夕阳余晖",
			Prompt: prompting.BilingualText{
				ZH: fmt.Sprintf("主题「%s」对应的治愈系小屋与落日阳台空镜，飘动白纱窗帘、木桌与窗台绿植，低饱和琥珀与暖青暮色，无人物", theme),
				EN: "cozy minimalist apartment corner and sunlit balcony empty reference shot, sheer white curtains, potted plants, muted amber and teal dusk",
			},
		},
		{
			ID: "a3", Role: "prop", Name: "摇头老电扇与冰水玻璃杯",
			Description: "薄荷绿金属老式摇头电扇与装满冰块气泡的透明玻璃水杯，桌面透出水纹光斑",
			Prompt: prompting.BilingualText{
				ZH: fmt.Sprintf("围绕「%s」的薄荷绿摇头老电扇与透明冰水玻璃杯特写参考图，夕阳水纹光斑，干净留白无文字", theme),
				EN: "vintage mint-green oscillating fan beside a clear glass of iced water with rising bubbles, sunset caustics on wood, clean background",
			},
		},
	}
	drafts := urbanSanctuaryDrafts(theme)
	return assets, expandShotDraftsToCount(drafts, shotCount)
}

func expandShotDraftsToCount(base []provider.ShotDraft, target int) []provider.ShotDraft {
	if target <= 0 || target <= len(base) {
		return base
	}
	out := make([]provider.ShotDraft, 0, target)
	for i := 0; i < target; i++ {
		src := base[i*len(base)/target]
		src.ShotID = fmt.Sprintf("s%d", i+1)
		out = append(out, src)
	}
	return out
}

// summer1998Drafts 精准贴合项目根目录 g1.jpeg ~ g10.jpeg 的 10 镜完整叙事弧线。
func summer1998Drafts() []provider.ShotDraft {
	boyZH := "8岁中国小男孩，寸头，微旧纯白短袖T恤，深蓝色短裤，黑色布鞋"
	boyEN := "8-year-old Chinese boy with buzz cut, slightly worn plain white t-shirt, dark blue shorts, black cloth shoes"
	bossZH := "40多岁小卖部老板，短黑发，浅灰色棉T恤，慈祥温和的微笑"
	bossEN := "kind 40s corner store owner with short black hair, light grey cotton t-shirt, gentle warm smile"
	buddyZH := "8岁发小男孩，寸头，浅蓝色棉T恤，黑色短裤，深色布鞋"
	buddyEN := "8-year-old childhood buddy with buzz cut, light blue cotton t-shirt, black shorts, dark canvas shoes"

	return []provider.ShotDraft{
		{
			ShotID:       "g1",
			Phase:        psychology.PhaseSensoryHook,
			PsychTrigger: "普鲁斯特感官唤起：午后斜阳、敞开的木门与门口汽水瓶瞬间建立1990年代时空坐标",
			Symbols:      []string{"斑驳水泥墙与木门", "绿玻璃汽水瓶", "老式白色卧式冰柜"},
			Narration:    "夏天的蝉鸣，从小卖部木门开始。",
			Emotion:      "calm",
			AmbientTag:   "summer_cicada",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "1990年代老旧胡同口小卖部外景，无主角近景，远处街坊缓步走过", EN: "1990s Chinese alley corner store exterior, quiet neighborhood residents walking in the far distance"},
				ActionRelation: prompting.BilingualText{ZH: "老木门半敞，门内隐约露出白色旧冰柜与满墙零食货架", EN: "weathered wooden doors propped open revealing a white chest freezer and stocked snack shelves inside"},
				EraEnvironment: prompting.BilingualText{ZH: "斑驳水泥墙面、门口红色木箱装满绿玻璃汽水瓶、手写价格牌、水泥电线杆与老自行车", EN: "weathered concrete walls, wooden crate of green glass soda bottles, handwritten price board, concrete utility poles and vintage bicycles"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏环境远景，巷道纵深构图，底部留出字幕呼吸空间", EN: "9:16 vertical establishing wide shot with deep alley perspective and clean lower breathing space"},
				LightingMood:   prompting.BilingualText{ZH: "午后两点明亮暖黄斜阳，树影与屋檐投下清晰长影", EN: "bright warm 2pm afternoon sunlight casting crisp diagonal shadows across the concrete lane"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "远处巷尾街坊背影极其缓慢地前行，画面整体宁静克制", EN: "distant neighbor walks slowly down the quiet alley while the foreground stays serene"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "午后热风轻拂晾晒的衣物与树梢，空气中细微阳光浮尘缓缓飘动", EN: "warm afternoon breeze gently sways hanging laundry and treetops, subtle dust motes drift in sunlight"},
				CameraMovement:     "slow_push_in",
				CameraDetail:       prompting.BilingualText{ZH: "摄影机沿着巷口极缓向前推进，仿佛放学后走向小卖部", EN: "ultra-slow steady camera push-in toward the open wooden shop door"},
				LightingPacing:     prompting.BilingualText{ZH: "明亮暖调午后光影平稳流动，24fps胶片慢拍", EN: "steady warm afternoon sunlight, unhurried 24fps film cadence"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "g2",
			Phase:        psychology.PhaseSensoryHook,
			PsychTrigger: "集体记忆微距锚点：玻璃罐里的彩色水果硬糖触发80/90后共同味觉回忆",
			Symbols:      []string{"玻璃糖罐", "水果硬糖"},
			Narration:    "玻璃糖罐，装着整条巷子的颜色。",
			Emotion:      "nostalgic",
			AmbientTag:   "summer_cicada",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "陈旧磨损的木质柜台上一排透明大玻璃糖罐", EN: "row of large clear glass candy jars resting on a scarred vintage wooden counter"},
				ActionRelation: prompting.BilingualText{ZH: "罐中堆满色彩斑斓的条纹水果硬糖与老式包装糖果", EN: "jars filled to the brim with colorful striped hard fruit candies in nostalgic twisted wrappers"},
				EraEnvironment: prompting.BilingualText{ZH: "背景虚化的木质货架摆满90年代膨化零食与铁罐饼干", EN: "softly blurred wooden shelves stocked with 1990s Chinese snacks and tin biscuit cans"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏微距特写，浅景深聚焦中央敞口玻璃糖罐", EN: "9:16 vertical close-up macro shot with shallow depth of field centered on the main glass candy jar"},
				LightingMood:   prompting.BilingualText{ZH: "侧逆窗光穿透透明玻璃壁，糖纸泛起柔和暖光反光", EN: "soft side window light refracting through glass walls onto colorful candy wrappers"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "静物特写保持凝视感，糖纸高光随视角产生极细微闪烁", EN: "contemplative still-life close-up with delicate specular shimmer across the candy wrappers"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "窗外斜射阳光里的微尘缓缓漂浮，背景虚化光斑柔和微动", EN: "tiny dust particles float slowly across the window beam as soft bokeh glows gently"},
				CameraMovement:     "gentle_pan_right",
				CameraDetail:       prompting.BilingualText{ZH: "摄影机极缓沿木柜台向右平移扫过玻璃糖罐", EN: "ultra-slow horizontal camera glide past the row of glass candy jars"},
				LightingPacing:     prompting.BilingualText{ZH: "柔和温暖室内自然光，克制舒缓不急促", EN: "soft warm indoor natural light with tranquil unhurried pacing"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "g3",
			Phase:        psychology.PhaseConcreteRitual,
			PsychTrigger: "代际仪式感：趴在老式卧式冰柜前纠结选哪根冰棍，引发高度评论区认亲",
			Symbols:      []string{"老式白色卧式冰柜", "算盘与铁皮零钱盒"},
			Narration:    "掀开冰柜，白雾扑在发烫的脸上。",
			Emotion:      "warm",
			AmbientTag:   "old_freezer_hum",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: boyZH, EN: boyEN},
				ActionRelation: prompting.BilingualText{ZH: "男孩踮起微抬的脚尖趴在白色老冰柜边，专注低头挑选冰棍", EN: "boy leaning slightly over the open white chest freezer gazing intently at popsicles inside"},
				EraEnvironment: prompting.BilingualText{ZH: "小卖部内部，老式掀盖白色冰柜、木柜台上的算盘、铁皮零钱盒与玻璃糖罐", EN: "corner store interior with flip-top white chest freezer, wooden abacus, metal cash box, and glass jars"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏侧面全身中景，门口自然光勾勒男孩轮廓", EN: "9:16 vertical side full-medium shot with doorway daylight rim-lighting the boy"},
				LightingMood:   prompting.BilingualText{ZH: "门外暖日光与室内阴凉形成舒爽夏日对比", EN: "bright doorway daylight contrasting gently with the cool shaded shop interior"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "男孩手搭在冰柜边缘轻轻挪动视线，微微低头认真打量冰棍", EN: "boy rests his hands on the freezer rim, subtly shifting his gaze as he chooses a popsicle"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "冰柜敞口处升起一缕极淡的白色冷雾缓缓散开，门口微风轻动", EN: "faint wisps of cool white freezer mist drift upward and dissipate softly"},
				CameraMovement:     "slow_push_in",
				CameraDetail:       prompting.BilingualText{ZH: "摄影机极缓向男孩与冰柜推进，突出专注挑选的童年瞬间", EN: "very slow gentle push-in toward the boy and the open freezer"},
				LightingPacing:     prompting.BilingualText{ZH: "胶片自然阴凉与暖光平衡，动作柔和稳定", EN: "balanced cool shade and warm doorway light, smooth stable cadence"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "g4",
			Phase:        psychology.PhaseConcreteRitual,
			PsychTrigger: "代际暗号具象化：摊开掌心数三枚一角/五角硬币，无需解释即唤醒稀缺年代的珍惜感",
			Symbols:      []string{"五毛硬币", "算盘与铁皮零钱盒"},
			Narration:    "掌心三枚硬币，是午后全部家当。",
			Emotion:      "nostalgic",
			AmbientTag:   "old_freezer_hum",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "小男孩摊开的稚嫩手掌特写，身着微旧纯白T恤", EN: "close-up of the young boy's open palm wearing a slightly worn plain white t-shirt"},
				ActionRelation: prompting.BilingualText{ZH: "掌心静静躺着三枚带使用痕迹的铝制硬币，男孩微微低头清点", EN: "three worn aluminum Chinese coins resting in his palm as he looks down counting them"},
				EraEnvironment: prompting.BilingualText{ZH: "虚化背景为旧木柜台、算盘与白色冰柜边角", EN: "softly blurred wooden counter, abacus, and white freezer corner in the background"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏俯拍近景特写，焦点锁定掌心硬币纹理", EN: "9:16 vertical high-angle close-up locked onto the texture of the coins in his palm"},
				LightingMood:   prompting.BilingualText{ZH: "柔和室内漫射光在金属硬币表面泛起温润哑光", EN: "soft diffused indoor daylight catching subtle matte reflections on the metal coins"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "男孩手指极轻微地微动托起硬币，低垂的眼神专注而珍惜", EN: "the boy's fingers shift subtly cradling the coins with quiet careful focus"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "金属硬币边缘随手掌微倾闪过一丝柔和自然反光，空气沉静", EN: "soft natural light glints gently across the worn coin edges as his palm tilts slightly"},
				CameraMovement:     "static_gaze",
				CameraDetail:       prompting.BilingualText{ZH: "近乎静止的克制微呼吸镜头，让观众看清掌心硬币", EN: "nearly static contemplative camera with minimal organic breathing"},
				LightingPacing:     prompting.BilingualText{ZH: "沉静细腻的慢节拍，无任何多余抖动", EN: "quiet intimate slow pacing with high temporal stability"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "g5",
			Phase:        psychology.PhaseRelationalWarmth,
			PsychTrigger: "关系微光（被无条件善待）：小卖部老板微笑递糖的瞬间，修复成年人的信任与温暖记忆",
			Symbols:      []string{"玻璃糖罐", "水果硬糖", "算盘与铁皮零钱盒"},
			Narration:    "老板笑着，顺手多递来一颗糖。",
			Emotion:      "warm",
			AmbientTag:   "alley_breeze",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: boyZH + "，与" + bossZH, EN: boyEN + ", facing " + bossEN},
				ActionRelation: prompting.BilingualText{ZH: "老板隔着老木柜台微笑微倾，指尖把一颗红色糖纸水果糖递到男孩手中，男孩仰头接过", EN: "shopkeeper leans gently across the wooden counter handing a red-wrapped fruit candy to the boy who looks up warmly"},
				EraEnvironment: prompting.BilingualText{ZH: "90年代小卖部内部，一排玻璃糖罐、铁皮零钱盒与满架零食", EN: "1990s corner store interior with glass candy jars, tin cash box, and snack shelves behind"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏双人侧面中景，温暖视线交汇处于画面黄金分割点", EN: "9:16 vertical side medium two-shot placing their warm eye-line and handoff at the focal point"},
				LightingMood:   prompting.BilingualText{ZH: "门口漫射暖光照亮两人侧脸与递糖的双手", EN: "warm doorway daylight illuminating their profiles and the gentle candy handoff"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "老板眼角笑意微深轻轻松手递出糖果，男孩手指小心接住糖果并微微抬头微笑", EN: "shopkeeper smiles softly releasing the candy into the boy's careful fingers as the boy looks up with a shy smile"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "门外暖光微尘在柜台上方缓缓漂移，玻璃糖罐折射柔和光晕", EN: "warm dust motes float softly above the wooden counter while glass jars catch gentle daylight"},
				CameraMovement:     "slow_push_in",
				CameraDetail:       prompting.BilingualText{ZH: "摄影机极缓向两人递糖的双手与微笑神情推进", EN: "gentle slow push-in toward the warm handoff and smiles"},
				LightingPacing:     prompting.BilingualText{ZH: "全片最温暖克制的抒情慢拍，面部自然稳定不变形", EN: "warmest tender restrained pacing with natural stable facial expressions"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "g6",
			Phase:        psychology.PhaseRelationalWarmth,
			PsychTrigger: "社会联结与同伴共享：与发小在巷口分咬同一根冰棍，触发“发小”社交转发冲动",
			Symbols:      []string{"老冰棍", "斑驳水泥墙与木门"},
			Narration:    "冰棍分着咬，甜味好像多了一倍。",
			Emotion:      "warm",
			AmbientTag:   "summer_cicada",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: boyZH + "，与" + buddyZH, EN: boyEN + ", standing opposite " + buddyEN},
				ActionRelation: prompting.BilingualText{ZH: "白T恤男孩笑着把剥开包装的老冰棍递给蓝T恤发小，发小开心俯身咬下一口", EN: "boy in white t-shirt smiles offering his peeled milk popsicle while his buddy in light blue happily takes a bite"},
				EraEnvironment: prompting.BilingualText{ZH: "阳光斑驳的老青砖胡同，晾晒的衣服、旧自行车与远处绿树", EN: "sun-dappled old grey-brick alley with hanging laundry, parked bicycles, and lush summer trees"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏双人全身侧面中景，自然抓拍感", EN: "9:16 vertical full-body candid side two-shot in the sunlit lane"},
				LightingMood:   prompting.BilingualText{ZH: "明朗柔和的午后阳光洒在青砖墙与两个孩子肩头", EN: "bright soft afternoon sunlight falling across brick walls and both boys' shoulders"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "蓝衣小伙伴轻轻咬下冰棍微微眯眼，白衣男孩笑着轻晃一下手腕，动作自然克制", EN: "buddy in blue gently takes a bite of the popsicle with happy squinted eyes while the boy in white smiles warmly"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "巷子里的微风轻拂两人衣角与远处的晾衣绳，树影在地面轻微摇曳", EN: "alley breeze lightly ripples their shirt hems and distant laundry as tree shadows sway softly"},
				CameraMovement:     "handheld_breath",
				CameraDetail:       prompting.BilingualText{ZH: "极轻柔纪实手持呼吸感，仿佛蹲在巷口记录童年日常", EN: "subtle organic documentary handheld breathing stability"},
				LightingPacing:     prompting.BilingualText{ZH: "轻快而不急躁的夏日胶片暖拍，人物面部稳定清晰", EN: "warm relaxed summer film pacing keeping both faces natural and stable"},
				TransitionMode:     prompting.TransitionFirstLastFrame,
				TransitionHint:     prompting.BilingualText{ZH: "分享冰棍后两人自然转身，衔接下一镜并肩走向巷尾的背影", EN: "after sharing the popsicle, smoothly bridge toward the two boys walking side by side down the alley"},
			},
		},
		{
			ShotID:       "g7",
			Phase:        psychology.PhaseRelationalWarmth,
			PsychTrigger: "背影留白效应：两个并肩走远的童年背影，让观众自然想起自己失联多年的发小",
			Symbols:      []string{"斑驳水泥墙与木门", "老冰棍"},
			Narration:    "那时以为，会一直这样并肩走。",
			Emotion:      "nostalgic",
			AmbientTag:   "alley_breeze",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: boyZH + "与" + buddyZH + "的并肩背影", EN: "back view of " + boyEN + " and " + buddyEN + " walking side by side"},
				ActionRelation: prompting.BilingualText{ZH: "两个孩子并肩慢步走入胡同深处，白衣男孩右手还握着一根木冰棍棒，蓝衣小伙伴侧头说着话", EN: "both boys stroll side by side deeper into the alley, white-shirt boy holding a wooden popsicle stick as his buddy turns slightly to chat"},
				EraEnvironment: prompting.BilingualText{ZH: "90年代青砖灰瓦胡同，老电线杆、木门与远处高大杨树", EN: "1990s grey-brick alley, utility poles, wooden doors, and tall poplar trees at the end"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏低机位跟拍背影中景，对称巷道引向远方", EN: "9:16 vertical low-angle tracking rear medium shot framed symmetrically by the alley"},
				LightingMood:   prompting.BilingualText{ZH: "午后偏斜暖光在水泥路面上投下两人并行的柔和长影", EN: "slanted late-afternoon sunlight casting two parallel shadows on the concrete lane"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "两个男孩迈着松弛慢步并肩向前缓走，蓝衣男孩微微偏头轻声交谈，手臂自然轻摆", EN: "both boys walk slowly forward side by side at an unhurried summer pace, shoulders relaxed"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "远处杨树叶在热风里沙沙微晃，阳光尘粒在巷道尽头柔和浮动", EN: "distant poplar leaves rustle gently in the warm wind while golden dust shimmers ahead"},
				CameraMovement:     "slow_push_in",
				CameraDetail:       prompting.BilingualText{ZH: "极慢平稳后置跟拍，保持一段温柔距离目送两个背影", EN: "gentle slow rear tracking shot keeping an affectionate distance"},
				LightingPacing:     prompting.BilingualText{ZH: "克制绵长的胶片暖光节奏，情绪由欢快缓缓转向怀念", EN: "restrained lingering warm film rhythm easing from joy into tender nostalgia"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "g8",
			Phase:        psychology.PhaseLiminalClosure,
			PsychTrigger: "时间对照冲击（Temporal Contrast）：同机位老木门变成褪色红牌“便利店”与铝合金推拉门，无声道出岁月变迁",
			Symbols:      []string{"斑驳水泥墙与木门"},
			Narration:    "后来木门换了，旧冰柜也搬走了。",
			Emotion:      "bittersweet",
			AmbientTag:   "alley_breeze",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "多年后的同一间巷口小店外景，无人空景", EN: "the exact same alley corner shop years later, quiet empty exterior"},
				ActionRelation: prompting.BilingualText{ZH: "斑驳水泥墙挂上褪色的红底白字「副食便利店」招牌，原本木门变为铝合金玻璃推拉门与现代立式冷饮柜", EN: "faded red signboard reading convenience store above an aluminum sliding glass door and upright beverage fridge"},
				EraEnvironment: prompting.BilingualText{ZH: "老巷子依旧但门口多了扫帚与电表箱，午后静谧无人", EN: "familiar old concrete alley now with a broom leaning beside plastic crates and utility meter box"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏与第一镜完全一致的同机位构图，制造强烈时空对照", EN: "9:16 vertical match-cut wide framing identical to shot 1 to heighten temporal contrast"},
				LightingMood:   prompting.BilingualText{ZH: "略显清冷平淡的午后斜光，饱和度微降", EN: "slightly quieter, lower-saturation afternoon light emphasizing the passage of years"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "空镜保持极度克制的寂静感，画面无人物闯入", EN: "quiet empty frame held in still contemplation with no abrupt movement"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "一阵干爽微风轻拂过巷口地面与电线，树影在斑驳红招牌上缓慢移动", EN: "a quiet breeze drifts down the empty lane as soft tree shadows shift slowly across the faded red sign"},
				CameraMovement:     "slow_pull_out",
				CameraDetail:       prompting.BilingualText{ZH: "摄影机极缓向后拉远，营造物是人非的距离感", EN: "ultra-slow gentle pull-out evoking quiet temporal distance"},
				LightingPacing:     prompting.BilingualText{ZH: "沉静低饱和胶片调性，留出沉默呼吸感", EN: "quiet muted film tone leaving generous emotional breathing space"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "g9",
			Phase:        psychology.PhaseLiminalClosure,
			PsychTrigger: "自我连续性触发（Self-Continuity）：现代高楼公寓里的成年双手轻抚旧相册里的童年自己，完成过去与当下的连接",
			Symbols:      []string{"发黄旧相册"},
			Narration:    "相册里，还留着冰柜前的自己。",
			Emotion:      "nostalgic",
			AmbientTag:   "evening_crickets",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "成年男性的过肩背影与放在泛黄相册边缘的一只手", EN: "over-the-shoulder view of a grown man resting his hand gently beside an old yellowed photo album"},
				ActionRelation: prompting.BilingualText{ZH: "原木书桌上摊开厚重泛黄旧相册，页面里嵌着当年白衣男孩趴在冰柜前的老照片", EN: "open aged photo album on a modern wooden desk displaying the faded photograph of the boy at the corner-store freezer"},
				EraEnvironment: prompting.BilingualText{ZH: "现代城市高楼公寓窗景、水杯、空调遥控器与散落的旧照片", EN: "modern high-rise apartment window view, glass of water, AC remote, and scattered vintage prints"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏过肩俯拍近景，焦点落在泛黄老照片与成年人手指", EN: "9:16 vertical over-the-shoulder downward close-up focused on the old photograph and adult fingers"},
				LightingMood:   prompting.BilingualText{ZH: "柔和现代窗光斜洒木桌，给泛黄纸页镀上温暖回忆感", EN: "soft natural apartment window light falling gently across the yellowed album pages"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "成年人的指尖极其轻柔地在泛黄相册纸页边缘微顿轻抚，仿佛隔着岁月触碰童年", EN: "adult fingertips pause and gently brush the yellowed album border as if softly touching childhood"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "窗帘白纱随微风轻轻鼓动，桌面光影柔和微移，水杯微微泛光", EN: "sheer window curtain drifts gently in the breeze as soft daylight shifts subtly across the desk"},
				CameraMovement:     "slow_push_in",
				CameraDetail:       prompting.BilingualText{ZH: "摄影机极缓向旧照片里的小男孩推进，建立记忆穿越感", EN: "very slow gentle push-in toward the childhood photo inside the album"},
				LightingPacing:     prompting.BilingualText{ZH: "静谧克制的电影慢拍，温柔而不煽情", EN: "intimate tranquil cinematic pacing, tender and unforced"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "g10",
			Phase:        psychology.PhaseLiminalClosure,
			PsychTrigger: "怀旧救赎与开放式留白：回到记忆中金色黄昏下的小卖部空凳，给予心理慰藉并引导评论区共鸣",
			Symbols:      []string{"斑驳水泥墙与木门", "二八大杠自行车", "绿玻璃汽水瓶"},
			Narration:    "你家巷口的那间小卖部，现在还在吗？",
			Emotion:      "calm",
			AmbientTag:   "evening_crickets",
			DurationSec:  6,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "记忆中黄昏时分的90年代老小卖部外景空镜", EN: "nostalgic 1990s alley corner store exterior at golden hour dusk, peaceful empty scene"},
				ActionRelation: prompting.BilingualText{ZH: "木门依旧敞开，门前静静摆着两张空小木凳、一辆老二八自行车与一箱绿玻璃汽水瓶", EN: "open wooden doorway with two empty small wooden stools, a parked vintage bicycle, and green soda bottles"},
				EraEnvironment: prompting.BilingualText{ZH: "上方绿树枝叶垂落，斑驳老灰砖楼与水泥墙沐浴在夕阳余晖中", EN: "overhanging green tree branches above old grey-brick apartments bathed in warm sunset glow"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏静谧中远景，大面积温暖夕阳地面留白", EN: "9:16 vertical contemplative medium-wide shot with generous sunlit foreground breathing room"},
				LightingMood:   prompting.BilingualText{ZH: "浓郁温柔的琥珀金黄昏低角度斜阳，长影静谧安宁", EN: "rich tender amber golden-hour sunlight casting long tranquil shadows"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "纯粹空镜凝视，两张空木凳在夕阳中静静守候", EN: "pure quiet contemplative still scene with two empty wooden stools bathed in dusk"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "上方枝叶在晚风中极缓摇曳，墙面夕阳树影轻晃，金色空气颗粒缓缓飘落", EN: "overhead branches sway gently in the evening breeze while golden sunset leaf shadows dance softly on the concrete wall"},
				CameraMovement:     "slow_pull_out",
				CameraDetail:       prompting.BilingualText{ZH: "摄影机极缓向后拉远定格，留出充足环境音与思绪余韵", EN: "ultra-slow pull-out fading into peaceful nostalgic stillness"},
				LightingPacing:     prompting.BilingualText{ZH: "黄昏琥珀暖光缓缓沉淀，极致温柔安宁", EN: "golden-hour amber light settling gently in serene stillness"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
	}
}

func familyBondDrafts(theme string) []provider.ShotDraft {
	if theme == "" {
		theme = "离家前父母塞满的后备箱"
	}
	return []provider.ShotDraft{
		{
			ShotID:       "s1",
			Phase:        psychology.PhaseSensoryHook,
			PsychTrigger: "依恋感官序幕：清晨厨房暖黄灯光与蒸汽声唤醒家的安全感",
			Symbols:      []string{"厨房暖黄白炽灯", "搪瓷汤盆"},
			Narration:    "天还没亮透，厨房那盏暖灯已经先亮了。",
			Emotion:      "calm",
			AmbientTag:   "kitchen_simmer",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "老家厨房里系着旧围裙的母亲背影", EN: "back view of a mother in a worn apron standing in an old home kitchen"},
				ActionRelation: prompting.BilingualText{ZH: "母亲在炉灶前安静煮着热汤，搪瓷盆里冒出细密白气", EN: "mother quietly simmering soup as soft steam rises from an enamel pot"},
				EraEnvironment: prompting.BilingualText{ZH: "90年代老瓷砖厨房、暖黄白炽灯、老式保温壶与木案板", EN: "vintage tiled Chinese home kitchen, warm tungsten bulb, old thermos flask, wooden chopping board"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏门框窥视中景，前景门沿柔和虚化", EN: "9:16 vertical medium shot framed gently through the kitchen doorway"},
				LightingMood:   prompting.BilingualText{ZH: "室内暖黄白炽灯与窗外微蓝晨曦形成温情对比", EN: "warm tungsten indoor glow contrasting against cool pale dawn outside the window"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "母亲轻轻搅动锅勺，动作缓慢熟练而安静", EN: "mother gently stirs the pot with calm practiced slowness"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "锅中白色热气缓缓升腾穿过暖黄灯光，窗外晨光微微泛亮", EN: "soft white steam curls slowly upward through the warm tungsten glow"},
				CameraMovement:     "slow_push_in",
				LightingPacing:     prompting.BilingualText{ZH: "家用DV柔和暖光慢拍，自然不刺眼", EN: "soft vintage home DV warm glow, unhurried gentle pacing"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "s2",
			Phase:        psychology.PhaseConcreteRitual,
			PsychTrigger: "含蓄照料动作：把土特产一层层塞进袋子，用具体动作替代直白抒情",
			Symbols:      []string{"塑料编织袋与土特产", "磨亮的木饭桌"},
			Narration:    "嘴上说着别带太多，手里却又塞进两袋花生。",
			Emotion:      "warm",
			AmbientTag:   "old_clock_tick",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "父母略显粗糙的双手特写，戴着老花镜的父亲在一旁帮忙系袋口", EN: "close-up of weathered parental hands as father wearing reading glasses helps tie plastic bags"},
				ActionRelation: prompting.BilingualText{ZH: "把晒干的花生、腊肉与鸡蛋仔细塞进厚塑料袋并打上双结", EN: "carefully packing dried peanuts and homemade food into bags and double-knotting them"},
				EraEnvironment: prompting.BilingualText{ZH: "磨亮的旧木饭桌，桌边放着老式行李箱与报纸包裹的玻璃罐", EN: "polished old wooden dining table beside a suitcase and newspaper-wrapped glass jars"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏俯拍手部近景，突出手指粗糙纹路", EN: "9:16 vertical high-angle close-up highlighting weathered loving hands"},
				LightingMood:   prompting.BilingualText{ZH: "柔和晨光斜照桌面，温暖平实", EN: "soft morning light slanting warmly across the table"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "双手认真抚平袋口轻轻系紧绳结，指节动作沉稳温柔", EN: "hands carefully smooth the bag edges and gently tighten the knot"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "晨光微尘在桌面上方缓缓浮游，背景老挂钟秒针轻摆", EN: "morning dust motes drift gently above the table while the wall clock ticks quietly"},
				CameraMovement:     "handheld_breath",
				LightingPacing:     prompting.BilingualText{ZH: "贴近日常纪实的自然缓拍", EN: "intimate documentary slow rhythm"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "s3",
			Phase:        psychology.PhaseRelationalWarmth,
			PsychTrigger: "岁月细节与安全基地：路口目送的微驼背影，触发低防御共情",
			Symbols:      []string{"老花镜与缝纫针线盒", "车站月台微风"},
			Narration:    "后视镜里，他们一直站到车影拐过路口。",
			Emotion:      "bittersweet",
			AmbientTag:   "evening_breeze",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "老小区路口并肩站立的父母，父亲穿着旧夹克，母亲抬手微挥", EN: "parents standing side by side at the old neighborhood gate, mother gently raising her hand"},
				ActionRelation: prompting.BilingualText{ZH: "两人并肩望向驶离的方向，神情克制而温和", EN: "both gazing calmly down the road with restrained tender expressions"},
				EraEnvironment: prompting.BilingualText{ZH: "老式居民楼红砖墙、清晨微风中的梧桐树与安静街道", EN: "old red-brick residential lane, plane trees in morning breeze, quiet street"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏微远景，透过微微虚化的车窗边缘构图", EN: "9:16 vertical medium-wide shot gently framed from a departing car window perspective"},
				LightingMood:   prompting.BilingualText{ZH: "温柔微暖的清晨斜阳拉长两人并肩的影子", EN: "soft warm morning sun stretching their side-by-side shadows"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "母亲极其缓慢地轻挥一下手又慢慢放下，父亲微微点头目送", EN: "mother gives one slow gentle wave and lowers her hand as father nods quietly"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "清晨微风拂动路边树叶与母亲衣襟，画面随车缓缓远去", EN: "morning breeze rustles leaves and clothes as the view glides smoothly away"},
				CameraMovement:     "slow_pull_out",
				LightingPacing:     prompting.BilingualText{ZH: "克制温柔的缓拉节奏，留足沉默与凝视", EN: "tender unhurried pull-out leaving quiet space for reflection"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "s4",
			Phase:        psychology.PhaseLiminalClosure,
			PsychTrigger: "情感锚点落点：老楼窗口始终亮着的暖灯，强化“心里有处可归”的安心感",
			Symbols:      []string{"厨房暖黄白炽灯", "阳台的旧晾衣绳"},
			Narration:    "走得再远，回头时那盏灯总还亮着。",
			Emotion:      "calm",
			AmbientTag:   "evening_breeze",
			DurationSec:  6,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "暮色中老居民楼三楼亮着暖黄灯光的一扇窗", EN: "a single third-floor apartment window glowing warm yellow in an old residential building at dusk"},
				ActionRelation: prompting.BilingualText{ZH: "阳台上搭着旧晾衣绳与绿植，屋内暖光透过白窗帘温柔溢出", EN: "warm indoor light spilling through sheer curtains onto a small balcony with potted plants"},
				EraEnvironment: prompting.BilingualText{ZH: "深蓝暮色天空、老楼灰墙与楼下静谧树影", EN: "deep blue twilight sky, weathered grey apartment facade, tranquil tree silhouettes below"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏仰拍静谧中远景，天空与夜色留白", EN: "9:16 vertical contemplative low-angle shot with generous twilight breathing space"},
				LightingMood:   prompting.BilingualText{ZH: "冷蓝暮色衬托唯一一扇暖琥珀窗口，极强心理庇护感", EN: "cool twilight blue framing one warm amber window evoking deep emotional shelter"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "空镜定格，窗口灯光温柔稳定地亮着", EN: "serene still frame as the warm window light glows steadily"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "晚风轻拂阳台纱帘与楼下树梢，暮色云影极缓流动", EN: "evening breeze gently sways the balcony curtain and treetops beneath drifting dusk clouds"},
				CameraMovement:     "static_gaze",
				LightingPacing:     prompting.BilingualText{ZH: "沉静安稳的尾镜余韵，留给观众深呼吸", EN: "calm steady lingering closure inviting a quiet breath"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
	}
}

func urbanSanctuaryDrafts(theme string) []provider.ShotDraft {
	if theme == "" {
		theme = "下班后吹着晚风的五分钟避风港"
	}
	return []provider.ShotDraft{
		{
			ShotID:       "u1",
			Phase:        psychology.PhaseSensoryHook,
			PsychTrigger: "都市疲劳对照切口：短暂冷青色晚高峰车窗雨滴，建立高压都市人的当下代入",
			Symbols:      []string{"玻璃杯里的气泡与冰块"},
			Narration:    "关掉最后一个会议窗，城市刚好下起细雨。",
			Emotion:      "calm",
			AmbientTag:   "urban_rain_soft",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "傍晚公交或地铁车窗玻璃上的细密雨滴与虚化车流灯影", EN: "fine raindrops on a commuter window with softly blurred evening city tail-lights outside"},
				ActionRelation: prompting.BilingualText{ZH: "车窗倒映着疲惫微垂眼眸的年轻人剪影", EN: "faint reflection of a tired young commuter resting quietly against the glass"},
				EraEnvironment: prompting.BilingualText{ZH: "现代都市暮色街道、冷青色写字楼玻璃幕墙与暖色路灯虚化光斑", EN: "dusk city street, cool teal office towers, soft glowing streetlamp bokeh"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏浅景深近景，聚焦玻璃水珠", EN: "9:16 vertical shallow-DOF close-up focused on raindrops on glass"},
				LightingMood:   prompting.BilingualText{ZH: "克制冷青蓝调配微弱琥珀街灯，营造微倦感", EN: "restrained cool teal twilight with subtle amber streetlamp highlights"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "玻璃上的一滴雨水极其缓慢地滑落，带出清晰水痕", EN: "a single raindrop slides slowly down the glass leaving a delicate trail"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "窗外虚化的城市光斑柔和流淌，细雨轻敲玻璃", EN: "blurred city bokeh glides softly outside as light rain taps the pane"},
				CameraMovement:     "static_gaze",
				LightingPacing:     prompting.BilingualText{ZH: "柔和克制的冷暖过渡节奏，不焦躁", EN: "calm restrained cadence easing tension gently"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "u2",
			Phase:        psychology.PhaseConcreteRitual,
			PsychTrigger: "注意力恢复（ART）感官切场：老电扇慢摇与玻璃杯上升气泡，迅速降低大脑认知负荷",
			Symbols:      []string{"摇头老电扇", "玻璃杯里的气泡与冰块"},
			Narration:    "回到小屋，先听老电扇慢慢转起风声。",
			Emotion:      "calm",
			AmbientTag:   "old_fan_hum",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "安静木桌上的透明玻璃水杯与一旁轻轻摇头的薄荷绿老电扇", EN: "clear glass of cool water on a wooden table beside a vintage mint-green oscillating fan"},
				ActionRelation: prompting.BilingualText{ZH: "冰块在杯中微微上浮，细小气泡沿着杯壁安静升起", EN: "ice cubes gently floating as tiny sparkling bubbles rise quietly along the glass"},
				EraEnvironment: prompting.BilingualText{ZH: "简洁温馨的租住小屋一角、白纱窗帘与夕阳斜照的木地板", EN: "cozy minimalist apartment corner with sheer white curtains and sunlit wooden floor"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏低信息密度静物中近景，大面积柔和留白", EN: "9:16 vertical minimalist low-density medium close-up with generous visual breathing space"},
				LightingMood:   prompting.BilingualText{ZH: "低饱和夕阳琥珀金穿透玻璃水杯，在桌面投出通透水纹光斑", EN: "low-saturation golden sunset piercing the water glass casting luminous caustics on wood"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "水杯里的气泡缓缓上升，冰块极轻微地转动", EN: "bubbles rise unhurriedly inside the glass as ice shifts imperceptibly"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "背景白纱窗帘随电扇微风轻柔鼓动，桌面水波光影缓慢摇曳", EN: "sheer curtain billows softly in the fan breeze while luminous water caustics ripple gently"},
				CameraMovement:     "slow_push_in",
				LightingPacing:     prompting.BilingualText{ZH: "极致松弛的治愈系慢拍，空气通透柔和", EN: "deeply restorative healing slow pacing, airy and luminous"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "u3",
			Phase:        psychology.PhaseRelationalWarmth,
			PsychTrigger: "无目的松弛片刻：阳台晒过太阳的棉被与打盹的橘猫，构建安全松弛体感",
			Symbols:      []string{"阳台的晚风与白纱帘", "橘猫在台阶打盹", "窗台绿植与夕阳影子"},
			Narration:    "橘猫在暖光里睡着，没有谁在催促下一步。",
			Emotion:      "warm",
			AmbientTag:   "evening_breeze",
			DurationSec:  5,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "一只毛发蓬松的橘猫蜷在洒满夕阳的旧木阳台台阶上打盹", EN: "fluffy ginger cat curled up napping on a sun-warmed wooden balcony step"},
				ActionRelation: prompting.BilingualText{ZH: "橘猫闭着眼微微起伏呼吸，身旁是几盆随风轻晃的绿植", EN: "cat sleeping peacefully with slow gentle breaths beside potted green plants"},
				EraEnvironment: prompting.BilingualText{ZH: "老式阳台铁艺栏杆、刚晒过太阳的棉被一角与远方温柔晚霞", EN: "balcony railing, corner of a sun-warmed cotton quilt, soft pastel sunset sky beyond"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏低机位柔和浅景深中景", EN: "9:16 vertical low-angle medium shot with creamy shallow bokeh"},
				LightingMood:   prompting.BilingualText{ZH: "温暖琥珀金夕阳包裹橘猫毛尖，泛起绒光轮廓", EN: "warm golden-hour light rim-lighting the cat's soft fur"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "橘猫胸口极其缓慢地起伏呼吸，耳朵偶尔轻轻抖动一下", EN: "ginger cat breathes slowly in deep rest with one tiny relaxed ear twitch"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "晚风拂动绿植叶片与棉被边角，温暖夕阳尘粒在空气中轻舞", EN: "evening breeze softly sways green leaves and quilt edge as golden dust motes drift"},
				CameraMovement:     "gentle_pan_left",
				LightingPacing:     prompting.BilingualText{ZH: "催眠般宁静的慢速电影运镜，极具松弛感", EN: "soothing tranquil slow glide evoking complete ease"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
		{
			ShotID:       "u4",
			Phase:        psychology.PhaseLiminalClosure,
			PsychTrigger: "心理许可（Stress Buffering）：不催促振作，给予“此刻可以停下来”的温柔允许，提高收藏率",
			Symbols:      []string{"落日下的旧天台", "阳台的晚风与白纱帘"},
			Narration:    "今晚的风很慢，你也先不用急着赶路。",
			Emotion:      "calm",
			AmbientTag:   "evening_breeze",
			DurationSec:  6,
			T2ISpec: prompting.T2IPromptSpec{
				SubjectAnchor:  prompting.BilingualText{ZH: "暮色降临前的老屋窗台与轻轻飘起的半透明白纱帘", EN: "quiet window sill at twilight with a translucent white sheer curtain lifting gently"},
				ActionRelation: prompting.BilingualText{ZH: "窗外是逐渐由橘粉转为静谧暖青的晚霞天空与零星亮起的万家灯火", EN: "outside the window, sunset fades from peach-amber into soft teal above glowing neighborhood lights"},
				EraEnvironment: prompting.BilingualText{ZH: "木窗框、一盆垂落的绿萝与宁静开阔的傍晚天际线", EN: "wooden window frame, trailing green plant, peaceful expansive evening skyline"},
				ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏对称空景构图，大片晚霞与纱帘留白", EN: "9:16 vertical symmetrical window framing with expansive twilight breathing space"},
				LightingMood:   prompting.BilingualText{ZH: "低饱和暖青与落日余晖交融，安静治愈", EN: "muted warm teal blending with soft sunset amber, deeply restorative"},
			},
			I2VSpec: prompting.I2VPromptSpec{
				SubjectMicroAction: prompting.BilingualText{ZH: "纯粹空镜呼吸，无人物干扰", EN: "pure tranquil breathing stillness"},
				EnvironmentPhysics: prompting.BilingualText{ZH: "白纱窗帘被舒缓晚风缓缓托起又轻柔落下，窗外晚霞云层极缓流动", EN: "sheer white curtain rises and settles gracefully on the evening breeze as dusk clouds drift slowly"},
				CameraMovement:     "slow_pull_out",
				LightingPacing:     prompting.BilingualText{ZH: "缓慢拉远沉入夜晚安全感，余韵悠长", EN: "gentle pull-out settling into peaceful evening sanctuary"},
				TransitionMode:     prompting.TransitionSingleFrame,
			},
		},
	}
}

// --- 2. LocalT2I：从零建项时生成 1080x1920 暖调胶片质感占位首帧图 ---

type LocalT2I struct{}

func NewLocalT2I() *LocalT2I { return &LocalT2I{} }

func (l *LocalT2I) Name() string { return "mock-local-t2i" }

func resolveMockDims(aspectRatio string) (int, int) {
	switch strings.TrimSpace(aspectRatio) {
	case "16:9":
		return 1920, 1080
	case "3:4":
		return 1080, 1440
	default:
		return 1080, 1920
	}
}

func (l *LocalT2I) Generate(_ context.Context, req provider.T2IRequest) (string, error) {
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return "", err
	}
	w, h := resolveMockDims(req.AspectRatio)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// 根据种子生成柔和怀旧胶片渐变底色（避免刺眼纯色）
	shift := int(req.Seed%40) - 20
	for y := 0; y < h; y++ {
		ratio := float64(y) / float64(h)
		r := clampByte(218 - int(ratio*78) + shift)
		g := clampByte(192 - int(ratio*82) + shift/2)
		b := clampByte(152 - int(ratio*72))
		for x := 0; x < w; x++ {
			// 轻暗角
			dx := (float64(x)/float64(w) - 0.5) * 2
			dy := (float64(y)/float64(h) - 0.5) * 2
			vig := 1.0 - 0.18*(dx*dx+dy*dy)
			if vig < 0.72 {
				vig = 0.72
			}
			img.SetRGBA(x, y, color.RGBA{
				R: clampByte(int(float64(r) * vig)),
				G: clampByte(int(float64(g) * vig)),
				B: clampByte(int(float64(b) * vig)),
				A: 255,
			})
		}
	}
	f, err := os.Create(req.OutputPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		return "", err
	}
	return req.OutputPath, nil
}

func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// --- 3. LocalMotionI2V：用本机 FFmpeg 将首帧图渲染为带真实电影运镜/首尾帧过渡的 MP4 ---

type LocalMotionI2V struct {
	workDir string
	mu      sync.Mutex
	tasks   map[string]provider.I2VStatus
	seq     atomic.Int64
}

func NewLocalMotionI2V(workDir string) *LocalMotionI2V {
	return &LocalMotionI2V{
		workDir: workDir,
		tasks:   make(map[string]provider.I2VStatus),
	}
}

func (m *LocalMotionI2V) Name() string { return "mock-local-ffmpeg-i2v" }

func (m *LocalMotionI2V) Submit(ctx context.Context, req provider.I2VSubmitRequest) (string, error) {
	if err := os.MkdirAll(m.workDir, 0o755); err != nil {
		return "", err
	}

	frameSrc := strings.TrimSpace(req.FirstFramePath)
	// 支持跳过分镜首帧定妆图模式：优先复用本镜绑定的首张资产参考图，否则自动合成一张胶片底板帧用于运镜
	if frameSrc == "" {
		for _, ref := range req.ReferenceImages {
			if _, statErr := os.Stat(ref); statErr == nil {
				frameSrc = ref
				break
			}
		}
	}
	if frameSrc == "" {
		platePath := filepath.Join(m.workDir, fmt.Sprintf("plate-%d.jpeg", time.Now().UnixNano()))
		t2i := NewLocalT2I()
		genPlate, genErr := t2i.Generate(ctx, provider.T2IRequest{
			Prompt:      req.Prompt,
			AspectRatio: req.AspectRatio,
			OutputPath:  platePath,
			Seed:        req.Seed,
		})
		if genErr != nil {
			return "", &provider.ProviderError{
				Provider: m.Name(), Code: "PLATE_GEN_FAILED", Message: genErr.Error(), Retryable: false, Err: genErr,
			}
		}
		frameSrc = genPlate
	}
	if _, err := os.Stat(frameSrc); err != nil {
		return "", &provider.ProviderError{
			Provider: m.Name(), Code: "FRAME_NOT_FOUND", Message: err.Error(), Retryable: false, Err: err,
		}
	}

	id := fmt.Sprintf("local-i2v-%d-%d", time.Now().UnixNano(), m.seq.Add(1))
	outPath := filepath.Join(m.workDir, id+".mp4")

	dur := req.DurationSec
	if dur <= 0 {
		dur = 5
	}
	w, h := resolveMockDims(req.AspectRatio)

	var err error
	if req.TransitionMode == prompting.TransitionFirstLastFrame && req.LastFramePath != "" {
		if _, statErr := os.Stat(req.LastFramePath); statErr == nil {
			err = renderFirstLastFrameClip(ctx, frameSrc, req.LastFramePath, outPath, dur, w, h)
		} else {
			err = renderKenBurnsClip(ctx, frameSrc, outPath, dur, req.CameraMovement, w, h)
		}
	} else {
		err = renderKenBurnsClip(ctx, frameSrc, outPath, dur, req.CameraMovement, w, h)
	}
	if err != nil {
		return "", &provider.ProviderError{
			Provider: m.Name(), Code: "FFMPEG_RENDER", Message: err.Error(), Retryable: false, Err: err,
		}
	}

	m.mu.Lock()
	m.tasks[id] = provider.I2VStatus{
		RemoteID: id,
		State:    provider.TaskSucceeded,
		VideoURL: "file://" + outPath,
	}
	m.mu.Unlock()
	return id, nil
}

func (m *LocalMotionI2V) Poll(_ context.Context, remoteID string) (*provider.I2VStatus, error) {
	m.mu.Lock()
	st, ok := m.tasks[remoteID]
	m.mu.Unlock()
	if !ok {
		// 支持进程重启后按约定路径恢复本地文件
		candidate := filepath.Join(m.workDir, remoteID+".mp4")
		if _, err := os.Stat(candidate); err == nil {
			return &provider.I2VStatus{
				RemoteID: remoteID,
				State:    provider.TaskSucceeded,
				VideoURL: "file://" + candidate,
			}, nil
		}
		return &provider.I2VStatus{
			RemoteID: remoteID,
			State:    provider.TaskFailed,
			ErrorMsg: "local render artifact not found",
		}, nil
	}
	return &st, nil
}

func renderKenBurnsClip(ctx context.Context, imgPath, outPath string, durSec int, move string, w, h int) error {
	if w <= 0 || h <= 0 {
		w, h = 1080, 1920
	}
	const fps = 30
	frames := durSec * fps
	if frames < 30 {
		frames = 30
	}
	dim := fmt.Sprintf("%dx%d", w, h)
	scaleCrop := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,", w, h, w, h)

	// 先缩放到目标画幅（等比裁切无黑边），再做柔和电影级慢运镜
	var zp string
	switch move {
	case "slow_pull_out":
		zp = fmt.Sprintf("zoompan=z='max(1.0,1.08-0.08*on/%d)':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=%d:s=%s:fps=%d", frames, frames, dim, fps)
	case "gentle_pan_right":
		zp = fmt.Sprintf("zoompan=z='1.08':x='(iw-iw/zoom)*on/%d':y='ih/2-(ih/zoom/2)':d=%d:s=%s:fps=%d", frames, frames, dim, fps)
	case "gentle_pan_left":
		zp = fmt.Sprintf("zoompan=z='1.08':x='(iw-iw/zoom)*(1-on/%d)':y='ih/2-(ih/zoom/2)':d=%d:s=%s:fps=%d", frames, frames, dim, fps)
	case "handheld_breath":
		zp = fmt.Sprintf("zoompan=z='1.03+0.015*sin(on*PI/%d)':x='iw/2-(iw/zoom/2)+6*sin(on*2*PI/%d)':y='ih/2-(ih/zoom/2)+4*cos(on*2*PI/%d)':d=%d:s=%s:fps=%d", frames, frames, frames, frames, dim, fps)
	case "static_gaze":
		zp = fmt.Sprintf("zoompan=z='1.01+0.015*on/%d':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=%d:s=%s:fps=%d", frames, frames, dim, fps)
	default: // slow_push_in
		zp = fmt.Sprintf("zoompan=z='min(1.08,1.0+0.08*on/%d)':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=%d:s=%s:fps=%d", frames, frames, dim, fps)
	}

	vf := scaleCrop + zp + ",format=yuv420p"
	args := []string{
		"-y",
		"-i", imgPath,
		"-vf", vf,
		"-t", fmt.Sprintf("%d", durSec),
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "20",
		"-pix_fmt", "yuv420p",
		"-an",
		outPath,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg ken-burns failed: %w: %s", err, trimFFmpegErr(string(out)))
	}
	return nil
}

func renderFirstLastFrameClip(ctx context.Context, firstImg, lastImg, outPath string, durSec int, w, h int) error {
	if w <= 0 || h <= 0 {
		w, h = 1080, 1920
	}
	if durSec < 2 {
		durSec = 4
	}
	fadeDur := float64(durSec) * 0.45
	if fadeDur < 0.8 {
		fadeDur = 0.8
	}
	offset := float64(durSec) - fadeDur
	if offset < 0.5 {
		offset = 0.5
	}
	fc := fmt.Sprintf(
		"[0:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,fps=30,format=yuv420p[v0];"+
			"[1:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,fps=30,format=yuv420p[v1];"+
			"[v0][v1]xfade=transition=fade:duration=%.2f:offset=%.2f,format=yuv420p[out]",
		w, h, w, h, w, h, w, h, fadeDur, offset,
	)
	args := []string{
		"-y",
		"-loop", "1", "-t", fmt.Sprintf("%d", durSec), "-i", firstImg,
		"-loop", "1", "-t", fmt.Sprintf("%d", durSec), "-i", lastImg,
		"-filter_complex", fc,
		"-map", "[out]",
		"-t", fmt.Sprintf("%d", durSec),
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "20",
		"-pix_fmt", "yuv420p",
		"-an",
		outPath,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg first-last-frame failed: %w: %s", err, trimFFmpegErr(string(out)))
	}
	return nil
}

// --- 4. LocalTTS：零费用本地配音（可选 macOS say，否则生成精确时长的柔和对齐音频 + 字幕时间戳） ---

type LocalTTS struct {
	UseMacOSSay bool
}

func NewLocalTTS(useMacOSSay bool) *LocalTTS {
	return &LocalTTS{UseMacOSSay: useMacOSSay}
}

func (t *LocalTTS) Name() string { return "mock-local-tts" }

func (t *LocalTTS) Synthesize(ctx context.Context, req provider.TTSRequest) (*provider.TTSResult, error) {
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(req.Text)
	runes := utf8.RuneCountInString(text)
	speed := req.Speed
	if speed <= 0 {
		speed = 0.92
	}
	// 按怀旧慢语速估算时长（约 3.1 字/秒 * speed）
	estSec := float64(runes) / (3.1 * speed)
	if estSec < 1.8 {
		estSec = 1.8
	}
	if text == "" {
		estSec = 1.0
	}

	// 可选使用 macOS 原生中文语音朗读（若失败自动回退为静音对齐音轨）
	synthesized := false
	if t.UseMacOSSay && runtime.GOOS == "darwin" && text != "" {
		aiffPath := req.OutputPath + ".aiff"
		rateWPM := int(165 * speed)
		macVoice := "Tingting"
		pitchAf := ""
		switch req.VoiceID {
		case "warm-male":
			pitchAf = ",asetrate=44100*0.92,aresample=44100,atempo=1.087"
		case "calm-senior":
			pitchAf = ",asetrate=44100*0.86,aresample=44100,atempo=1.163"
		case "childhood-boy":
			pitchAf = ",asetrate=44100*1.06,aresample=44100,atempo=0.943"
		}
		sayCmd := exec.CommandContext(ctx, "say", "-v", macVoice, "-r", fmt.Sprintf("%d", rateWPM), "-o", aiffPath, text)
		if err := sayCmd.Run(); err == nil {
			defer os.Remove(aiffPath)
			convArgs := []string{"-y", "-i", aiffPath}
			if pitchAf != "" {
				convArgs = append(convArgs, "-af", strings.TrimPrefix(pitchAf, ","))
			}
			convArgs = append(convArgs, "-ar", "44100", "-ac", "2", req.OutputPath)
			conv := exec.CommandContext(ctx, "ffmpeg", convArgs...)
			if err := conv.Run(); err == nil {
				synthesized = true
			}
		}
	}

	if !synthesized {
		// 生成精准时长的干净空白音轨，保障下游混音、侧链与字幕时间轴真实可用
		args := []string{
			"-y",
			"-f", "lavfi",
			"-i", "anullsrc=r=44100:cl=stereo",
			"-t", fmt.Sprintf("%.2f", estSec),
			req.OutputPath,
		}
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("ffmpeg mock tts failed: %w: %s", err, trimFFmpegErr(string(out)))
		}
	}

	dur := time.Duration(estSec * float64(time.Second))
	cues := buildSubtitleCues(text, dur)
	return &provider.TTSResult{
		AudioPath: req.OutputPath,
		Duration:  dur,
		Cues:      cues,
	}, nil
}

func buildSubtitleCues(text string, total time.Duration) []provider.SubtitleCue {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	// 按中文句末/停顿标点切分，保持每屏简洁克制
	parts := splitByPunctuation(text)
	if len(parts) == 0 {
		return []provider.SubtitleCue{{Text: text, Start: 0, End: total}}
	}
	totalRunes := 0
	for _, p := range parts {
		totalRunes += utf8.RuneCountInString(p)
	}
	if totalRunes == 0 {
		totalRunes = 1
	}
	var cues []provider.SubtitleCue
	cursor := time.Duration(0)
	for i, p := range parts {
		frac := float64(utf8.RuneCountInString(p)) / float64(totalRunes)
		seg := time.Duration(float64(total) * frac)
		end := cursor + seg
		if i == len(parts)-1 {
			end = total
		}
		cues = append(cues, provider.SubtitleCue{
			Text:  p,
			Start: cursor,
			End:   end,
		})
		cursor = end
	}
	return cues
}

func splitByPunctuation(s string) []string {
	var out []string
	var cur strings.Builder
	for _, r := range s {
		cur.WriteRune(r)
		if r == '，' || r == '。' || r == '？' || r == '！' || r == '；' {
			seg := strings.TrimSpace(cur.String())
			if seg != "" {
				out = append(out, seg)
			}
			cur.Reset()
		}
	}
	if rest := strings.TrimSpace(cur.String()); rest != "" {
		out = append(out, rest)
	}
	return out
}

// --- 5. LocalSFX：支持本地自定义声景优先 + FFmpeg 柔和程序化环境音合成 ---

type LocalSFX struct {
	CustomDir string // 可选：用户放入自定义 <tag>.mp3 / <tag>.wav 的目录
}

func NewLocalSFX(customDir string) *LocalSFX {
	return &LocalSFX{CustomDir: customDir}
}

func (s *LocalSFX) Name() string { return "mock-local-sfx" }

func (s *LocalSFX) Generate(ctx context.Context, req provider.SFXRequest) (string, error) {
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return "", err
	}
	dur := req.DurationSec
	if dur <= 0 {
		dur = 5
	}

	// 1. 若本地存在同名真实声景素材，直接裁切/循环适配长度
	if s.CustomDir != "" && req.Tag != "" {
		for _, ext := range []string{".wav", ".mp3", ".m4a"} {
			candidate := filepath.Join(s.CustomDir, req.Tag+ext)
			if _, err := os.Stat(candidate); err == nil {
				args := []string{
					"-y", "-stream_loop", "-1", "-i", candidate,
					"-t", fmt.Sprintf("%d", dur),
					"-af", "afade=t=in:st=0:d=0.4,afade=t=out:st=" + fmt.Sprintf("%.2f", float64(dur)-0.4) + ":d=0.4",
					"-ar", "44100", "-ac", "2",
					req.OutputPath,
				}
				if err := exec.CommandContext(ctx, "ffmpeg", args...).Run(); err == nil {
					return req.OutputPath, nil
				}
			}
		}
	}

	// 2. 使用 FFmpeg 声学滤镜程序化合成温暖柔和的环境声景（非刺耳白噪）
	af := ambientFilterForTag(req.Tag, dur)
	args := []string{
		"-y",
		"-f", "lavfi",
		"-i", fmt.Sprintf("anoisesrc=c=pink:r=44100:a=0.12:d=%d", dur),
		"-af", af,
		"-ar", "44100", "-ac", "2",
		req.OutputPath,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg mock sfx failed: %w: %s", err, trimFFmpegErr(string(out)))
	}
	return req.OutputPath, nil
}

func ambientFilterForTag(tag string, dur int) string {
	fadeOut := float64(dur) - 0.5
	if fadeOut < 0.2 {
		fadeOut = 0.2
	}
	fade := fmt.Sprintf("afade=t=in:st=0:d=0.4,afade=t=out:st=%.2f:d=0.5", fadeOut)
	switch tag {
	case "summer_cicada":
		// 柔和夏日暖风 + 轻颤蝉鸣呼吸感
		return "bandpass=f=2400:width_type=h:w=1600,tremolo=f=5.5:d=0.45,volume=0.35," + fade
	case "old_freezer_hum", "old_fan_hum":
		// 温暖低频老电器平稳嗡鸣
		return "lowpass=f=260,highpass=f=55,volume=0.65," + fade
	case "evening_crickets":
		// 静谧夏夜微风与轻缓虫鸣包络
		return "bandpass=f=3200:width_type=h:w=1200,tremolo=f=3.2:d=0.6,volume=0.25," + fade
	case "urban_rain_soft", "kitchen_simmer":
		// 柔和细雨/厨房轻沸低噪
		return "lowpass=f=1800,highpass=f=180,volume=0.45," + fade
	default: // alley_breeze / evening_breeze
		return "lowpass=f=750,highpass=f=90,tremolo=f=0.35:d=0.35,volume=0.48," + fade
	}
}

func trimFFmpegErr(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	return strings.Join(lines, " | ")
}
