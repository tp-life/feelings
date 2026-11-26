package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"feelings-studio/internal/composer"
	"feelings-studio/internal/project"
	"feelings-studio/internal/prompting"
	"feelings-studio/internal/psychology"
	"feelings-studio/internal/settings"
	"feelings-studio/internal/store"
	"feelings-studio/internal/studio"
)

//go:embed web/*
var webFS embed.FS

type Server struct {
	svc *studio.Service
	mux *http.ServeMux
}

func New(svc *studio.Service) *Server {
	s := &Server{
		svc: svc,
		mux: http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	sub, _ := fs.Sub(webFS, "web")
	fileSrv := http.FileServer(http.FS(sub))

	s.mux.HandleFunc("GET /api/meta", s.handleMeta)
	s.mux.HandleFunc("GET /api/providers", s.handleGetProviders)
	s.mux.HandleFunc("PUT /api/providers", s.handleSaveProviders)
	s.mux.HandleFunc("POST /api/providers/test", s.handleTestProvider)
	s.mux.HandleFunc("GET /api/projects", s.handleListProjects)
	s.mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	s.mux.HandleFunc("GET /api/projects/{id}", s.handleGetProject)
	s.mux.HandleFunc("PUT /api/projects/{id}", s.handleUpdateProject)
	s.mux.HandleFunc("DELETE /api/projects/{id}", s.handleDeleteProject)
	s.mux.HandleFunc("POST /api/projects/{id}/plan", s.handlePlanProject)
	s.mux.HandleFunc("POST /api/projects/{id}/story/plan", s.handlePlanStory)
	s.mux.HandleFunc("POST /api/projects/{id}/storyboard/plan", s.handlePlanStoryboardOnly)
	s.mux.HandleFunc("POST /api/projects/{id}/assets/extract", s.handleExtractAssets)
	s.mux.HandleFunc("POST /api/projects/{id}/stage", s.handleUpdateStage)
	s.mux.HandleFunc("POST /api/projects/{id}/assets/render", s.handleRenderAssets)
	s.mux.HandleFunc("POST /api/projects/{id}/assets/{assetId}/upload", s.handleUploadAsset)
	s.mux.HandleFunc("POST /api/projects/{id}/shots/{shotId}/upload-frame", s.handleUploadShotFrame)
	s.mux.HandleFunc("POST /api/projects/{id}/bgm/upload", s.handleUploadBGM)
	s.mux.HandleFunc("GET /api/bgm/preview", s.handlePreviewBGM)
	s.mux.HandleFunc("POST /api/projects/{id}/render", s.handleRenderProject)
	s.mux.HandleFunc("POST /api/projects/{id}/compose", s.handleComposeProject)
	s.mux.HandleFunc("POST /api/projects/{id}/animatic", s.handleRenderAnimatic)
	s.mux.HandleFunc("GET /api/projects/{id}/autopilot", s.handleGetAutoPilot)
	s.mux.HandleFunc("POST /api/projects/{id}/autopilot", s.handleStartAutoPilot)
	s.mux.HandleFunc("POST /api/projects/{id}/autopilot/cancel", s.handleCancelAutoPilot)
	s.mux.HandleFunc("GET /api/projects/{id}/export-pack", s.handleExportPack)

	s.mux.HandleFunc("GET /api/templates", s.handleGetTemplates)
	s.mux.HandleFunc("PUT /api/templates", s.handleSaveTemplate)
	s.mux.HandleFunc("POST /api/templates/reset", s.handleResetTemplate)

	s.mux.HandleFunc("GET /api/events", s.handleEvents)
	s.mux.HandleFunc("GET /api/media", s.handleMedia)

	s.mux.Handle("/", fileSrv)
}

func (s *Server) handleMeta(w http.ResponseWriter, _ *http.Request) {
	provs := s.svc.GetProviders()
	writeJSON(w, http.StatusOK, map[string]any{
		"strategies":    s.svc.StrategyRepo.List(),
		"styles":        s.svc.Compiler.ListStyles(),
		"voices":        project.BuiltInVoices(),
		"bgm_presets":   project.BuiltInBGMPresets(),
		"aspect_ratios": []string{"9:16", "16:9", "3:4"},
		"llm_is_mock":   strings.HasPrefix(provs.LLM.Name(), "mock-"),
		"t2i_is_mock":   strings.HasPrefix(provs.T2I.Name(), "mock-"),
		"providers": map[string]string{
			"llm": provs.LLM.Name(),
			"t2i": provs.T2I.Name(),
			"i2v": provs.I2V.Name(),
			"tts": provs.TTS.Name(),
			"sfx": provs.SFX.Name(),
		},
	})
}

func (s *Server) handleGetProviders(w http.ResponseWriter, _ *http.Request) {
	provs := s.svc.GetProviders()
	cfg := s.svc.GetProviderSettings()
	writeJSON(w, http.StatusOK, map[string]any{
		"config_path": settings.ConfigPath(s.svc.RootDir),
		"catalog":     settings.Catalog(),
		"config":      cfg.MaskedCopy(),
		"runtime": map[string]string{
			"llm": provs.LLM.Name(),
			"t2i": provs.T2I.Name(),
			"i2v": provs.I2V.Name(),
			"tts": provs.TTS.Name(),
			"sfx": provs.SFX.Name(),
		},
	})
}

func (s *Server) handleSaveProviders(w http.ResponseWriter, r *http.Request) {
	var incoming settings.ProvidersConfig
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	saved, err := s.svc.UpdateProviderSettings(&incoming)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	provs := s.svc.GetProviders()
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "saved",
		"config": saved.MaskedCopy(),
		"runtime": map[string]string{
			"llm": provs.LLM.Name(),
			"t2i": provs.T2I.Name(),
			"i2v": provs.I2V.Name(),
			"tts": provs.TTS.Name(),
			"sfx": provs.SFX.Name(),
		},
	})
}

