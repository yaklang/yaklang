package aiforge

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ffmpegutils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// AnalyzeVideoOmni 关键词: omni 视频分析, qwen omni 端到端, 视频切片知识抽取
//
// 与既有 AnalyzeVideo 并存（旧函数仍然走"抽帧+图像分析"路径），AnalyzeVideoOmni
// 直接把视频切片喂给 Qwen Omni 等支持 video_url 的多模态模型，让模型一次性
// 完成"视觉+音频"的端到端理解，输出结构化 JSON 用于后续知识库构建。

// VideoOmniSegmentResult 单个视频切片的 omni 分析结果，实现 AnalysisResult 接口。
// 关键词: VideoOmniSegmentResult, omni 切片结果
type VideoOmniSegmentResult struct {
	// SourceVideo 源视频路径
	SourceVideo string `json:"source_video"`
	// SegmentPath 切片落盘路径
	SegmentPath string `json:"segment_path"`
	// SegmentIndex 切片序号
	SegmentIndex int `json:"segment_index"`
	// StartTime 切片起始时间
	StartTime time.Duration `json:"start_time"`
	// EndTime 切片结束时间
	EndTime time.Duration `json:"end_time"`
	// SizeBytes 切片字节数
	SizeBytes int64 `json:"size_bytes"`
	// Model 实际调用的模型名
	Model string `json:"model"`
	// Title 模型抽取的标题（结构化 JSON 字段）
	Title string `json:"title"`
	// Storyline 视频段叙事/事件主线
	Storyline string `json:"storyline"`
	// VisibleText 屏幕/字幕中可见的关键文本
	VisibleText []string `json:"visible_text"`
	// Speakers 说话人 + 重要语音内容
	Speakers []string `json:"speakers"`
	// KeyKnowledge 抽取的若干条专业知识点（每条独立为知识库 entry 用）
	KeyKnowledge []string `json:"key_knowledge"`
	// Tags 主题标签
	Tags []string `json:"tags"`
	// RawText 模型原始返回文本（包括 JSON 之前/之后的多余文字）
	RawText string `json:"raw_text"`
	// LatencyMs 调用延迟
	LatencyMs int64 `json:"latency_ms"`
	// ErrMsg 不致命错误说明
	ErrMsg string `json:"err_msg,omitempty"`
}

// GetCumulativeSummary 累积摘要：用于 RAG 上下文，帮助下游入库逻辑做关联
func (v *VideoOmniSegmentResult) GetCumulativeSummary() string {
	if v == nil {
		return ""
	}
	if v.Storyline != "" {
		return v.Storyline
	}
	return v.Title
}

// Dump 把切片分析结果序列化为可读文本，作为 RAG 入库内容
func (v *VideoOmniSegmentResult) Dump() string {
	if v == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Video Segment %d (%s ~ %s)\n", v.SegmentIndex, v.StartTime, v.EndTime))
	sb.WriteString(fmt.Sprintf("Source: %s\n", v.SourceVideo))
	sb.WriteString(fmt.Sprintf("Model: %s\n\n", v.Model))
	if v.Title != "" {
		sb.WriteString(fmt.Sprintf("Title: %s\n", v.Title))
	}
	if v.Storyline != "" {
		sb.WriteString("\n## Storyline\n")
		sb.WriteString(v.Storyline)
		sb.WriteString("\n")
	}
	if len(v.VisibleText) > 0 {
		sb.WriteString("\n## Visible Text\n")
		for _, t := range v.VisibleText {
			sb.WriteString("- ")
			sb.WriteString(t)
			sb.WriteString("\n")
		}
	}
	if len(v.Speakers) > 0 {
		sb.WriteString("\n## Speakers / Audio\n")
		for _, t := range v.Speakers {
			sb.WriteString("- ")
			sb.WriteString(t)
			sb.WriteString("\n")
		}
	}
	if len(v.KeyKnowledge) > 0 {
		sb.WriteString("\n## Key Knowledge\n")
		for _, k := range v.KeyKnowledge {
			sb.WriteString("- ")
			sb.WriteString(k)
			sb.WriteString("\n")
		}
	}
	if len(v.Tags) > 0 {
		sb.WriteString("\n## Tags: ")
		sb.WriteString(strings.Join(v.Tags, ", "))
		sb.WriteString("\n")
	}
	if v.ErrMsg != "" {
		sb.WriteString("\n[err] ")
		sb.WriteString(v.ErrMsg)
		sb.WriteString("\n")
	}
	return sb.String()
}

