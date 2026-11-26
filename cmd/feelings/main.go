package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"feelings-studio/internal/project"
	"feelings-studio/internal/prompting"
	"feelings-studio/internal/psychology"
	"feelings-studio/internal/server"
	"feelings-studio/internal/settings"
	"feelings-studio/internal/store"
	"feelings-studio/internal/studio"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	rootDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	svc, err := studio.Open(rootDir, "")
	if err != nil {
		log.Fatalf("init studio failed: %v", err)
	}
	defer svc.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "strategies":
		runStrategies(svc)
	case "providers":
		runProviders(svc)
	case "init":
		runInit(svc, args)
	case "plan":
		runPlan(ctx, svc, args)
	case "prompts":
		runPrompts(svc, args)
	case "render":
		runRender(ctx, svc, args)
	case "compose":
		runCompose(ctx, svc, args)
	case "auto":
		runAuto(ctx, svc, args)
	case "animatic":
		runAnimatic(ctx, svc, args)
	case "export":
		runExport(ctx, svc, args)
	case "serve":
		runServe(ctx, svc, args)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`feelings-studio — 社会心理学驱动的情怀 AI 视频生成工作台

用法:
  feelings strategies                                          列出内置社会心理学策略与视觉风格预设
  feelings providers                                           查看当前模型供应商配置与支持的国内/国外供应商列表
  feelings init --id <id> --track <track> [--style <style>] [--theme ...] [--import-dir .]
  feelings plan --project <id>                                 生成心理学剧本与双模式双语分镜提示词
  feelings prompts --project <id> [--mode director|concise] [--lang zh|en]
  feelings render --project <id> [--shot g5] [--kind i2v] [--force]
  feelings compose --project <id>                              通过 FFmpeg 精编输出 final.mp4 + cover.jpg
  feelings auto --project <id> [--skip-keyframes] [--force]    一键全自动出片（故事->分镜->资产->首帧->视频->混音硬字幕成片）
  feelings animatic --project <id>                             零视频API消耗极速合成动态故事板预演样片 animatic.mp4
  feelings export --project <id>                               一键打包剪映专业版草稿与分轨工程 ZIP 包
  feelings serve [--port 8080]                                 启动本地 Web 创作工作台（含模型设置页 & Prompt Studio）`)
}

func runProviders(svc *studio.Service) {
	provs := svc.GetProviders()
	fmt.Printf("配置文件: %s\n", settings.ConfigPath(svc.RootDir))
	fmt.Printf("当前启用: LLM=%s | T2I=%s | I2V=%s | TTS=%s | SFX=%s\n\n",
		provs.LLM.Name(), provs.T2I.Name(), provs.I2V.Name(), provs.TTS.Name(), provs.SFX.Name())
	fmt.Println("支持的模型供应商目录（可在 `feelings serve` 的「模型供应商设置」页直接可视化配置）：")
	for _, c := range settings.Catalog() {
		fmt.Printf("  [%-3s][%-8s] %-15s %s\n", strings.ToUpper(c.Stage), c.Region, c.ID, c.Name)
	}
}

func runStrategies(svc *studio.Service) {
	fmt.Println("=== 社会心理学赛道策略包 ===")
	for _, s := range svc.StrategyRepo.List() {
		fmt.Printf("\n[%s] %s\n  定位: %s\n  默认风格: %s\n  时代暗号: %s\n",
			s.ID, s.Name, s.Tagline, s.DefaultStyle, strings.Join(s.EraSymbols, "、"))
	}
	fmt.Println("\n=== 视觉风格预设 ===")
	for _, st := range svc.Compiler.ListStyles() {
		fmt.Printf("\n[%s] %s\n  说明: %s\n", st.ID, st.Name, st.Description)
	}
	fmt.Printf("\n=== 当前活跃 Provider（默认零成本 Mock + FFmpeg）===\n  LLM=%s | T2I=%s | I2V=%s | TTS=%s | SFX=%s\n",
		svc.Providers.LLM.Name(), svc.Providers.T2I.Name(), svc.Providers.I2V.Name(), svc.Providers.TTS.Name(), svc.Providers.SFX.Name())
}

