package settings_test

import (
	"context"
	"strings"
	"testing"

	"feelings-studio/internal/settings"
)

func TestProvidersConfigMergeMaskAndCatalog(t *testing.T) {
	cat := settings.Catalog()
	found := map[string]bool{}
	for _, c := range cat {
		found[c.ID] = true
	}
	for _, want := range []string{"jimeng", "wan", "minimax_hailuo", "kling", "veo", "runway", "luma", "elevenlabs", "deepseek"} {
		if !found[want] {
			t.Fatalf("expected provider %q in catalog", want)
		}
	}

	tmp := t.TempDir()
	cfg := settings.DefaultConfig()
	cfg.Active.I2V = "jimeng"
	p := cfg.Profiles["jimeng"]
	p.APIKey = "ark-secret-1234567890"
	cfg.Profiles["jimeng"] = p

	saved, err := settings.MergeAndSave(tmp, cfg)
	if err != nil {
		t.Fatalf("MergeAndSave: %v", err)
	}
	masked := saved.MaskedCopy()
	if !strings.Contains(masked.Profiles["jimeng"].APIKey, "****") {
		t.Fatalf("expected masked api key, got %q", masked.Profiles["jimeng"].APIKey)
	}

	// 再次保存脱敏副本时，不得覆盖掉真实密钥
	masked.Active.I2V = "wan"
	reloaded, err := settings.MergeAndSave(tmp, masked)
	if err != nil {
		t.Fatalf("second MergeAndSave: %v", err)
	}
	if reloaded.Profiles["jimeng"].APIKey != "ark-secret-1234567890" {
		t.Fatalf("expected existing secret preserved, got %q", reloaded.Profiles["jimeng"].APIKey)
	}

	res := settings.TestProvider(context.Background(), reloaded, "mock")
	if !res.OK {
		t.Fatalf("expected mock test ok, got %+v", res)
	}
	missing := settings.TestProvider(context.Background(), reloaded, "wan")
	if missing.OK {
		t.Fatalf("expected wan without api key to fail validation")
	}
}
