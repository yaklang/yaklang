package aiforge

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// videoSegmentArchiver 流式 zip 归档器，把视频 omni 管线运行过程中
// 的每一个分片素材（流复制 mp4、重编码 mp4、omni raw response、分析 JSON、
// 人类可读 dump）落入同一个 zip，方便后续离线蒸馏 skills 等再加工。
//
// 关键词: videoSegmentArchiver, omni 视频归档, 视频知识清洗 zip
type videoSegmentArchiver struct {
	mu        sync.Mutex
	zipPath   string
	file      *os.File
	writer    *zip.Writer
	closed    bool
	createdAt time.Time

	// manifest 在 Close 时写入 zip 顶层
	manifest *archiveManifest

	// segments 累积每段的元信息，按 index 唯一
	// 关键词: archive manifest, 分片 manifest 索引
	segmentsByIdx map[int]*archiveSegmentEntry
}

// archiveManifest zip 顶层 manifest.json 的结构
// 关键词: zip manifest 结构
type archiveManifest struct {
	SourceVideo      string                  `json:"source_video"`
	Model            string                  `json:"model"`
	KBName           string                  `json:"kb_name"`
	CreatedAt        string                  `json:"created_at"`
	Generator        string                  `json:"generator"`
	GeneratorVersion string                  `json:"generator_version"`
	Segments         []*archiveSegmentEntry  `json:"segments"`
	ErrorCount       int                     `json:"error_count"`
	Notes            map[string]string       `json:"notes,omitempty"`
}

// archiveSegmentEntry 单个分片在 manifest 中的条目
// 关键词: archive segment entry
type archiveSegmentEntry struct {
	Index           int      `json:"index"`
	StartSeconds    float64  `json:"start_seconds"`
	EndSeconds      float64  `json:"end_seconds"`
	StreamCopyMP4   string   `json:"stream_copy_mp4,omitempty"`
	StreamCopyBytes int64    `json:"stream_copy_bytes,omitempty"`
	ReencodedMP4    string   `json:"reencoded_mp4,omitempty"`
	ReencodedBytes  int64    `json:"reencoded_bytes,omitempty"`
	AnalysisJSON    string   `json:"analysis_json,omitempty"`
	OmniRawResponse string   `json:"omni_raw_response,omitempty"`
	DumpMarkdown    string   `json:"dump_md,omitempty"`
	Title           string   `json:"title,omitempty"`
	LatencyMs       int64    `json:"latency_ms,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	HasError        bool     `json:"has_error,omitempty"`
	ErrorMessage    string   `json:"error_message,omitempty"`
	// PromptTokens / CompletionTokens / TotalTokens 来自模型 SSE 末帧
	// usage 字段（dashscope omni 在 stream_options.include_usage=true 时返回）。
	// 仅在调用成功时非零；用于离线对账与成本核算。
	// 关键词: 视频 token 用量, manifest 实测 usage
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	// 多模态输入 token 拆分（dashscope omni / openai 多模态在 SSE 末帧
	// usage.prompt_tokens_details 返回）。omni-plus 文本/图片/视频帧 与 音频
	// 不同价格，此拆分用于精确成本核算。
	// 关键词: manifest 多模态 token 拆分
	TextTokens   int `json:"text_tokens,omitempty"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
	ImageTokens  int `json:"image_tokens,omitempty"`
	VideoTokens  int `json:"video_tokens,omitempty"`
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// newVideoSegmentArchiver 创建并打开一个流式 zip 归档器。
// zipPath 为最终落盘的 zip 文件完整路径；如果父目录不存在会自动创建。
//
// 关键词: 创建视频归档器, newVideoSegmentArchiver
func newVideoSegmentArchiver(zipPath, sourceVideo, model, kbName string) (*videoSegmentArchiver, error) {
	if zipPath == "" {
		return nil, fmt.Errorf("archiver zip path is empty")
	}
	dir := filepath.Dir(zipPath)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("mkdir %s failed: %w", dir, err)
		}
	}
	f, err := os.Create(zipPath)
	if err != nil {
		return nil, fmt.Errorf("create zip file %s failed: %w", zipPath, err)
	}
	now := time.Now()
	a := &videoSegmentArchiver{
		zipPath:       zipPath,
		file:          f,
		writer:        zip.NewWriter(f),
		createdAt:     now,
		segmentsByIdx: make(map[int]*archiveSegmentEntry),
		manifest: &archiveManifest{
			SourceVideo:      sourceVideo,
			Model:            model,
			KBName:           kbName,
			CreatedAt:        now.UTC().Format(time.RFC3339),
			Generator:        "yaklang aiforge AnalyzeVideoOmni",
			GeneratorVersion: "1",
			Notes: map[string]string{
				"layout":       "segments/slice_<index>/<files>",
				"streamcopy":   "the original stream-copied mp4, prefer this for re-distillation",
				"reencoded":    "720p/2fps reencoded mp4 actually fed to omni model",
				"analysis":     "parsed VideoOmniSegmentResult JSON",
				"raw_response": "raw text returned by omni model (may contain non-JSON wrapping)",
				"dump_md":      "human-readable dump used as RAG entry seed",
			},
		},
	}
	log.Infof("video segment archiver opened: %s", zipPath)
	return a, nil
}