// VideoOmniConfig omni 视频分析配置
// 关键词: VideoOmniConfig, omni 视频参数
type VideoOmniConfig struct {
	// AIType ai 提供方，默认 "tongyi"
	AIType string
	// Model 模型名，默认 qwen3-omni-flash
	Model string
	// APIKey 模型 key
	APIKey string
	// BaseURL 自定义 base url（可选）
	BaseURL string
	// Preset omni 预设：turbo / flash / plus（决定段长）
	Preset string
	// SegmentSeconds 切片段长（秒）；非 0 时覆盖 Preset
	SegmentSeconds float64
	// Reencode 是否重编码（默认 true：保证 base64 体积可控）
	Reencode bool
	// MaxHeight 重编码最大高度，默认 720
	MaxHeight int
	// TargetFPS 重编码目标 FPS，默认 2（贴合 omni 内部抽样率）
	TargetFPS float64
	// MaxBase64Bytes 单段允许的最大字节数（< 10MB 给阿里云留余量）
	MaxBase64Bytes int64
	// MaxSegments 最多分析多少段（<=0 表示无限制；调试用）
	MaxSegments int
	// SystemPrompt 自定义系统提示，默认走内置中文专业知识抽取 prompt
	SystemPrompt string
	// QueryPrompt 自定义查询，默认走内置 query
	QueryPrompt string
	// Ctx 上下文
	Ctx context.Context
	// ProgressCallback 段进度回调
	ProgressCallback func(*VideoOmniSegmentResult)
	// Timeout 单段调用超时
	Timeout time.Duration

	// 关键词: omni 视频 zip 归档参数
	// ZipFile 完整 zip 文件路径（优先级最高，存在则忽略 ZipDir）
	ZipFile string
	// ZipDir 仅目录；运行时自动生成 {kbName|video-omni}-{model}-{ts}.zip
	ZipDir string
	// KBNameForZip 由 BuildVideoKnowledgeFromOmni 自动注入，用于生成 zip 文件名
	KBNameForZip string
}

// VideoOmniOption 配置选项类型
type VideoOmniOption func(*VideoOmniConfig)

// NewDefaultVideoOmniConfig 默认值
func NewDefaultVideoOmniConfig() *VideoOmniConfig {
	return &VideoOmniConfig{
		AIType:         "tongyi",
		Model:          "qwen3-omni-flash",
		Preset:         "flash",
		Reencode:       true,
		MaxHeight:      720,
		TargetFPS:      2,
		MaxBase64Bytes: 7 * 1024 * 1024, // 7MB 切片，base64 约 9.3MB，留 0.7MB 余量
		Ctx:            context.Background(),
		Timeout:        300 * time.Second,
	}
}

// 关键词: VideoOmniOption, omni 视频选项

func WithVideoOmniType(t string) VideoOmniOption       { return func(c *VideoOmniConfig) { c.AIType = t } }
func WithVideoOmniModel(m string) VideoOmniOption      { return func(c *VideoOmniConfig) { c.Model = m } }
func WithVideoOmniAPIKey(k string) VideoOmniOption     { return func(c *VideoOmniConfig) { c.APIKey = k } }
func WithVideoOmniBaseURL(u string) VideoOmniOption    { return func(c *VideoOmniConfig) { c.BaseURL = u } }
func WithVideoOmniSystemPrompt(p string) VideoOmniOption {
	return func(c *VideoOmniConfig) { c.SystemPrompt = p }
}
func WithVideoOmniQueryPrompt(q string) VideoOmniOption {
	return func(c *VideoOmniConfig) { c.QueryPrompt = q }
}
func WithVideoOmniContext(ctx context.Context) VideoOmniOption {
	return func(c *VideoOmniConfig) {
		if ctx != nil {
			c.Ctx = ctx
		}
	}
}
func WithVideoOmniTimeout(d time.Duration) VideoOmniOption {
	return func(c *VideoOmniConfig) {
		if d > 0 {
			c.Timeout = d
		}
	}
}
func WithVideoOmniSegmentSeconds(s float64) VideoOmniOption {
	return func(c *VideoOmniConfig) {
		if s > 0 {
			c.SegmentSeconds = s
		}
	}
}
func WithVideoOmniReencode(b bool) VideoOmniOption    { return func(c *VideoOmniConfig) { c.Reencode = b } }
func WithVideoOmniMaxHeight(h int) VideoOmniOption    { return func(c *VideoOmniConfig) { c.MaxHeight = h } }
func WithVideoOmniTargetFPS(f float64) VideoOmniOption { return func(c *VideoOmniConfig) { c.TargetFPS = f } }
func WithVideoOmniMaxBase64Bytes(n int64) VideoOmniOption {
	return func(c *VideoOmniConfig) { c.MaxBase64Bytes = n }
}
func WithVideoOmniMaxSegments(n int) VideoOmniOption {
	return func(c *VideoOmniConfig) { c.MaxSegments = n }
}
func WithVideoOmniProgressCallback(cb func(*VideoOmniSegmentResult)) VideoOmniOption {
	return func(c *VideoOmniConfig) { c.ProgressCallback = cb }
}

