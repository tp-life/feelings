package scheduler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"feelings-studio/internal/provider"
	"feelings-studio/internal/store"
)

type ProgressEvent struct {
	ProjectID  string        `json:"project_id"`
	ShotID     string        `json:"shot_id"`
	Kind       store.JobKind `json:"kind"`
	Stage      string        `json:"stage"` // cached | submitted | running | succeeded | failed | skipped
	Message    string        `json:"message"`
	OutputPath string        `json:"output_path,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}

type ShotTask struct {
	ProjectID  string
	ShotID     string
	T2IReq     *provider.T2IRequest // 为空或首帧已存在时自动跳过 T2I
	I2VReq     provider.I2VSubmitRequest
	TTSReq     *provider.TTSRequest // 为空则跳过配音
	SFXReq     *provider.SFXRequest // 为空则跳过环境声景
	OutputDir  string
	ForceRetry bool
	OnlyKind   store.JobKind // 可选：仅重绘指定类型（如仅重绘 i2v 或 tts）
	Locked     bool          // 锁定镜头：除非 ForceRetry=true，否则跳过重新生成
}

type Option func(*Scheduler)

func WithPollInterval(d time.Duration) Option {
	return func(s *Scheduler) { s.pollInterval = d }
}

func WithRetryBackoff(d time.Duration) Option {
	return func(s *Scheduler) { s.retryBackoff = d }
}

func WithHTTPClient(c *http.Client) Option {
	return func(s *Scheduler) { s.httpClient = c }
}

func WithLogger(l *slog.Logger) Option {
	return func(s *Scheduler) { s.logger = l }
}

func WithT2I(p provider.T2IProvider) Option {
	return func(s *Scheduler) { s.t2i = p }
}

func WithSFX(p provider.SFXProvider) Option {
	return func(s *Scheduler) { s.sfx = p }
}

func WithProgressHook(fn func(ProgressEvent)) Option {
	return func(s *Scheduler) { s.onProgress = fn }
}

// WithContinueOnError 开启后，单镜失败不会连坐取消同批次其他健康分镜，便于保留已付费成功产物并仅重试失败项。
func WithContinueOnError(v bool) Option {
	return func(s *Scheduler) { s.continueOnErr = v }
}

type Scheduler struct {
	store         *store.Store
	t2i           provider.T2IProvider
	i2v           provider.I2VProvider
	tts           provider.TTSProvider
	sfx           provider.SFXProvider
	submitLimit   *rate.Limiter
	maxWorkers    int
	pollInterval  time.Duration
	retryBackoff  time.Duration
	maxRetries    int
	continueOnErr bool
	httpClient    *http.Client
	logger        *slog.Logger
	onProgress    func(ProgressEvent)
}

func New(st *store.Store, i2v provider.I2VProvider, tts provider.TTSProvider, maxWorkers int, qps float64, opts ...Option) *Scheduler {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	if qps <= 0 {
		qps = 1
	}
	s := &Scheduler{
		store:        st,
		i2v:          i2v,
		tts:          tts,
		submitLimit:  rate.NewLimiter(rate.Limit(qps), 1),
		maxWorkers:   maxWorkers,
		pollInterval: 5 * time.Second,
		retryBackoff: 2 * time.Second,
		maxRetries:   3,
		httpClient:   http.DefaultClient,
		logger:       slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Scheduler) emit(ev ProgressEvent) {
	ev.Timestamp = time.Now().UTC()
	if s.onProgress != nil {
		s.onProgress(ev)
	}
}

// RunSingleShot 执行单个分镜的定向生成或重绘。
func (s *Scheduler) RunSingleShot(ctx context.Context, task ShotTask) error {
	return s.RunBatch(ctx, []ShotTask{task})
}

// RunBatch 并发调度一批分镜任务（可选 T2I -> I2V + TTS + SFX）。
func (s *Scheduler) RunBatch(ctx context.Context, tasks []ShotTask) error {
	if !s.continueOnErr {
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(s.maxWorkers)
		for _, t := range tasks {
			task := t
			g.Go(func() error {
				return s.runShot(gctx, task)
			})
		}
		return g.Wait()
	}

	sem := make(chan struct{}, s.maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, t := range tasks {
		if ctx.Err() != nil {
			mu.Lock()
			errs = append(errs, ctx.Err())
			mu.Unlock()
			break
		}
		task := t
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				errs = append(errs, ctx.Err())
				mu.Unlock()
				return
			}
			if err := s.runShot(ctx, task); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (s *Scheduler) runShot(ctx context.Context, task ShotTask) error {
	shouldRun := func(kind store.JobKind) bool {
		return task.OnlyKind == "" || task.OnlyKind == kind
	}

	// 1. 可选 T2I：若配置了 T2IReq 且本地尚无现成首帧（或强制重绘 T2I），先生成首帧图
	if shouldRun(store.KindT2I) && task.T2IReq != nil && s.t2i != nil {
		needT2I := task.I2VReq.FirstFramePath == "" || task.ForceRetry
		if !needT2I {
			if _, err := os.Stat(task.I2VReq.FirstFramePath); err != nil {
				needT2I = true
			}
		}
		if needT2I {
			framePath, err := s.runT2I(ctx, task)
			if err != nil {
				return fmt.Errorf("shot %s t2i failed: %w", task.ShotID, err)
			}
			task.I2VReq.FirstFramePath = framePath
		} else {
			s.emit(ProgressEvent{
				ProjectID:  task.ProjectID,
				ShotID:     task.ShotID,
				Kind:       store.KindT2I,
				Stage:      "skipped",
				Message:    "使用已导入首帧图，跳过 T2I",
				OutputPath: task.I2VReq.FirstFramePath,
			})
		}
	}

	// 2. I2V 图生视频
	if shouldRun(store.KindI2V) && s.i2v != nil {
		if err := s.runI2V(ctx, task); err != nil {
			return fmt.Errorf("shot %s i2v failed: %w", task.ShotID, err)
		}
	}

	// 3. TTS 情感旁白
	if shouldRun(store.KindTTS) && task.TTSReq != nil && s.tts != nil {
		if err := s.runTTS(ctx, task); err != nil {
			return fmt.Errorf("shot %s tts failed: %w", task.ShotID, err)
		}
	}

	// 4. SFX 环境声景
	if shouldRun(store.KindSFX) && task.SFXReq != nil && s.sfx != nil {
		if err := s.runSFX(ctx, task); err != nil {
			return fmt.Errorf("shot %s sfx failed: %w", task.ShotID, err)
		}
	}
	return nil
}

func (s *Scheduler) runT2I(ctx context.Context, task ShotTask) (string, error) {
	inputHash := store.ComputeHash(struct {
		Provider string
		Req      provider.T2IRequest
	}{s.t2i.Name(), *task.T2IReq})

	force := task.ForceRetry && (!task.Locked || task.OnlyKind == store.KindT2I)
	job, err := s.store.GetOrPrepareJob(ctx, task.ProjectID, task.ShotID, store.KindT2I, inputHash, force)
	if err != nil {
		return "", err
	}
	if job.Status == store.StatusSucceeded {
		s.logger.Info("命中缓存，跳过首帧生图", "shot", task.ShotID, "file", job.OutputPath)
		s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindT2I, Stage: "cached", Message: "命中缓存", OutputPath: job.OutputPath})
		return job.OutputPath, nil
	}

	s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindT2I, Stage: "running", Message: "正在生成首帧图"})
	outPath, err := s.withRetry(ctx, func() (string, error) {
		if err := s.submitLimit.Wait(ctx); err != nil {
			return "", err
		}
		return s.t2i.Generate(ctx, *task.T2IReq)
	})
	if err != nil {
		_ = s.store.MarkFailed(ctx, job.ID, err.Error())
		s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindT2I, Stage: "failed", Message: err.Error()})
		return "", err
	}
	if err := s.store.MarkSucceeded(ctx, job.ID, outPath); err != nil {
		return "", err
	}
	s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindT2I, Stage: "succeeded", Message: "首帧生成完成", OutputPath: outPath})
	return outPath, nil
}

func (s *Scheduler) runI2V(ctx context.Context, task ShotTask) error {
	inputHash := store.ComputeHash(struct {
		Provider string
		Req      provider.I2VSubmitRequest
	}{s.i2v.Name(), task.I2VReq})

	force := task.ForceRetry && (!task.Locked || task.OnlyKind == store.KindI2V)
	job, err := s.store.GetOrPrepareJob(ctx, task.ProjectID, task.ShotID, store.KindI2V, inputHash, force)
	if err != nil {
		return err
	}
	if job.Status == store.StatusSucceeded {
		s.logger.Info("命中缓存，跳过视频生成", "shot", task.ShotID, "file", job.OutputPath)
		s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindI2V, Stage: "cached", Message: "命中缓存", OutputPath: job.OutputPath})
		return nil
	}

	// 阶段 1：仅在尚未拿到 remote_task_id 时提交，防止重启重复扣费
	remoteID := job.RemoteTaskID
	if remoteID == "" {
		s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindI2V, Stage: "running", Message: "提交图生视频任务"})
		remoteID, err = s.withRetry(ctx, func() (string, error) {
			if err := s.submitLimit.Wait(ctx); err != nil {
				return "", err
			}
			return s.i2v.Submit(ctx, task.I2VReq)
		})
		if err != nil {
			_ = s.store.MarkFailed(ctx, job.ID, err.Error())
			s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindI2V, Stage: "failed", Message: err.Error()})
			return err
		}
		if err := s.store.MarkSubmitted(ctx, job.ID, remoteID); err != nil {
			return err
		}
		s.logger.Info("已提交图生视频任务", "shot", task.ShotID, "remote_id", remoteID)
		s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindI2V, Stage: "submitted", Message: "已提交 " + remoteID})
	} else {
		s.logger.Info("断点续跑：恢复轮询远端任务", "shot", task.ShotID, "remote_id", remoteID)
		s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindI2V, Stage: "running", Message: "断点续跑轮询 " + remoteID})
	}

	// 阶段 2：轮询直至完成或取消
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		status, err := s.i2v.Poll(ctx, remoteID)
		if err != nil {
			if !provider.IsRetryable(err) {
				_ = s.store.MarkFailed(ctx, job.ID, err.Error())
				s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindI2V, Stage: "failed", Message: err.Error()})
				return err
			}
			s.logger.Warn("轮询遇临时错误，稍后重试", "shot", task.ShotID, "err", err)
		} else {
			switch status.State {
			case provider.TaskSucceeded:
				dest := filepath.Join(task.OutputDir, task.ShotID+".mp4")
				if err := s.downloadFile(ctx, status.VideoURL, dest); err != nil {
					_ = s.store.MarkFailed(ctx, job.ID, err.Error())
					s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindI2V, Stage: "failed", Message: err.Error()})
					return err
				}
				if err := s.store.MarkSucceeded(ctx, job.ID, dest); err != nil {
					return err
				}
				s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindI2V, Stage: "succeeded", Message: "视频生成完成", OutputPath: dest})
				return nil
			case provider.TaskFailed:
				_ = s.store.MarkFailed(ctx, job.ID, status.ErrorMsg)
				s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindI2V, Stage: "failed", Message: status.ErrorMsg})
				return fmt.Errorf("remote task %s failed: %s", remoteID, status.ErrorMsg)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err() // 保持 submitted 状态，下次运行自动续跑
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) runTTS(ctx context.Context, task ShotTask) error {
	inputHash := store.ComputeHash(struct {
		Provider string
		Req      provider.TTSRequest
	}{s.tts.Name(), *task.TTSReq})

	force := task.ForceRetry && (!task.Locked || task.OnlyKind == store.KindTTS)
	job, err := s.store.GetOrPrepareJob(ctx, task.ProjectID, task.ShotID, store.KindTTS, inputHash, force)
	if err != nil {
		return err
	}
	if job.Status == store.StatusSucceeded {
		s.logger.Info("命中缓存，跳过配音生成", "shot", task.ShotID, "file", job.OutputPath)
		s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindTTS, Stage: "cached", Message: "命中缓存", OutputPath: job.OutputPath})
		return nil
	}

	s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindTTS, Stage: "running", Message: "正在合成情感旁白"})
	res, err := s.withRetry(ctx, func() (*provider.TTSResult, error) {
		if err := s.submitLimit.Wait(ctx); err != nil {
			return nil, err
		}
		return s.tts.Synthesize(ctx, *task.TTSReq)
	})
	if err != nil {
		_ = s.store.MarkFailed(ctx, job.ID, err.Error())
		s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindTTS, Stage: "failed", Message: err.Error()})
		return err
	}
	if err := s.store.MarkSucceeded(ctx, job.ID, res.AudioPath); err != nil {
		return err
	}
	s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindTTS, Stage: "succeeded", Message: "旁白合成完成", OutputPath: res.AudioPath})
	return nil
}

func (s *Scheduler) runSFX(ctx context.Context, task ShotTask) error {
	inputHash := store.ComputeHash(struct {
		Provider string
		Req      provider.SFXRequest
	}{s.sfx.Name(), *task.SFXReq})

	force := task.ForceRetry && (!task.Locked || task.OnlyKind == store.KindSFX)
	job, err := s.store.GetOrPrepareJob(ctx, task.ProjectID, task.ShotID, store.KindSFX, inputHash, force)
	if err != nil {
		return err
	}
	if job.Status == store.StatusSucceeded {
		s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindSFX, Stage: "cached", Message: "命中缓存", OutputPath: job.OutputPath})
		return nil
	}

	s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindSFX, Stage: "running", Message: "生成环境声景 " + task.SFXReq.Tag})
	outPath, err := s.withRetry(ctx, func() (string, error) {
		return s.sfx.Generate(ctx, *task.SFXReq)
	})
	if err != nil {
		_ = s.store.MarkFailed(ctx, job.ID, err.Error())
		s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindSFX, Stage: "failed", Message: err.Error()})
		return err
	}
	if err := s.store.MarkSucceeded(ctx, job.ID, outPath); err != nil {
		return err
	}
	s.emit(ProgressEvent{ProjectID: task.ProjectID, ShotID: task.ShotID, Kind: store.KindSFX, Stage: "succeeded", Message: "声景生成完成", OutputPath: outPath})
	return nil
}

func (s *Scheduler) withRetry[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var zero T
	backoff := s.retryBackoff
	for i := 0; i <= s.maxRetries; i++ {
		val, err := fn()
		if err == nil {
			return val, nil
		}
		if !provider.IsRetryable(err) || i == s.maxRetries {
			return zero, err
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
		}
	}
	return zero, context.Canceled
}

func (s *Scheduler) downloadFile(ctx context.Context, rawURL, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	// 支持本地 file:// 协议（供零费用本地 FFmpeg I2V 引擎无缝走同一原子落盘逻辑）
	if strings.HasPrefix(rawURL, "file://") {
		srcPath := strings.TrimPrefix(rawURL, "file://")
		return copyFileAtomic(srcPath, dest)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest) // 原子落盘，避免半截文件污染缓存
}

func copyFileAtomic(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dest + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
