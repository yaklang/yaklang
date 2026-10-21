package liteforgeapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/utils/ffmpegutils"
	"github.com/yaklang/yaklang/common/utils/imageutils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"golang.org/x/image/draw"
)

// DefaultKnowledgeOmniModel is the AIBalance Qwen3.8 Omni Flash route. Only
// locally configured providers are used; no credentials are embedded here.
const DefaultKnowledgeOmniModel = "qwen3.8-omni-flash"

type OmniModelInfo struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// OmniMediaAnalysisResult keeps source locations with the extracted knowledge.
// Video offsets are relative to the original input, including subdivided clips.
type OmniMediaAnalysisResult struct {
	OmniModelInfo
	Source    string        `json:"source"`
	Page      int           `json:"page,omitempty"`
	StartTime time.Duration `json:"start_time,omitempty"`
	EndTime   time.Duration `json:"end_time,omitempty"`
	Content   string        `json:"content"`
	Fallback  bool          `json:"fallback,omitempty"`
}

func (r *OmniMediaAnalysisResult) GetCumulativeSummary() string { return "" }
func (r *OmniMediaAnalysisResult) Dump() string {
	location := ""
	if r.Page > 0 {
		location = fmt.Sprintf("\nPage: %d", r.Page)
	} else if r.EndTime > 0 {
		location = fmt.Sprintf("\nTime: %s ~ %s", r.StartTime, r.EndTime)
	}
	return fmt.Sprintf("Source: %s%s\n\n%s", r.Source, location, r.Content)
}

type omniMediaSession struct {
	info    OmniModelInfo
	chat    aispec.GeneralChatter
	video   *VideoOmniConfig
	config  *AnalysisConfig
	options []any
	failed  bool
}

// CheckOmniAvailable probes the local Omni configuration with a small image.
// Its return value contains provider/model identifiers, never credentials.
// Example: info = liteforge.CheckOmniAvailable()~
func CheckOmniAvailable(options ...any) (*OmniModelInfo, error) {
	s, err := newOmniMediaSession(NewAnalysisConfig(options...), options)
	if err != nil {
		return nil, err
	}
	return &s.info, nil
}

