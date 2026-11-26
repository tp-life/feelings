package studio

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"feelings-studio/internal/composer"
	"feelings-studio/internal/project"
	"feelings-studio/internal/provider"
	"feelings-studio/internal/provider/mock"
	"feelings-studio/internal/scheduler"
	"feelings-studio/internal/store"
)

// RenderAnimatic 生成「零视频 API 消耗」的动态故事板预演样片（animatic.mp4）：
// 复用真实 TTS 旁白与环境声景，对分镜首帧图（或绑定资产图/占位板）施加本地 FFmpeg 电影慢运镜，
// 并完整执行三轨闪避混音与硬压电影字幕，方便用户在花费真实 I2V 视频费用前先行审片验节奏。
func (s *Service) RenderAnimatic(ctx context.Context, projectID string) (*composer.VideoInfo, error) {
	p, err := project.Load(s.RootDir, projectID)
	if err != nil {
		return nil, err
	}
	if len(p.Shots) == 0 {
		return nil, fmt.Errorf("project %s has no storyboard shots yet", projectID)
	}
	style, err := s.Compiler.GetStyle(p.StylePreset)
	if err != nil {
		return nil, err
	}

	s.Broadcast(scheduler.ProgressEvent{
		ProjectID: projectID,
		Kind:      "animatic",
		Stage:     "running",
		Message:   "正在合成动态预演样片：准备旁白配音与环境声景（不消耗云端视频 API）...",
	})

	// 1. 先行生成/复用 TTS 旁白与 SFX 声景（写入正式缓存，后续跑真实视频直接命中缓存不重扣费）
	if err := s.RenderProject(ctx, projectID, RenderOptions{OnlyKind: store.KindTTS}); err != nil {
		return nil, fmt.Errorf("animatic tts failed: %w", err)
	}
	if err := s.RenderProject(ctx, projectID, RenderOptions{OnlyKind: store.KindSFX}); err != nil {
		return nil, fmt.Errorf("animatic sfx failed: %w", err)
	}

	outDir := project.OutputDir(s.RootDir, projectID)
	animWorkDir := filepath.Join(outDir, ".animatic_work")
	animFramesDir := filepath.Join(animWorkDir, "frames")
	animClipsDir := filepath.Join(animWorkDir, "clips")
	audioDir := filepath.Join(outDir, "audio")
	sfxDir := filepath.Join(outDir, "sfx")
	if err := os.MkdirAll(animFramesDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(animClipsDir, 0o755); err != nil {
		return nil, err
	}

	localT2I := mock.NewLocalT2I()
	localI2V := mock.NewLocalMotionI2V(animClipsDir)

	var clips []composer.ShotClip
	for i, sh := range p.Shots {
		frameSrc := strings.TrimSpace(sh.FirstFramePath)
		if frameSrc != "" {
			if _, statErr := os.Stat(frameSrc); statErr != nil {
				frameSrc = ""
			}
		}
		if frameSrc == "" {
			refs := p.ResolveShotReferenceImages(sh)
			if len(refs) > 0 {
				frameSrc = refs[0]
			}
		}
		if frameSrc == "" {
			plate := filepath.Join(animFramesDir, sh.ShotID+".jpeg")
			if _, statErr := os.Stat(plate); statErr != nil {
				genPath, genErr := localT2I.Generate(ctx, provider.T2IRequest{
					Prompt:      sh.StorySummary,
					AspectRatio: p.AspectRatio,
					OutputPath:  plate,
					Seed:        int64(300 + i*13),
				})
				if genErr != nil {
					return nil, genErr
				}
				frameSrc = genPath
			} else {
				frameSrc = plate
			}
		}

		dur := sh.DurationSec
		if dur <= 0 {
			dur = 5
		}
		s.Broadcast(scheduler.ProgressEvent{
			ProjectID: projectID,
			ShotID:    sh.ShotID,
			Kind:      "animatic",
			Stage:     "running",
			Message:   fmt.Sprintf("正在为分镜 %s 合成本地动态运镜预演片段...", sh.ShotID),
		})

		remoteID, subErr := localI2V.Submit(ctx, provider.I2VSubmitRequest{
			FirstFramePath: frameSrc,
			LastFramePath:  sh.LastFramePath,
			TransitionMode: sh.I2VSpec.TransitionMode,
			CameraMovement: sh.I2VSpec.CameraMovement,
			AspectRatio:    p.AspectRatio,
			DurationSec:    dur,
			Seed:           int64(500 + i),
		})
		if subErr != nil {
			return nil, subErr
		}
		st, pollErr := localI2V.Poll(ctx, remoteID)
		if pollErr != nil || st.State != provider.TaskSucceeded {
			return nil, fmt.Errorf("animatic clip %s failed", sh.ShotID)
		}
		clipPath := strings.TrimPrefix(st.VideoURL, "file://")

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
			VideoPath:   clipPath,
			VoicePath:   voicePath,
			AmbientPath: ambientPath,
			Narration:   sh.Narration,
			DurationSec: float64(dur),
		})
	}

	animMix := p.Mix
	noCover := false
	animMix.GenerateCover = &noCover

	animPath := filepath.Join(outDir, "animatic.mp4")
	info, err := composer.Compose(ctx, composer.ComposeRequest{
		Title:       p.Title + "（动态预演）",
		Subtitle:    p.Theme,
		AspectRatio: p.AspectRatio,
		Clips:       clips,
		Style:       style,
		Mix:         animMix,
		OutputPath:  animPath,
		WorkDir:     filepath.Join(animWorkDir, "compose"),
		VideoPreset: "ultrafast",
	})
	if err != nil {
		s.Broadcast(scheduler.ProgressEvent{
			ProjectID: projectID,
			Kind:      "animatic",
			Stage:     "failed",
			Message:   err.Error(),
		})
		return nil, err
	}

	s.Broadcast(scheduler.ProgressEvent{
		ProjectID:  projectID,
		Kind:       "animatic",
		Stage:      "succeeded",
		Message:    fmt.Sprintf("动态故事板预演样片已就绪（%.1fs，零视频API消耗）", info.DurationSec),
		OutputPath: animPath,
	})
	return info, nil
}