func runInit(svc *studio.Service, args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	id := fs.String("id", "", "项目唯一 ID")
	title := fs.String("title", "", "项目标题")
	theme := fs.String("theme", "", "创作主题")
	track := fs.String("track", string(psychology.TrackChildhood90s), "心理学赛道: childhood_90s | family_bond | urban_sanctuary")
	style := fs.String("style", "", "视觉风格预设: film_90s | retro_dv | cinematic_heal")
	lang := fs.String("lang", "zh", "提示词默认语言: zh | en")
	aspect := fs.String("aspect", "9:16", "画幅比例: 9:16 | 16:9 | 3:4")
	voice := fs.String("voice", "warm-male", "默认旁白声线: warm-male | healing-female | childhood-boy | calm-senior")
	shots := fs.Int("shots", 0, "分镜数量（0 表示由整体故事剧情自然决定）")
	importDir := fs.String("import-dir", "", "可选：导入包含 资产*.jpeg 与 g1~g10.jpeg 的目录")
	_ = fs.Parse(args)

	p, err := svc.InitProject(studio.InitOptions{
		ID:            *id,
		Title:         *title,
		Theme:         *theme,
		Track:         psychology.TrackID(*track),
		StylePreset:   *style,
		PromptLang:    prompting.Lang(*lang),
		AspectRatio:   *aspect,
		NarratorVoice: *voice,
		ShotCount:     *shots,
		ImportDir:     *importDir,
	})
	if err != nil {
		log.Fatalf("init failed: %v", err)
	}
	fmt.Printf("项目已初始化: %s\n  清单路径: %s\n  赛道: %s | 风格: %s | 已导入资产: %d | 已绑定首帧: %d\n",
		p.ID, project.ManifestPath(svc.RootDir, p.ID), p.Track, p.StylePreset, len(p.Assets), len(p.Shots))
}

func runPlan(ctx context.Context, svc *studio.Service, args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	projID := fs.String("project", "", "项目 ID")
	_ = fs.Parse(args)

	p, err := svc.PlanProject(ctx, *projID)
	if err != nil {
		log.Fatalf("plan failed: %v", err)
	}
	fmt.Printf("心理学分镜编排完成: %s（共 %d 镜）\n", p.ID, len(p.Shots))
	if p.CriticReport != nil {
		fmt.Printf("  PsychologyCritic 得分: %d/100 | 通过: %v | 平均留白比: %.0f%% | 时代暗号命中: %d\n",
			p.CriticReport.Score, p.CriticReport.Passed, p.CriticReport.AvgSilenceRatio*100, p.CriticReport.SymbolHitCount)
		for _, iss := range p.CriticReport.Issues {
			fmt.Printf("  - [%s] %s (%s): %s\n", iss.Severity, iss.ShotID, iss.Rule, iss.Message)
		}
	}
	for _, sh := range p.Shots {
		fmt.Printf("  %s [%s] (%ds) 声景:%s | 旁白: %s\n", sh.ShotID, sh.Phase, sh.DurationSec, sh.AmbientTag, sh.Narration)
	}
}

func runPrompts(svc *studio.Service, args []string) {
	fs := flag.NewFlagSet("prompts", flag.ExitOnError)
	projID := fs.String("project", "", "项目 ID")
	mode := fs.String("mode", "director", "提示词模式: director (工业级导演时序详版) | concise (精炼电影版)")
	lang := fs.String("lang", "zh", "展示语言: zh | en")
	shotFilter := fs.String("shot", "", "可选：只查看指定分镜（如 g5）")
	_ = fs.Parse(args)

	p, err := project.Load(svc.RootDir, *projID)
	if err != nil {
		log.Fatalf("load project: %v", err)
	}
	sys, shots, err := svc.CompileProjectPrompts(p)
	if err != nil {
		log.Fatalf("compile prompts: %v", err)
	}

	if *shotFilter == "" {
		fmt.Println("================ 1. 导演级系统提示词 (Director Storyboard System Prompt) ================")
		fmt.Println(sys.DirectorStoryboard)
		fmt.Println("\n================ 2. 首帧生图美术指导系统提示词 (T2I System Prompt) ================")
		fmt.Println(sys.T2ISystem)
		fmt.Println("\n================ 3. 图生视频动态导演系统提示词 (I2V System Prompt) ================")
		fmt.Println(sys.I2VSystem)
	}

	fmt.Printf("\n================ 4. 分镜编译提示词 [mode=%s | lang=%s] ================\n", *mode, strings.ToUpper(*lang))
	for _, c := range shots {
		if *shotFilter != "" && c.ShotID != *shotFilter {
			continue
		}
		t2i := c.T2IZH
		i2vCon := c.I2VZH
		i2vDir := c.I2VDirectorZH
		if prompting.Lang(*lang) == prompting.LangEN {
			t2i = c.T2IEN
			i2vCon = c.I2VEN
			i2vDir = c.I2VDirectorEN
		}
		fmt.Printf("\n--- Shot %s ---\n[T2I Prompt] %s\n[T2I Negative] %s\n", c.ShotID, t2i.Prompt, t2i.NegativePrompt)
		if *mode == string(prompting.ModeConcise) {
			fmt.Printf("[I2V Concise] %s\n[I2V Negative] %s\n", i2vCon.Prompt, i2vCon.NegativePrompt)
		} else {
			fmt.Printf("\n%s\n", i2vDir.FullSheet)
		}
	}
}