// 关键词: omni 视频 zip 选项, omniZipFile, omniZipDir

// WithVideoOmniZipFile 指定 zip 文件完整输出路径。
// 设置后会忽略 WithVideoOmniZipDir。
func WithVideoOmniZipFile(p string) VideoOmniOption {
	return func(c *VideoOmniConfig) { c.ZipFile = p }
}

// WithVideoOmniZipDir 指定 zip 输出目录，运行时自动按
// {kbName|video-omni}-{model}-{ts}.zip 命名落盘。
func WithVideoOmniZipDir(dir string) VideoOmniOption {
	return func(c *VideoOmniConfig) { c.ZipDir = dir }
}

// withVideoOmniKBName 内部使用，由 BuildVideoKnowledgeFromOmni 注入
// kb 名给自动 zip 命名使用。
func withVideoOmniKBName(name string) VideoOmniOption {
	return func(c *VideoOmniConfig) { c.KBNameForZip = name }
}

// VideoOmniPresetTurbo / Flash / Plus 提供 forge 友好的预设
// 关键词: omniPresetTurbo, omniPresetFlash, omniPresetPlus
// 预设会强制覆盖 Preset 与 Model；若需要再覆盖 Model 请在 preset 之后再调用 WithVideoOmniModel
func VideoOmniPresetTurbo() VideoOmniOption {
	return func(c *VideoOmniConfig) {
		c.Preset = "turbo"
		c.Model = "qwen-omni-turbo"
	}
}
func VideoOmniPresetFlash() VideoOmniOption {
	return func(c *VideoOmniConfig) {
		c.Preset = "flash"
		c.Model = "qwen3-omni-flash"
	}
}
func VideoOmniPresetPlus() VideoOmniOption {
	return func(c *VideoOmniConfig) {
		c.Preset = "plus"
		c.Model = "qwen3.5-omni-plus"
	}
}

// 默认中文专业知识抽取提示，针对 omni 视频
const defaultOmniVideoSystemPrompt = `你是一位资深的安全研究/技术教学视频知识抽取助手。
你将看到一段连续视频（含画面与音频）。请把这一段视频里的内容浓缩为可入库的专业知识。
要求：
1) 准确，只描述真实出现的内容，不要编造。
2) 专注于该段中可形成"知识"的部分（概念、命令、攻击/防御步骤、要点、易错点等）。
3) 回答必须严格使用 JSON 输出，不要任何其他多余文字。`

const defaultOmniVideoQueryPrompt = `请按下面 JSON Schema 严格输出该视频段的知识抽取结果：
{
  "title": "用一句中文概括本段主题",
  "storyline": "本段画面+音频的连贯叙述（200~400 字中文）",
  "visible_text": ["画面中出现的关键文本/命令/代码片段，按出现顺序"],
  "speakers": ["音频中重要的解说原文（中文，每条一句）"],
  "key_knowledge": ["可作为单条知识库 entry 的专业知识点（中文，每条一句话精炼）"],
  "tags": ["主题标签，例如 XSS、Stored XSS、过滤绕过 等"]
}
仅输出 JSON，不要 Markdown 代码块包裹。`