// BuildExportBundleZip 一键打包导出「剪映专业版草稿 + 分轨工程素材 ZIP 包」：
// 包含：final.mp4 / animatic.mp4 / cover.jpg、独立 SRT/ASS 字幕、按镜编号的视频/人声/音效/首帧/资产图、
// 完整双语导演提示词归档稿，以及可直接导入剪映桌面端的时间线草稿（draft_content.json & draft_meta_info.json）。
func (s *Service) BuildExportBundleZip(ctx context.Context, projectID string) (string, error) {
	p, err := project.Load(s.RootDir, projectID)
	if err != nil {
		return "", err
	}
	outDir := project.OutputDir(s.RootDir, projectID)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}

	// 1. 生成本地剪映草稿目录（projects/<id>/output/jianying_draft/<id>/）
	draftDir := filepath.Join(outDir, "jianying_draft", p.ID)
	if err := os.MkdirAll(draftDir, 0o755); err != nil {
		return "", err
	}
	draftContent, draftMeta, srtItems := s.buildJianyingDraftJSON(ctx, p, outDir)
	_ = os.WriteFile(filepath.Join(draftDir, "draft_content.json"), draftContent, 0o644)
	_ = os.WriteFile(filepath.Join(draftDir, "draft_meta_info.json"), draftMeta, 0o644)

	// 若尚未合成成片，也确保生成最新的独立 SRT/ASS 字幕文件供剪映直接拖入
	srtPath := filepath.Join(outDir, "final.srt")
	assPath := filepath.Join(outDir, "final.ass")
	if _, err := os.Stat(srtPath); err != nil && len(srtItems) > 0 {
		w, h := project.ResolveDimensions(p.AspectRatio)
		_ = composer.WriteSRT(srtPath, srtItems)
		_ = composer.WriteNostalgicASS(assPath, p.Title, w, h, srtItems)
	}

	zipPath := filepath.Join(outDir, p.ID+"_jianying_export.zip")
	tmpZip := zipPath + ".tmp"
	zf, err := os.Create(tmpZip)
	if err != nil {
		return "", err
	}
	zw := zip.NewWriter(zf)

	// 写入导入说明 README
	readme := fmt.Sprintf(`Feelings Studio · 分轨工程与剪映专业版导出包
================================================
项目名称：%s (%s)
创作主题：%s
画幅规格：%s
导出时间：%s

目录结构说明：
1. final.mp4 / cover.jpg         — 已调色、混音、压制字幕的完整成片与封面海报（可直接发布）
2. subtitles/final.srt & .ass    — 独立时间轴字幕文件（拖入剪映/PR/Final Cut 即可二次修改字体位置）
3. tracks/clips/                 — 按镜头顺序命名的纯净分镜视频片段
4. tracks/voice/                 — 按镜头顺序命名的独立情感旁白干声
5. tracks/sfx/                   — 按镜头顺序命名的独立环境声景轨
6. tracks/bgm/                   — 背景配乐轨
7. assets_and_frames/            — 全套角色/场景/道具定妆图与分镜首帧图
8. jianying_draft/%s/            — 剪映专业版草稿文件夹：
   将此文件夹整体复制到剪映草稿目录（macOS 默认路径：~/Movies/JianyingPro/User Data/Projects/com.lveditor.draft/）
   重新打开剪映即可看到按时间轴排好的视频轨、旁白轨、环境音轨与文本字幕轨！
`, p.Title, p.ID, p.Theme, p.AspectRatio, time.Now().Format("2006-01-02 15:04:05"), p.ID)
	_ = writeBytesToZip(zw, "README_后期与剪映导入说明.txt", []byte(readme))

	// 写入完整双语导演稿归档
	if sysPrompts, compiledShots, compErr := s.CompileProjectPrompts(p); compErr == nil {
		var sb strings.Builder
		fmt.Fprintf(&sb, "# %s (%s) — 完整导演提示词归档\n\n", p.Title, p.ID)
		if p.Story != nil {
			fmt.Fprintf(&sb, "## 整体故事大纲\n一句话母题：%s\n\n%s\n\n", p.Story.Logline, p.Story.Synopsis)
		}
		for _, cs := range compiledShots {
			fmt.Fprintf(&sb, "----------------------------------------\n### 分镜 %s\n[T2I 中文] %s\n\n%s\n\n",
				cs.ShotID, cs.T2IZH.Prompt, cs.I2VDirectorZH.FullSheet)
		}
		_ = sysPrompts
		_ = writeBytesToZip(zw, "prompts_完整导演提示词归档.md", []byte(sb.String()))
	}

	// 写入成片、预演片、封面与字幕
	_ = addFileToZipIfExists(zw, filepath.Join(outDir, "final.mp4"), "final.mp4")
	_ = addFileToZipIfExists(zw, filepath.Join(outDir, "animatic.mp4"), "animatic_动态预演.mp4")
	_ = addFileToZipIfExists(zw, filepath.Join(outDir, "cover.jpg"), "cover.jpg")
	_ = addFileToZipIfExists(zw, srtPath, "subtitles/final.srt")
	_ = addFileToZipIfExists(zw, assPath, "subtitles/final.ass")
	_ = addFileToZipIfExists(zw, ManifestPathForExport(s.RootDir, p.ID), "project.yaml")

	// 写入按序命名的分轨素材
	for i, sh := range p.Shots {
		prefix := fmt.Sprintf("%02d_%s", i+1, sh.ShotID)
		_ = addFileToZipIfExists(zw, filepath.Join(outDir, "clips", sh.ShotID+".mp4"), "tracks/clips/"+prefix+".mp4")
		_ = addFileToZipIfExists(zw, filepath.Join(outDir, "audio", sh.ShotID+".wav"), "tracks/voice/"+prefix+".wav")
		_ = addFileToZipIfExists(zw, filepath.Join(outDir, "sfx", sh.ShotID+".wav"), "tracks/sfx/"+prefix+".wav")
		if sh.FirstFramePath != "" {
			ext := filepath.Ext(sh.FirstFramePath)
			if ext == "" {
				ext = ".jpeg"
			}
			_ = addFileToZipIfExists(zw, sh.FirstFramePath, "assets_and_frames/frames/"+prefix+ext)
		}
	}
	_ = addFileToZipIfExists(zw, filepath.Join(outDir, ".compose_work", "track_voice.wav"), "tracks/voice/track_voice_full.wav")
	_ = addFileToZipIfExists(zw, filepath.Join(outDir, ".compose_work", "track_ambient.wav"), "tracks/sfx/track_ambient_full.wav")
	if p.Mix.BGMPath != "" {
		_ = addFileToZipIfExists(zw, p.Mix.BGMPath, "tracks/bgm/"+filepath.Base(p.Mix.BGMPath))
	} else {
		preset := p.Mix.BGMPreset
		if preset == "" {
			preset = "warm_pad"
		}
		_ = addFileToZipIfExists(zw, filepath.Join(outDir, ".compose_work", "track_bgm_"+preset+".wav"), "tracks/bgm/track_bgm_"+preset+".wav")
	}

	for i, a := range p.Assets {
		if a.ImagePath != "" {
			ext := filepath.Ext(a.ImagePath)
			if ext == "" {
				ext = ".jpeg"
			}
			_ = addFileToZipIfExists(zw, a.ImagePath, fmt.Sprintf("assets_and_frames/assets/%02d_%s%s", i+1, a.ID, ext))
		}
	}

	// 写入剪映专业版草稿 JSON
	_ = writeBytesToZip(zw, fmt.Sprintf("jianying_draft/%s/draft_content.json", p.ID), draftContent)
	_ = writeBytesToZip(zw, fmt.Sprintf("jianying_draft/%s/draft_meta_info.json", p.ID), draftMeta)

	if err := zw.Close(); err != nil {
		_ = zf.Close()
		return "", err
	}
	if err := zf.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpZip, zipPath); err != nil {
		return "", err
	}
	return zipPath, nil
}