func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProviderID string `json:"provider_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg := s.svc.GetProviderSettings()
	res := settings.TestProvider(r.Context(), cfg, req.ProviderID)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleListProjects(w http.ResponseWriter, _ *http.Request) {
	ids, err := project.ListIDs(s.svc.RootDir)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	type item struct {
		ID          string                `json:"id"`
		Title       string                `json:"title"`
		Theme       string                `json:"theme"`
		Track       psychology.TrackID    `json:"track"`
		StylePreset string                `json:"style_preset"`
		Stage       project.WorkflowStage `json:"stage"`
		ShotCount   int                   `json:"shot_count"`
		AssetCount  int                   `json:"asset_count"`
		HasFinal    bool                  `json:"has_final"`
		UpdatedAt   string                `json:"updated_at"`
	}
	out := make([]item, 0, len(ids))
	for _, id := range ids {
		p, err := project.Load(s.svc.RootDir, id)
		if err != nil {
			continue
		}
		_, finalErr := os.Stat(filepath.Join(project.OutputDir(s.svc.RootDir, id), "final.mp4"))
		updatedAt := ""
		if !p.UpdatedAt.IsZero() {
			updatedAt = p.UpdatedAt.Format("2006-01-02 15:04")
		}
		out = append(out, item{
			ID:          p.ID,
			Title:       p.Title,
			Theme:       p.Theme,
			Track:       p.Track,
			StylePreset: p.StylePreset,
			Stage:       p.Stage,
			ShotCount:   len(p.Shots),
			AssetCount:  len(p.Assets),
			HasFinal:    finalErr == nil,
			UpdatedAt:   updatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID            string             `json:"id"`
		Title         string             `json:"title"`
		Theme         string             `json:"theme"`
		Track         psychology.TrackID `json:"track"`
		StylePreset   string             `json:"style_preset"`
		PromptLang    prompting.Lang     `json:"prompt_lang"`
		AspectRatio   string             `json:"aspect_ratio"`
		NarratorVoice string             `json:"narrator_voice"`
		NarratorSpeed float64            `json:"narrator_speed"`
		TargetPacing  string             `json:"target_pacing"`
		ShotCount     int                `json:"shot_count"`
		ImportDir     string             `json:"import_dir"`
		SkipKeyframes bool               `json:"skip_keyframes"`
		AutoPlan      bool               `json:"auto_plan"`
		OneClick      bool               `json:"one_click"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	p, err := s.svc.InitProject(studio.InitOptions{
		ID:            req.ID,
		Title:         req.Title,
		Theme:         req.Theme,
		Track:         req.Track,
		StylePreset:   req.StylePreset,
		PromptLang:    req.PromptLang,
		AspectRatio:   req.AspectRatio,
		NarratorVoice: req.NarratorVoice,
		NarratorSpeed: req.NarratorSpeed,
		TargetPacing:  req.TargetPacing,
		ShotCount:     req.ShotCount,
		ImportDir:     req.ImportDir,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.SkipKeyframes {
		skip := true
		p, _ = s.svc.UpdateWorkflowStage(p.ID, p.Stage, nil, nil, nil, &skip)
	}
	if req.OneClick {
		skip := req.SkipKeyframes
		if _, err := s.svc.StartAutoPilotAsync(p.ID, studio.AutoPilotOptions{
			FromStage:     project.StageStory,
			SkipKeyframes: &skip,
		}); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, p)
		return
	}
	if req.AutoPlan {
		// 新建无素材项目时，严格先生成 Step 1「整体故事剧情」，等待人工确认后再由剧情自动决定分镜数；
		// 若指定了本地导入目录，则一次性完成导入素材的编排。
		if strings.TrimSpace(req.ImportDir) == "" {
			p, err = s.svc.PlanStory(r.Context(), p.ID)
		} else {
			p, err = s.svc.PlanProject(r.Context(), p.ID)
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := project.Load(s.svc.RootDir, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	_, _ = s.svc.EvaluateProject(p)
	sysPrompts, compiledShots, err := s.svc.CompileProjectPrompts(p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	jobs, _ := s.svc.Store.ListProjectJobs(r.Context(), id)

	outDir := project.OutputDir(s.svc.RootDir, id)
	finalPath := filepath.Join(outDir, "final.mp4")
	var finalInfo *composer.VideoInfo
	if info, err := composer.ProbeVideo(r.Context(), finalPath); err == nil {
		finalInfo = info
	}
	animPath := filepath.Join(outDir, "animatic.mp4")
	var animInfo *composer.VideoInfo
	if info, err := composer.ProbeVideo(r.Context(), animPath); err == nil {
		animInfo = info
	}
	coverPath := ""
	if _, err := os.Stat(filepath.Join(outDir, "cover.jpg")); err == nil {
		coverPath = filepath.Join(outDir, "cover.jpg")
	}

	clipPaths := make(map[string]string, len(p.Shots))
	voicePaths := make(map[string]string, len(p.Shots))
	for _, sh := range p.Shots {
		cp := filepath.Join(outDir, "clips", sh.ShotID+".mp4")
		if _, err := os.Stat(cp); err == nil {
			clipPaths[sh.ShotID] = cp
		}
		vp := filepath.Join(outDir, "audio", sh.ShotID+".wav")
		if _, err := os.Stat(vp); err == nil {
			voicePaths[sh.ShotID] = vp
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"project":          p,
		"system_prompts":   sysPrompts,
		"compiled_prompts": compiledShots,
		"jobs":             jobs,
		"clip_paths":       clipPaths,
		"voice_paths":      voicePaths,
		"final_video":      finalInfo,
		"animatic_video":   animInfo,
		"cover_path":       coverPath,
		"autopilot":        s.svc.GetAutoPilotStatus(id),
	})
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var updated project.Project
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	updated.ID = id
	if _, err := s.svc.EvaluateProject(&updated); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := project.Save(s.svc.RootDir, &updated); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	sysPrompts, compiledShots, _ := s.svc.CompileProjectPrompts(&updated)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":          &updated,
		"system_prompts":   sysPrompts,
		"compiled_prompts": compiledShots,
	})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.svc.DeleteProject(r.Context(), id); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
		"id":     id,
	})
}