func newOmniMediaSession(cfg *AnalysisConfig, options []any) (*omniMediaSession, error) {
	v := NewDefaultVideoOmniConfig()
	v.AIType, v.Model = "aibalance", DefaultKnowledgeOmniModel
	v.SegmentSeconds = 20
	// This limit covers the encoded payload, including its data URI prefix.
	v.MaxBase64Bytes = 7 * 1024 * 1024
	v.Ctx = cfg.Ctx
	for _, opt := range options {
		if fn, ok := opt.(VideoOmniOption); ok {
			fn(v)
		}
	}
	if v.Ctx != nil {
		cfg.Ctx = v.Ctx
	}
	if v.Timeout <= 0 || v.SegmentSeconds <= 0 || v.MaxBase64Bytes < 1024 || v.MaxHeight < 2 || v.TargetFPS <= 0 {
		return nil, fmt.Errorf("invalid Omni media timeout, segment duration, payload limit, height or FPS")
	}
	explicit := new(aispec.AIConfig)
	for _, opt := range cfg.OmniAIOptions {
		opt(explicit)
	}
	requestedType := explicit.Type
	if requestedType == "" && v.AIType != "aibalance" {
		requestedType = v.AIType
	}
	if explicit.Model != "" {
		v.Model = explicit.Model
	}
	// A model explicitly configured in Yakit takes precedence over the default
	// AIBalance route. Keep the complete provider options (endpoint/proxy/headers).
	var selected *ypb.AIModelConfig
	var aiOptions []aispec.AIConfigOption
	useLocalProvider := explicit.Type == "" || explicit.APIKey == "" || (explicit.BaseURL == "" && !explicit.EnableEndpoint)
	if useLocalProvider {
		aiconfig.EnsureConfigLoaded()
		manager := aiconfig.GetGlobalManager()
		models := append(append(append([]*ypb.AIModelConfig{}, manager.GetVisionConfigs()...), manager.GetLightweightConfigs()...), manager.GetIntelligentConfigs()...)
		bestScore := 0
		for _, model := range models {
			if model.GetProvider() == nil || (requestedType != "" && model.GetProvider().GetType() != requestedType) {
				continue
			}
			name := strings.ToLower(model.GetModelName())
			score := 0
			switch {
			case strings.EqualFold(name, v.Model):
				score = 3
			case v.Model == DefaultKnowledgeOmniModel && strings.Contains(name, "omni"):
				score = 2
			case model.GetProvider().GetType() == v.AIType || model.GetProvider().GetType() == requestedType:
				score = 1
			}
			if score > bestScore {
				bestScore = score
				selected = model
			}
		}
		if selected != nil {
			v.AIType = selected.GetProvider().GetType()
			if bestScore >= 2 && v.Model == DefaultKnowledgeOmniModel && explicit.Model == "" {
				v.Model = selected.GetModelName()
			}
			aiOptions = aispec.BuildOptionsFromConfig(selected)
		}
	}
	if requestedType != "" {
		v.AIType = requestedType
	}
	if selected == nil && useLocalProvider {
		if provider, err := consts.GetCommonThirdPartyApplicationConfig(v.AIType); err == nil {
			aiOptions = aispec.BuildOptionsFromConfig(&ypb.AIModelConfig{Provider: provider})
		}
	}
	aiOptions = append(aiOptions, aispec.WithType(v.AIType), aispec.WithModel(v.Model))
	if v.APIKey != "" {
		aiOptions = append(aiOptions, aispec.WithAPIKey(v.APIKey))
	}
	if v.BaseURL != "" {
		aiOptions = append(aiOptions, aispec.WithBaseURL(v.BaseURL))
	}
	aiOptions = append(aiOptions, aispec.WithStreamHandler(func(r io.Reader) { io.Copy(io.Discard, r) }))
	aiOptions = append(aiOptions, cfg.OmniAIOptions...)
	resolved := aispec.NewDefaultAIConfig(aiOptions...)
	chatter, err := ai.LoadChater(resolved.Type, aiOptions...)
	if err != nil {
		return nil, err
	}
	s := &omniMediaSession{info: OmniModelInfo{Provider: resolved.Type, Model: resolved.Model}, chat: chatter, video: v, config: cfg, options: options}
	probeCtx, cancel := context.WithTimeout(cfg.Ctx, 15*time.Second)
	defer cancel()
	var probe bytes.Buffer
	if err := png.Encode(&probe, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		return nil, err
	}
	finishProbe := cfg.trackProgress(probeCtx, "Omni availability probe "+s.info.Provider+"/"+s.info.Model, nil)
	response, err := s.chat("Check image input support. Reply with OMNI_READY.", aispec.WithContext(probeCtx), aispec.WithTimeout(15), aispec.WithImageRaw(probe.Bytes()), aispec.WithMaxTokens(32), aispec.WithEnableThinking(false))
	if err == nil && !strings.Contains(response, "OMNI_READY") {
		err = fmt.Errorf("Omni availability probe returned no confirmation")
	}
	finishProbe(err)
	if err != nil {
		return nil, fmt.Errorf("Omni availability probe failed for %s/%s: %w", s.info.Provider, s.info.Model, err)
	}
	return s, nil
}

