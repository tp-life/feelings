package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"feelings-studio/internal/project"
	"feelings-studio/internal/prompting"
	"feelings-studio/internal/psychology"
)

func TestImportFromDirAndSaveLoad(t *testing.T) {
	tmp := t.TempDir()
	assetsDir := filepath.Join(tmp, "raw")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"资产 男孩.jpeg",
		"资产 小卖店老板.jpeg",
		"g1.jpeg",
		"g2.jpeg",
		"g10.jpeg",
	} {
		if err := os.WriteFile(filepath.Join(assetsDir, name), []byte("img"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	p := &project.Project{
		ID:          "summer-1998",
		Title:       "1998年的夏天小卖部",
		Theme:       "90年代夏天小卖部的五毛钱冰棍",
		Track:       psychology.TrackChildhood90s,
		StylePreset: "film_90s",
		PromptLang:  prompting.LangZH,
		Mix:         project.DefaultMixConfig(),
	}
	if err := project.ImportFromDir(p, assetsDir); err != nil {
		t.Fatalf("ImportFromDir: %v", err)
	}
	if len(p.Assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(p.Assets))
	}
	if len(p.Shots) != 3 {
		t.Fatalf("expected 3 shots, got %d", len(p.Shots))
	}
	// 验证按数字自然序排序：g1 -> g2 -> g10
	if p.Shots[0].ShotID != "g1" || p.Shots[1].ShotID != "g2" || p.Shots[2].ShotID != "g10" {
		t.Fatalf("unexpected shot order: %s, %s, %s", p.Shots[0].ShotID, p.Shots[1].ShotID, p.Shots[2].ShotID)
	}

	if err := project.Save(tmp, p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := project.Load(tmp, "summer-1998")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Title != p.Title || len(loaded.Shots) != 3 {
		t.Fatalf("loaded manifest mismatch: %+v", loaded)
	}
}
