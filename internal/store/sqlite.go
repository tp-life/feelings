package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

type JobKind string

const (
	KindT2I JobKind = "t2i"
	KindI2V JobKind = "i2v"
	KindTTS JobKind = "tts"
	KindSFX JobKind = "sfx"
)

type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusSubmitted JobStatus = "submitted"
	StatusSucceeded JobStatus = "succeeded"
	StatusFailed    JobStatus = "failed"
)

type Job struct {
	ID           int64
	ProjectID    string
	ShotID       string
	Kind         JobKind
	InputHash    string
	Status       JobStatus
	RemoteTaskID string
	OutputPath   string
	ErrorMsg     string
	Attempts     int
	UpdatedAt    time.Time
}

type Store struct {
	db *sql.DB
}

func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	// SQLite 单连接写入最稳妥，读并发由 WAL 保障
	db.SetMaxOpenConns(1)

	const pragmas = `
	PRAGMA journal_mode = WAL;
	PRAGMA busy_timeout = 5000;
	PRAGMA synchronous = NORMAL;`
	if _, err := db.Exec(pragmas); err != nil {
		_ = db.Close()
		return nil, err
	}

	const schema = `
	CREATE TABLE IF NOT EXISTS jobs (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id     TEXT NOT NULL,
		shot_id        TEXT NOT NULL,
		kind           TEXT NOT NULL,
		input_hash     TEXT NOT NULL,
		status         TEXT NOT NULL,
		remote_task_id TEXT NOT NULL DEFAULT '',
		output_path    TEXT NOT NULL DEFAULT '',
		error_msg      TEXT NOT NULL DEFAULT '',
		attempts       INTEGER NOT NULL DEFAULT 0,
		updated_at     DATETIME NOT NULL,
		UNIQUE(project_id, shot_id, kind)
	);`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ComputeHash 将模型名、提示词、参数等序列化为稳定指纹
func ComputeHash(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *Store) GetJob(ctx context.Context, projectID, shotID string, kind JobKind) (*Job, error) {
	var j Job
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, shot_id, kind, input_hash, status, remote_task_id, output_path, error_msg, attempts, updated_at
		FROM jobs WHERE project_id = ? AND shot_id = ? AND kind = ?`,
		projectID, shotID, kind,
	).Scan(&j.ID, &j.ProjectID, &j.ShotID, &j.Kind, &j.InputHash, &j.Status, &j.RemoteTaskID, &j.OutputPath, &j.ErrorMsg, &j.Attempts, &j.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// GetOrPrepareJob 支持断点续跑与单镜头重绘：
// 1) 若同参数已成功且本地产物文件存在，直接返回 succeeded；
// 2) 若同参数已提交远端（submitted），保留 remote_task_id 供继续轮询；
// 3) 若参数变化或强制重绘（force=true），重置为 pending。
func (s *Store) GetOrPrepareJob(ctx context.Context, projectID, shotID string, kind JobKind, inputHash string, force bool) (*Job, error) {
	j, err := s.GetJob(ctx, projectID, shotID, kind)
	if err == nil && !force && j.InputHash == inputHash {
		if j.Status == StatusSucceeded {
			if _, statErr := os.Stat(j.OutputPath); statErr == nil {
				return j, nil // 命中本地缓存
			}
		} else if j.Status == StatusSubmitted && j.RemoteTaskID != "" {
			return j, nil // 崩溃恢复：直接继续轮询远端任务
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO jobs (project_id, shot_id, kind, input_hash, status, remote_task_id, output_path, error_msg, attempts, updated_at)
		VALUES (?, ?, ?, ?, ?, '', '', '', 0, ?)
		ON CONFLICT(project_id, shot_id, kind) DO UPDATE SET
			input_hash = excluded.input_hash,
			status = excluded.status,
			remote_task_id = '',
			output_path = '',
			error_msg = '',
			attempts = 0,
			updated_at = excluded.updated_at`,
		projectID, shotID, kind, inputHash, StatusPending, now,
	)
	if err != nil {
		return nil, err
	}
	return s.GetJob(ctx, projectID, shotID, kind)
}

func (s *Store) MarkSubmitted(ctx context.Context, id int64, remoteID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = ?, remote_task_id = ?, attempts = attempts + 1, updated_at = ?
		WHERE id = ?`, StatusSubmitted, remoteID, time.Now().UTC(), id)
	return err
}

func (s *Store) MarkSucceeded(ctx context.Context, id int64, outputPath string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = ?, output_path = ?, error_msg = '', updated_at = ?
		WHERE id = ?`, StatusSucceeded, outputPath, time.Now().UTC(), id)
	return err
}

func (s *Store) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = ?, error_msg = ?, updated_at = ?
		WHERE id = ?`, StatusFailed, errMsg, time.Now().UTC(), id)
	return err
}

// ListProjectJobs 返回指定项目下所有分镜与任务类型的最新状态，供 CLI 和 Web 工作台展示。
func (s *Store) ListProjectJobs(ctx context.Context, projectID string) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, shot_id, kind, input_hash, status, remote_task_id, output_path, error_msg, attempts, updated_at
		FROM jobs WHERE project_id = ? ORDER BY shot_id ASC, kind ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.ProjectID, &j.ShotID, &j.Kind, &j.InputHash, &j.Status, &j.RemoteTaskID, &j.OutputPath, &j.ErrorMsg, &j.Attempts, &j.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// DeleteProjectJobs 清理指定项目在 SQLite 中的全部任务状态与缓存记录。
func (s *Store) DeleteProjectJobs(ctx context.Context, projectID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE project_id = ?`, projectID)
	return err
}