// AnalyzeVideoOmni 把视频切片送进 omni 模型做端到端理解，按段返回 AnalysisResult。
//
// example:
//
//	ch, err := aiforge.AnalyzeVideoOmni("xss-learn.mp4",
//	    aiforge.VideoOmniPresetFlash(),
//	    aiforge.WithVideoOmniAPIKey(key),
//	)
//
// 关键词: AnalyzeVideoOmni, omni 视频端到端
func AnalyzeVideoOmni(video string, options ...any) (<-chan AnalysisResult, error) {
	cfg := NewDefaultVideoOmniConfig()
	for _, opt := range options {
		if fn, ok := opt.(VideoOmniOption); ok {
			fn(cfg)
		}
	}
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = defaultOmniVideoSystemPrompt
	}
	if cfg.QueryPrompt == "" {
		cfg.QueryPrompt = defaultOmniVideoQueryPrompt
	}
	if cfg.APIKey == "" {
		return nil, utils.Error("AnalyzeVideoOmni: APIKey is required (use WithVideoOmniAPIKey)")
	}

	// 给 ffmpeg 切片建立可取消 ctx，达到 MaxSegments 时立刻 cancel 让 ffmpeg 退出
	// 关键词: omni 切片可取消, slice cancellable
	parentCtx := cfg.Ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	sliceCtx, cancelSlice := context.WithCancel(parentCtx)

	// 决定是否启用 zip 归档
	// 关键词: omni zip 启用判定, zip path resolve
	zipPath := resolveOmniZipPath(cfg)
	var archiver *videoSegmentArchiver
	if zipPath != "" {
		a, err := newVideoSegmentArchiver(zipPath, video, cfg.Model, cfg.KBNameForZip)
		if err != nil {
			log.Errorf("init video segment archiver failed: %v (zip will be skipped)", err)
		} else {
			archiver = a
		}
	}

	// 准备主路（reencode）切片选项
	// 关键词: omni 主路切片, reencode slice
	mainSliceOpts := []ffmpegutils.Option{
		ffmpegutils.WithContext(sliceCtx),
	}
	if cfg.SegmentSeconds > 0 {
		mainSliceOpts = append(mainSliceOpts, ffmpegutils.WithSliceDurationSeconds(cfg.SegmentSeconds))
	} else if cfg.Preset != "" {
		mainSliceOpts = append(mainSliceOpts, ffmpegutils.WithSlicePresetForOmni(cfg.Preset))
	}
	if cfg.Reencode {
		mainSliceOpts = append(mainSliceOpts,
			ffmpegutils.WithSliceReencode(true),
			ffmpegutils.WithSliceMaxHeight(cfg.MaxHeight),
			ffmpegutils.WithSliceTargetFPS(cfg.TargetFPS),
		)
	}

	sliceChan, err := ffmpegutils.ExtractVideoSliceFromVideo(video, mainSliceOpts...)
	if err != nil {
		cancelSlice()
		if archiver != nil {
			_ = archiver.WriteManifestAndClose()
		}
		return nil, fmt.Errorf("video slice failed: %w", err)
	}

	// 启动副路（stream-copy）切片，仅在启用 archiver 时执行
	// 关键词: 副路切片 stream copy, secondary slice
	var streamCopyDone chan struct{}
	if archiver != nil {
		streamCopyDone = make(chan struct{})
		go runStreamCopyArchive(sliceCtx, video, cfg, archiver, streamCopyDone)
	}

	out := make(chan AnalysisResult, 4)
	go func() {
		defer close(out)
		defer cancelSlice()
		// 关闭顺序: 先等副路 goroutine 退出，再写 manifest 关闭 zip
		// 关键词: archiver 关闭顺序, zip close order
		defer func() {
			if archiver != nil {
				if streamCopyDone != nil {
					select {
					case <-streamCopyDone:
					case <-time.After(15 * time.Second):
						log.Warnf("stream copy goroutine did not exit in 15s, force closing archiver")
					}
				}
				if err := archiver.WriteManifestAndClose(); err != nil {
					log.Errorf("close archiver failed: %v", err)
				}
			}
		}()

		processed := 0
		for slice := range sliceChan {
			if cfg.Ctx != nil {
				select {
				case <-cfg.Ctx.Done():
					log.Warnf("AnalyzeVideoOmni canceled by context: %v", cfg.Ctx.Err())
					return
				default:
				}
			}
			if slice == nil {
				continue
			}
			if slice.Error != nil {
				log.Errorf("video slice error: %v", slice.Error)
				continue
			}
			if cfg.MaxSegments > 0 && processed >= cfg.MaxSegments {
				log.Infof("AnalyzeVideoOmni reached MaxSegments=%d, cancel slicing and drain", cfg.MaxSegments)
				cancelSlice()
				// 继续 drain channel 但不再处理，直到 ffmpeg 退出 + channel close
				continue
			}
			processed++

			// 主路切片在送 omni 之前先入 zip（reencoded.mp4）
			// 关键词: 主路切片入 zip, reencoded mp4 archive
			if archiver != nil {
				if err := archiver.WriteSegmentMP4(slice.Index, "reencoded", slice.FilePath); err != nil {
					log.Errorf("archive reencoded segment idx=%d failed: %v", slice.Index, err)
				}
			}

			result := analyzeSingleOmniSegment(video, slice, cfg)

			// omni 结果落 zip
			// 关键词: omni 分析结果入 zip
			if archiver != nil {
				if err := archiver.WriteAnalysis(result); err != nil {
					log.Errorf("archive analysis idx=%d failed: %v", slice.Index, err)
				}
			}

			if cfg.ProgressCallback != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Errorf("AnalyzeVideoOmni progress callback panic: %v", r)
						}
					}()
					cfg.ProgressCallback(result)
				}()
			}
			select {
			case out <- result:
			case <-ctxDone(cfg.Ctx):
				return
			}
		}
	}()
	return out, nil
}