func tryAnalyzeOmniMedia(path string, mime *mimetype.MIME, cfg *AnalysisConfig, options []any) (<-chan AnalysisResult, bool, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.OmniMediaMode))
	switch mode {
	case "disabled":
		return nil, false, nil
	case "auto", "required":
	default:
		return nil, true, fmt.Errorf("invalid omniMode %q: use auto, required or disabled", mode)
	}
	cfg.OmniMediaMode = mode
	s, err := newOmniMediaSession(cfg, options)
	if err != nil {
		if cfg.Ctx.Err() != nil {
			return nil, true, cfg.Ctx.Err()
		}
		if mode == "required" {
			return nil, true, err
		}
		cfg.AnalyzeLog("Omni unavailable; using original media extraction: %v", err)
		cfg.AnalyzeStatusCard("media extraction", "legacy: Omni unavailable")
		return nil, false, nil
	}
	cfg.AnalyzeLog("using Omni media extraction: %s/%s", s.info.Provider, s.info.Model)
	cfg.AnalyzeStatusCard("media extraction", "omni: "+s.info.Model)
	if mime.IsVideo() {
		ch, err := s.analyzeVideo(path)
		if err != nil && mode == "auto" && cfg.Ctx.Err() == nil {
			cfg.AnalyzeLog("Omni video preparation failed; using original media extraction: %v", err)
			return nil, false, nil
		}
		return ch, true, err
	}
	if mime.IsImage() {
		raw, err := readOmniMediaFile(path, 64*1024*1024)
		if err != nil {
			return nil, true, err
		}
		out := make(chan AnalysisResult, 1)
		go func() {
			defer close(out)
			s.analyzeImage(path, 0, raw, out)
		}()
		return out, true, nil
	}
	pages, err := imageutils.ExtractDocumentPagesContext(cfg.Ctx, path)
	if err != nil {
		if mode == "auto" {
			cfg.AnalyzeLog("Omni PDF preparation failed; using original extraction: %v", err)
			return nil, false, nil
		}
		return nil, true, err
	}
	out := make(chan AnalysisResult, 1)
	go func() {
		defer close(out)
		count := 0
		for page := range pages {
			if cfg.Ctx.Err() != nil {
				continue // drain the producer before returning
			}
			if page.Error != nil {
				cfg.reportError(page.Error)
				continue
			}
			count++
			s.analyzeImage(path, page.PageNumber, page.RawImage, out)
		}
		if count == 0 && cfg.Ctx.Err() == nil {
			cfg.reportError(fmt.Errorf("PDF produced no pages: %s", path))
		}
	}()
	return out, true, nil
}

const omniKnowledgePrompt = `Extract knowledge from this media faithfully for a searchable knowledge base. Preserve visible text, names, numbers, code, diagrams, causal relations and procedures. For video integrate speech, sound and visuals in temporal order. Describe uncertainty explicitly. Return detailed Markdown suitable for retrieval; do not invent missing information.`

type omniResponseCounter struct{ bytes atomic.Int64 }

func (c *omniResponseCounter) Write(p []byte) (int, error) {
	c.bytes.Add(int64(len(p)))
	return len(p), nil
}

func (s *omniMediaSession) call(label, prompt string, media aispec.AIConfigOption) (text string, err error) {
	ctx, cancel := context.WithTimeout(s.config.Ctx, s.video.Timeout)
	defer cancel()
	for attempt := 1; attempt <= 2; attempt++ {
		counter := new(omniResponseCounter)
		finish := s.config.trackProgress(ctx, fmt.Sprintf("Omni %s model=%s attempt=%d/2 timeout=%s", label, s.info.Model, attempt, s.video.Timeout), func() string { return fmt.Sprintf("response_bytes=%d", counter.bytes.Load()) })
		text, err = s.chat(prompt+"\n"+s.config.ExtraPrompt, aispec.WithContext(ctx), aispec.WithTimeout(s.video.Timeout.Seconds()), media, aispec.WithEnableThinking(false), aispec.WithMaxTokens(4096),
			aispec.WithStreamHandler(func(r io.Reader) { io.Copy(counter, r) }))
		if err == nil && strings.TrimSpace(text) == "" {
			err = fmt.Errorf("Omni returned empty media knowledge")
		}
		finish(err)
		if err == nil || ctx.Err() != nil || !isOmniTransientError(err) || attempt == 2 {
			return text, err
		}
		s.config.AnalyzeLog("Omni %s transient transport failure; retrying once within the same %s budget: %v", label, s.video.Timeout, err)
	}
	return text, err
}

func isOmniTransientError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unexpected eof") || strings.Contains(message, "connection reset") || strings.Contains(message, "i/o timeout") || strings.Contains(message, "context deadline exceeded")
}

func (s *omniMediaSession) emit(r *OmniMediaAnalysisResult, out chan<- AnalysisResult) {
	select {
	case out <- r:
	case <-s.config.Ctx.Done():
	}
}

