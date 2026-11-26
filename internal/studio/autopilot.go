package studio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"feelings-studio/internal/project"
	"feelings-studio/internal/scheduler"
	"feelings-studio/internal/store"
)

// AutoPilotOptions 控制「全自动一键出片」或「确认当前步后一键到底」的编排行为。
type AutoPilotOptions struct {
	FromStage       project.WorkflowStage `json:"from_stage"`       // 空/"auto" 表示按项目当前缺口自动续跑；也可指定 story/storyboard/assets_plan/assets_render/keyframes/video
	SkipKeyframes   *bool                 `json:"skip_keyframes"`   // nil 表示沿用项目设置（默认 false，即先生成首帧定妆图再图生视频）
	ForceRegenerate bool                  `json:"force_regenerate"` // 是否强制重跑已存在的阶段产物
}

// AutoPilotStatus 表示项目后台一键出片流水线的实时执行状态。
type AutoPilotStatus struct {
	ProjectID      string    `json:"project_id"`
	Running        bool      `json:"running"`
	State          string    `json:"state"`        // idle | running | succeeded | failed | cancelled
	CurrentStep    string    `json:"current_step"` // story | storyboard | assets_plan | assets_render | keyframes | video_render | compose | done
	StepIndex      int       `json:"step_index"`
	TotalSteps     int       `json:"total_steps"`
	Percent        int       `json:"percent"`
	Message        string    `json:"message"`
	Error          string    `json:"error,omitempty"`
	FinalVideoPath string    `json:"final_video_path,omitempty"`
	CoverPath      string    `json:"cover_path,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	FinishedAt     time.Time `json:"finished_at,omitempty"`
}

type autoRunEntry struct {
	status AutoPilotStatus
	cancel context.CancelFunc
}

func (s *Service) GetAutoPilotStatus(projectID string) AutoPilotStatus {
	s.autoMu.RLock()
	defer s.autoMu.RUnlock()
	if ent, ok := s.autoRuns[projectID]; ok {
		return ent.status
	}
	return AutoPilotStatus{
		ProjectID: projectID,
		Running:   false,
		State:     "idle",
	}
}

func (s *Service) CancelAutoPilot(projectID string) bool {
	s.autoMu.Lock()
	ent, ok := s.autoRuns[projectID]
	if !ok || !ent.status.Running {
		s.autoMu.Unlock()
		return false
	}
	cancel := ent.cancel
	ent.status.Message = "正在取消后台一键出片任务..."
	s.autoMu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.Broadcast(scheduler.ProgressEvent{
		ProjectID: projectID,
		Kind:      "autopilot",
		Stage:     "cancelled",
		Message:   "用户已请求取消一键出片流水线",
	})
	return true
}

// StartAutoPilotAsync 在服务端后台协程启动「一键出片」全自动流水线，不受浏览器刷新或单次 HTTP 超时影响。
func (s *Service) StartAutoPilotAsync(projectID string, opts AutoPilotOptions) (AutoPilotStatus, error) {
	if _, err := project.Load(s.RootDir, projectID); err != nil {
		return AutoPilotStatus{}, err
	}

	s.autoMu.Lock()
	if prev, ok := s.autoRuns[projectID]; ok && prev.status.Running {
		st := prev.status
		s.autoMu.Unlock()
		return st, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	initStatus := AutoPilotStatus{
		ProjectID:   projectID,
		Running:     true,
		State:       "running",
		CurrentStep: "init",
		StepIndex:   0,
		TotalSteps:  7,
		Percent:     2,
		Message:     "已启动后台一键出片流水线...",
		StartedAt:   time.Now().UTC(),
	}
	s.autoRuns[projectID] = &autoRunEntry{
		status: initStatus,
		cancel: cancel,
	}
	s.autoMu.Unlock()

	go func() {
		defer cancel()
		_ = s.RunAutoPilot(ctx, projectID, opts)
	}()

	return initStatus, nil
}

func (s *Service) updateAutoStatus(projectID, step string, stepIdx, totalSteps, pct int, msg string) {
	s.autoMu.Lock()
	if ent, ok := s.autoRuns[projectID]; ok {
		ent.status.CurrentStep = step
		ent.status.StepIndex = stepIdx
		ent.status.TotalSteps = totalSteps
		ent.status.Percent = pct
		ent.status.Message = msg
	}
	s.autoMu.Unlock()

	s.Broadcast(scheduler.ProgressEvent{
		ProjectID: projectID,
		Kind:      "autopilot",
		Stage:     "running",
		Message:   fmt.Sprintf("[%d/%d] %s", stepIdx, totalSteps, msg),
	})
}

func (s *Service) finishAutoStatus(projectID, state, finalPath, coverPath, msg, errMsg string) {
	now := time.Now().UTC()
	s.autoMu.Lock()
	ent, ok := s.autoRuns[projectID]
	if !ok {
		ent = &autoRunEntry{status: AutoPilotStatus{ProjectID: projectID, StartedAt: now}}
		s.autoRuns[projectID] = ent
	}
	ent.status.Running = false
	ent.status.State = state
	ent.status.FinishedAt = now
	if state == "succeeded" {
		ent.status.CurrentStep = "done"
		ent.status.Percent = 100
		ent.status.FinalVideoPath = finalPath
		ent.status.CoverPath = coverPath
	}
	if msg != "" {
		ent.status.Message = msg
	}
	ent.status.Error = errMsg
	s.autoMu.Unlock()

	s.Broadcast(scheduler.ProgressEvent{
		ProjectID:  projectID,
		Kind:       "autopilot",
		Stage:      state,
		Message:    msg,
		OutputPath: finalPath,
	})
}

// RunAutoPilot 同步执行「一键出片」流水线（供 CLI `feelings auto`、单元测试或后台协程复用）：
// 自动按序推进：1.故事大纲 -> 2.拆解故事板 -> 3.提取资产与导演详版 -> 4.生成资产定妆图 -> 5.生成分镜首帧图（默认开启，可按需跳过） -> 6.并发生成视频/旁白/环境音效 -> 7.FFmpeg 调色混音硬字幕与封面成片。
func (s *Service) RunAutoPilot(ctx context.Context, projectID string, opts AutoPilotOptions) error {
	s.autoMu.Lock()
	if prev, ok := s.autoRuns[projectID]; !ok || !prev.status.Running {
		s.autoRuns[projectID] = &autoRunEntry{
			status: AutoPilotStatus{
				ProjectID:  projectID,
				Running:    true,
				State:      "running",
				TotalSteps: 7,
				StartedAt:  time.Now().UTC(),
			},
		}
	}
	s.autoMu.Unlock()

	failWith := func(err error) error {
		if errors.Is(err, context.Canceled) {
			s.finishAutoStatus(projectID, "cancelled", "", "", "一键出片已取消（已完成的分镜与图片均已缓存保留，随时可续跑）", err.Error())
			return err
		}
		s.finishAutoStatus(projectID, "failed", "", "", "一键出片中断："+err.Error(), err.Error())
		return err
	}

	p, err := project.Load(s.RootDir, projectID)
	if err != nil {
		return failWith(err)
	}
	if opts.SkipKeyframes != nil {
		p.KeyframesSkipped = *opts.SkipKeyframes
		_ = project.Save(s.RootDir, p)
	}

	startRank := stageOrderRank(opts.FromStage)
	const totalSteps = 7

	// Step 1: 整体故事剧情
	hasStory := p.Story != nil && (strings.TrimSpace(p.Story.Synopsis) != "" || strings.TrimSpace(p.Story.Logline) != "")
	if (!hasStory && len(p.Shots) == 0) || (startRank == 1 && opts.ForceRegenerate) {
		if err := ctx.Err(); err != nil {
			return failWith(err)
		}
		s.updateAutoStatus(projectID, "story", 1, totalSteps, 8, "正在创作 Step 1 整体故事剧情与 4 幕情绪弧线...")
		p, err = s.PlanStory(ctx, projectID)
		if err != nil {
			return failWith(err)
		}
	}
	if !p.StoryConfirmed {
		tr := true
		p, _ = s.UpdateWorkflowStage(projectID, p.Stage, &tr, nil, nil, nil)
	}

	// Step 2: 按剧情拆解故事板分镜
	if len(p.Shots) == 0 || (startRank > 0 && startRank <= 2 && opts.ForceRegenerate) {
		if err := ctx.Err(); err != nil {
			return failWith(err)
		}
		s.updateAutoStatus(projectID, "storyboard", 2, totalSteps, 20, "正在按故事剧情自然决定镜头数并拆解故事板分镜...")
		p, err = s.PlanStoryboardOnly(ctx, projectID)
		if err != nil {
			return failWith(err)
		}
	}

	// Step 3: 根据故事板反推资产清单与导演时序详版
	needExtract := len(p.Assets) == 0 || !hasAnyDirectorSheet(p) || (startRank > 0 && startRank <= 3 && opts.ForceRegenerate)
	if needExtract {
		if err := ctx.Err(); err != nil {
			return failWith(err)
		}
		s.updateAutoStatus(projectID, "assets_plan", 3, totalSteps, 32, "正在从故事板提取角色/场景/道具资产与秒级导演详版...")
		p, err = s.ExtractAssetsFromStoryboard(ctx, projectID)
		if err != nil {
			return failWith(err)
		}
	}
	tr := true
	p, _ = s.UpdateWorkflowStage(projectID, project.StageAssetsRender, &tr, &tr, &tr, nil)

	// Step 4: 批量生成缺失的资产定妆图
	if len(p.Assets) > 0 && (startRank == 0 || startRank <= 4) {
		if err := ctx.Err(); err != nil {
			return failWith(err)
		}
		s.updateAutoStatus(projectID, "assets_render", 4, totalSteps, 45, "正在生成角色/场景/道具一致性定妆参考图...")
		p, err = s.RenderAssets(ctx, projectID, "", opts.ForceRegenerate && startRank <= 4)
		if err != nil {
			return failWith(err)
		}
	}

	// Step 5: 生成分镜首帧定妆图（默认开启以稳控构图与人物一致性；若用户勾选跳过首帧则直接进入多图参考生视频）
	p, err = project.Load(s.RootDir, projectID)
	if err != nil {
		return failWith(err)
	}
	if !p.KeyframesSkipped && (startRank == 0 || startRank <= 5) {
		if err := ctx.Err(); err != nil {
			return failWith(err)
		}
		_, _ = s.UpdateWorkflowStage(projectID, project.StageKeyframes, nil, nil, nil, nil)
		s.updateAutoStatus(projectID, "keyframes", 5, totalSteps, 60, "正在按每镜绑定资产并发生成分镜定妆首帧图 (T2I)...")
		if err := s.RenderProject(ctx, projectID, RenderOptions{
			OnlyKind:   store.KindT2I,
			ForceRetry: opts.ForceRegenerate && startRank <= 5,
		}); err != nil {
			return failWith(err)
		}
	}

	// Step 6: 并发生成全片视频镜头 (I2V) + 情感旁白 (TTS) + 环境声景 (SFX)
	if err := ctx.Err(); err != nil {
		return failWith(err)
	}
	_, _ = s.UpdateWorkflowStage(projectID, project.StageVideo, nil, nil, nil, nil)
	s.updateAutoStatus(projectID, "video_render", 6, totalSteps, 78, "正在并发生成全片分镜视频 (I2V)、情感旁白 (TTS) 与环境声景 (SFX)...")
	if err := s.RenderProject(ctx, projectID, RenderOptions{
		ForceRetry: opts.ForceRegenerate && startRank <= 6,
	}); err != nil {
		return failWith(err)
	}

	// Step 7: FFmpeg 胶片调色、语音时长智能对齐、硬压电影字幕、三轨闪避混音与封面图输出
	if err := ctx.Err(); err != nil {
		return failWith(err)
	}
	s.updateAutoStatus(projectID, "compose", 7, totalSteps, 92, "正在执行 FFmpeg 音画对齐、三轨混音、硬压电影字幕与封面合成...")
	info, err := s.ComposeProject(ctx, projectID)
	if err != nil {
		return failWith(err)
	}

	coverCandidate := filepath.Join(project.OutputDir(s.RootDir, projectID), "cover.jpg")
	coverOut := ""
	if _, statErr := os.Stat(coverCandidate); statErr == nil {
		coverOut = coverCandidate
	}
	s.finishAutoStatus(
		projectID,
		"succeeded",
		info.Path,
		coverOut,
		fmt.Sprintf("一键出片完成！成片 %.1fs (%dx%d)", info.DurationSec, info.Width, info.Height),
		"",
	)
	return nil
}

func stageOrderRank(st project.WorkflowStage) int {
	switch st {
	case project.StageStory:
		return 1
	case project.StageStoryboard:
		return 2
	case project.StageAssetsPlan:
		return 3
	case project.StageAssetsRender:
		return 4
	case project.StageKeyframes:
		return 5
	case project.StageVideo:
		return 6
	default:
		return 0
	}
}

func hasAnyDirectorSheet(p *project.Project) bool {
	for _, sh := range p.Shots {
		if len(sh.DirectorSheet.TimelineBeats) > 0 || len(sh.DirectorSheet.AssetBindings) > 0 {
			return true
		}
	}
	return false
}