func ManifestPathForExport(rootDir, projectID string) string {
	return project.ManifestPath(rootDir, projectID)
}

func (s *Service) buildJianyingDraftJSON(ctx context.Context, p *project.Project, outDir string) ([]byte, []byte, []composer.SubtitleItem) {
	w, h := project.ResolveDimensions(p.AspectRatio)
	const usPerSec = 1_000_000

	var videoMats []map[string]any
	var audioMats []map[string]any
	var textMats []map[string]any

	var videoSegs []map[string]any
	var voiceSegs []map[string]any
	var sfxSegs []map[string]any
	var textSegs []map[string]any
	var srtItems []composer.SubtitleItem

	var timelineCursorSec float64
	for i, sh := range p.Shots {
		clipPath := filepath.Join(outDir, "clips", sh.ShotID+".mp4")
		if _, err := os.Stat(clipPath); err != nil {
			clipPath = filepath.Join(outDir, ".animatic_work", "clips", sh.ShotID+".mp4")
		}
		voicePath := filepath.Join(outDir, "audio", sh.ShotID+".wav")
		sfxPath := filepath.Join(outDir, "sfx", sh.ShotID+".wav")

		durSec := float64(sh.DurationSec)
		if durSec <= 0 {
			durSec = 5.0
		}
		if d := probeSecQuick(ctx, clipPath); d > 0.25 {
			durSec = d
		}
		voiceDur := probeSecQuick(ctx, voicePath)
		if voiceDur > 0 && 0.40+voiceDur+0.42 > durSec {
			durSec = 0.40 + voiceDur + 0.42
		}

		startUS := int64(timelineCursorSec * usPerSec)
		durUS := int64(durSec * usPerSec)

		vidMatID := fmt.Sprintf("vid_%02d_%s", i+1, sh.ShotID)
		videoMats = append(videoMats, map[string]any{
			"id":            vidMatID,
			"type":          "video",
			"path":          clipPath,
			"material_name": sh.ShotID + ".mp4",
			"width":         w,
			"height":        h,
			"duration":      durUS,
		})
		videoSegs = append(videoSegs, map[string]any{
			"id":          "seg_" + vidMatID,
			"material_id": vidMatID,
			"target_timerange": map[string]int64{
				"start":    startUS,
				"duration": durUS,
			},
			"source_timerange": map[string]int64{
				"start":    0,
				"duration": durUS,
			},
		})

		if _, err := os.Stat(voicePath); err == nil && voiceDur > 0.1 {
			vMatID := fmt.Sprintf("voc_%02d_%s", i+1, sh.ShotID)
			vDurUS := int64(voiceDur * usPerSec)
			vLeadUS := int64(0.40 * usPerSec)
			audioMats = append(audioMats, map[string]any{
				"id":       vMatID,
				"type":     "extract_music",
				"path":     voicePath,
				"name":     sh.ShotID + "_voice.wav",
				"duration": vDurUS,
			})
			voiceSegs = append(voiceSegs, map[string]any{
				"id":          "seg_" + vMatID,
				"material_id": vMatID,
				"volume":      p.Mix.VoiceVolume,
				"target_timerange": map[string]int64{
					"start":    startUS + vLeadUS,
					"duration": vDurUS,
				},
				"source_timerange": map[string]int64{
					"start":    0,
					"duration": vDurUS,
				},
			})
		}

		if _, err := os.Stat(sfxPath); err == nil {
			sMatID := fmt.Sprintf("sfx_%02d_%s", i+1, sh.ShotID)
			audioMats = append(audioMats, map[string]any{
				"id":       sMatID,
				"type":     "sound",
				"path":     sfxPath,
				"name":     sh.ShotID + "_sfx.wav",
				"duration": durUS,
			})
			sfxSegs = append(sfxSegs, map[string]any{
				"id":          "seg_" + sMatID,
				"material_id": sMatID,
				"volume":      p.Mix.AmbientVolume,
				"target_timerange": map[string]int64{
					"start":    startUS,
					"duration": durUS,
				},
				"source_timerange": map[string]int64{
					"start":    0,
					"duration": durUS,
				},
			})
		}

		subs := composer.BuildShotSubtitles(sh.Narration, timelineCursorSec, durSec, voiceDur, 0.40)
		srtItems = append(srtItems, subs...)
		for subIdx, sub := range subs {
			tMatID := fmt.Sprintf("txt_%02d_%d", i+1, subIdx+1)
			tStartUS := sub.Start.Microseconds()
			tDurUS := (sub.End - sub.Start).Microseconds()
			textMats = append(textMats, map[string]any{
				"id":        tMatID,
				"type":      "subtitle",
				"content":   sub.Text,
				"font_size": 7.0,
			})
			textSegs = append(textSegs, map[string]any{
				"id":          "seg_" + tMatID,
				"material_id": tMatID,
				"target_timerange": map[string]int64{
					"start":    tStartUS,
					"duration": tDurUS,
				},
			})
		}

		timelineCursorSec += durSec
	}

	totalUS := int64(timelineCursorSec * usPerSec)
	draftContentObj := map[string]any{
		"id":       "feelings_" + p.ID,
		"name":     p.Title,
		"duration": totalUS,
		"fps":      30.0,
		"canvas_config": map[string]any{
			"width":  w,
			"height": h,
			"ratio":  strings.ReplaceAll(p.AspectRatio, ":", "_"),
		},
		"materials": map[string]any{
			"videos": videoMats,
			"audios": audioMats,
			"texts":  textMats,
		},
		"tracks": []map[string]any{
			{"id": "track_video_main", "type": "video", "segments": videoSegs},
			{"id": "track_audio_voice", "type": "audio", "segments": voiceSegs},
			{"id": "track_audio_sfx", "type": "audio", "segments": sfxSegs},
			{"id": "track_text_sub", "type": "text", "segments": textSegs},
		},
	}
	nowUS := time.Now().UnixMicro()
	draftMetaObj := map[string]any{
		"draft_id":           "feelings_" + p.ID,
		"draft_name":         p.Title,
		"draft_fold_path":    filepath.Join(outDir, "jianying_draft", p.ID),
		"draft_root_path":    filepath.Join(outDir, "jianying_draft"),
		"tm_draft_create":    nowUS,
		"tm_draft_modified":  nowUS,
		"tm_duration":        totalUS,
		"draft_is_invisible": false,
	}

	contentBytes, _ := json.MarshalIndent(draftContentObj, "", "  ")
	metaBytes, _ := json.MarshalIndent(draftMetaObj, "", "  ")
	return contentBytes, metaBytes, srtItems
}

func probeSecQuick(ctx context.Context, path string) float64 {
	if _, err := os.Stat(path); err != nil {
		return 0
	}
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path,
	).Output()
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return v
}

func writeBytesToZip(zw *zip.Writer, relName string, data []byte) error {
	w, err := zw.Create(relName)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func addFileToZipIfExists(zw *zip.Writer, srcPath, relName string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return err
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = relName
	hdr.Method = zip.Deflate
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}
