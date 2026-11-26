# feelings-studio

社会心理学驱动的情怀 AI 短视频生成工作台：故事大纲 → 故事板分镜 → 资产定妆图 → 分镜首帧 → 图生视频 / 旁白 / 环境声 → FFmpeg 调色混音成片。

默认所有阶段都使用本地零费用 Mock 供应商，不配置任何 API Key 也能跑通全流程。

## 依赖

- Go 1.27+
- `ffmpeg` / `ffprobe`（合成、预演样片和 Mock 视频都需要）

## 快速开始

```bash
go build -o bin/feelings ./cmd/feelings

# 启动本地 Web 工作台（仅监听 127.0.0.1）
./bin/feelings serve --port 8080

# 或命令行一键出片
./bin/feelings init --id demo --track childhood_90s --theme "放学后的小卖部"
./bin/feelings auto --project demo
```

以当前工作目录作为工作区，产物写到 `projects/<id>/output/`（已在 `.gitignore` 中忽略）。

常用子命令：

| 命令 | 作用 |
| --- | --- |
| `strategies` | 列出心理学赛道策略包与视觉风格预设 |
| `providers` | 查看当前模型供应商配置 |
| `init` | 新建项目（可用 `--import-dir` 导入 `资产*.jpeg` 与 `g1~gN.jpeg`） |
| `plan` / `render` / `compose` | 分步：编排分镜 / 渲染 / 合成成片 |
| `auto` | 一键全自动出片 |
| `animatic` | 不调用视频 API，合成动态故事板预演 |
| `export` | 导出剪映草稿与分轨工程 ZIP |

## 模型供应商配置

在 Web 工作台「模型供应商设置」页配置，保存到 `.feelings/providers.yaml`（权限 0600，已被 git 忽略，**不要提交**）。
也可以用环境变量覆盖：

```
FEELINGS_LLM_PROVIDER  FEELINGS_LLM_BASE_URL  FEELINGS_LLM_API_KEY  FEELINGS_LLM_MODEL
FEELINGS_T2I_PROVIDER  FEELINGS_T2I_BASE_URL  FEELINGS_T2I_API_KEY  FEELINGS_T2I_MODEL
FEELINGS_I2V_PROVIDER  FEELINGS_TTS_PROVIDER
```

## 目录结构

```
cmd/feelings/          CLI 入口
configs/               内置提示词模板、心理学策略包、风格预设（embed 打包）
internal/studio/       核心编排服务（项目级锁串行化 project.yaml 读写）
internal/server/       Web 工作台 HTTP API 与前端页面
internal/project/      项目清单读写与 ID 校验
internal/provider/     LLM / T2I / I2V / TTS / SFX 供应商适配
internal/scheduler/    并发渲染任务池
internal/composer/     FFmpeg 合成
```

## 安全说明

Web 工作台只为本机单用户设计：

- 只监听 `127.0.0.1`，并校验 `Host`（防 DNS rebinding）和写请求的 `Origin`（防跨站请求）；
- `/api/media` 只返回工作区内非隐藏目录下的图片 / 音视频文件；
- 项目、镜头、资产 ID 只允许字母、数字、`-`、`_`、`.`。

不要把端口通过反向代理或端口转发暴露到公网。

## 测试

```bash
go test ./...        # composer / server 测试会调用 ffmpeg，耗时约 2 分钟
```