// segmentDir 返回一个分片在 zip 内部的子目录前缀
// 关键词: 分片目录命名, segment dir naming
func segmentDir(idx int) string {
	return fmt.Sprintf("segments/slice_%05d/", idx)
}

// ensureSegment 取得或新建 manifest 中对应 index 的条目
// 关键词: manifest segment 取或建
func (a *videoSegmentArchiver) ensureSegment(idx int) *archiveSegmentEntry {
	if e, ok := a.segmentsByIdx[idx]; ok {
		return e
	}
	e := &archiveSegmentEntry{Index: idx}
	a.segmentsByIdx[idx] = e
	return e
}

// WriteFile 把任意字节内容作为 zip 内一个文件写入。internalPath 为 zip 内相对路径。
// 关键词: zip 写入字节, archiver WriteFile
func (a *videoSegmentArchiver) WriteFile(internalPath string, content []byte) error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return fmt.Errorf("archiver already closed")
	}
	w, err := a.writer.Create(internalPath)
	if err != nil {
		return fmt.Errorf("zip create %s failed: %w", internalPath, err)
	}
	if _, err := w.Write(content); err != nil {
		return fmt.Errorf("zip write %s failed: %w", internalPath, err)
	}
	return nil
}

// WriteSegmentMP4 把 srcPath 指向的 mp4 文件流式拷贝到 zip 中的对应分片目录。
// kind 取值 "streamcopy" 或 "reencoded"，决定文件名与 manifest 字段。
// 关键词: zip 流式 mp4 拷贝, WriteSegmentMP4
func (a *videoSegmentArchiver) WriteSegmentMP4(idx int, kind string, srcPath string) error {
	if a == nil {
		return nil
	}
	if srcPath == "" {
		return fmt.Errorf("source mp4 path is empty for idx=%d kind=%s", idx, kind)
	}
	stat, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat %s failed: %w", srcPath, err)
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open %s failed: %w", srcPath, err)
	}
	defer src.Close()

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return fmt.Errorf("archiver already closed")
	}

	var fileName string
	switch kind {
	case "streamcopy":
		fileName = "streamcopy.mp4"
	case "reencoded":
		fileName = "reencoded.mp4"
	default:
		return fmt.Errorf("unsupported segment kind: %s", kind)
	}
	internalPath := segmentDir(idx) + fileName

	w, err := a.writer.Create(internalPath)
	if err != nil {
		return fmt.Errorf("zip create %s failed: %w", internalPath, err)
	}
	n, err := io.Copy(w, src)
	if err != nil {
		return fmt.Errorf("zip copy %s failed: %w", internalPath, err)
	}
	entry := a.ensureSegment(idx)
	switch kind {
	case "streamcopy":
		entry.StreamCopyMP4 = internalPath
		entry.StreamCopyBytes = n
	case "reencoded":
		entry.ReencodedMP4 = internalPath
		entry.ReencodedBytes = n
		_ = stat // 仅用作存在性校验
	}
	return nil
}

