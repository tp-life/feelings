package psychology_test

import (
	"testing"

	"feelings-studio/internal/psychology"
)

func TestLoadStrategiesAndEvaluate(t *testing.T) {
	repo, err := psychology.LoadRepository("")
	if err != nil {
		t.Fatalf("LoadRepository failed: %v", err)
	}
	packs := repo.List()
	if len(packs) < 3 {
		t.Fatalf("expected at least 3 built-in strategies, got %d", len(packs))
	}

	pack, err := repo.Get(psychology.TrackChildhood90s)
	if err != nil {
		t.Fatalf("Get childhood_90s failed: %v", err)
	}

	goodShots := []psychology.ShotForReview{
		{
			ShotID:         "g1",
			Phase:          psychology.PhaseSensoryHook,
			Symbols:        []string{"斑驳水泥墙与木门"},
			Narration:      "那年夏天的蝉鸣，总从小卖部门口开始。",
			DurationSec:    6,
			AmbientTag:     "summer_cicada",
			MotionActionZH: "远处行人缓缓走过巷口",
			EnvPhysicsZH:   "午后热风轻拂门帘，阳光中微尘缓缓漂浮",
		},
		{
			ShotID:         "g2",
			Phase:          psychology.PhaseConcreteRitual,
			Symbols:        []string{"五毛硬币", "老式白色卧式冰柜"},
			Narration:      "掌心里三枚硬币，够挑整个下午的甜。",
			DurationSec:    6,
			AmbientTag:     "old_freezer_hum",
			MotionActionZH: "男孩轻轻打开冰柜盖，手指停在冰棍上",
			EnvPhysicsZH:   "淡淡冷白雾气从冰柜边缘缓缓散开",
		},
		{
			ShotID:         "g3",
			Phase:          psychology.PhaseRelationalWarmth,
			Symbols:        []string{"水果硬糖"},
			Narration:      "老板笑着用指尖，多塞来一颗水果糖。",
			DurationSec:    6,
			AmbientTag:     "alley_breeze",
			MotionActionZH: "老板微倾递糖，男孩抬头露出腼腆微笑",
			EnvPhysicsZH:   "斜射光斑在木柜台表面柔和明暗轻移",
		},
		{
			ShotID:         "g4",
			Phase:          psychology.PhaseLiminalClosure,
			Symbols:        []string{"发黄旧相册"},
			Narration:      "巷口那间小卖部，现在还在吗？",
			DurationSec:    6,
			AmbientTag:     "evening_crickets",
			MotionActionZH: "夕阳把空木凳的影子缓缓拉长",
			EnvPhysicsZH:   "傍晚微风轻吹树梢，光影柔和沉静",
		},
	}

	report := psychology.Evaluate(pack, goodShots)
	if !report.Passed {
		t.Fatalf("expected good storyboard to pass, got issues: %+v", report.Issues)
	}
	if report.Score < 90 {
		t.Fatalf("expected high score >= 90, got %d (issues: %+v)", report.Score, report.Issues)
	}

	// 测试反说教禁词拦截与缺幕拦截
	badShots := []psychology.ShotForReview{
		{
			ShotID:         "b1",
			Phase:          psychology.PhaseSensoryHook,
			Narration:      "时光飞逝，那一刻我终于明白且行且珍惜，致敬逝去的青春！",
			DurationSec:    4,
			MotionActionZH: "人物狂奔大喊大叫",
		},
		{
			ShotID:      "b2",
			Phase:       psychology.PhaseSensoryHook,
			Narration:   "泪目了",
			DurationSec: 4,
		},
		{
			ShotID:      "b3",
			Phase:       psychology.PhaseSensoryHook,
			Narration:   "破防了",
			DurationSec: 4,
		},
		{
			ShotID:      "b4",
			Phase:       psychology.PhaseSensoryHook,
			Narration:   "满满的回忆杀",
			DurationSec: 4,
		},
	}
	badReport := psychology.Evaluate(pack, badShots)
	if badReport.Passed {
		t.Fatalf("expected preachy storyboard to fail critic")
	}
	if badReport.RevisionFeedback() == "" {
		t.Fatalf("expected non-empty revision feedback")
	}
}
