package composer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"feelings-studio/internal/project"
	"feelings-studio/internal/prompting"
)

type ShotClip struct {
	ShotID      string
	VideoPath   string
	VoicePath   string // 可选
	AmbientPath string // 可选
	Narration   string
	DurationSec float64
}

type ComposeRequest struct {
	Title           string
	Subtitle        string // 可选：用于封面图副标题（如故事一句话母题或主题）
	AspectRatio     string // 9:16 | 16:9 | 3:4
	Clips           []ShotClip
	Style           *prompting.StylePreset
	Mix             project.AudioMixConfig
	OutputPath      string
	CoverOutputPath string // 可选：为空时默认在 OutputPath 同目录输出 cover.jpg
	WorkDir         string
	VideoPreset     string // 可选：默认 medium；动态预演样片可传 ultrafast
}

type VideoInfo struct {
	Path        string  `json:"path"`
	CoverPath   string  `json:"cover_path,omitempty"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	DurationSec float64 `json:"duration_sec"`
	VideoCodec  string  `json:"video_codec"`
	AudioCodec  string  `json:"audio_codec"`
	SizeBytes   int64   `json:"size_bytes"`
}

// Compose 执行 FFmpeg 感官精编流水线：
// 1) 探测每镜真实视频与 TTS 语音时长；若旁白超出视频时长，保持人声自然语速不变，对视频做轻柔慢放(≤1.25x)+尾帧定格延展
// 2) 生成对齐真实人声起止的电影感 ASS/SRT 字幕，并默认硬压进画面（支持 libass 或 macOS 原生透明 PNG 图层硬压）
// 3) 合成三轨音频：旁白轨 + 环境声景轨 + 怀旧配乐预设/自定义 BGM（含侧链闪避与响度统一）
// 4) 输出多画幅 H.264/AAC 高兼容性成片及自动排版封面图 cover.jpg。
func Compose(ctx context.Context, req ComposeRequest) (*VideoInfo, error) {
	if len(req.Clips) == 0 {
		return nil, fmt.Errorf("no shot clips provided for composition")
	}
	if req.WorkDir == "" {
		req.WorkDir = filepath.Join(filepath.Dir(req.OutputPath), ".compose_work")
	}
	if err := os.MkdirAll(req.WorkDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return nil, err
	}

	targetW, targetH := project.ResolveDimensions(req.AspectRatio)

	crossfade := req.Mix.CrossfadeSec
	if crossfade < 0 {
		crossfade = 0
	}
	if crossfade > 1.0 {
		crossfade = 1.0
	}

	// 1. 探测每镜真实视频时长与 TTS 旁白时长，智能计算延展后的镜头时长（保人声自然语速不截断、不叠词）
	rawVideoDurs := make([]float64, len(req.Clips))
	voiceDurs := make([]float64, len(req.Clips))
	durations := make([]float64, len(req.Clips))
	starts := make([]float64, len(req.Clips))
	const voiceLeadSec = 0.40
	const voiceTailSec = 0.42

	var totalDur float64
	for i, c := range req.Clips {
		vd := c.DurationSec
		if probed, err := probeMediaDuration(ctx, c.VideoPath); err == nil && probed > 0.25 {
			vd = probed
		}
		if vd <= 0 {
			vd = 5.0
		}
		rawVideoDurs[i] = vd

		if c.VoicePath != "" {
			if pv, err := probeMediaDuration(ctx, c.VoicePath); err == nil && pv > 0.15 {
				voiceDurs[i] = pv
			}
		}

		effDur := vd
		if voiceDurs[i] > 0 {
			cfMargin := 0.0
			if i < len(req.Clips)-1 {
				cfMargin = crossfade
			}
			minNeeded := voiceLeadSec + voiceDurs[i] + voiceTailSec + cfMargin
			if minNeeded > effDur {
				effDur = minNeeded
			}
		}
		durations[i] = effDur

		if i == 0 {
			starts[i] = 0
			totalDur = effDur
		} else {
			cf := crossfade
			if cf >= effDur*0.4 || cf >= durations[i-1]*0.4 {
				cf = 0.25
			}
			starts[i] = totalDur - cf
			totalDur = starts[i] + effDur
		}
	}

	// 2. 生成严丝合缝贴合人声时长的 ASS / SRT 独立字幕文件
	var subs []SubtitleItem
	for i, c := range req.Clips {
		lead := voiceLeadSec
		if durations[i] < 2.2 {
			lead = 0.20
		}
		shotSubs := BuildShotSubtitles(c.Narration, starts[i], durations[i], voiceDurs[i], lead)
		subs = append(subs, shotSubs...)
	}
	baseOut := strings.TrimSuffix(req.OutputPath, filepath.Ext(req.OutputPath))
	assPath := baseOut + ".ass"
	srtPath := baseOut + ".srt"
	if err := WriteNostalgicASS(assPath, req.Title, targetW, targetH, subs); err != nil {
		return nil, fmt.Errorf("write ass subtitles: %w", err)
	}
	if err := WriteSRT(srtPath, subs); err != nil {
		return nil, fmt.Errorf("write srt subtitles: %w", err)
	}

	// 3. 准备硬压字幕方案（优先 libass；无 libass 时使用系统原生字体 PNG 图层硬压）
	wantBurn := req.Mix.ShouldBurnSubtitles() && len(subs) > 0
	hasLibASS := wantBurn && hasFFmpegFilter(ctx, "ass")
	var pngOverlays []subtitleOverlay
	if wantBurn && !hasLibASS {
		if ovs, ovErr := renderSubtitleOverlays(ctx, req.WorkDir, targetW, targetH, subs); ovErr == nil && len(ovs) > 0 {
			pngOverlays = ovs
		}
	}
	burnedHard := wantBurn && (hasLibASS || len(pngOverlays) > 0)

	videoOnlyPath := filepath.Join(req.WorkDir, "video_styled.mp4")
	preset := req.VideoPreset
	if preset == "" {
		preset = "medium"
	}
	if err := renderVideoTrack(
		ctx,
		req.Clips,
		rawVideoDurs,
		durations,
		crossfade,
		targetW,
		targetH,
		req.Style,
		req.Mix.GrainStrength,
		assPath,
		hasLibASS,
		pngOverlays,
		preset,
		videoOnlyPath,
	); err != nil {
		return nil, err
	}

	// 4. 准备三轨音频：旁白轨 + 环境声景轨 + 怀旧 BGM（预设或本地上传，含侧链闪避）
	voiceTrackPath := filepath.Join(req.WorkDir, "track_voice.wav")
	hasVoice, err := buildDelayedAudioTrack(ctx, req.Clips, starts, totalDur, voiceLeadSec, true, voiceTrackPath)
	if err != nil {
		return nil, fmt.Errorf("build voice track: %w", err)
	}

	ambientTrackPath := filepath.Join(req.WorkDir, "track_ambient.wav")
	hasAmbient, err := buildDelayedAudioTrack(ctx, req.Clips, starts, totalDur, 0.0, false, ambientTrackPath)
	if err != nil {
		return nil, fmt.Errorf("build ambient track: %w", err)
	}

	bgmPath := strings.TrimSpace(req.Mix.BGMPath)
	hasBGM := true
	bgmPreset := strings.TrimSpace(req.Mix.BGMPreset)
	if bgmPreset == "none" {
		bgmPath = filepath.Join(req.WorkDir, "track_bgm_silent.wav")
		hasBGM, err = SynthesizeBGMPreset(ctx, "none", totalDur, bgmPath)
		if err != nil {
			return nil, err
		}
	} else if bgmPath != "" && (bgmPreset == "" || bgmPreset == "custom") {
		if _, statErr := os.Stat(bgmPath); statErr != nil {
			// 自定义 BGM 路径不存在时安全回退到温暖弦垫，避免合成中断
			bgmPath = filepath.Join(req.WorkDir, "track_bgm_pad.wav")
			hasBGM, err = SynthesizeBGMPreset(ctx, "warm_pad", totalDur, bgmPath)
			if err != nil {
				return nil, err
			}
		}
	} else {
		if bgmPreset == "" || bgmPreset == "custom" {
			bgmPreset = "warm_pad"
		}
		bgmPath = filepath.Join(req.WorkDir, "track_bgm_"+bgmPreset+".wav")
		hasBGM, err = SynthesizeBGMPreset(ctx, bgmPreset, totalDur, bgmPath)
		if err != nil {
			return nil, err
		}
	}

	mixedAudioPath := filepath.Join(req.WorkDir, "audio_mixed.wav")
	if err := mixThreeTracks(ctx, voiceTrackPath, hasVoice, ambientTrackPath, hasAmbient, bgmPath, hasBGM, req.Mix, totalDur, mixedAudioPath); err != nil {
		return nil, fmt.Errorf("mix three audio tracks: %w", err)
	}

	// 5. 封装成片 MP4（若未硬压字幕且存在台词，则附带 mov_text 软字幕轨；若已硬压字幕则不再挂重复软轨）
	tmpOut := req.OutputPath + ".tmp.mp4"
	muxArgs := []string{
		"-y",
		"-i", videoOnlyPath,
		"-i", mixedAudioPath,
	}
	if !burnedHard && len(subs) > 0 {
		muxArgs = append(muxArgs,
			"-i", srtPath,
			"-map", "0:v:0",
			"-map", "1:a:0",
			"-map", "2:s:0",
			"-c:v", "copy",
			"-c:a", "aac",
			"-b:a", "192k",
			"-c:s", "mov_text",
			"-metadata:s:s:0", "language=zho",
			"-shortest",
			"-movflags", "+faststart",
			tmpOut,
		)
	} else {
		muxArgs = append(muxArgs,
			"-map", "0:v:0",
			"-map", "1:a:0",
			"-c:v", "copy",
			"-c:a", "aac",
			"-b:a", "192k",
			"-shortest",
			"-movflags", "+faststart",
			tmpOut,
		)
	}
	if out, err := exec.CommandContext(ctx, "ffmpeg", muxArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg mux failed: %w: %s", err, trimErr(string(out)))
	}
	if err := os.Rename(tmpOut, req.OutputPath); err != nil {
		return nil, err
	}

	info, err := ProbeVideo(ctx, req.OutputPath)
	if err != nil {
		return nil, err
	}

	// 6. 自动抽取调色后高光帧压制带片名排版的封面海报 cover.jpg
	if req.Mix.ShouldGenerateCover() {
		coverPath := req.CoverOutputPath
		if coverPath == "" {
			coverPath = filepath.Join(filepath.Dir(req.OutputPath), "cover.jpg")
		}
		// 从不含硬字幕的调色主轨抽取高光帧，避免封面底部叠上台词字幕
		srcForCover := videoOnlyPath
		if burnedHard {
			if len(req.Clips) > 0 {
				midIdx := len(req.Clips) / 2
				if _, stErr := os.Stat(req.Clips[midIdx].VideoPath); stErr == nil {
					srcForCover = req.Clips[midIdx].VideoPath
				}
			}
		}
		srcDur := info.DurationSec
		if d, pErr := probeMediaDuration(ctx, srcForCover); pErr == nil && d > 0.3 {
			srcDur = d
		}
		if covErr := RenderCoverPoster(ctx, srcForCover, coverPath, req.Title, req.Subtitle, targetW, targetH, srcDur); covErr == nil {
			info.CoverPath = coverPath
		}
	}

	return info, nil
}

func renderVideoTrack(
	ctx context.Context,
	clips []ShotClip,
	rawDurations []float64,
	durations []float64,
	crossfade float64,
	targetW int,
	targetH int,
	style *prompting.StylePreset,
	grainOverride int,
	assPath string,
	burnASS bool,
	pngOverlays []subtitleOverlay,
	videoPreset string,
	outPath string,
) error {
	var args []string
	args = append(args, "-y")
	for _, c := range clips {
		args = append(args, "-i", c.VideoPath)
	}
	overlayStartIdx := len(clips)
	for _, ov := range pngOverlays {
		args = append(args, "-i", ov.PNGPath)
	}

	var filters []string
	for i := range clips {
		chain := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1", targetW, targetH, targetW, targetH)
		// 若旁白真实时长大于原视频时长：先做最高 1.25x 轻柔慢放，仍不足部分做尾帧柔和定格延展，绝不截断旁白
		rawD := rawDurations[i]
		effD := durations[i]
		if rawD > 0.25 && effD > rawD+0.06 {
			slowFactor := effD / rawD
			if slowFactor > 1.25 {
				slowFactor = 1.25
			}
			if slowFactor > 1.01 {
				chain += fmt.Sprintf(",setpts=%.4f*PTS", slowFactor)
			}
			stretched := rawD * slowFactor
			if effD > stretched+0.04 {
				chain += fmt.Sprintf(",tpad=stop_mode=clone:stop_duration=%.2f", effD-stretched)
			}
		}
		chain += ",fps=30,format=yuv420p"
		filters = append(filters, fmt.Sprintf("[%d:v]%s[v%d]", i, chain, i))
	}

	lastLabel := "v0"
	if len(clips) > 1 {
		cum := durations[0]
		for i := 1; i < len(clips); i++ {
			cf := crossfade
			if cf >= durations[i]*0.4 || cf >= durations[i-1]*0.4 {
				cf = 0.25
			}
			offset := cum - cf
			if offset < 0 {
				offset = 0
			}
			outLabel := fmt.Sprintf("x%d", i)
			if cf > 0.05 {
				filters = append(filters, fmt.Sprintf("[%s][v%d]xfade=transition=fade:duration=%.2f:offset=%.2f[%s]", lastLabel, i, cf, offset, outLabel))
			} else {
				filters = append(filters, fmt.Sprintf("[%s][v%d]concat=n=2:v=1:a=0[%s]", lastLabel, i, outLabel))
			}
			lastLabel = outLabel
			cum = offset + durations[i]
		}
	}

	// 应用视觉风格预设滤镜 + 片头淡入片尾淡出
	postChain := []string{}
	if style != nil && strings.TrimSpace(style.FFmpegFilter) != "" {
		f := style.FFmpegFilter
		if grainOverride >= 0 && strings.Contains(f, "noise=alls=") {
			f = replaceGrainStrength(f, grainOverride)
		}
		postChain = append(postChain, f)
	}
	totalEst := durations[0]
	for i := 1; i < len(durations); i++ {
		cf := crossfade
		if cf >= durations[i]*0.4 || cf >= durations[i-1]*0.4 {
			cf = 0.25
		}
		totalEst += durations[i] - cf
	}
	if totalEst > 1.5 {
		postChain = append(postChain, fmt.Sprintf("fade=t=in:st=0:d=0.45,fade=t=out:st=%.2f:d=0.75", totalEst-0.75))
	}
	if burnASS && assPath != "" {
		postChain = append(postChain, fmt.Sprintf("ass='%s'", escapeFFmpegFilterPath(assPath)))
	}
	postChain = append(postChain, "format=yuv420p")

	styledLabel := "vstyled"
	if len(pngOverlays) == 0 {
		styledLabel = "vout"
	}
	filters = append(filters, fmt.Sprintf("[%s]%s[%s]", lastLabel, strings.Join(postChain, ","), styledLabel))

	// 若使用透明 PNG 字幕层硬压，按时间轴逐条叠加到底部安全区
	curLabel := styledLabel
	for idx, ov := range pngOverlays {
		inIdx := overlayStartIdx + idx
		nextLabel := fmt.Sprintf("vsub%d", idx)
		if idx == len(pngOverlays)-1 {
			nextLabel = "vout"
		}
		filters = append(filters, fmt.Sprintf(
			"[%s][%d:v]overlay=x=(W-w)/2:y=H-h-%d:enable='between(t,%.2f,%.2f)'[%s]",
			curLabel, inIdx, ov.MarginV, ov.StartSec, ov.EndSec, nextLabel,
		))
		curLabel = nextLabel
	}

	args = append(args,
		"-filter_complex", strings.Join(filters, ";"),
		"-map", "[vout]",
		"-c:v", "libx264",
		"-preset", videoPreset,
		"-crf", "20",
		"-pix_fmt", "yuv420p",
		"-an",
		outPath,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg render video track failed: %w: %s", err, trimErr(string(out)))
	}
	return nil
}

func buildDelayedAudioTrack(
	ctx context.Context,
	clips []ShotClip,
	starts []float64,
	totalDur float64,
	extraLeadSec float64,
	isVoice bool,
	outPath string,
) (bool, error) {
	var args []string
	args = append(args, "-y", "-f", "lavfi", "-i", fmt.Sprintf("anullsrc=r=44100:cl=stereo:d=%.2f", totalDur))

	var filters []string
	var mixInputs []string
	mixInputs = append(mixInputs, "[0:a]")

	inputIdx := 1
	foundAny := false
	for i, c := range clips {
		p := c.AmbientPath
		if isVoice {
			p = c.VoicePath
		}
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		foundAny = true
		args = append(args, "-i", p)
		delayMs := int((starts[i] + extraLeadSec) * 1000)
		if delayMs < 0 {
			delayMs = 0
		}
		label := fmt.Sprintf("a%d", inputIdx)
		filters = append(filters, fmt.Sprintf("[%d:a]aformat=sample_rates=44100:channel_layouts=stereo,adelay=%d|%d[%s]", inputIdx, delayMs, delayMs, label))
		mixInputs = append(mixInputs, "["+label+"]")
		inputIdx++
	}

	if !foundAny {
		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-f", "lavfi", "-i", fmt.Sprintf("anullsrc=r=44100:cl=stereo:d=%.2f", totalDur), outPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return false, fmt.Errorf("generate silent track: %w: %s", err, trimErr(string(out)))
		}
		return false, nil
	}

	filters = append(filters, fmt.Sprintf("%samix=inputs=%d:duration=first:normalize=0[aout]", strings.Join(mixInputs, ""), len(mixInputs)))
	args = append(args,
		"-filter_complex", strings.Join(filters, ";"),
		"-map", "[aout]",
		"-t", fmt.Sprintf("%.2f", totalDur),
		"-ar", "44100", "-ac", "2",
		outPath,
	)
	if out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
		return false, fmt.Errorf("ffmpeg build audio track: %w: %s", err, trimErr(string(out)))
	}
	return true, nil
}

func mixThreeTracks(
	ctx context.Context,
	voicePath string, hasVoice bool,
	ambientPath string, hasAmbient bool,
	bgmPath string, hasBGM bool,
	mix project.AudioMixConfig,
	totalDur float64,
	outPath string,
) error {
	vVol := mix.VoiceVolume
	if vVol < 0 {
		vVol = 1.0
	}
	aVol := mix.AmbientVolume
	if aVol < 0 {
		aVol = 0.34
	}
	bVol := mix.BGMVolume
	if !hasBGM {
		bVol = 0
	} else if bVol < 0 {
		bVol = 0.22
	}

	fadeOut := totalDur - 1.0
	if fadeOut < 0.2 {
		fadeOut = 0.2
	}

	args := []string{
		"-y",
		"-i", voicePath,
		"-i", ambientPath,
		"-stream_loop", "-1", "-i", bgmPath,
	}

	var fc string
	if hasVoice && bVol > 0.005 {
		// 使用 sidechaincompress：当旁白响起时，BGM 自动柔和闪避压低，旁白结束后平缓抬升
		fc = fmt.Sprintf(
			"[0:a]volume=%.2f,asplit=2[v_main][v_sc];"+
				"[1:a]volume=%.2f[amb];"+
				"[2:a]atrim=0:%.2f,asetpts=PTS-STARTPTS,volume=%.2f[bgm_raw];"+
				"[bgm_raw][v_sc]sidechaincompress=threshold=0.03:ratio=4:attack=80:release=450[bgm_duck];"+
				"[v_main][amb][bgm_duck]amix=inputs=3:duration=first:normalize=0,afade=t=out:st=%.2f:d=1.0,alimiter=limit=0.95[aout]",
			vVol, aVol, totalDur, bVol, fadeOut,
		)
	} else if hasVoice {
		fc = fmt.Sprintf(
			"[0:a]volume=%.2f[v_main];"+
				"[1:a]volume=%.2f[amb];"+
				"[v_main][amb]amix=inputs=2:duration=first:normalize=0,afade=t=out:st=%.2f:d=1.0,alimiter=limit=0.95[aout]",
			vVol, aVol, fadeOut,
		)
	} else if hasAmbient && bVol > 0.005 {
		fc = fmt.Sprintf(
			"[1:a]volume=%.2f[amb];"+
				"[2:a]atrim=0:%.2f,asetpts=PTS-STARTPTS,volume=%.2f[bgm];"+
				"[amb][bgm]amix=inputs=2:duration=first:normalize=0,afade=t=out:st=%.2f:d=1.0,alimiter=limit=0.95[aout]",
			aVol, totalDur, bVol, fadeOut,
		)
	} else if hasAmbient {
		fc = fmt.Sprintf("[1:a]volume=%.2f,afade=t=out:st=%.2f:d=1.0[aout]", aVol, fadeOut)
	} else {
		fc = fmt.Sprintf(
			"[2:a]atrim=0:%.2f,asetpts=PTS-STARTPTS,volume=%.2f,afade=t=out:st=%.2f:d=1.0[aout]",
			totalDur, bVol, fadeOut,
		)
	}

	args = append(args,
		"-filter_complex", fc,
		"-map", "[aout]",
		"-t", fmt.Sprintf("%.2f", totalDur),
		"-ar", "44100", "-ac", "2",
		outPath,
	)
	if out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg mix audio failed: %w: %s", err, trimErr(string(out)))
	}
	return nil
}

// ProbeVideo 使用 ffprobe 读取输出视频的分辨率、时长与编解码元数据。
func ProbeVideo(ctx context.Context, path string) (*VideoInfo, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration:stream=codec_type,codec_name,width,height",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}
	var parsed struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}
	dur, _ := strconv.ParseFloat(strings.TrimSpace(parsed.Format.Duration), 64)
	info := &VideoInfo{
		Path:        path,
		DurationSec: dur,
		SizeBytes:   stat.Size(),
	}
	for _, s := range parsed.Streams {
		if s.CodecType == "video" {
			info.Width = s.Width
			info.Height = s.Height
			info.VideoCodec = s.CodecName
		} else if s.CodecType == "audio" {
			info.AudioCodec = s.CodecName
		}
	}
	coverCand := filepath.Join(filepath.Dir(path), "cover.jpg")
	if _, covErr := os.Stat(coverCand); covErr == nil {
		info.CoverPath = coverCand
	}
	return info, nil
}

func probeMediaDuration(ctx context.Context, path string) (float64, error) {
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path,
	).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

func hasFFmpegFilter(ctx context.Context, name string) bool {
	out, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-filters").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), " "+name+" ")
}

func escapeFFmpegFilterPath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.ReplaceAll(p, ":", `\:`)
	p = strings.ReplaceAll(p, "'", `\'`)
	return p
}

func replaceGrainStrength(filter string, strength int) string {
	idx := strings.Index(filter, "noise=alls=")
	if idx < 0 {
		return filter
	}
	start := idx + len("noise=alls=")
	end := start
	for end < len(filter) && filter[end] >= '0' && filter[end] <= '9' {
		end++
	}
	return filter[:start] + strconv.Itoa(strength) + filter[end:]
}

func trimErr(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return strings.Join(lines, " | ")
}