func (s *omniMediaSession) analyzeImage(source string, page int, raw []byte, out chan<- AnalysisResult) {
	r := &OmniMediaAnalysisResult{OmniModelInfo: s.info, Source: source, Page: page}
	compressed, err := boundedOmniImage(raw, s.video.MaxBase64Bytes)
	if err == nil && !s.failed {
		r.Content, err = s.call(fmt.Sprintf("image source=%q page=%d bytes=%d", source, page, len(compressed)), omniKnowledgePrompt, aispec.WithImageRaw(compressed))
	} else if s.failed {
		err = fmt.Errorf("Omni unavailable after a previous media request")
	}
	if err != nil {
		if s.config.Ctx.Err() != nil || s.config.OmniMediaMode == "required" {
			s.config.reportError(fmt.Errorf("Omni image source=%q page=%d failed: %w", source, page, err))
			return
		}
		// A broken stream does not make Omni unavailable for subsequent pages.
		s.failed = !isOmniTransientError(err)
		s.config.AnalyzeLog("Omni image extraction failed; using original vision extraction: %v", err)
		legacy, legacyErr := AnalyzeImage(raw, s.options...)
		if legacyErr != nil {
			s.config.reportError(fmt.Errorf("Omni and original image extraction failed: %w", legacyErr))
			return
		}
		r.Content, r.Fallback = legacy.Dump(), true
	}
	s.emit(r, out)
}

func (s *omniMediaSession) analyzeVideo(path string) (<-chan AnalysisResult, error) {
	tempDir, err := os.MkdirTemp("", "knowledge-omni-video-*")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(s.config.Ctx)
	finish := s.config.trackProgress(ctx, fmt.Sprintf("video extraction (ffmpeg + Omni) source=%q segment_seconds=%g", path, s.video.SegmentSeconds), nil)
	clips, err := s.sliceVideo(ctx, path, tempDir, s.video.SegmentSeconds)
	if err != nil {
		finish(err)
		cancel()
		os.RemoveAll(tempDir)
		return nil, err
	}
	out := make(chan AnalysisResult, 1)
	go func() {
		defer close(out)
		defer func() { finish(s.config.Ctx.Err()) }()
		defer os.RemoveAll(tempDir)
		defer cancel()
		count := 0
		for clip := range clips {
			if ctx.Err() != nil {
				continue
			}
			if clip.Error != nil {
				s.config.reportError(clip.Error)
				continue
			}
			count++
			s.config.AnalyzeLog("video segment %d ready range=%s~%s bytes=%d", count, clip.StartTime, clip.EndTime, clip.SizeBytes)
			if s.video.MaxSegments > 0 && count > s.video.MaxSegments {
				cancel()
				continue
			}
			s.analyzeClip(path, clip, 0, 0, out)
		}
		if count == 0 && s.config.Ctx.Err() == nil {
			s.config.reportError(fmt.Errorf("video produced no clips: %s", path))
		}
	}()
	return out, nil
}

func (s *omniMediaSession) sliceVideo(ctx context.Context, path, dir string, seconds float64) (<-chan *ffmpegutils.VideoSliceResult, error) {
	return ffmpegutils.ExtractVideoSliceFromVideo(path, ffmpegutils.WithContext(ctx), ffmpegutils.WithSliceOutputDir(dir),
		ffmpegutils.WithSliceDurationSeconds(seconds), ffmpegutils.WithSliceReencode(true),
		ffmpegutils.WithSliceMaxHeight(s.video.MaxHeight), ffmpegutils.WithSliceTargetFPS(s.video.TargetFPS))
}

