package psychology

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ShotForReview 是心理学质检器所需的最小分镜视图，避免循环依赖。
type ShotForReview struct {
	ShotID          string
	Phase           ActPhase
	PsychTrigger    string
	Symbols         []string
	Narration       string
	DurationSec     int
	AmbientTag      string
	MotionActionZH  string
	EnvPhysicsZH    string
	CameraMovement  string
}

type IssueSeverity string

const (
	SeverityError   IssueSeverity = "error"
	SeverityWarning IssueSeverity = "warning"
)

type CriticIssue struct {
	ShotID   string        `yaml:"shot_id" json:"shot_id"`
	Rule     string        `yaml:"rule" json:"rule"`
	Severity IssueSeverity `yaml:"severity" json:"severity"`
	Message  string        `yaml:"message" json:"message"`
}

type CriticReport struct {
	Passed             bool                `yaml:"passed" json:"passed"`
	Score              int                 `yaml:"score" json:"score"`
	PhaseDistribution  map[ActPhase]int    `yaml:"phase_distribution" json:"phase_distribution"`
	AvgSilenceRatio    float64             `yaml:"avg_silence_ratio" json:"avg_silence_ratio"`
	SymbolHitCount     int                 `yaml:"symbol_hit_count" json:"symbol_hit_count"`
	Issues             []CriticIssue       `yaml:"issues" json:"issues"`
}

// RevisionFeedback 将质检问题汇总为发回 LLM 的纠偏提示，用于自动润色分镜稿。
func (r CriticReport) RevisionFeedback() string {
	if len(r.Issues) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("上版分镜未通过社会心理学质检，请按以下要点逐条修正：\n")
	for _, issue := range r.Issues {
		loc := "全局"
		if issue.ShotID != "" {
			loc = "镜头 " + issue.ShotID
		}
		fmt.Fprintf(&b, "- [%s][%s] %s：%s\n", issue.Severity, issue.Rule, loc, issue.Message)
	}
	return b.String()
}