func (s *Server) handlePlanProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.svc.PlanProject(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	sysPrompts, compiledShots, _ := s.svc.CompileProjectPrompts(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":          p,
		"system_prompts":   sysPrompts,
		"compiled_prompts": compiledShots,
	})
}

func (s *Server) handlePlanStory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// 若前端调整了主题或篇幅倾向，先落盘再重新生成故事
	if r.Body != nil {
		var incoming project.Project
		if err := json.NewDecoder(r.Body).Decode(&incoming); err == nil && incoming.Theme != "" {
			incoming.ID = id
			_ = project.Save(s.svc.RootDir, &incoming)
		}
	}
	p, err := s.svc.PlanStory(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	sysPrompts, compiledShots, _ := s.svc.CompileProjectPrompts(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":          p,
		"system_prompts":   sysPrompts,
		"compiled_prompts": compiledShots,
	})
}

func (s *Server) handlePlanStoryboardOnly(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Step 1 → Step 2：先保存用户刚刚在界面里修改确认后的整体故事剧情，再按剧情拆解分镜
	if r.Body != nil {
		var incoming project.Project
		if err := json.NewDecoder(r.Body).Decode(&incoming); err == nil && (incoming.Story != nil || incoming.Theme != "") {
			incoming.ID = id
			_ = project.Save(s.svc.RootDir, &incoming)
		}
	}
	p, err := s.svc.PlanStoryboardOnly(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	sysPrompts, compiledShots, _ := s.svc.CompileProjectPrompts(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":          p,
		"system_prompts":   sysPrompts,
		"compiled_prompts": compiledShots,
	})
}

