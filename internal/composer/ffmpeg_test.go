package composer_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"feelings-studio/internal/composer"
	"feelings-studio/internal/project"
	"feelings-studio/internal/prompting"
	"feelings-studio/internal/provider"
	"feelings-studio/internal/provider/mock"
)

func TestComposeRealFFmpegEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed in PATH")
	}

	ctx := context.Background()
	tmp := t.TempDir()

	comp, err := prompting.NewCompiler("")
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}
	style, err := comp.GetStyle("film_90s")
	if err != nil {
		t.Fatalf("GetStyle: %v", err)
	}

	t2i := mock.NewLocalT2I()
	i2v := mock.NewLocalMotionI2V(filepath.Join(tmp, "i2v"))
	tts := mock.NewLocalTTS(false)
	sfx := mock.NewLocalSFX("")

	var clips []composer.ShotClip
	for i, text := range []string{"那年夏天的蝉鸣，总从小卖部门口开始。", "你家巷口的小卖部，现在还在吗？"} {
		frame := filepath.Join(tmp, "frame_"+string(rune('1'+i))+".jpeg")
		if _, err := t2i.Generate(ctx, provider.T2IRequest{OutputPath: frame, Seed: int64(i + 1)}); err != nil {
			t.Fatalf("t2i: %v", err)
		}
		rid, err := i2v.Submit(ctx, provider.I2VSubmitRequest{
			FirstFramePath: frame,
			DurationSec:    2,
			CameraMovement: "slow_push_in",
		})
		if err != nil {
			t.Fatalf("i2v submit: %v", err)
		}
		st, err := i2v.Poll(ctx, rid)
		if err != nil || st.State != provider.TaskSucceeded {
			t.Fatalf("i2v poll: %+v err=%v", st, err)
		}
		videoPath := st.VideoURL[len("file://"):]

		voicePath := filepath.Join(tmp, "voice_"+string(rune('1'+i))+".wav")
		if _, err := tts.Synthesize(ctx, provider.TTSRequest{Text: text, Speed: 0.92, OutputPath: voicePath}); err != nil {
			t.Fatalf("tts: %v", err)
		}
		sfxPath := filepath.Join(tmp, "sfx_"+string(rune('1'+i))+".wav")
		if _, err := sfx.Generate(ctx, provider.SFXRequest{Tag: "summer_cicada", DurationSec: 2, OutputPath: sfxPath}); err != nil {
			t.Fatalf("sfx: %v", err)
		}

		clips = append(clips, composer.ShotClip{
			ShotID:      "s" + string(rune('1'+i)),
			VideoPath:   videoPath,
			VoicePath:   voicePath,
			AmbientPath: sfxPath,
			Narration:   text,
			DurationSec: 2,
		})
	}

	outMP4 := filepath.Join(tmp, "final.mp4")
	info, err := composer.Compose(ctx, composer.ComposeRequest{
		Title:      "Test Nostalgia",
		Clips:      clips,
		Style:      style,
		Mix:        project.DefaultMixConfig(),
		OutputPath: outMP4,
		WorkDir:    filepath.Join(tmp, "work"),
	})
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	if info.Width != 1080 || info.Height != 1920 {
		t.Fatalf("expected 1080x1920 output, got %dx%d", info.Width, info.Height)
	}
	if info.VideoCodec != "h264" || info.AudioCodec != "aac" {
		t.Fatalf("expected h264+aac, got video=%s audio=%s", info.VideoCodec, info.AudioCodec)
	}
	if info.DurationSec < 3.0 {
		t.Fatalf("expected duration >= 3s, got %.2f", info.DurationSec)
	}
	if info.CoverPath == "" {
		t.Fatalf("expected auto-generated cover.jpg path, got empty")
	}
}

func TestComposeVoiceStretchAndLandscape16x9(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed in PATH")
	}

	ctx := context.Background()
	tmp := t.TempDir()

	comp, err := prompting.NewCompiler("")
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}
	style, err := comp.GetStyle("retro_dv")
	if err != nil {
		t.Fatalf("GetStyle: %v", err)
	}

	t2i := mock.NewLocalT2I()
	i2v := mock.NewLocalMotionI2V(filepath.Join(tmp, "i2v"))
	tts := mock.NewLocalTTS(false)

	frame := filepath.Join(tmp, "f1.jpeg")
	if _, err := t2i.Generate(ctx, provider.T2IRequest{AspectRatio: "16:9", OutputPath: frame, Seed: 7}); err != nil {
		t.Fatalf("t2i: %v", err)
	}
	rid, err := i2v.Submit(ctx, provider.I2VSubmitRequest{
		FirstFramePath: frame,
		AspectRatio:    "16:9",
		DurationSec:    2,
		CameraMovement: "slow_push_in",
	})
	if err != nil {
		t.Fatalf("i2v submit: %v", err)
	}
	st, err := i2v.Poll(ctx, rid)
	if err != nil || st.State != provider.TaskSucceeded {
		t.Fatalf("i2v poll: %+v err=%v", st, err)
	}
	videoPath := st.VideoURL[len("file://"):]

	// 长旁白（约 30 字 -> 约 10s 语音）配 2s 原始视频，验证视频自动慢放+尾帧定格延展以完整容纳人声
	longNarration := "离家那天凌晨五点厨房亮起的暖灯，和父母塞满整个后备箱的笨拙牵挂。"
	voicePath := filepath.Join(tmp, "v1.wav")
	ttsRes, err := tts.Synthesize(ctx, provider.TTSRequest{Text: longNarration, Speed: 0.92, OutputPath: voicePath})
	if err != nil {
		t.Fatalf("tts: %v", err)
	}

	mix := project.DefaultMixConfig()
	mix.BGMPreset = "tape_piano"

	outMP4 := filepath.Join(tmp, "landscape_final.mp4")
	info, err := composer.Compose(ctx, composer.ComposeRequest{
		Title:       "离家前的后备箱",
		Subtitle:    "总有一盏窗为你亮着",
		AspectRatio: "16:9",
		Clips: []composer.ShotClip{
			{
				ShotID:      "s1",
				VideoPath:   videoPath,
				VoicePath:   voicePath,
				Narration:   longNarration,
				DurationSec: 2,
			},
		},
		Style:       style,
		Mix:         mix,
		OutputPath:  outMP4,
		WorkDir:     filepath.Join(tmp, "work169"),
		VideoPreset: "ultrafast",
	})
	if err != nil {
		t.Fatalf("Compose 16:9 failed: %v", err)
	}
	if info.Width != 1920 || info.Height != 1080 {
		t.Fatalf("expected 1920x1080 landscape output, got %dx%d", info.Width, info.Height)
	}
	if info.DurationSec < ttsRes.Duration.Seconds() {
		t.Fatalf("expected stretched video duration (%.2fs) >= voice duration (%.2fs)", info.DurationSec, ttsRes.Duration.Seconds())
	}
	if info.CoverPath == "" {
		t.Fatalf("expected cover.jpg generated for 16:9 compose")
	}
}
