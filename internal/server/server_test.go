package server_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"feelings-studio/internal/server"
	"feelings-studio/internal/studio"
)

func doReq(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestWebStudioAndPromptStudioAPI(t *testing.T) {
	tmp := t.TempDir()
	svc, err := studio.Open(tmp, filepath.Join(tmp, "configs"))
	if err != nil {
		t.Fatalf("studio.Open: %v", err)
	}
	defer svc.Close()

	h := server.New(svc).Handler()

	// 1. 首页 HTML 正常返回且包含模型供应商设置页，且不包含破坏 onclick 属性的 JSON.stringify 写法
	w := doReq(h, http.MethodGet, "/", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "模型供应商设置") {
		t.Fatalf("GET /: status=%d", w.Code)
	}
	if strings.Contains(w.Body.String(), `onclick="copyText(${JSON.stringify`) {
		t.Fatalf("HTML must not embed JSON.stringify directly inside double-quoted onclick attributes")
	}

	// 2. Meta 接口返回三大策略包与三套风格预设
	w = doReq(h, http.MethodGet, "/api/meta", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/meta: status=%d", w.Code)
	}
	var meta struct {
		Strategies []any `json:"strategies"`
		Styles     []any `json:"styles"`
	}
	_ = json.NewDecoder(w.Body).Decode(&meta)
	if len(meta.Strategies) != 3 || len(meta.Styles) != 3 {
		t.Fatalf("expected 3 strategies and 3 styles, got %d and %d", len(meta.Strategies), len(meta.Styles))
	}

	// 3. 创建项目（不传 shot_count，走 Step 1 整体故事先行），再按剧情自动拆解故事板
	createBody := `{"id":"urban-demo","title":"深夜五分钟","theme":"下班后的老电扇与晚风","track":"urban_sanctuary","style_preset":"cinematic_heal","target_pacing":"auto","auto_plan":true}`
	w = doReq(h, http.MethodPost, "/api/projects", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /api/projects failed: status=%d body=%s", w.Code, w.Body.String())
	}
	// Step 1 → Step 2：确认整体故事并按剧情自然拆镜
	w = doReq(h, http.MethodPost, "/api/projects/urban-demo/storyboard/plan", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/projects/urban-demo/storyboard/plan failed: status=%d body=%s", w.Code, w.Body.String())
	}

	// 4. 获取项目详情，验证五层系统提示词与双模式编译提示词齐全
	w = doReq(h, http.MethodGet, "/api/projects/urban-demo", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/projects/urban-demo failed: status=%d", w.Code)
	}
	var detail struct {
		SystemPrompts struct {
			StoryOutline       string `json:"story_outline"`
			DirectorStoryboard string `json:"director_storyboard"`
			AssetExtraction    string `json:"asset_extraction"`
			T2ISystem          string `json:"t2i_system"`
			I2VSystem          string `json:"i2v_system"`
		} `json:"system_prompts"`
		CompiledPrompts []struct {
			ShotID        string `json:"shot_id"`
			I2VDirectorZH struct {
				GenerationPrompt string `json:"generation_prompt"`
				FullSheet        string `json:"full_sheet"`
			} `json:"i2v_director_zh"`
		} `json:"compiled_prompts"`
	}
	_ = json.NewDecoder(w.Body).Decode(&detail)
	if detail.SystemPrompts.StoryOutline == "" || detail.SystemPrompts.DirectorStoryboard == "" || detail.SystemPrompts.AssetExtraction == "" || detail.SystemPrompts.T2ISystem == "" || detail.SystemPrompts.I2VSystem == "" {
		t.Fatalf("expected all 5 system prompts populated")
	}
	if len(detail.CompiledPrompts) < 4 || !strings.Contains(detail.CompiledPrompts[0].I2VDirectorZH.GenerationPrompt, "时间段：") {
		t.Fatalf("expected story-driven compiled shots (>=4) with director sheet timeline beats, got %d", len(detail.CompiledPrompts))
	}

	// 5. Prompt Studio：读取并保存自定义系统提示词模板
	w = doReq(h, http.MethodGet, "/api/templates", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/templates failed: status=%d", w.Code)
	}
	var tmpls struct {
		Templates map[string]string `json:"templates"`
	}
	_ = json.NewDecoder(w.Body).Decode(&tmpls)
	orig := tmpls.Templates["prompts/i2v_system.tmpl"]
	if orig == "" {
		t.Fatalf("expected non-empty i2v template")
	}
	saveReq, _ := json.Marshal(map[string]string{
		"name":    "prompts/i2v_system.tmpl",
		"content": orig + "\n# custom studio note",
	})
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/templates", bytes.NewReader(saveReq))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/templates failed: status=%d body=%s", w.Code, w.Body.String())
	}

	// 6. 模型供应商设置页 API：获取目录 -> 保存脱敏密钥 -> 热生效切换 Wan -> 校验测试
	w = doReq(h, http.MethodGet, "/api/providers", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/providers failed: status=%d", w.Code)
	}
	var provResp struct {
		Catalog []struct {
			ID string `json:"id"`
		} `json:"catalog"`
	}
	_ = json.NewDecoder(w.Body).Decode(&provResp)
	if len(provResp.Catalog) < 15 {
		t.Fatalf("expected rich domestic+overseas provider catalog, got %d", len(provResp.Catalog))
	}

	saveProvBody := `{"active":{"llm":"mock","t2i":"mock","i2v":"wan","tts":"mock","sfx":"mock"},"profiles":{"wan":{"base_url":"https://dashscope.aliyuncs.com","model":"wan2.1-i2v-plus","api_key":"sk-test-wan-999999"}}}`
	w = doReq(h, http.MethodPut, "/api/providers", saveProvBody)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/providers failed: status=%d body=%s", w.Code, w.Body.String())
	}
	var putProvOut struct {
		Runtime map[string]string `json:"runtime"`
		Config  struct {
			Profiles map[string]struct {
				APIKey string `json:"api_key"`
			} `json:"profiles"`
		} `json:"config"`
	}
	_ = json.NewDecoder(w.Body).Decode(&putProvOut)
	if putProvOut.Runtime["i2v"] != "wan:wan2.1-i2v-plus" {
		t.Fatalf("expected hot-reloaded wan i2v runtime, got %q", putProvOut.Runtime["i2v"])
	}
	if !strings.Contains(putProvOut.Config.Profiles["wan"].APIKey, "****") {
		t.Fatalf("expected masked wan api key in response, got %q", putProvOut.Config.Profiles["wan"].APIKey)
	}

	// 7. 验证 6 步导演流水线：
	//    Step 1: 新建无图项目（不填分镜数）只先生成整体故事剧情（stage=story, shots=0, assets=0, story_confirmed=false），绝不串用“小卖部”
	newProjBody := `{"id":"grandma-fan","title":"外婆家的老蒲扇","theme":"","track":"childhood_90s","style_preset":"film_90s","target_pacing":"auto","auto_plan":true}`
	w = doReq(h, http.MethodPost, "/api/projects", newProjBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /api/projects grandma-fan failed: status=%d body=%s", w.Code, w.Body.String())
	}
	w = doReq(h, http.MethodGet, "/api/projects/grandma-fan", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/projects/grandma-fan failed: status=%d", w.Code)
	}
	var fanDetail struct {
		Project struct {
			Theme               string `json:"theme"`
			Stage               string `json:"stage"`
			StoryConfirmed      bool   `json:"story_confirmed"`
			StoryboardConfirmed bool   `json:"storyboard_confirmed"`
			Story               *struct {
				Logline  string `json:"logline"`
				Synopsis string `json:"synopsis"`
				Acts     []any  `json:"acts"`
			} `json:"story"`
			Assets []struct {
				ID string `json:"id"`
			} `json:"assets"`
			Shots []struct {
				ShotID string `json:"shot_id"`
			} `json:"shots"`
		} `json:"project"`
		SystemPrompts struct {
			StoryOutline       string `json:"story_outline"`
			DirectorStoryboard string `json:"director_storyboard"`
			AssetExtraction    string `json:"asset_extraction"`
		} `json:"system_prompts"`
	}
	_ = json.NewDecoder(w.Body).Decode(&fanDetail)
	if fanDetail.Project.Theme != "外婆家的老蒲扇" {
		t.Fatalf("expected empty theme to fall back to title '外婆家的老蒲扇', got %q", fanDetail.Project.Theme)
	}
	if fanDetail.Project.Stage != "story" || fanDetail.Project.StoryConfirmed {
		t.Fatalf("new project should start at unconfirmed story stage, got stage=%q confirmed=%v", fanDetail.Project.Stage, fanDetail.Project.StoryConfirmed)
	}
	if fanDetail.Project.Story == nil || fanDetail.Project.Story.Synopsis == "" || len(fanDetail.Project.Story.Acts) != 4 {
		t.Fatalf("expected full overall story with 4 acts in Step 1")
	}
	if len(fanDetail.Project.Shots) != 0 || len(fanDetail.Project.Assets) != 0 {
		t.Fatalf("Step 1 must NOT prematurely create shots or assets before story confirmation, got shots=%d assets=%d", len(fanDetail.Project.Shots), len(fanDetail.Project.Assets))
	}
	if strings.Contains(fanDetail.Project.Story.Synopsis, "小卖部") ||
		strings.Contains(fanDetail.SystemPrompts.DirectorStoryboard, "小卖部") {
		t.Fatalf("new project must NOT inherit corner-store (小卖部) content from previous demo")
	}

	//    Step 1 → Step 2: 人工确认整体故事后，由剧情自动决定分镜数并拆解故事板（不提前建资产）
	w = doReq(h, http.MethodPost, "/api/projects/grandma-fan/storyboard/plan", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/projects/grandma-fan/storyboard/plan failed: status=%d body=%s", w.Code, w.Body.String())
	}
	var splitResp struct {
		Project struct {
			Stage               string `json:"stage"`
			StoryConfirmed      bool   `json:"story_confirmed"`
			StoryboardConfirmed bool   `json:"storyboard_confirmed"`
			Assets              []any  `json:"assets"`
			Shots               []struct {
				ShotID       string `json:"shot_id"`
				StorySummary string `json:"story_summary"`
			} `json:"shots"`
		} `json:"project"`
	}
	_ = json.NewDecoder(w.Body).Decode(&splitResp)
	if splitResp.Project.Stage != "storyboard" || !splitResp.Project.StoryConfirmed || splitResp.Project.StoryboardConfirmed {
		t.Fatalf("Step 2 split failed: stage=%q storyConfirmed=%v storyboardConfirmed=%v", splitResp.Project.Stage, splitResp.Project.StoryConfirmed, splitResp.Project.StoryboardConfirmed)
	}
	if len(splitResp.Project.Shots) < 5 {
		t.Fatalf("expected story-driven breakdown to naturally create >=5 shots for full story, got %d", len(splitResp.Project.Shots))
	}
	if len(splitResp.Project.Assets) != 0 {
		t.Fatalf("Step 2 must NOT create assets before storyboard confirmation, got %d", len(splitResp.Project.Assets))
	}

	//    Step 2 → Step 3: 人工确认故事板，根据故事板反推项目资产清单与每镜资产绑定
	w = doReq(h, http.MethodPost, "/api/projects/grandma-fan/assets/extract", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/projects/grandma-fan/assets/extract failed: status=%d body=%s", w.Code, w.Body.String())
	}
	var extractResp struct {
		Project struct {
			Stage               string `json:"stage"`
			StoryboardConfirmed bool   `json:"storyboard_confirmed"`
			Assets              []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"assets"`
			Shots []struct {
				ShotID string `json:"shot_id"`
			} `json:"shots"`
		} `json:"project"`
	}
	_ = json.NewDecoder(w.Body).Decode(&extractResp)
	if extractResp.Project.Stage != "assets_plan" || !extractResp.Project.StoryboardConfirmed || len(extractResp.Project.Assets) < 2 {
		t.Fatalf("Step 3 extraction failed: stage=%q confirmed=%v assets=%d", extractResp.Project.Stage, extractResp.Project.StoryboardConfirmed, len(extractResp.Project.Assets))
	}

	//    Step 3: 批量生成资产定妆图 (T2I) + 浏览器上传资产图
	w = doReq(h, http.MethodPost, "/api/projects/grandma-fan/assets/render", `{"force":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/projects/grandma-fan/assets/render failed: status=%d body=%s", w.Code, w.Body.String())
	}

	uploadFile := func(url string) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "sample.jpeg")
		_, _ = fw.Write([]byte{0xFF, 0xD8, 0xFF, 0xD9})
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, url, &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	w = uploadFile("/api/projects/grandma-fan/assets/" + extractResp.Project.Assets[0].ID + "/upload")
	if w.Code != http.StatusOK {
		t.Fatalf("asset upload failed: status=%d body=%s", w.Code, w.Body.String())
	}

	//    Step 4: 分镜定妆图（生成 + 上传 + 可选跳过标记）
	w = uploadFile("/api/projects/grandma-fan/shots/" + extractResp.Project.Shots[0].ShotID + "/upload-frame")
	if w.Code != http.StatusOK {
		t.Fatalf("shot frame upload failed: status=%d body=%s", w.Code, w.Body.String())
	}
	w = doReq(h, http.MethodPost, "/api/projects/grandma-fan/stage", `{"stage":"video","assets_confirmed":true,"keyframes_skipped":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/projects/grandma-fan/stage failed: status=%d body=%s", w.Code, w.Body.String())
	}

	// 10. 项目级模板独立覆盖：仅修改 grandma-fan 的导演模板，urban-demo 不受污染
	w = doReq(h, http.MethodGet, "/api/templates?project_id=grandma-fan", "")
	var fanTmpls struct {
		Templates map[string]string `json:"templates"`
		Overrides map[string]bool   `json:"overrides"`
	}
	_ = json.NewDecoder(w.Body).Decode(&fanTmpls)
	projSaveReq, _ := json.Marshal(map[string]string{
		"name":       "prompts/director_storyboard.tmpl",
		"content":    fanTmpls.Templates["prompts/director_storyboard.tmpl"] + "\n# ONLY_FOR_GRANDMA_FAN",
		"project_id": "grandma-fan",
		"scope":      "project",
	})
	w = doReq(h, http.MethodPut, "/api/templates", string(projSaveReq))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT project template override failed: status=%d body=%s", w.Code, w.Body.String())
	}
	w = doReq(h, http.MethodGet, "/api/projects/grandma-fan", "")
	if !strings.Contains(w.Body.String(), "ONLY_FOR_GRANDMA_FAN") {
		t.Fatalf("expected grandma-fan system prompt to use its project-level override")
	}
	w = doReq(h, http.MethodGet, "/api/projects/urban-demo", "")
	if strings.Contains(w.Body.String(), "ONLY_FOR_GRANDMA_FAN") {
		t.Fatalf("urban-demo must NOT be polluted by grandma-fan project template override")
	}

	// 11. 项目管理：删除项目（含路径穿越防护与列表清理校验）
	w = doReq(h, http.MethodDelete, "/api/projects/grandma-fan", "")
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE /api/projects/grandma-fan failed: status=%d body=%s", w.Code, w.Body.String())
	}
	w = doReq(h, http.MethodGet, "/api/projects/grandma-fan", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after deleting grandma-fan, got %d", w.Code)
	}
	w = doReq(h, http.MethodDelete, "/api/projects/non-existent", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when deleting non-existent project, got %d", w.Code)
	}
}

func TestOneClickAutoPilotAnimaticAndExportPack(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed in PATH")
	}

	tmp := t.TempDir()
	svc, err := studio.Open(tmp, filepath.Join(tmp, "configs"))
	if err != nil {
		t.Fatalf("studio.Open: %v", err)
	}
	defer svc.Close()

	h := server.New(svc).Handler()

	// 1. 新建 3:4 画幅 + 治愈女声 + 精炼短片项目，并执行同步一键全自动出片（P0 + P1）
	createBody := `{"id":"oneclick-demo","title":"阳台的晚风","theme":"下班后吹着晚风看落日","track":"urban_sanctuary","style_preset":"cinematic_heal","aspect_ratio":"3:4","narrator_voice":"healing-female","target_pacing":"short","auto_plan":false}`
	w := doReq(h, http.MethodPost, "/api/projects", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project failed: status=%d body=%s", w.Code, w.Body.String())
	}

	w = doReq(h, http.MethodPost, "/api/projects/oneclick-demo/autopilot", `{"sync":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("autopilot sync failed: status=%d body=%s", w.Code, w.Body.String())
	}
	var apStatus struct {
		State          string `json:"state"`
		Percent        int    `json:"percent"`
		FinalVideoPath string `json:"final_video_path"`
		CoverPath      string `json:"cover_path"`
	}
	_ = json.NewDecoder(w.Body).Decode(&apStatus)
	if apStatus.State != "succeeded" || apStatus.Percent != 100 || apStatus.FinalVideoPath == "" || apStatus.CoverPath == "" {
		t.Fatalf("unexpected autopilot result: %+v", apStatus)
	}

	// 验证详情中包含 1080x1440 (3:4) 成片、cover_path 与逐镜配音路径
	w = doReq(h, http.MethodGet, "/api/projects/oneclick-demo", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get project failed: status=%d", w.Code)
	}
	var detail struct {
		VoicePaths map[string]string `json:"voice_paths"`
		CoverPath  string            `json:"cover_path"`
		FinalVideo *struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"final_video"`
	}
	_ = json.NewDecoder(w.Body).Decode(&detail)
	if detail.FinalVideo == nil || detail.FinalVideo.Width != 1080 || detail.FinalVideo.Height != 1440 {
		t.Fatalf("expected 1080x1440 (3:4) final video, got %+v", detail.FinalVideo)
	}
	if detail.CoverPath == "" || len(detail.VoicePaths) == 0 {
		t.Fatalf("expected cover_path and voice_paths populated")
	}

	// 2. 验证内置配乐试听与零成本动态预演样片 Animatic（P1 + P2）
	w = doReq(h, http.MethodGet, "/api/bgm/preview?preset=music_box", "")
	if w.Code != http.StatusOK || w.Body.Len() < 1000 {
		t.Fatalf("bgm preview failed: status=%d len=%d", w.Code, w.Body.Len())
	}

	w = doReq(h, http.MethodPost, "/api/projects/oneclick-demo/animatic", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("animatic failed: status=%d body=%s", w.Code, w.Body.String())
	}

	// 3. 验证一键导出剪映专业版草稿 & 分轨工程 ZIP 包（P2）
	w = doReq(h, http.MethodGet, "/api/projects/oneclick-demo/export-pack", "")
	if w.Code != http.StatusOK {
		t.Fatalf("export-pack failed: status=%d body=%s", w.Code, w.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("invalid export zip: %v", err)
	}
	seen := map[string]bool{}
	for _, f := range zr.File {
		seen[f.Name] = true
	}
	for _, mustHave := range []string{
		"final.mp4",
		"animatic_动态预演.mp4",
		"cover.jpg",
		"subtitles/final.srt",
		"jianying_draft/oneclick-demo/draft_content.json",
		"jianying_draft/oneclick-demo/draft_meta_info.json",
	} {
		if !seen[mustHave] {
			t.Fatalf("export zip missing expected entry %q; got entries=%v", mustHave, seen)
		}
	}

	// 4. 验证「跳过分镜首帧定妆图 (skip_keyframes=true)」直出视频模式也能一键全自动跑通（P0）
	skipProj := `{"id":"skip-kf-demo","title":"老家木桌","theme":"父母打包家乡特产","track":"family_bond","target_pacing":"short","skip_keyframes":true,"auto_plan":false}`
	w = doReq(h, http.MethodPost, "/api/projects", skipProj)
	if w.Code != http.StatusCreated {
		t.Fatalf("create skip-kf-demo failed: status=%d", w.Code)
	}
	w = doReq(h, http.MethodPost, "/api/projects/skip-kf-demo/autopilot", `{"sync":true,"skip_keyframes":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("skip-kf autopilot failed: status=%d body=%s", w.Code, w.Body.String())
	}
}