func (s *Server) handleExtractAssets(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// 若前端携带了刚修改的故事板项目体，先落盘保存再提取资产
	if r.Body != nil {
		var incoming project.Project
		if err := json.NewDecoder(r.Body).Decode(&incoming); err == nil && len(incoming.Shots) > 0 {
			incoming.ID = id
			_, _ = s.svc.EvaluateProject(&incoming)
			_ = project.Save(s.svc.RootDir, &incoming)
		}
	}
	p, err := s.svc.ExtractAssetsFromStoryboard(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	sysPrompts, compiledShots, _ := s.svc.CompileProjectPrompts(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":          p,
		"system_prompts":   sysPrompts,
		"compiled_prompts": compiledShots,
	})
}

func (s *Server) handleUpdateStage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Stage               project.WorkflowStage `json:"stage"`
		StoryConfirmed      *bool                 `json:"story_confirmed"`
		StoryboardConfirmed *bool                 `json:"storyboard_confirmed"`
		AssetsConfirmed     *bool                 `json:"assets_confirmed"`
		KeyframesSkipped    *bool                 `json:"keyframes_skipped"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	p, err := s.svc.UpdateWorkflowStage(id, req.Stage, req.StoryConfirmed, req.StoryboardConfirmed, req.AssetsConfirmed, req.KeyframesSkipped)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	sysPrompts, compiledShots, _ := s.svc.CompileProjectPrompts(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":          p,
		"system_prompts":   sysPrompts,
		"compiled_prompts": compiledShots,
	})
}

func (s *Server) handleRenderAssets(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		AssetID string `json:"asset_id"`
		Force   bool   `json:"force"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	p, err := s.svc.RenderAssets(r.Context(), id, req.AssetID, req.Force)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	sysPrompts, compiledShots, _ := s.svc.CompileProjectPrompts(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":          p,
		"system_prompts":   sysPrompts,
		"compiled_prompts": compiledShots,
	})
}

func (s *Server) handleUploadAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	assetID := r.PathValue("assetId")
	ext, data, err := readUploadedImage(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	p, err := s.svc.SaveUploadedAssetImage(id, assetID, ext, data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	sysPrompts, compiledShots, _ := s.svc.CompileProjectPrompts(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":          p,
		"system_prompts":   sysPrompts,
		"compiled_prompts": compiledShots,
	})
}

func (s *Server) handleUploadShotFrame(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	shotID := r.PathValue("shotId")
	ext, data, err := readUploadedImage(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	p, err := s.svc.SaveUploadedShotFrame(id, shotID, ext, data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	sysPrompts, compiledShots, _ := s.svc.CompileProjectPrompts(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":          p,
		"system_prompts":   sysPrompts,
		"compiled_prompts": compiledShots,
	})
}

func readUploadedImage(r *http.Request) (string, []byte, error) {
	const maxSize = 20 << 20
	if err := r.ParseMultipartForm(maxSize); err != nil {
		return "", nil, fmt.Errorf("parse multipart upload: %w", err)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return "", nil, fmt.Errorf("missing file field: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return "", nil, err
	}
	if len(data) == 0 {
		return "", nil, fmt.Errorf("empty file")
	}
	if len(data) > maxSize {
		return "", nil, fmt.Errorf("file exceeds 20MB limit")
	}
	return filepath.Ext(header.Filename), data, nil
}

func (s *Server) handleRenderProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		ShotID     string        `json:"shot_id"`
		OnlyKind   store.JobKind `json:"only_kind"`
		ForceRetry bool          `json:"force_retry"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := s.svc.RenderProject(r.Context(), id, studio.RenderOptions{
		ShotID:     req.ShotID,
		OnlyKind:   req.OnlyKind,
		ForceRetry: req.ForceRetry,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	jobs, _ := s.svc.Store.ListProjectJobs(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "jobs": jobs})
}

func (s *Server) handleComposeProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	info, err := s.svc.ComposeProject(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"final_video": info})
}

func (s *Server) handleUploadBGM(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	const maxAudioSize = 32 << 20
	if err := r.ParseMultipartForm(maxAudioSize); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("parse multipart audio upload: %w", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("missing file field: %w", err))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAudioSize+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if len(data) == 0 || len(data) > maxAudioSize {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("audio file must be between 1B and 32MB"))
		return
	}
	p, err := s.svc.SaveUploadedBGM(id, filepath.Ext(header.Filename), data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": p})
}

func (s *Server) handlePreviewBGM(w http.ResponseWriter, r *http.Request) {
	preset := strings.TrimSpace(r.URL.Query().Get("preset"))
	audioPath, err := s.svc.EnsureBGMPresetPreview(r.Context(), preset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	http.ServeFile(w, r, audioPath)
}

func (s *Server) handleRenderAnimatic(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	info, err := s.svc.RenderAnimatic(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"animatic_video": info})
}

func (s *Server) handleGetAutoPilot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	writeJSON(w, http.StatusOK, s.svc.GetAutoPilotStatus(id))
}

func (s *Server) handleStartAutoPilot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		FromStage       project.WorkflowStage `json:"from_stage"`
		SkipKeyframes   *bool                 `json:"skip_keyframes"`
		ForceRegenerate bool                  `json:"force_regenerate"`
		Sync            bool                  `json:"sync"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	opts := studio.AutoPilotOptions{
		FromStage:       req.FromStage,
		SkipKeyframes:   req.SkipKeyframes,
		ForceRegenerate: req.ForceRegenerate,
	}
	if req.Sync {
		if err := s.svc.RunAutoPilot(r.Context(), id, opts); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, s.svc.GetAutoPilotStatus(id))
		return
	}
	st, err := s.svc.StartAutoPilotAsync(id, opts)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, st)
}

