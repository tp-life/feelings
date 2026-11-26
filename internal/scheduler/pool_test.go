package scheduler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"feelings-studio/internal/provider"
	"feelings-studio/internal/scheduler"
	"feelings-studio/internal/store"
)

type mockI2V struct {
	mu          sync.Mutex
	videoURL    string
	submitCount map[string]int // FirstFramePath -> submit times
	pollCount   map[string]int // remoteID -> poll times
	onFirstPoll func(remoteID string)
	seq         atomic.Int64
}

func newMockI2V(videoURL string) *mockI2V {
	return &mockI2V{
		videoURL:    videoURL,
		submitCount: make(map[string]int),
		pollCount:   make(map[string]int),
	}
}

func (m *mockI2V) Name() string { return "mock-i2v" }

func (m *mockI2V) Submit(_ context.Context, req provider.I2VSubmitRequest) (string, error) {
	m.mu.Lock()
	m.submitCount[req.FirstFramePath]++
	m.mu.Unlock()
	id := fmt.Sprintf("remote-%d", m.seq.Add(1))
	return id, nil
}

func (m *mockI2V) Poll(_ context.Context, remoteID string) (*provider.I2VStatus, error) {
	m.mu.Lock()
	m.pollCount[remoteID]++
	n := m.pollCount[remoteID]
	cb := m.onFirstPoll
	m.mu.Unlock()

	if n == 1 {
		if cb != nil {
			cb(remoteID)
		}
		return &provider.I2VStatus{
			RemoteID: remoteID,
			State:    provider.TaskRunning,
		}, nil
	}
	return &provider.I2VStatus{
		RemoteID: remoteID,
		State:    provider.TaskSucceeded,
		VideoURL: m.videoURL,
	}, nil
}

func (m *mockI2V) Submits(frame string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.submitCount[frame]
}

type mockTTS struct {
	mu    sync.Mutex
	calls map[string]int
}

func newMockTTS() *mockTTS {
	return &mockTTS{calls: make(map[string]int)}
}

func (m *mockTTS) Name() string { return "mock-tts" }

func (m *mockTTS) Synthesize(_ context.Context, req provider.TTSRequest) (*provider.TTSResult, error) {
	m.mu.Lock()
	m.calls[req.OutputPath]++
	m.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(req.OutputPath, []byte("mock-audio-"+req.Text), 0o644); err != nil {
		return nil, err
	}
	return &provider.TTSResult{
		AudioPath: req.OutputPath,
		Duration:  3 * time.Second,
	}, nil
}

func (m *mockTTS) Calls(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[path]
}

