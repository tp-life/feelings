package psychology

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"feelings-studio/configs"
)

type TrackID string

const (
	TrackChildhood90s   TrackID = "childhood_90s"
	TrackFamilyBond     TrackID = "family_bond"
	TrackUrbanSanctuary TrackID = "urban_sanctuary"
)

type ActPhase string

const (
	PhaseSensoryHook      ActPhase = "sensory_hook"
	PhaseConcreteRitual   ActPhase = "concrete_ritual"
	PhaseRelationalWarmth ActPhase = "relational_warmth"
	PhaseLiminalClosure   ActPhase = "liminal_closure"
)

// OrderedPhases 定义社会心理学怀旧救赎弧线的标准四幕顺序。
var OrderedPhases = []ActPhase{
	PhaseSensoryHook,
	PhaseConcreteRitual,
	PhaseRelationalWarmth,
	PhaseLiminalClosure,
}

type ArcStageRule struct {
	Phase          ActPhase `yaml:"phase" json:"phase"`
	Label          string   `yaml:"label" json:"label"`
	PsychologyGoal string   `yaml:"psychology_goal" json:"psychology_goal"`
	MinRatio       float64  `yaml:"min_ratio" json:"min_ratio"`
	MaxRatio       float64  `yaml:"max_ratio" json:"max_ratio"`
	PacingHint     string   `yaml:"pacing_hint" json:"pacing_hint"`
}

type SensoryProfile struct {
	VisualPalette string   `yaml:"visual_palette" json:"visual_palette"`
	AmbientTags   []string `yaml:"ambient_tags" json:"ambient_tags"`
	BGMMood       string   `yaml:"bgm_mood" json:"bgm_mood"`
}

type NarrationGuardrail struct {
	Perspective         string   `yaml:"perspective" json:"perspective"`
	CharsPerSecondMin   float64  `yaml:"chars_per_second_min" json:"chars_per_second_min"`
	CharsPerSecondMax   float64  `yaml:"chars_per_second_max" json:"chars_per_second_max"`
	MinSilenceRatio     float64  `yaml:"min_silence_ratio" json:"min_silence_ratio"`
	MaxCharsPerShot     int      `yaml:"max_chars_per_shot" json:"max_chars_per_shot"`
	ForbiddenCliches    []string `yaml:"forbidden_cliches" json:"forbidden_cliches"`
	PreferredSensoryVerbs []string `yaml:"preferred_sensory_verbs" json:"preferred_sensory_verbs"`
}

type StrategyPack struct {
	ID             TrackID            `yaml:"id" json:"id"`
	Name           string             `yaml:"name" json:"name"`
	Tagline        string             `yaml:"tagline" json:"tagline"`
	TheoryBasis    []string           `yaml:"theory_basis" json:"theory_basis"`
	TargetAudience string             `yaml:"target_audience" json:"target_audience"`
	DefaultStyle   string             `yaml:"default_style" json:"default_style"`
	EmotionalArc   []ArcStageRule     `yaml:"emotional_arc" json:"emotional_arc"`
	EraSymbols     []string           `yaml:"era_symbols" json:"era_symbols"`
	SensoryAnchors SensoryProfile     `yaml:"sensory_anchors" json:"sensory_anchors"`
	NarrationRules NarrationGuardrail `yaml:"narration_rules" json:"narration_rules"`
	ClosureHooks   []string           `yaml:"closure_hooks" json:"closure_hooks"`
}

// Repository 管理社会心理学策略包的加载与枚举。
type Repository struct {
	overrideDir string
	packs       map[TrackID]*StrategyPack
}

func LoadRepository(overrideDir string) (*Repository, error) {
	files, err := configs.ListDir(overrideDir, "strategies")
	if err != nil {
		return nil, fmt.Errorf("list strategies: %w", err)
	}
	repo := &Repository{
		overrideDir: overrideDir,
		packs:       make(map[TrackID]*StrategyPack),
	}
	for _, f := range files {
		if !strings.HasSuffix(f, ".yaml") && !strings.HasSuffix(f, ".yml") {
			continue
		}
		data, err := configs.ReadFile(overrideDir, f)
		if err != nil {
			return nil, fmt.Errorf("read strategy %s: %w", f, err)
		}
		var pack StrategyPack
		if err := yaml.Unmarshal(data, &pack); err != nil {
			return nil, fmt.Errorf("parse strategy %s: %w", f, err)
		}
		if pack.ID == "" {
			base := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
			pack.ID = TrackID(base)
		}
		p := pack
		repo.packs[pack.ID] = &p
	}
	return repo, nil
}

func (r *Repository) Get(id TrackID) (*StrategyPack, error) {
	p, ok := r.packs[id]
	if !ok {
		return nil, fmt.Errorf("unknown psychology strategy track: %q", id)
	}
	return p, nil
}

func (r *Repository) List() []*StrategyPack {
	out := make([]*StrategyPack, 0, len(r.packs))
	for _, p := range r.packs {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}
