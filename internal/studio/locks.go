package studio

import (
	"sync"

	"feelings-studio/internal/project"
)

// lockProject 获取项目级互斥锁，串行化同一项目 project.yaml 的「读-改-写」，
// 避免长时间渲染/LLM 调用结束时用旧快照覆盖期间用户在界面上的修改。
// 注意：持锁期间不要调用外部模型接口，也不要调用其他会再次加锁的 Service 方法。
func (s *Service) lockProject(projectID string) func() {
	s.projLocksMu.Lock()
	if s.projLocks == nil {
		s.projLocks = make(map[string]*sync.Mutex)
	}
	mu, ok := s.projLocks[projectID]
	if !ok {
		mu = &sync.Mutex{}
		s.projLocks[projectID] = mu
	}
	s.projLocksMu.Unlock()

	mu.Lock()
	return mu.Unlock
}

// mutateProject 在项目锁内重新加载最新的 project.yaml，执行 fn 修改后落盘。
// fn 返回错误时不会保存。
func (s *Service) mutateProject(projectID string, fn func(p *project.Project) error) (*project.Project, error) {
	unlock := s.lockProject(projectID)
	defer unlock()

	p, err := project.Load(s.RootDir, projectID)
	if err != nil {
		return nil, err
	}
	if err := fn(p); err != nil {
		return nil, err
	}
	if err := project.Save(s.RootDir, p); err != nil {
		return nil, err
	}
	return p, nil
}

// SaveProject 在项目锁内整体保存前端提交的项目清单。
func (s *Service) SaveProject(p *project.Project) error {
	if err := project.ValidateID(p.ID); err != nil {
		return err
	}
	unlock := s.lockProject(p.ID)
	defer unlock()
	return project.Save(s.RootDir, p)
}
