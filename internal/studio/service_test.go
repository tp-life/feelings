package studio

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"feelings-studio/internal/project"
)

func openTestService(t *testing.T) *Service {
	t.Helper()
	tmp := t.TempDir()
	svc, err := Open(tmp, filepath.Join(tmp, "configs"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func TestInitProjectRejectsDuplicateAndInvalidIDs(t *testing.T) {
	svc := openTestService(t)
	if _, err := svc.InitProject(InitOptions{ID: "p1"}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	if _, err := svc.InitProject(InitOptions{ID: "p1"}); err == nil {
		t.Fatalf("InitProject must not silently overwrite an existing project")
	}
	for _, id := range []string{"", "..", "../x", "a/b", ".hidden", "a b"} {
		if _, err := svc.InitProject(InitOptions{ID: id}); err == nil {
			t.Errorf("InitProject(%q) should fail", id)
		}
	}
}

// 并发的「读-改-写」必须串行化，任何一次修改都不能被别的写入覆盖丢失。
func TestConcurrentMutationsAreNotLost(t *testing.T) {
	svc := openTestService(t)
	if _, err := svc.InitProject(InitOptions{ID: "p1"}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.mutateProject("p1", func(p *project.Project) error {
				p.Assets = append(p.Assets, project.AssetRef{ID: fmt.Sprintf("x%d", i), Name: fmt.Sprintf("asset %d", i)})
				return nil
			})
			if err != nil {
				t.Errorf("mutateProject: %v", err)
			}
		}(i)
	}
	wg.Wait()

	p, err := project.Load(svc.RootDir, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Assets) != n {
		t.Fatalf("got %d assets after %d concurrent mutations, want %d (lost updates)", len(p.Assets), n, n)
	}
}

func TestPlanProjectKeepsConcurrentEditsOutsideStoryboard(t *testing.T) {
	svc := openTestService(t)
	if _, err := svc.InitProject(InitOptions{ID: "p1", Theme: "夏天的小卖部"}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	p, err := svc.PlanProject(context.Background(), "p1")
	if err != nil {
		t.Fatalf("PlanProject: %v", err)
	}
	if len(p.Shots) == 0 {
		t.Fatalf("PlanProject produced no shots")
	}

	// 设置项改动 + 再次编排：编排结果回写时必须基于最新清单，不能把标题改回旧值
	p.Title = "新标题"
	if err := svc.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	p2, err := svc.PlanProject(context.Background(), "p1")
	if err != nil {
		t.Fatalf("PlanProject #2: %v", err)
	}
	if p2.Title != "新标题" {
		t.Fatalf("title = %q, want 新标题", p2.Title)
	}
}

func TestGetProviderSettingsReturnsCopy(t *testing.T) {
	svc := openTestService(t)
	cfg := svc.GetProviderSettings()
	cfg.Active.LLM = "tampered"
	for k := range cfg.Profiles {
		delete(cfg.Profiles, k)
	}
	again := svc.GetProviderSettings()
	if again.Active.LLM == "tampered" || len(again.Profiles) == 0 {
		t.Fatalf("GetProviderSettings must return an independent copy")
	}
}

func TestBGMPreviewRejectsUnknownPreset(t *testing.T) {
	svc := openTestService(t)
	if _, err := svc.EnsureBGMPresetPreview(context.Background(), "../../evil"); err == nil {
		t.Fatalf("unknown preset should be rejected")
	}
}