// WriteAnalysis 把 omni 返回 + 解析结果写入 zip：
// segments/slice_xxxxx/{omni_raw_response.txt, analysis.json, dump.md}
// 关键词: zip 写入分析结果, WriteAnalysis
func (a *videoSegmentArchiver) WriteAnalysis(seg *VideoOmniSegmentResult) error {
	if a == nil || seg == nil {
		return nil
	}
	idx := seg.SegmentIndex
	dir := segmentDir(idx)

	// raw response
	if seg.RawText != "" {
		if err := a.WriteFile(dir+"omni_raw_response.txt", []byte(seg.RawText)); err != nil {
			return err
		}
	}

	// analysis.json
	jb, err := json.MarshalIndent(seg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal segment json failed: %w", err)
	}
	if err := a.WriteFile(dir+"analysis.json", jb); err != nil {
		return err
	}

	// dump.md
	if dump := seg.Dump(); dump != "" {
		if err := a.WriteFile(dir+"dump.md", []byte(dump)); err != nil {
			return err
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	entry := a.ensureSegment(idx)
	entry.StartSeconds = seg.StartTime.Seconds()
	entry.EndSeconds = seg.EndTime.Seconds()
	entry.AnalysisJSON = dir + "analysis.json"
	if seg.RawText != "" {
		entry.OmniRawResponse = dir + "omni_raw_response.txt"
	}
	entry.DumpMarkdown = dir + "dump.md"
	entry.Title = seg.Title
	entry.LatencyMs = seg.LatencyMs
	entry.Tags = append([]string{}, seg.Tags...)
	// 关键词: manifest 写入实测 token 用量
	entry.PromptTokens = seg.PromptTokens
	entry.CompletionTokens = seg.CompletionTokens
	entry.TotalTokens = seg.TotalTokens
	entry.TextTokens = seg.TextTokens
	entry.AudioTokens = seg.AudioTokens
	entry.ImageTokens = seg.ImageTokens
	entry.VideoTokens = seg.VideoTokens
	entry.CachedTokens = seg.CachedTokens
	if seg.ErrMsg != "" {
		entry.HasError = true
		entry.ErrorMessage = seg.ErrMsg
	}
	return nil
}

// WriteManifestAndClose 写出 manifest.json + README.md 并关闭底层 zip 与文件。
// 多次调用是幂等的（仅第一次生效）。
// 关键词: 关闭归档器, WriteManifestAndClose
func (a *videoSegmentArchiver) WriteManifestAndClose() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	// 排序 segments 输出
	segs := make([]*archiveSegmentEntry, 0, len(a.segmentsByIdx))
	maxIdx := -1
	for _, v := range a.segmentsByIdx {
		segs = append(segs, v)
		if v.Index > maxIdx {
			maxIdx = v.Index
		}
	}
	// 按 index 升序
	for i := 0; i < len(segs); i++ {
		for j := i + 1; j < len(segs); j++ {
			if segs[j].Index < segs[i].Index {
				segs[i], segs[j] = segs[j], segs[i]
			}
		}
	}
	a.manifest.Segments = segs
	errCount := 0
	for _, s := range segs {
		if s.HasError {
			errCount++
		}
	}
	a.manifest.ErrorCount = errCount

	manifestBytes, mErr := json.MarshalIndent(a.manifest, "", "  ")
	if mErr == nil {
		if w, err := a.writer.Create("manifest.json"); err == nil {
			_, _ = w.Write(manifestBytes)
		}
	} else {
		log.Errorf("marshal manifest failed: %v", mErr)
	}

	readme := buildArchiveReadme(a.manifest)
	if w, err := a.writer.Create("README.md"); err == nil {
		_, _ = w.Write([]byte(readme))
	}

	a.closed = true
	a.mu.Unlock()

	if err := a.writer.Close(); err != nil {
		log.Errorf("close zip writer failed: %v", err)
	}
	if err := a.file.Close(); err != nil {
		log.Errorf("close zip file failed: %v", err)
	}
	log.Infof("video segment archiver closed: %s (segments=%d, errors=%d)", a.zipPath, len(segs), errCount)
	return nil
}

// ZipPath 返回最终 zip 文件路径
// 关键词: archiver zip 路径
func (a *videoSegmentArchiver) ZipPath() string {
	if a == nil {
		return ""
	}
	return a.zipPath
}

// buildArchiveReadme 生成 zip 里的 README.md，便于人类离线查看
// 关键词: zip README 生成
func buildArchiveReadme(m *archiveManifest) string {
	var sb strings.Builder
	sb.WriteString("# Video Omni Knowledge Archive\n\n")
	sb.WriteString(fmt.Sprintf("- source video: `%s`\n", m.SourceVideo))
	sb.WriteString(fmt.Sprintf("- model: `%s`\n", m.Model))
	sb.WriteString(fmt.Sprintf("- kb name: `%s`\n", m.KBName))
	sb.WriteString(fmt.Sprintf("- created at: %s\n", m.CreatedAt))
	sb.WriteString(fmt.Sprintf("- segments: %d\n", len(m.Segments)))
	sb.WriteString(fmt.Sprintf("- error count: %d\n\n", m.ErrorCount))

	sb.WriteString("## Layout\n\n")
	sb.WriteString("```\n")
	sb.WriteString("manifest.json\n")
	sb.WriteString("README.md\n")
	sb.WriteString("segments/\n")
	sb.WriteString("  slice_00000/\n")
	sb.WriteString("    streamcopy.mp4        # original stream-copied slice (best for re-distillation)\n")
	sb.WriteString("    reencoded.mp4         # 720p 2fps slice actually fed to omni model\n")
	sb.WriteString("    omni_raw_response.txt # raw model output\n")
	sb.WriteString("    analysis.json         # parsed VideoOmniSegmentResult\n")
	sb.WriteString("    dump.md               # human readable summary used as RAG entry seed\n")
	sb.WriteString("  slice_00001/...\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## Suggested next steps\n\n")
	sb.WriteString("1. Re-distill skills from `streamcopy.mp4` with a different prompt using `aiforge.AnalyzeVideoOmni` directly.\n")
	sb.WriteString("2. Audit `dump.md` and edit before re-ingesting via `aiforge.BuildKnowledgeFromBytes` or a custom RAG flow.\n")
	sb.WriteString("3. Combine `analysis.json` across segments to build cross-segment storyline graphs.\n\n")

	if len(m.Segments) > 0 {
		sb.WriteString("## Segment index\n\n")
		sb.WriteString("| idx | seconds | title | latency_ms | tokens (in/out/total) | error |\n")
		sb.WriteString("| --- | --- | --- | --- | --- | --- |\n")
		for _, s := range m.Segments {
			title := s.Title
			if title == "" {
				title = "(untitled)"
			}
			errCol := "-"
			if s.HasError {
				errCol = "yes"
			}
			tokenCol := "-"
			if s.TotalTokens > 0 {
				tokenCol = fmt.Sprintf("%d/%d/%d", s.PromptTokens, s.CompletionTokens, s.TotalTokens)
			}
			sb.WriteString(fmt.Sprintf("| %d | %.0f - %.0f | %s | %d | %s | %s |\n",
				s.Index, s.StartSeconds, s.EndSeconds, title, s.LatencyMs, tokenCol, errCol))
		}
	}

	return sb.String()
}