func (s *Server) handleCancelAutoPilot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok := s.svc.CancelAutoPilot(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"cancelled": ok,
		"autopilot": s.svc.GetAutoPilotStatus(id),
	})
}

func (s *Server) handleExportPack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	zipPath, err := s.svc.BuildExportBundleZip(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(zipPath)))
	http.ServeFile(w, r, zipPath)
}

func (s *Server) handleGetTemplates(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	projectDir := ""
	if projectID != "" {
		projectDir = project.ProjectDir(s.svc.RootDir, projectID)
	}

	names := []string{
		prompting.TemplateStory,
		prompting.TemplateDirector,
		prompting.TemplateAssetExtraction,
		prompting.TemplateT2I,
		prompting.TemplateI2V,
	}
	out := make(map[string]string, len(names))
	overrides := make(map[string]bool, len(names))
	for _, n := range names {
		content, isOverride, err := s.svc.Compiler.ReadTemplateForProject(projectDir, n)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		out[n] = content
		overrides[n] = isOverride
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"templates":  out,
		"overrides":  overrides,
		"project_id": projectID,
	})
}

func (s *Server) handleSaveTemplate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		Content   string `json:"content"`
		ProjectID string `json:"project_id"`
		Scope     string `json:"scope"` // "project" | "global"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !isAllowedTemplate(req.Name) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid template name: %s", req.Name))
		return
	}
	if req.Scope == "project" && strings.TrimSpace(req.ProjectID) != "" {
		projectDir := project.ProjectDir(s.svc.RootDir, strings.TrimSpace(req.ProjectID))
		if err := s.svc.Compiler.SaveProjectTemplate(projectDir, req.Name, req.Content); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "scope": "project"})
		return
	}
	if err := s.svc.Compiler.SaveTemplate(req.Name, req.Content); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "scope": "global"})
}

func (s *Server) handleResetTemplate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		ProjectID string `json:"project_id"`
		Scope     string `json:"scope"` // "project" | "global"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !isAllowedTemplate(req.Name) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid template name: %s", req.Name))
		return
	}
	if req.Scope == "project" && strings.TrimSpace(req.ProjectID) != "" {
		projectDir := project.ProjectDir(s.svc.RootDir, strings.TrimSpace(req.ProjectID))
		content, err := s.svc.Compiler.ResetProjectTemplate(projectDir, req.Name)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"content": content, "scope": "project"})
		return
	}
	content, err := s.svc.Compiler.ResetTemplate(req.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content, "scope": "global"})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := s.svc.Subscribe()
	defer s.svc.Unsubscribe(ch)

	fmt.Fprintf(w, "event: ready\ndata: {\"ok\":true}\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	rawPath := r.URL.Query().Get("path")
	if rawPath == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	// 安全边界：仅允许访问工作区目录内部的图片/视频/音频文件
	rel, err := filepath.Rel(s.svc.RootDir, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.Error(w, "forbidden path outside workspace", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, absPath)
}

func isAllowedTemplate(name string) bool {
	return name == prompting.TemplateStory ||
		name == prompting.TemplateDirector ||
		name == prompting.TemplateAssetExtraction ||
		name == prompting.TemplateT2I ||
		name == prompting.TemplateI2V
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
