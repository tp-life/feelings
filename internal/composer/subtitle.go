package composer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type SubtitleItem struct {
	Text  string
	Start time.Duration
	End   time.Duration
}

// WriteNostalgicASS 生成克制电影感 ASS 字幕（自动适配 9:16 / 16:9 / 3:4 画幅与底部安全区）。
func WriteNostalgicASS(path, title string, playResX, playResY int, items []SubtitleItem) error {
	if playResX <= 0 || playResY <= 0 {
		playResX, playResY = 1080, 1920
	}
	fontSize := 44
	marginV := 220
	if playResX > playResY { // 16:9 横屏
		fontSize = 42
		marginV = 78
	} else if playResY <= 1500 { // 3:4 竖屏
		fontSize = 42
		marginV = 150
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("[Script Info]\n")
	fmt.Fprintf(&b, "Title: %s\n", title)
	b.WriteString("ScriptType: v4.00+\n")
	fmt.Fprintf(&b, "PlayResX: %d\n", playResX)
	fmt.Fprintf(&b, "PlayResY: %d\n", playResY)
	b.WriteString("ScaledBorderAndShadow: yes\n\n")

	// 暖象牙白 &H00E8F4FA (BGR顺序：接近 #FAF4E8)，柔和深棕黑阴影
	b.WriteString("[V4+ Styles]\n")
	b.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	fmt.Fprintf(&b, "Style: Nostalgic,PingFang SC,%d,&H00E8F4FA,&H00E8F4FA,&H80141414,&H96000000,0,0,0,0,100,100,2.5,0,1,1.6,1.2,2,84,84,%d,1\n\n", fontSize, marginV)

	b.WriteString("[Events]\n")
	b.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

	for _, it := range items {
		txt := escapeASSText(strings.TrimSpace(it.Text))
		if txt == "" || it.End <= it.Start {
			continue
		}
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Nostalgic,,0,0,0,,{\\fad(180,200)}%s\n",
			formatASSTime(it.Start),
			formatASSTime(it.End),
			txt,
		)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// BuildShotSubtitles 根据真实语音时长（若有）或画面有效时长，将单镜旁白智能切分为不拥挤、严丝合缝对齐人声的字幕条目。
func BuildShotSubtitles(narration string, shotStartSec, clipDurSec, voiceDurSec, leadSec float64) []SubtitleItem {
	text := strings.TrimSpace(narration)
	if text == "" {
		return nil
	}
	subStart := shotStartSec + leadSec
	var spanSec float64
	if voiceDurSec > 0.25 {
		// 严格对齐 TTS 真实发音时长（尾留 0.15s 自然余韵，绝不干挂或提前消失）
		spanSec = voiceDurSec + 0.15
		maxSpan := clipDurSec - leadSec - 0.08
		if maxSpan > 0.4 && spanSec > maxSpan {
			spanSec = maxSpan
		}
	} else {
		tail := 0.50
		if clipDurSec < 2.2 {
			tail = 0.20
		}
		spanSec = clipDurSec - leadSec - tail
	}
	if spanSec < 0.45 {
		spanSec = 0.45
	}

	// 长句按中文标点自然拆成多屏，单屏保持干净短句电影感
	clauses := splitLongClauses(text, 18)
	if len(clauses) <= 1 {
		clean := trimTrailingPunct(text)
		return []SubtitleItem{{
			Text:  clean,
			Start: time.Duration(subStart * float64(time.Second)),
			End:   time.Duration((subStart + spanSec) * float64(time.Second)),
		}}
	}

	totalRunes := 0
	for _, cl := range clauses {
		totalRunes += maxInt(1, utf8.RuneCountInString(cl))
	}

	var out []SubtitleItem
	cursor := subStart
	for i, cl := range clauses {
		frac := float64(maxInt(1, utf8.RuneCountInString(cl))) / float64(totalRunes)
		dur := spanSec * frac
		end := cursor + dur
		if i == len(clauses)-1 {
			end = subStart + spanSec
		}
		out = append(out, SubtitleItem{
			Text:  trimTrailingPunct(cl),
			Start: time.Duration(cursor * float64(time.Second)),
			End:   time.Duration(end * float64(time.Second)),
		})
		cursor = end
	}
	return out
}

func splitLongClauses(s string, thresholdRunes int) []string {
	if utf8.RuneCountInString(s) <= thresholdRunes {
		return []string{s}
	}
	var raw []string
	var cur strings.Builder
	for _, r := range s {
		cur.WriteRune(r)
		if r == '，' || r == '。' || r == '；' || r == '！' || r == '？' || r == ',' || r == ';' {
			part := strings.TrimSpace(cur.String())
			if part != "" {
				raw = append(raw, part)
			}
			cur.Reset()
		}
	}
	if rest := strings.TrimSpace(cur.String()); rest != "" {
		raw = append(raw, rest)
	}
	if len(raw) <= 1 {
		return []string{s}
	}
	// 合并过短碎句（小于 6 字的与相邻句合并），避免字幕闪跳
	var merged []string
	buf := ""
	for _, seg := range raw {
		if buf == "" {
			buf = seg
			continue
		}
		if utf8.RuneCountInString(buf) < 6 || utf8.RuneCountInString(buf+seg) <= thresholdRunes {
			buf += seg
		} else {
			merged = append(merged, buf)
			buf = seg
		}
	}
	if buf != "" {
		merged = append(merged, buf)
	}
	return merged
}

func trimTrailingPunct(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "，。；、,. ")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatASSTime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	cs := int(d / (10 * time.Millisecond))
	h := cs / 360000
	cs %= 360000
	m := cs / 6000
	cs %= 6000
	s := cs / 100
	cs %= 100
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, cs)
}

func escapeASSText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "{", "（")
	s = strings.ReplaceAll(s, "}", "）")
	s = strings.ReplaceAll(s, "\n", "\\N")
	return s
}

// WriteSRT 生成通用 SRT 字幕，便于剪映/播放器直接加载或在净版成片中封装 mov_text 软字幕。
func WriteSRT(path string, items []SubtitleItem) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	idx := 1
	for _, it := range items {
		txt := strings.TrimSpace(it.Text)
		if txt == "" || it.End <= it.Start {
			continue
		}
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", idx, formatSRTTime(it.Start), formatSRTTime(it.End), txt)
		idx++
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func formatSRTTime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	ms := int(d / time.Millisecond)
	h := ms / 3600000
	ms %= 3600000
	m := ms / 60000
	ms %= 60000
	s := ms / 1000
	ms %= 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}