func (s *omniMediaSession) analyzeClip(source string, clip *ffmpegutils.VideoSliceResult, offset time.Duration, depth int, out chan<- AnalysisResult) {
	if s.config.Ctx.Err() != nil {
		return
	}
	// Split oversized clips again before allocating or base64 encoding them.
	if omniEncodedSize(clip.SizeBytes, len("data:;base64,")) > s.video.MaxBase64Bytes && depth < 8 && clip.EndTime-clip.StartTime > time.Second {
		dir, err := os.MkdirTemp("", "knowledge-omni-subclip-*")
		if err == nil {
			defer os.RemoveAll(dir)
			seconds := (clip.EndTime - clip.StartTime).Seconds() / 2
			s.config.AnalyzeLog("video segment range=%s~%s exceeds encoded payload limit=%d; subdividing to %gs", offset+clip.StartTime, offset+clip.EndTime, s.video.MaxBase64Bytes, seconds)
			subclips, splitErr := s.sliceVideo(s.config.Ctx, clip.FilePath, dir, seconds)
			if splitErr == nil {
				for sub := range subclips {
					if sub.Error != nil {
						s.config.reportError(sub.Error)
						continue
					}
					s.analyzeClip(source, sub, offset+clip.StartTime, depth+1, out)
				}
				return
			}
		}
	}
	r := &OmniMediaAnalysisResult{OmniModelInfo: s.info, Source: source, StartTime: offset + clip.StartTime, EndTime: offset + clip.EndTime}
	rawLimit := (s.video.MaxBase64Bytes - int64(len("data:;base64,"))) / 4 * 3
	raw, err := readOmniMediaFile(clip.FilePath, rawLimit)
	if err == nil && !s.failed {
		dataURI := "data:;base64," + base64.StdEncoding.EncodeToString(raw)
		r.Content, err = s.call(fmt.Sprintf("video range=%s~%s payload_bytes=%d", r.StartTime, r.EndTime, len(dataURI)), omniKnowledgePrompt, aispec.WithVideoUrl(dataURI))
	} else if s.failed {
		err = fmt.Errorf("Omni unavailable after a previous media request")
	}
	if err != nil {
		if s.config.Ctx.Err() != nil || s.config.OmniMediaMode == "required" {
			s.config.reportError(fmt.Errorf("Omni video range=%s~%s failed: %w", r.StartTime, r.EndTime, err))
			return
		}
		// Prefer Omni again for the next clip after a transient transport failure.
		s.failed = !isOmniTransientError(err)
		s.config.AnalyzeLog("Omni video extraction failed; using original extraction for clip %s~%s: %v", r.StartTime, r.EndTime, err)
		omniErr := err
		legacyOptions := append(append([]any(nil), s.options...), WithAnalyzeErrorHandler(func(cause error) {
			s.config.reportError(fmt.Errorf("video range=%s~%s: Omni failed: %v; original extraction failed: %w", r.StartTime, r.EndTime, omniErr, cause))
		}))
		legacy, legacyErr := AnalyzeVideo(clip.FilePath, legacyOptions...)
		if legacyErr != nil {
			s.config.reportError(fmt.Errorf("video range=%s~%s: Omni failed: %v; original extraction failed: %w", r.StartTime, r.EndTime, err, legacyErr))
			return
		}
		count := 0
		for item := range legacy {
			count++
			copy := *r
			copy.Content, copy.Fallback = item.Dump(), true
			s.emit(&copy, out)
		}
		if count == 0 && s.config.Ctx.Err() == nil {
			s.config.reportError(fmt.Errorf("original video extraction produced no knowledge for %s~%s after Omni failed: %w; see original extraction logs for ffmpeg/audio errors", r.StartTime, r.EndTime, err))
		}
		return
	}
	s.emit(r, out)
}

func readOmniMediaFile(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err == nil && int64(len(raw)) > maxBytes {
		err = fmt.Errorf("media exceeds payload limit: %s", path)
	}
	return raw, err
}

func omniEncodedSize(rawBytes int64, prefixBytes int) int64 {
	return (rawBytes+2)/3*4 + int64(prefixBytes)
}

func boundedOmniImage(raw []byte, maxBytes int64) ([]byte, error) {
	info, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if info.Width <= 0 || info.Height <= 0 || int64(info.Width)*int64(info.Height) > 40_000_000 {
		return nil, fmt.Errorf("image dimensions exceed Omni preprocessing limit")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	for edge := 2048; edge >= 128; edge /= 2 {
		width, height := info.Width, info.Height
		if width > edge || height > edge {
			if width >= height {
				height, width = max(1, height*edge/width), edge
			} else {
				width, height = max(1, width*edge/height), edge
			}
		}
		resized := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(resized, resized.Bounds(), img, img.Bounds(), draw.Over, nil)
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, resized, &jpeg.Options{Quality: 85}); err != nil {
			return nil, err
		}
		if omniEncodedSize(int64(encoded.Len()), len("data:image/jpeg;base64,")) <= maxBytes {
			return encoded.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("image cannot fit Omni payload limit of %d bytes", maxBytes)
}
