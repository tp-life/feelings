package composer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type subtitleOverlay struct {
	PNGPath  string
	StartSec float64
	EndSec   float64
	MarginV  int
}

type jxaSubtitleSpec struct {
	OutPath  string  `json:"out_path"`
	Text     string  `json:"text"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	FontSize float64 `json:"font_size"`
}

type jxaCoverSpec struct {
	SrcPath  string `json:"src_path"`
	OutPath  string `json:"out_path"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type jxaBatchInput struct {
	Subtitles []jxaSubtitleSpec `json:"subtitles,omitempty"`
	Cover     *jxaCoverSpec     `json:"cover,omitempty"`
}

const macOSAppKitRenderScript = `
ObjC.import('Foundation');
ObjC.import('AppKit');

function run(argv) {
  var specPath = argv[0];
  var nsData = $.NSData.dataWithContentsOfFile($(specPath));
  if (!nsData) throw new Error("cannot read spec file: " + specPath);
  var rawStr = $.NSString.alloc.initWithDataEncoding(nsData, $.NSUTF8StringEncoding).js;
  var spec = JSON.parse(rawStr);

  function pickFont(size, bold) {
    var names = bold
      ? ["PingFangSC-Semibold", "Songti SC Bold", "STHeitiSC-Medium", "HiraginoSansGB-W6"]
      : ["PingFangSC-Regular", "PingFangSC-Medium", "STHeitiSC-Light", "HiraginoSansGB-W3"];
    for (var i = 0; i < names.length; i++) {
      var f = $.NSFont.fontWithNameSize($(names[i]), size);
      if (f && !f.isNil()) return f;
    }
    return bold ? $.NSFont.boldSystemFontOfSize(size) : $.NSFont.systemFontOfSize(size);
  }

  function newCanvas(w, h) {
    var rep = $.NSBitmapImageRep.alloc.initWithBitmapDataPlanesPixelsWidePixelsHighBitsPerSampleSamplesPerPixelHasAlphaIsPlanarColorSpaceNameBytesPerRowBitsPerPixel(
      null, w, h, 8, 4, true, false, $.NSCalibratedRGBColorSpace, 0, 0
    );
    rep.setSize($.NSMakeSize(w, h));
    return rep;
  }

  if (spec.subtitles && spec.subtitles.length > 0) {
    for (var idx = 0; idx < spec.subtitles.length; idx++) {
      var item = spec.subtitles[idx];
      var w = item.width, h = item.height;
      var rep = newCanvas(w, h);
      $.NSGraphicsContext.saveGraphicsState();
      var ctx = $.NSGraphicsContext.graphicsContextWithBitmapImageRep(rep);
      $.NSGraphicsContext.setCurrentContext(ctx);

      $.NSColor.clearColor.set();
      $.NSRectFill($.NSMakeRect(0, 0, w, h));

      var font = pickFont(item.font_size, false);
      var para = $.NSMutableParagraphStyle.alloc.init;
      para.setAlignment($.NSTextAlignmentCenter);
      para.setLineSpacing(6.0);

      var nsText = $(item.text);
      var maxTextW = w * 0.84;
      var bound = nsText.boundingRectWithSizeOptionsAttributes(
        $.NSMakeSize(maxTextW, h * 0.9),
        $.NSStringDrawingUsesLineFragmentOrigin | $.NSStringDrawingUsesFontLeading,
        $({ NSFont: font, NSParagraphStyle: para })
      );

      var textW = Math.ceil(bound.size.width);
      var textH = Math.ceil(bound.size.height);
      var drawX = (w - maxTextW) / 2.0;
      var drawY = (h - textH) / 2.0;

      // 柔和电影暗底托衬（低透明度圆角暖黑胶囊，兼顾亮景与暗景可读性）
      var pillPadX = 26, pillPadY = 12;
      var pillW = Math.min(w * 0.90, textW + pillPadX * 2);
      var pillH = textH + pillPadY * 2;
      var pillX = (w - pillW) / 2.0;
      var pillY = drawY - pillPadY + 2;
      var pillPath = $.NSBezierPath.bezierPathWithRoundedRectXRadiusYRadius(
        $.NSMakeRect(pillX, pillY, pillW, pillH), 14, 14
      );
      $.NSColor.colorWithCalibratedRedGreenBlueAlpha(0.07, 0.06, 0.05, 0.42).setFill();
      pillPath.fill();

      // 四向柔化深棕描边 + 暖象牙白主字 (#FAF4E8)
      var strokeColor = $.NSColor.colorWithCalibratedRedGreenBlueAlpha(0.06, 0.05, 0.04, 0.90);
      var strokeAttrs = $({
        NSFont: font,
        NSForegroundColor: strokeColor,
        NSParagraphStyle: para,
        NSKern: 1.5
      });
      var offsets = [[-2, 0], [2, 0], [0, -2], [0, 2], [1.5, -1.5], [-1.5, -1.5]];
      for (var k = 0; k < offsets.length; k++) {
        nsText.drawInRectWithAttributes(
          $.NSMakeRect(drawX + offsets[k][0], drawY + offsets[k][1], maxTextW, textH + 12),
          strokeAttrs
        );
      }

      var shadow = $.NSShadow.alloc.init;
      shadow.setShadowOffset($.NSMakeSize(0, -2));
      shadow.setShadowBlurRadius(4.0);
      shadow.setShadowColor($.NSColor.colorWithCalibratedRedGreenBlueAlpha(0, 0, 0, 0.65));

      var fillColor = $.NSColor.colorWithCalibratedRedGreenBlueAlpha(0.98, 0.957, 0.91, 1.0);
      var mainAttrs = $({
        NSFont: font,
        NSForegroundColor: fillColor,
        NSParagraphStyle: para,
        NSShadow: shadow,
        NSKern: 1.5
      });
      nsText.drawInRectWithAttributes($.NSMakeRect(drawX, drawY, maxTextW, textH + 12), mainAttrs);

      $.NSGraphicsContext.restoreGraphicsState();
      var pngData = rep.representationUsingTypeProperties($.NSBitmapImageFileTypePNG, $({}));
      pngData.writeToFileAtomically($(item.out_path), true);
    }
  }

  if (spec.cover && spec.cover.src_path && spec.cover.out_path) {
    var cv = spec.cover;
    var cw = cv.width, ch = cv.height;
    var srcImg = $.NSImage.alloc.initWithContentsOfFile($(cv.src_path));
    if (srcImg && !srcImg.isNil()) {
      var rep2 = newCanvas(cw, ch);
      $.NSGraphicsContext.saveGraphicsState();
      var ctx2 = $.NSGraphicsContext.graphicsContextWithBitmapImageRep(rep2);
      $.NSGraphicsContext.setCurrentContext(ctx2);

      srcImg.drawInRectFromRectOperationFraction(
        $.NSMakeRect(0, 0, cw, ch),
        $.NSZeroRect,
        $.NSCompositingOperationSourceOver,
        1.0
      );

      // 底部电影海报渐变暗场遮罩
      var gradH = Math.round(ch * 0.46);
      var cTop = $.NSColor.colorWithCalibratedRedGreenBlueAlpha(0.06, 0.05, 0.04, 0.0);
      var cMid = $.NSColor.colorWithCalibratedRedGreenBlueAlpha(0.06, 0.05, 0.04, 0.58);
      var cBot = $.NSColor.colorWithCalibratedRedGreenBlueAlpha(0.05, 0.04, 0.03, 0.86);
      var grad = $.NSGradient.alloc.initWithColors($([cBot, cMid, cTop]));
      grad.drawInRectAngle($.NSMakeRect(0, 0, cw, gradH), 90.0);

      // 琥珀金装饰细线
      var lineW = Math.round(cw * 0.12);
      var lineY = Math.round(ch * 0.255);
      $.NSColor.colorWithCalibratedRedGreenBlueAlpha(0.878, 0.663, 0.427, 0.92).setFill();
      $.NSRectFill($.NSMakeRect((cw - lineW) / 2.0, lineY, lineW, 4));

      var pCenter = $.NSMutableParagraphStyle.alloc.init;
      pCenter.setAlignment($.NSTextAlignmentCenter);
      pCenter.setLineSpacing(8.0);

      var titleSize = cw >= 1600 ? 74 : 64;
      var titleFont = pickFont(titleSize, true);
      var tShadow = $.NSShadow.alloc.init;
      tShadow.setShadowOffset($.NSMakeSize(0, -3));
      tShadow.setShadowBlurRadius(8.0);
      tShadow.setShadowColor($.NSColor.colorWithCalibratedRedGreenBlueAlpha(0, 0, 0, 0.78));

      var titleAttrs = $({
        NSFont: titleFont,
        NSForegroundColor: $.NSColor.colorWithCalibratedRedGreenBlueAlpha(0.98, 0.955, 0.90, 1.0),
        NSParagraphStyle: pCenter,
        NSShadow: tShadow,
        NSKern: 3.0
      });
      var titleRect = $.NSMakeRect(cw * 0.09, ch * 0.135, cw * 0.82, ch * 0.11);
      $(cv.title || "").drawInRectWithAttributes(titleRect, titleAttrs);

      if (cv.subtitle) {
        var subSize = cw >= 1600 ? 30 : 28;
        var subFont = pickFont(subSize, false);
        var subAttrs = $({
          NSFont: subFont,
          NSForegroundColor: $.NSColor.colorWithCalibratedRedGreenBlueAlpha(0.88, 0.78, 0.64, 0.92),
          NSParagraphStyle: pCenter,
          NSShadow: tShadow,
          NSKern: 1.2
        });
        var subRect = $.NSMakeRect(cw * 0.11, ch * 0.055, cw * 0.78, ch * 0.085);
        $(cv.subtitle).drawInRectWithAttributes(subRect, subAttrs);
      }

      $.NSGraphicsContext.restoreGraphicsState();
      var jpgData = rep2.representationUsingTypeProperties($.NSBitmapImageFileTypeJPEG, $({ NSImageCompressionFactor: 0.92 }));
      jpgData.writeToFileAtomically($(cv.out_path), true);
    }
  }
  return "ok";
}
`

// renderSubtitleOverlays 在缺少 libass 的环境（如 macOS 默认 Homebrew FFmpeg）中，
// 使用原生系统字体批量渲染透明 PNG 电影感字幕层，供 FFmpeg 内置 overlay 滤镜硬压进成片。
func renderSubtitleOverlays(ctx context.Context, workDir string, videoW, videoH int, subs []SubtitleItem) ([]subtitleOverlay, error) {
	if len(subs) == 0 || runtime.GOOS != "darwin" {
		return nil, nil
	}
	if _, err := exec.LookPath("osascript"); err != nil {
		return nil, nil
	}

	overlayDir := filepath.Join(workDir, "sub_pngs")
	if err := os.MkdirAll(overlayDir, 0o755); err != nil {
		return nil, err
	}

	barH := 230
	fontSize := 42.0
	marginV := 155
	if videoW > videoH { // 16:9 横屏
		barH = 190
		fontSize = 40.0
		marginV = 64
	} else if videoH <= 1500 { // 3:4 小红书竖屏
		barH = 210
		fontSize = 40.0
		marginV = 110
	}

	var specs []jxaSubtitleSpec
	var overlays []subtitleOverlay
	for i, s := range subs {
		txt := strings.TrimSpace(s.Text)
		if txt == "" || s.End <= s.Start {
			continue
		}
		pngPath := filepath.Join(overlayDir, fmt.Sprintf("sub_%03d.png", i))
		specs = append(specs, jxaSubtitleSpec{
			OutPath:  pngPath,
			Text:     txt,
			Width:    videoW,
			Height:   barH,
			FontSize: fontSize,
		})
		overlays = append(overlays, subtitleOverlay{
			PNGPath:  pngPath,
			StartSec: s.Start.Seconds(),
			EndSec:   s.End.Seconds(),
			MarginV:  marginV,
		})
	}
	if len(specs) == 0 {
		return nil, nil
	}

	if err := runJXABatch(ctx, workDir, jxaBatchInput{Subtitles: specs}); err != nil {
		return nil, err
	}
	// 确认所有 PNG 均已落盘
	for _, ov := range overlays {
		if _, err := os.Stat(ov.PNGPath); err != nil {
			return nil, err
		}
	}
	return overlays, nil
}

// RenderCoverPoster 从成片抽取高光帧并压制带电影片名与情绪副标题的封面海报 cover.jpg。
func RenderCoverPoster(ctx context.Context, videoPath, coverOutPath, title, subtitle string, width, height int, videoDur float64) error {
	if err := os.MkdirAll(filepath.Dir(coverOutPath), 0o755); err != nil {
		return err
	}
	seekSec := videoDur * 0.36
	if seekSec < 0.8 {
		seekSec = 0.5
	}
	if videoDur > 1.2 && seekSec > videoDur-0.6 {
		seekSec = videoDur * 0.5
	}

	rawCover := coverOutPath + ".raw.jpg"
	defer os.Remove(rawCover)

	extractArgs := []string{
		"-y",
		"-ss", fmt.Sprintf("%.2f", seekSec),
		"-i", videoPath,
		"-frames:v", "1",
		"-q:v", "2",
		rawCover,
	}
	if out, err := exec.CommandContext(ctx, "ffmpeg", extractArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("extract cover frame failed: %w: %s", err, trimErr(string(out)))
	}

	cleanSub := truncateRunes(strings.TrimSpace(subtitle), 34)
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("osascript"); err == nil {
			workDir := filepath.Dir(coverOutPath)
			jxaErr := runJXABatch(ctx, workDir, jxaBatchInput{
				Cover: &jxaCoverSpec{
					SrcPath:  rawCover,
					OutPath:  coverOutPath,
					Title:    strings.TrimSpace(title),
					Subtitle: cleanSub,
					Width:    width,
					Height:   height,
				},
			})
			if jxaErr == nil {
				if _, statErr := os.Stat(coverOutPath); statErr == nil {
					return nil
				}
			}
		}
	}

	// 跨平台兜底：保留干净的高光调色帧作为封面
	data, err := os.ReadFile(rawCover)
	if err != nil {
		return err
	}
	return os.WriteFile(coverOutPath, data, 0o644)
}

func runJXABatch(ctx context.Context, workDir string, batch jxaBatchInput) error {
	specPath := filepath.Join(workDir, ".jxa_render_spec.json")
	scriptPath := filepath.Join(workDir, ".jxa_render.js")
	rawJSON, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	if err := os.WriteFile(specPath, rawJSON, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(scriptPath, []byte(macOSAppKitRenderScript), 0o644); err != nil {
		return err
	}
	defer os.Remove(specPath)
	defer os.Remove(scriptPath)

	cmd := exec.CommandContext(ctx, "osascript", "-l", "JavaScript", scriptPath, specPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript render failed: %w: %s", err, trimErr(string(out)))
	}
	return nil
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