func TestResumeFromSubmittedAndForceRetrySingleShot(t *testing.T) {
	// 启动本地模拟视频下载服务
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake-mp4-bytes"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")
	outDir := filepath.Join(tmpDir, "out")

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	i2v := newMockI2V(srv.URL + "/clip.mp4")
	tts := newMockTTS()

	// 1. 模拟“提交后在轮询期间发生进程中断（Ctrl+C / 崩溃）”
	ctx1, cancel1 := context.WithCancel(context.Background())
	i2v.onFirstPoll = func(_ string) {
		cancel1()
	}

	sched := scheduler.New(
		st, i2v, tts, 2, 100,
		scheduler.WithPollInterval(10*time.Millisecond),
		scheduler.WithRetryBackoff(10*time.Millisecond),
		scheduler.WithHTTPClient(srv.Client()),
	)

	tasks := []scheduler.ShotTask{
		{
			ProjectID: "summer-1998",
			ShotID:    "g1",
			I2VReq: provider.I2VSubmitRequest{
				FirstFramePath: "g1.jpeg",
				Prompt:         "午后巷口小卖部，微风吹动门帘，胶片暖色调慢推镜头",
				AspectRatio:    "9:16",
				DurationSec:    5,
			},
			TTSReq: &provider.TTSRequest{
				Text:       "那年夏天的蝉鸣，总是从小卖部门口开始的。",
				VoiceID:    "warm-male",
				Emotion:    "nostalgic",
				Speed:      0.92,
				OutputPath: filepath.Join(outDir, "g1.mp3"),
			},
			OutputDir: outDir,
		},
		{
			ProjectID: "summer-1998",
			ShotID:    "g2",
			I2VReq: provider.I2VSubmitRequest{
				FirstFramePath: "g2.jpeg",
				Prompt:         "男孩握着五毛钱硬币跑进小卖部，阳光斜射灰尘颗粒",
				AspectRatio:    "9:16",
				DurationSec:    5,
			},
			TTSReq: &provider.TTSRequest{
				Text:       "兜里攥紧的五毛钱，能换来整个下午的快乐。",
				VoiceID:    "warm-male",
				Emotion:    "nostalgic",
				Speed:      0.92,
				OutputPath: filepath.Join(outDir, "g2.mp3"),
			},
			OutputDir: outDir,
		},
	}

	if err := sched.RunBatch(ctx1, tasks); err == nil {
		t.Fatalf("expected context canceled error on interrupted run")
	}

	// 确认已有至少一个镜头停留在 submitted 且保存了 remote_task_id
	g1Before, _ := st.GetJob(context.Background(), "summer-1998", "g1", store.KindI2V)
	g2Before, _ := st.GetJob(context.Background(), "summer-1998", "g2", store.KindI2V)
	if (g1Before == nil || g1Before.Status != store.StatusSubmitted) &&
		(g2Before == nil || g2Before.Status != store.StatusSubmitted) {
		t.Fatalf("expected at least one job in submitted state after interruption")
	}

	// 2. 断点续跑：取消中断钩子，重新运行 RunBatch
	i2v.onFirstPoll = nil
	if err := sched.RunBatch(context.Background(), tasks); err != nil {
		t.Fatalf("resume run failed: %v", err)
	}

	// 验证每个镜头都只调用了 1 次 Submit（已提交的镜头直接恢复轮询，没有重复扣费）
	if got := i2v.Submits("g1.jpeg"); got != 1 {
		t.Fatalf("g1 expected 1 submit (resumed without resubmit), got %d", got)
	}
	if got := i2v.Submits("g2.jpeg"); got != 1 {
		t.Fatalf("g2 expected 1 submit (resumed without resubmit), got %d", got)
	}
	if _, err := os.Stat(filepath.Join(outDir, "g1.mp4")); err != nil {
		t.Fatalf("expected g1.mp4 to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "g2.mp4")); err != nil {
		t.Fatalf("expected g2.mp4 to exist: %v", err)
	}

	// 3. 再次运行完整批次：全部命中缓存，零新增调用
	if err := sched.RunBatch(context.Background(), tasks); err != nil {
		t.Fatalf("cached run failed: %v", err)
	}
	if i2v.Submits("g1.jpeg") != 1 || i2v.Submits("g2.jpeg") != 1 {
		t.Fatalf("expected cache hit with no extra submits, got g1=%d g2=%d", i2v.Submits("g1.jpeg"), i2v.Submits("g2.jpeg"))
	}

	// 4. 单镜头定向重绘：只把 g2 的 ForceRetry 设为 true
	tasks[1].ForceRetry = true
	if err := sched.RunBatch(context.Background(), tasks); err != nil {
		t.Fatalf("force retry run failed: %v", err)
	}
	if got := i2v.Submits("g1.jpeg"); got != 1 {
		t.Fatalf("g1 should stay cached (1 submit), got %d", got)
	}
	if got := i2v.Submits("g2.jpeg"); got != 2 {
		t.Fatalf("g2 should be re-submitted (2 submits), got %d", got)
	}
	if got := tts.Calls(filepath.Join(outDir, "g1.mp3")); got != 1 {
		t.Fatalf("g1 tts should stay cached (1 call), got %d", got)
	}
	if got := tts.Calls(filepath.Join(outDir, "g2.mp3")); got != 2 {
		t.Fatalf("g2 tts should be re-synthesized (2 calls), got %d", got)
	}
}