// Evaluate 对整套分镜执行社会心理学与反 AI 味规则校验。
func Evaluate(pack *StrategyPack, shots []ShotForReview) CriticReport {
	report := CriticReport{
		Passed:            true,
		Score:             100,
		PhaseDistribution: make(map[ActPhase]int),
	}
	if pack == nil || len(shots) == 0 {
		report.Passed = false
		report.Score = 0
		report.Issues = append(report.Issues, CriticIssue{
			Rule:     "empty_storyboard",
			Severity: SeverityError,
			Message:  "分镜列表为空，无法进行心理学弧线评估",
		})
		return report
	}

	var totalSilence float64
	symbolSet := make(map[string]struct{})
	lastPhaseOrder := -1
	phaseIndex := map[ActPhase]int{
		PhaseSensoryHook:      0,
		PhaseConcreteRitual:   1,
		PhaseRelationalWarmth: 2,
		PhaseLiminalClosure:   3,
	}

	// 剧烈动作词检查（情怀片忌剧烈崩坏动作）
	aggressiveMotionWords := []string{"狂奔", "剧烈摇晃", "飞速旋转", "大喊大叫", "跳跃翻滚"}

	for _, shot := range shots {
		dur := shot.DurationSec
		if dur <= 0 {
			dur = 5
		}

		// 1. 情绪弧线幕次统计与顺序校验
		if idx, ok := phaseIndex[shot.Phase]; ok {
			report.PhaseDistribution[shot.Phase]++
			if idx < lastPhaseOrder {
				report.Issues = append(report.Issues, CriticIssue{
					ShotID:   shot.ShotID,
					Rule:     "arc_order",
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("情绪幕次 %s 出现回退，建议按四幕弧线顺次递进", shot.Phase),
				})
				report.Score -= 5
			}
			lastPhaseOrder = idx
		} else {
			report.Issues = append(report.Issues, CriticIssue{
				ShotID:   shot.ShotID,
				Rule:     "invalid_phase",
				Severity: SeverityError,
				Message:  fmt.Sprintf("未知的心理幕次标签 %q", shot.Phase),
			})
			report.Score -= 10
		}

		// 2. 反说教 / 反 AI 味禁词检测
		for _, cliche := range pack.NarrationRules.ForbiddenCliches {
			if cliche != "" && strings.Contains(shot.Narration, cliche) {
				report.Issues = append(report.Issues, CriticIssue{
					ShotID:   shot.ShotID,
					Rule:     "anti_reactance_cliche",
					Severity: SeverityError,
					Message:  fmt.Sprintf("旁白包含说教/AI套话 %q，易引发心理抗拒，请替换为具体感官动作描写", cliche),
				})
				report.Score -= 12
			}
		}

		// 3. 旁白字数与留白呼吸比例检查
		cleanText := countSpokenRunes(shot.Narration)
		maxChars := pack.NarrationRules.MaxCharsPerShot
		if maxChars > 0 && cleanText > maxChars {
			report.Issues = append(report.Issues, CriticIssue{
				ShotID:   shot.ShotID,
				Rule:     "narration_too_long",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("旁白共 %d 字，超过单镜建议上限 %d 字，可能压榨画面呼吸感", cleanText, maxChars),
			})
			report.Score -= 5
		}

		cps := pack.NarrationRules.CharsPerSecondMax
		if cps <= 0 {
			cps = 3.2
		}
		spokenSec := float64(cleanText) / cps
		silenceRatio := 1.0 - (spokenSec / float64(dur))
		if silenceRatio < 0 {
			silenceRatio = 0
		}
		totalSilence += silenceRatio
		if pack.NarrationRules.MinSilenceRatio > 0 && silenceRatio < pack.NarrationRules.MinSilenceRatio-0.05 {
			report.Issues = append(report.Issues, CriticIssue{
				ShotID:   shot.ShotID,
				Rule:     "insufficient_breathing_room",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("预估留白占比 %.0f%% 低于阈值 %.0f%%，建议精简旁白或延长镜头时长", silenceRatio*100, pack.NarrationRules.MinSilenceRatio*100),
			})
			report.Score -= 4
		}

		// 4. 时代暗号与声景完整性
		for _, s := range shot.Symbols {
			if strings.TrimSpace(s) != "" {
				symbolSet[strings.TrimSpace(s)] = struct{}{}
			}
		}
		if strings.TrimSpace(shot.AmbientTag) == "" {
			report.Issues = append(report.Issues, CriticIssue{
				ShotID:   shot.ShotID,
				Rule:     "missing_ambient_sfx",
				Severity: SeverityWarning,
				Message:  "缺少环境声景标签（普鲁斯特听觉线索），建议补充如蝉鸣/晚风/老风扇声景",
			})
			report.Score -= 3
		}

		// 5. I2V 动作克制与环境微动态检查
		if strings.TrimSpace(shot.EnvPhysicsZH) == "" {
			report.Issues = append(report.Issues, CriticIssue{
				ShotID:   shot.ShotID,
				Rule:     "missing_env_physics",
				Severity: SeverityWarning,
				Message:  "I2V 缺少环境微动态描写（如浮尘、树影、微风），静态画面容易缺乏生命力",
			})
			report.Score -= 3
		}
		for _, bad := range aggressiveMotionWords {
			if strings.Contains(shot.MotionActionZH, bad) {
				report.Issues = append(report.Issues, CriticIssue{
					ShotID:   shot.ShotID,
					Rule:     "overly_aggressive_motion",
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("I2V 动作词含剧烈运动 %q，容易导致图生视频画面崩坏并破坏克制情绪", bad),
				})
				report.Score -= 5
			}
		}
	}

	// 6. 全局四幕覆盖检查（>=4镜时要求覆盖完整四幕）
	if len(shots) >= 4 {
		for _, p := range OrderedPhases {
			if report.PhaseDistribution[p] == 0 {
				report.Issues = append(report.Issues, CriticIssue{
					Rule:     "missing_arc_phase",
					Severity: SeverityError,
					Message:  fmt.Sprintf("整片缺少心理学弧线阶段 %q，无法形成完整的怀旧救赎闭环", p),
				})
				report.Score -= 15
			}
		}
	}

	report.AvgSilenceRatio = totalSilence / float64(len(shots))
	report.SymbolHitCount = len(symbolSet)
	if len(shots) >= 4 && report.SymbolHitCount < 2 {
		report.Issues = append(report.Issues, CriticIssue{
			Rule:     "low_era_symbol_density",
			Severity: SeverityWarning,
			Message:  "整片时代暗号/具象物件数量偏少（<2），可能削弱集体记忆与评论区认同触发",
		})
		report.Score -= 6
	}

	if report.Score < 0 {
		report.Score = 0
	}
	for _, iss := range report.Issues {
		if iss.Severity == SeverityError {
			report.Passed = false
			break
		}
	}
	return report
}

func countSpokenRunes(s string) int {
	replacer := strings.NewReplacer(
		" ", "", "\n", "", "\t", "",
		"，", "", "。", "", "、", "", "？", "", "！", "", "…", "", "—", "",
		",", "", ".", "", "?", "", "!", "", "“", "", "”", "", "\"", "",
	)
	return utf8.RuneCountInString(replacer.Replace(s))
}