// resolveOmniZipPath 根据 cfg.ZipFile / cfg.ZipDir 计算最终 zip 路径。
// 关键词: omni zip 路径解析, zip path resolve
func resolveOmniZipPath(cfg *VideoOmniConfig) string {
	if cfg == nil {
		return ""
	}
	if cfg.ZipFile != "" {
		return cfg.ZipFile
	}
	if cfg.ZipDir == "" {
		return ""
	}
	stem := cfg.KBNameForZip
	if stem == "" {
		stem = "video-omni"
	}
	model := cfg.Model
	if model == "" {
		model = "unknown"
	}
	ts := time.Now().Format("20060102_150405")
	return filepath.Join(cfg.ZipDir, fmt.Sprintf("%s-%s-%s.zip", sanitizeFileStem(stem), sanitizeFileStem(model), ts))
}

// sanitizeFileStem 把不利于文件名的字符替换为下划线
// 关键词: 文件名安全化
func sanitizeFileStem(s string) string {
	if s == "" {
		return "_"
	}
	bad := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", " "}
	out := s
	for _, b := range bad {
		out = strings.ReplaceAll(out, b, "_")
	}
	return out
}

// runStreamCopyArchive 副路 goroutine，跑一遍 stream-copy 切片，把结果直接写入 archiver。
// 关键词: 副路 goroutine, stream copy archive
func runStreamCopyArchive(ctx context.Context, video string, cfg *VideoOmniConfig, archiver *videoSegmentArchiver, done chan<- struct{}) {
	defer close(done)
	if archiver == nil {
		return
	}
	// 关键词: 副路切片配置
	opts := []ffmpegutils.Option{
		ffmpegutils.WithContext(ctx),
		ffmpegutils.WithSliceReencode(false),
	}
	if cfg.SegmentSeconds > 0 {
		opts = append(opts, ffmpegutils.WithSliceDurationSeconds(cfg.SegmentSeconds))
	} else if cfg.Preset != "" {
		opts = append(opts, ffmpegutils.WithSlicePresetForOmni(cfg.Preset))
	}

	sc, err := ffmpegutils.ExtractVideoSliceFromVideo(video, opts...)
	if err != nil {
		log.Errorf("stream copy slice for archive failed: %v", err)
		return
	}
	for s := range sc {
		select {
		case <-ctx.Done():
			// drain 剩余 channel 让 ffmpeg goroutine 收尾
			continue
		default:
		}
		if s == nil || s.Error != nil {
			if s != nil && s.Error != nil {
				log.Warnf("stream copy slice error idx=%d: %v", s.Index, s.Error)
			}
			continue
		}
		if err := archiver.WriteSegmentMP4(s.Index, "streamcopy", s.FilePath); err != nil {
			log.Errorf("archive streamcopy segment idx=%d failed: %v", s.Index, err)
		}
	}
}

func ctxDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}