func runRender(ctx context.Context, svc *studio.Service, args []string) {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	projID := fs.String("project", "", "项目 ID")
	shotID := fs.String("shot", "", "可选：仅渲染/重绘指定镜头（如 g5）")
	kind := fs.String("kind", "", "可选：仅执行某类型任务（t2i | i2v | tts | sfx）")
	force := fs.Bool("force", false, "强制跳过缓存重绘")
	workers := fs.Int("workers", 3, "并发 Worker 数")
	_ = fs.Parse(args)

	ch := svc.Subscribe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range ch {
			fmt.Printf("[%s] %-4s %-5s -> %s (%s)\n", ev.Timestamp.Format("15:04:05"), ev.ShotID, ev.Kind, ev.Stage, ev.Message)
		}
	}()

	err := svc.RenderProject(ctx, *projID, studio.RenderOptions{
		ShotID:     *shotID,
		OnlyKind:   store.JobKind(*kind),
		ForceRetry: *force,
		Workers:    *workers,
	})
	svc.Unsubscribe(ch)
	<-done
	if err != nil {
		log.Fatalf("render failed: %v", err)
	}
	fmt.Println("渲染完成。")
}

func runCompose(ctx context.Context, svc *studio.Service, args []string) {
	fs := flag.NewFlagSet("compose", flag.ExitOnError)
	projID := fs.String("project", "", "项目 ID")
	_ = fs.Parse(args)

	info, err := svc.ComposeProject(ctx, *projID)
	if err != nil {
		log.Fatalf("compose failed: %v", err)
	}
	fmt.Printf("成片合成完成!\n  输出路径: %s\n  封面路径: %s\n  规格: %dx%d | 时长: %.2fs | 编码: %s/%s | 体积: %.2f MB\n",
		info.Path, info.CoverPath, info.Width, info.Height, info.DurationSec, info.VideoCodec, info.AudioCodec, float64(info.SizeBytes)/1024/1024)
}

func runAuto(ctx context.Context, svc *studio.Service, args []string) {
	fs := flag.NewFlagSet("auto", flag.ExitOnError)
	projID := fs.String("project", "", "项目 ID")
	from := fs.String("from", "", "可选：指定起始阶段 story|storyboard|assets_plan|assets_render|keyframes|video")
	skipKf := fs.Bool("skip-keyframes", false, "跳过分镜首帧定妆图，直接多参考图/文生视频")
	force := fs.Bool("force", false, "强制重新生成既有阶段产物")
	_ = fs.Parse(args)

	ch := svc.Subscribe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range ch {
			fmt.Printf("[%s] %-9s %-10s -> %s\n", ev.Timestamp.Format("15:04:05"), ev.Kind, ev.Stage, ev.Message)
		}
	}()

	var skipPtr *bool
	if *skipKf {
		v := true
		skipPtr = &v
	}
	err := svc.RunAutoPilot(ctx, *projID, studio.AutoPilotOptions{
		FromStage:       project.WorkflowStage(*from),
		SkipKeyframes:   skipPtr,
		ForceRegenerate: *force,
	})
	svc.Unsubscribe(ch)
	<-done
	if err != nil {
		log.Fatalf("autopilot failed: %v", err)
	}
	st := svc.GetAutoPilotStatus(*projID)
	fmt.Printf("一键全自动出片完成!\n  成片路径: %s\n  封面路径: %s\n", st.FinalVideoPath, st.CoverPath)
}

func runAnimatic(ctx context.Context, svc *studio.Service, args []string) {
	fs := flag.NewFlagSet("animatic", flag.ExitOnError)
	projID := fs.String("project", "", "项目 ID")
	_ = fs.Parse(args)

	info, err := svc.RenderAnimatic(ctx, *projID)
	if err != nil {
		log.Fatalf("animatic failed: %v", err)
	}
	fmt.Printf("动态预演样片合成完成（零视频API消耗）!\n  输出路径: %s\n  规格: %dx%d | 时长: %.2fs\n",
		info.Path, info.Width, info.Height, info.DurationSec)
}

func runExport(ctx context.Context, svc *studio.Service, args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	projID := fs.String("project", "", "项目 ID")
	_ = fs.Parse(args)

	zipPath, err := svc.BuildExportBundleZip(ctx, *projID)
	if err != nil {
		log.Fatalf("export failed: %v", err)
	}
	fmt.Printf("剪映草稿与分轨工程包已生成:\n  ZIP 路径: %s\n", zipPath)
}

func runServe(ctx context.Context, svc *studio.Service, args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 8080, "本地 Web 工作台监听端口")
	_ = fs.Parse(args)

	srv := server.New(svc)
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}

	go func() {
		<-ctx.Done()
		_ = httpSrv.Close()
	}()

	fmt.Printf("Feelings Studio 本地创作工作台已启动: http://%s\n按 Ctrl+C 退出。\n", addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
