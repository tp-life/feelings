package prompting_test

import (
	"strings"
	"testing"

	"feelings-studio/internal/prompting"
	"feelings-studio/internal/psychology"
)

func TestCompilerSystemAndBilingualShots(t *testing.T) {
	repo, err := psychology.LoadRepository("")
	if err != nil {
		t.Fatalf("LoadRepository: %v", err)
	}
	pack, err := repo.Get(psychology.TrackChildhood90s)
	if err != nil {
		t.Fatalf("Get strategy: %v", err)
	}

	comp, err := prompting.NewCompiler("")
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}
	if len(comp.ListStyles()) != 3 {
		t.Fatalf("expected 3 style presets, got %d", len(comp.ListStyles()))
	}
	style, err := comp.GetStyle("film_90s")
	if err != nil {
		t.Fatalf("GetStyle: %v", err)
	}

	sys, err := comp.RenderSystemPrompts(prompting.SystemTemplateContext{
		Theme:     "1998年夏天巷口小卖部与五毛钱冰棍",
		ShotCount: 10,
		Strategy:  pack,
		Style:     style,
		Assets: map[string]string{
			"男孩": "8岁寸头小男孩，微旧纯白T恤，深蓝短裤，黑布鞋",
		},
	})
	if err != nil {
		t.Fatalf("RenderSystemPrompts: %v", err)
	}
	if !strings.Contains(sys.DirectorStoryboard, "时光飞逝") {
		t.Fatalf("director prompt should embed forbidden cliches")
	}
	if !strings.Contains(sys.T2ISystem, "9:16") {
		t.Fatalf("t2i system prompt should mention 9:16 framing")
	}
	if !strings.Contains(sys.I2VSystem, "first_last_frame") {
		t.Fatalf("i2v system prompt should mention first_last_frame guidance")
	}

	compiled := comp.CompileShot("g5", prompting.T2IPromptSpec{
		SubjectAnchor:  prompting.BilingualText{ZH: "8岁寸头男孩与穿灰T恤的小卖部老板", EN: "8-year-old boy with buzz cut and shopkeeper in grey t-shirt"},
		ActionRelation: prompting.BilingualText{ZH: "老板微笑微倾，隔着木柜台递出一颗水果糖", EN: "shopkeeper leaning with a gentle smile handing over a wrapped fruit candy"},
		EraEnvironment: prompting.BilingualText{ZH: "90年代小卖部内部，玻璃糖罐与老铁皮零钱盒", EN: "1990s corner store interior with glass candy jars and tin cash box"},
		ShotFraming:    prompting.BilingualText{ZH: "9:16竖屏侧面中景", EN: "9:16 vertical side medium shot"},
		LightingMood:   prompting.BilingualText{ZH: "午后柔和窗光侧照", EN: "soft warm afternoon window light"},
	}, prompting.I2VPromptSpec{
		SubjectMicroAction: prompting.BilingualText{ZH: "老板手指轻递糖果，男孩伸手接住微微扬起嘴角", EN: "shopkeeper gently hands over candy as the boy reaches out with a shy smile"},
		EnvironmentPhysics: prompting.BilingualText{ZH: "窗外斜阳微尘轻浮，老木门外树影微晃", EN: "dust motes drifting in warm sunlight, soft tree shadows swaying outside"},
		CameraMovement:     "slow_push_in",
		LightingPacing:     prompting.BilingualText{ZH: "克制舒缓24fps胶片节奏", EN: "restrained unhurried 24fps film cadence"},
		TransitionMode:     prompting.TransitionFirstLastFrame,
		TransitionHint:     prompting.BilingualText{ZH: "镜头由柜台递糖平稳衔接至巷口分冰棍", EN: "smoothly bridge from candy handoff toward alley sunlight"},
	}, style)

	if !strings.Contains(compiled.I2VZH.Prompt, "首尾帧平滑过渡") {
		t.Fatalf("expected ZH I2V prompt to include transition hint, got %q", compiled.I2VZH.Prompt)
	}
	if !strings.Contains(compiled.I2VEN.Prompt, "First-to-last frame transition") {
		t.Fatalf("expected EN I2V prompt to include transition hint, got %q", compiled.I2VEN.Prompt)
	}
	if compiled.T2IZH.NegativePrompt == "" || compiled.I2VEN.NegativePrompt == "" {
		t.Fatalf("expected non-empty compiled negative prompts")
	}

	// 验证导演时序详版将「纯净视频生成指令」与「后期心理字幕/转场说明」严格隔离
	dirSheet := comp.CompileDirectorSheet("g5", 5, 5, prompting.T2IPromptSpec{}, prompting.I2VPromptSpec{
		CameraMovement: "slow_push_in",
	}, prompting.DirectorSheetSpec{
		ShotTitle: prompting.BilingualText{ZH: "关系微光 · 递糖", EN: "Candy Handoff"},
		AssetBindings: []prompting.AssetBinding{
			{
				Role: "character", AssetName: "男孩", RefToken: "{{Asset 3}}",
				UsageRule: prompting.BilingualText{ZH: "只参考外貌与服装，不参考站姿", EN: "Face and outfit only"},
			},
		},
		PostProduction: prompting.PostProductionNotes{
			SubtitleStrategy: "后期压字",
			TransitionDesign: prompting.TransitionDesignSpec{ToNext: "0.4s 叠化"},
		},
	}, style, prompting.LangZH)

	if !strings.Contains(dirSheet.GenerationPrompt, "资产图锚定") || !strings.Contains(dirSheet.GenerationPrompt, "时间段：") {
		t.Fatalf("expected director sheet sections in generation prompt, got:\n%s", dirSheet.GenerationPrompt)
	}
	if strings.Contains(dirSheet.GenerationPrompt, "后期压字") {
		t.Fatalf("post-production subtitle note must NOT leak into generation prompt")
	}
	if !strings.Contains(dirSheet.FullSheet, "后期压字") || !strings.Contains(dirSheet.FullSheet, "0.4s 叠化") {
		t.Fatalf("expected post-production notes in full sheet")
	}
}