// analyzeSingleOmniSegment 调用 omni 模型对单个切片做端到端理解。
// 关键词: omni 单段调用, single segment omni
func analyzeSingleOmniSegment(video string, slice *ffmpegutils.VideoSliceResult, cfg *VideoOmniConfig) *VideoOmniSegmentResult {
	r := &VideoOmniSegmentResult{
		SourceVideo:  video,
		SegmentPath:  slice.FilePath,
		SegmentIndex: slice.Index,
		StartTime:    slice.StartTime,
		EndTime:      slice.EndTime,
		SizeBytes:    slice.SizeBytes,
		Model:        cfg.Model,
	}

	if cfg.MaxBase64Bytes > 0 && slice.SizeBytes > cfg.MaxBase64Bytes {
		r.ErrMsg = fmt.Sprintf("segment size %d exceeds MaxBase64Bytes %d, skip omni call (consider enabling reencode or smaller segment)", slice.SizeBytes, cfg.MaxBase64Bytes)
		log.Warnf("video slice idx=%d skipped: %s", slice.Index, r.ErrMsg)
		return r
	}

	rawData, err := ioutil.ReadFile(slice.FilePath)
	if err != nil {
		r.ErrMsg = fmt.Sprintf("read slice file failed: %v", err)
		return r
	}

	b64 := codec.EncodeBase64(rawData)
	// 关键词: omni 视频 data uri 格式
	// 阿里云 omni 模型要求 video_url 的 base64 不带 mime type（与 image_url 不同），
	// 否则会报 "Multiple inputs of the same modality or mixed modality inputs..." 错误。
	dataURI := "data:;base64," + b64

	startCall := time.Now()
	prompt := cfg.SystemPrompt + "\n\n" + cfg.QueryPrompt

	chatOpts := []aispec.AIConfigOption{
		aispec.WithType(cfg.AIType),
		aispec.WithModel(cfg.Model),
		aispec.WithAPIKey(cfg.APIKey),
		aispec.WithVideoUrl(dataURI),
	}
	if cfg.BaseURL != "" {
		chatOpts = append(chatOpts, aispec.WithBaseURL(cfg.BaseURL))
	}
	if cfg.Ctx != nil {
		// timeout context
		callCtx, cancel := context.WithTimeout(cfg.Ctx, cfg.Timeout)
		defer cancel()
		chatOpts = append(chatOpts, aispec.WithContext(callCtx))
	}

	log.Infof("calling omni model %s for segment idx=%d size=%d", cfg.Model, slice.Index, slice.SizeBytes)
	resp, err := ai.Chat(prompt, chatOpts...)
	r.LatencyMs = time.Since(startCall).Milliseconds()
	if err != nil {
		r.ErrMsg = fmt.Sprintf("omni chat failed: %v", err)
		log.Errorf("omni chat failed for segment idx=%d: %v", slice.Index, err)
		return r
	}
	r.RawText = resp

	// 抽取 JSON
	// 关键词: omni JSON 抽取, jsonextractor
	if jsonStr := extractFirstJSON(resp); jsonStr != "" {
		var parsed struct {
			Title        string   `json:"title"`
			Storyline    string   `json:"storyline"`
			VisibleText  []string `json:"visible_text"`
			Speakers     []string `json:"speakers"`
			KeyKnowledge []string `json:"key_knowledge"`
			Tags         []string `json:"tags"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
			r.Title = parsed.Title
			r.Storyline = parsed.Storyline
			r.VisibleText = parsed.VisibleText
			r.Speakers = parsed.Speakers
			r.KeyKnowledge = parsed.KeyKnowledge
			r.Tags = parsed.Tags
		} else {
			log.Warnf("omni segment idx=%d json parse failed: %v, raw=%s", slice.Index, err, utils.ShrinkString(resp, 500))
			r.ErrMsg = "json parse failed: " + err.Error()
		}
	} else {
		// 没有 JSON 块，把全文当做 storyline
		// 关键词: omni 无结构化 fallback
		r.Storyline = resp
	}
	return r
}

func extractFirstJSON(text string) string {
	if text == "" {
		return ""
	}
	// 先尝试 jsonextractor 内置抽取
	results := jsonextractor.ExtractStandardJSON(text)
	for _, item := range results {
		item = strings.TrimSpace(item)
		if strings.HasPrefix(item, "{") {
			return item
		}
	}
	// 兜底：扫描首个 { 与最后一个 }
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return ""
}
