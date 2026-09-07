package scannode

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	ppath "path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
)

type SSAArtifactUploadConfig struct {
	ObjectKey string
	Codec     string // zstd | gzip | identity

	Endpoint string
	Bucket   string
	Region   string
	UseSSL   bool
	// TLSVerify is retained for input compatibility. Object-store TLS is
	// verified by default; use AllowInsecureTLS only for an explicitly trusted
	// development endpoint, or configure TLSCAFile for a private CA.
	TLSVerify        bool
	AllowInsecureTLS bool
	TLSCAFile        string
	AllowHTTP        bool
	VirtualHostStyle bool

	// These exported string fields preserve the pre-existing Go API for callers
	// that build or inspect configs with dynamic strings. String, GoString, and
	// JSON formatting on the containing config redact them; request signing
	// converts them to the internal redacting value.
	STSAccessKey    string `json:"-"`
	STSSecretKey    string `json:"-"`
	STSSessionToken string `json:"-"`
	STSExpiresAt    int64
}

func (cfg *SSAArtifactUploadConfig) accessKeySecret() secretValue {
	if cfg == nil {
		return ""
	}
	return newSecretValue(cfg.STSAccessKey)
}

func (cfg *SSAArtifactUploadConfig) secretKeySecret() secretValue {
	if cfg == nil {
		return ""
	}
	return newSecretValue(cfg.STSSecretKey)
}

func (cfg *SSAArtifactUploadConfig) sessionTokenSecret() secretValue {
	if cfg == nil {
		return ""
	}
	return newSecretValue(cfg.STSSessionToken)
}

func (cfg *SSAArtifactUploadConfig) setSTSCredentials(accessKey, secretKey, sessionToken string) {
	if cfg == nil {
		return
	}
	cfg.STSAccessKey = accessKey
	cfg.STSSecretKey = secretKey
	cfg.STSSessionToken = sessionToken
}

func (cfg SSAArtifactUploadConfig) String() string {
	return fmt.Sprintf("SSAArtifactUploadConfig{ObjectKey:%q Codec:%q Endpoint:%q Bucket:%q Region:%q UseSSL:%t TLSVerify:%t AllowInsecureTLS:%t TLSCAFile:%q AllowHTTP:%t VirtualHostStyle:%t STSAccessKey:[REDACTED] STSSecretKey:[REDACTED] STSSessionToken:[REDACTED] STSExpiresAt:%d}",
		cfg.ObjectKey, cfg.Codec, cfg.Endpoint, cfg.Bucket, cfg.Region, cfg.UseSSL, cfg.TLSVerify, cfg.AllowInsecureTLS, cfg.TLSCAFile, cfg.AllowHTTP, cfg.VirtualHostStyle, cfg.STSExpiresAt)
}

func (cfg SSAArtifactUploadConfig) GoString() string { return cfg.String() }

// ssaUploadMetrics accumulates upload-phase observability data across the
// collector's lifetime. All fields are accessed under the collector's mutex.
type ssaUploadMetrics struct {
	TotalUploadMs   int64  `json:"total_upload_ms"`
	TicketFetchMs   int64  `json:"ticket_fetch_ms"`
	Segments        int    `json:"segments"`
	Retries         int    `json:"retries"`
	RawBytes        uint64 `json:"raw_bytes"`
	CompressedBytes uint64 `json:"compressed_bytes"`
}

type SSAArtifactBuildResult struct {
	ObjectKey        string
	Codec            string
	ArtifactPath     string
	ArtifactFormat   string
	UncompressedSize int64
	CompressedSize   int64
	SHA256           string
	ProgramName      string
	ReportType       string
	RiskCount        int64
	FileCount        int64
	FlowCount        int64
}

type SSAArtifactCollector struct {
	mu sync.Mutex

	taskID    string
	runtimeID string
	subTaskID string

	startedAt   time.Time
	programName string
	reportType  string

	spoolDir  string
	partsPath string
	partsFile *os.File

	riskCount int64
	fileCount int64
	flowCount int64
	rawBytes  int64

	hasData bool
	initErr error

	continuousEnabled       bool
	continuousCodec         string
	continuousProvider      ssaUploadConfigProvider
	continuousStarted       bool
	continuousInput         chan []byte
	continuousDone          chan struct{}
	continuousClosed        bool
	continuousErr           error
	continuousBuild         *SSAArtifactBuildResult
	continuousFlushInterval time.Duration
	uploadCtx               context.Context

	uploadMetrics ssaUploadMetrics
}

const (
	defaultSSAMultipartPartSizeBytes = 16 * 1024 * 1024
	minSSAMultipartPartSizeBytes     = 5 * 1024 * 1024
	maxSSAMultipartPartSizeBytes     = 128 * 1024 * 1024
	defaultSSAMultipartConcurrency   = 2
	maxSSAMultipartConcurrency       = 2
)

type ssaUploadConfigProvider func(force bool) (*SSAArtifactUploadConfig, error)

func normalizeArtifactCodec(codec string) string {
	actualCodec := strings.ToLower(strings.TrimSpace(codec))
	if actualCodec == "" {
		return "zstd"
	}
	return actualCodec
}

func (c *SSAArtifactCollector) EnableContinuousUpload(codec string, provider ssaUploadConfigProvider) error {
	return c.EnableContinuousUploadWithFlushInterval(codec, provider, 0)
}

func (c *SSAArtifactCollector) EnableContinuousUploadWithFlushInterval(
	codec string,
	provider ssaUploadConfigProvider,
	flushInterval time.Duration,
) error {
	if c == nil {
		return utils.Errorf("collector is nil")
	}
	if provider == nil {
		return utils.Errorf("empty upload config provider")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.continuousEnabled = true
	c.continuousCodec = normalizeArtifactCodec(codec)
	c.continuousProvider = provider
	c.continuousFlushInterval = flushInterval
	return nil
}

func (c *SSAArtifactCollector) startContinuousUploadIfNeeded() error {
	c.mu.Lock()
	if !c.continuousEnabled || c.continuousStarted {
		c.mu.Unlock()
		return c.continuousErr
	}
	codec := c.continuousCodec
	if codec == "" {
		codec = "zstd"
	}
	provider := c.continuousProvider
	ctx := c.uploadCtx
	if ctx == nil {
		ctx = context.Background()
	}
	flushInterval := c.continuousFlushInterval
	taskID := c.taskID
	programName := c.programName
	reportType := c.reportType
	if provider == nil {
		c.mu.Unlock()
		return utils.Errorf("continuous upload provider is nil")
	}
	input := make(chan []byte, 256)
	done := make(chan struct{})
	c.continuousInput = input
	c.continuousDone = done
	c.continuousStarted = true
	c.mu.Unlock()

	go func() {
		build, err := runContinuousSegmentedUpload(ctx, codec, flushInterval, provider, taskID, programName, reportType, input, func(uploadMs int64) {
			c.recordUploadMs(uploadMs)
			c.recordSegment()
		})
		c.mu.Lock()
		c.continuousBuild = build
		c.continuousErr = err
		if build != nil {
			c.setUploadBytesLocked(uint64(build.UncompressedSize), uint64(build.CompressedSize))
		}
		close(done)
		c.mu.Unlock()
	}()
	return nil
}

func (c *SSAArtifactCollector) enqueueContinuousPayload(payload []byte) error {
	c.mu.Lock()
	input := c.continuousInput
	done := c.continuousDone
	err := c.continuousErr
	closed := c.continuousClosed
	c.mu.Unlock()

	if err != nil {
		return err
	}
	if closed || input == nil || done == nil {
		return nil
	}

	if queueCap := cap(input); queueCap > 0 {
		if queueLen := len(input); queueLen >= queueCap*9/10 {
			log.Warnf("upload_queue_backlog task=%s len=%d cap=%d", c.taskID, queueLen, queueCap)
		}
	}

	select {
	case input <- payload:
		return nil
	case <-c.uploadContext().Done():
		return c.uploadContext().Err()
	case <-done:
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.continuousErr != nil {
			return c.continuousErr
		}
		return utils.Errorf("continuous upload exited unexpectedly")
	}
}

func (c *SSAArtifactCollector) uploadContext() context.Context {
	if c == nil || c.uploadCtx == nil {
		return context.Background()
	}
	return c.uploadCtx
}

func NewSSAArtifactCollector(taskID, runtimeID, subTaskID string) *SSAArtifactCollector {
	return NewSSAArtifactCollectorWithContext(context.Background(), taskID, runtimeID, subTaskID)
}

func NewSSAArtifactCollectorWithContext(ctx context.Context, taskID, runtimeID, subTaskID string) *SSAArtifactCollector {
	if ctx == nil {
		ctx = context.Background()
	}
	c := &SSAArtifactCollector{
		taskID:    taskID,
		runtimeID: runtimeID,
		subTaskID: subTaskID,
		startedAt: time.Now(),
		uploadCtx: ctx,
	}
	if err := c.initSpoolLocked(); err != nil {
		c.initErr = err
	}
	return c
}

func (c *SSAArtifactCollector) recordUploadMs(ms int64) {
	if c == nil || ms <= 0 {
		return
	}
	c.mu.Lock()
	c.uploadMetrics.TotalUploadMs += ms
	c.mu.Unlock()
}

func (c *SSAArtifactCollector) recordTicketFetchMs(ms int64) {
	if c == nil || ms <= 0 {
		return
	}
	c.mu.Lock()
	c.uploadMetrics.TicketFetchMs += ms
	c.mu.Unlock()
}

func (c *SSAArtifactCollector) recordRetry() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.uploadMetrics.Retries++
	c.mu.Unlock()
}

func (c *SSAArtifactCollector) recordSegment() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.uploadMetrics.Segments++
	c.mu.Unlock()
}

func (c *SSAArtifactCollector) setUploadBytes(raw, compressed uint64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.setUploadBytesLocked(raw, compressed)
	c.mu.Unlock()
}

func (c *SSAArtifactCollector) setUploadBytesLocked(raw, compressed uint64) {
	if c == nil {
		return
	}
	c.uploadMetrics.RawBytes = raw
	c.uploadMetrics.CompressedBytes = compressed
}

func (c *SSAArtifactCollector) snapshotUploadMetrics() ssaUploadMetrics {
	if c == nil {
		return ssaUploadMetrics{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.uploadMetrics
}

func (c *SSAArtifactCollector) initSpoolLocked() error {
	if c.partsFile != nil {
		return nil
	}
	prefix := sanitizePathComponent(c.taskID)
	if prefix == "" {
		prefix = "task"
	}
	dir, err := os.MkdirTemp("", fmt.Sprintf("ssa-artifact-%s-", prefix))
	if err != nil {
		return err
	}
	partsPath := filepath.Join(dir, "ssa_result_parts.ndjson")
	f, err := os.OpenFile(partsPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	c.spoolDir = dir
	c.partsPath = partsPath
	c.partsFile = f
	return nil
}

func (c *SSAArtifactCollector) AddStreamPayload(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	line := append([]byte(raw), '\n')
	copyLine := append([]byte{}, line...)

	var parts sfreport.SSAResultParts
	if err := json.Unmarshal([]byte(raw), &parts); err != nil {
		return err
	}

	c.mu.Lock()
	if c.initErr != nil {
		c.mu.Unlock()
		return c.initErr
	}
	if err := c.initSpoolLocked(); err != nil {
		c.initErr = err
		c.mu.Unlock()
		return err
	}

	if c.programName == "" {
		c.programName = strings.TrimSpace(parts.ProgramName)
	}
	if c.reportType == "" {
		c.reportType = strings.TrimSpace(parts.ReportType)
	}

	c.riskCount += int64(len(parts.Risks))
	c.fileCount += int64(len(parts.Files))
	c.flowCount += int64(len(parts.Dataflows))
	c.rawBytes += int64(len(line))
	if len(parts.Risks) > 0 || len(parts.Files) > 0 || len(parts.Dataflows) > 0 {
		c.hasData = true
	}

	if _, err := c.partsFile.Write(line); err != nil {
		c.mu.Unlock()
		return err
	}
	if err := c.continuousErr; err != nil {
		c.mu.Unlock()
		return err
	}

	needStart := c.continuousEnabled && !c.continuousStarted
	enabled := c.continuousEnabled
	c.mu.Unlock()

	if needStart {
		if err := c.startContinuousUploadIfNeeded(); err != nil {
			return err
		}
	}
	if enabled {
		if err := c.enqueueContinuousPayload(copyLine); err != nil {
			return err
		}
	}
	return nil
}

func (c *SSAArtifactCollector) HasData() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasData
}

func (c *SSAArtifactCollector) FinalizeUploadWithProvider(codec string, provider ssaUploadConfigProvider) (*SSAArtifactBuildResult, error) {
	return c.FinalizeUploadWithProviderContext(context.Background(), codec, provider)
}

func (c *SSAArtifactCollector) FinalizeUploadWithProviderContext(ctx context.Context, codec string, provider ssaUploadConfigProvider) (*SSAArtifactBuildResult, error) {
	if c == nil {
		return nil, utils.Errorf("collector is nil")
	}
	if ctx == nil {
		ctx = c.uploadContext()
	}
	c.mu.Lock()
	hasData := c.hasData
	continuousEnabled := c.continuousEnabled
	started := c.continuousStarted
	if c.continuousEnabled && provider != nil {
		c.continuousProvider = provider
	}
	if c.continuousCodec == "" {
		c.continuousCodec = normalizeArtifactCodec(codec)
	}
	input := c.continuousInput
	done := c.continuousDone
	if continuousEnabled && started && !c.continuousClosed && input != nil {
		close(input)
		c.continuousClosed = true
	}
	c.mu.Unlock()

	if !hasData {
		// No stream payload was produced (e.g. zero findings, rule groups yield nothing).
		// Still upload an "empty" artifact so the Server can receive SSAArtifactReady and
		// finalize the task (otherwise it may get stuck at "finalizing").
		//
		// We intentionally fall back to the single-object parts format here. The Server
		// currently rejects an empty segments manifest.
		//
		// IMPORTANT: for some codecs (e.g. zstd), compressing an empty file may yield
		// a zero-byte stream which breaks multipart upload. Ensure the parts file has at
		// least one valid JSON object ("{}") so the artifact is non-empty and importable.
		c.mu.Lock()
		if c.initErr != nil {
			err := c.initErr
			c.mu.Unlock()
			return nil, err
		}
		if err := c.initSpoolLocked(); err != nil {
			c.initErr = err
			c.mu.Unlock()
			return nil, err
		}
		if c.partsFile != nil {
			_, err := c.partsFile.Write([]byte("{}\n"))
			if err != nil {
				c.mu.Unlock()
				return nil, err
			}
			c.rawBytes += int64(len("{}\n"))
			_ = c.partsFile.Sync()
		}
		c.mu.Unlock()
		return c.BuildAndUploadCompressedArtifactWithProviderContext(ctx, codec, provider)
	}

	if continuousEnabled && started && done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		c.mu.Lock()
		err := c.continuousErr
		base := c.continuousBuild
		var result *SSAArtifactBuildResult
		if base != nil {
			cp := *base
			result = &cp
		} else {
			result = &SSAArtifactBuildResult{}
		}
		result.ProgramName = c.programName
		result.ReportType = c.reportType
		result.RiskCount = c.riskCount
		result.FileCount = c.fileCount
		result.FlowCount = c.flowCount
		if result.UncompressedSize <= 0 {
			result.UncompressedSize = c.rawBytes
		}
		c.mu.Unlock()
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(result.ArtifactFormat) == "" {
			result.ArtifactFormat = spec.SSAArtifactFormatPartsNDJSONV1
		}
		if strings.TrimSpace(result.Codec) == "" {
			result.Codec = normalizeArtifactCodec(codec)
		}
		if strings.TrimSpace(result.ReportType) == "" {
			result.ReportType = string(sfreport.IRifyFullReportType)
		}
		return result, nil
	}

	return c.BuildAndUploadCompressedArtifactWithProviderContext(ctx, codec, provider)
}

const (
	defaultSSASegmentMaxBytes int64 = 8 * 1024 * 1024
	defaultSSASegmentFlushSec       = 20
)

func readSSASegmentMaxBytes() int64 {
	raw := strings.TrimSpace(os.Getenv("SCANNODE_SSA_SEGMENT_MAX_MB"))
	if raw == "" {
		return defaultSSASegmentMaxBytes
	}
	mb, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || mb <= 0 {
		return defaultSSASegmentMaxBytes
	}
	return mb * 1024 * 1024
}

func readSSASegmentFlushInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("SCANNODE_SSA_SEGMENT_FLUSH_SEC"))
	if raw == "" {
		return time.Duration(defaultSSASegmentFlushSec) * time.Second
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec <= 0 {
		return time.Duration(defaultSSASegmentFlushSec) * time.Second
	}
	return time.Duration(sec) * time.Second
}

func runContinuousSegmentedUpload(ctx context.Context, codec string, customFlushInterval time.Duration, provider ssaUploadConfigProvider, taskID string, programName string, reportType string, input <-chan []byte, onSegment func(uploadMs int64)) (*SSAArtifactBuildResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if provider == nil {
		return nil, utils.Errorf("empty upload config provider")
	}
	baseCfg, err := provider(false)
	if err != nil {
		return nil, err
	}
	if _, err := validateSSAUploadConfig(baseCfg); err != nil {
		return nil, err
	}
	session, err := newSSAObjectStoreUploadSession(provider, nil)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	segmentPrefix, manifestKey := deriveSSAContinuousObjectKeys(strings.TrimSpace(baseCfg.ObjectKey))
	segmentMaxBytes := readSSASegmentMaxBytes()
	if segmentMaxBytes <= 0 {
		segmentMaxBytes = defaultSSASegmentMaxBytes
	}
	flushInterval := customFlushInterval
	if flushInterval <= 0 {
		flushInterval = readSSASegmentFlushInterval()
	}
	if flushInterval <= 0 {
		flushInterval = time.Duration(defaultSSASegmentFlushSec) * time.Second
	}
	uploadCodec := normalizeArtifactCodec(codec)

	tmpDir, err := os.MkdirTemp("", "ssa-continuous-segments-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	openRawSegment := func(seq int) (*os.File, string, error) {
		rawPath := filepath.Join(tmpDir, fmt.Sprintf("segment-%06d.ndjson", seq))
		f, err := os.OpenFile(rawPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return nil, "", err
		}
		return f, rawPath, nil
	}

	var (
		seq                   = 1
		totalRawBytes   int64 = 0
		totalCompressed       = int64(0)
		segments              = make([]spec.SSAArtifactSegment, 0, 128)
		rawFile         *os.File
		rawPath         string
		rawBytes        int64
	)
	rawFile, rawPath, err = openRawSegment(seq)
	if err != nil {
		return nil, err
	}
	defer func() {
		if rawFile != nil {
			_ = rawFile.Close()
		}
	}()

	flushSegment := func() error {
		if rawBytes <= 0 {
			return nil
		}
		if rawFile != nil {
			if err := rawFile.Close(); err != nil {
				return err
			}
			rawFile = nil
		}
		compPath := rawPath + "." + codecExt(uploadCodec)
		compressedSize, compressedSHA, err := compressSSAArtifactFile(rawPath, compPath, uploadCodec)
		if err != nil {
			return err
		}
		segmentKey := ppath.Join(segmentPrefix, fmt.Sprintf("segment-%06d.ndjson.%s", seq, codecExt(uploadCodec)))
		uploadStart := time.Now()
		if _, err := session.uploadFile(ctx, compPath, compressedSize, segmentKey); err != nil {
			return err
		}
		uploadMS := time.Since(uploadStart).Milliseconds()
		log.Infof("ssa artifact segment uploaded task=%s seq=%d key=%s codec=%s raw=%d compressed=%d upload_ms=%d",
			taskID, seq, segmentKey, uploadCodec, rawBytes, compressedSize, uploadMS)
		if onSegment != nil {
			onSegment(uploadMS)
		}
		segments = append(segments, spec.SSAArtifactSegment{
			Seq:              seq,
			ObjectKey:        segmentKey,
			Codec:            uploadCodec,
			CompressedSize:   compressedSize,
			UncompressedSize: rawBytes,
			UploadMS:         uploadMS,
			SHA256:           compressedSHA,
		})
		totalRawBytes += rawBytes
		totalCompressed += compressedSize
		_ = os.Remove(rawPath)
		_ = os.Remove(compPath)
		seq++
		rawBytes = 0
		var openErr error
		rawFile, rawPath, openErr = openRawSegment(seq)
		return openErr
	}

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case chunk, ok := <-input:
			if !ok {
				if err := flushSegment(); err != nil {
					return nil, err
				}
				break loop
			}
			if len(chunk) == 0 {
				continue
			}
			if rawFile == nil {
				var openErr error
				rawFile, rawPath, openErr = openRawSegment(seq)
				if openErr != nil {
					return nil, openErr
				}
			}
			n, err := rawFile.Write(chunk)
			if err != nil {
				return nil, err
			}
			rawBytes += int64(n)
			if rawBytes >= segmentMaxBytes {
				if err := flushSegment(); err != nil {
					return nil, err
				}
			}
		case <-ticker.C:
			if rawBytes > 0 {
				if err := flushSegment(); err != nil {
					return nil, err
				}
			}
		}
	}

	if len(segments) == 0 {
		return nil, utils.Errorf("no continuous segment uploaded")
	}

	manifest := &spec.SSAArtifactManifestV1{
		Version:               "v1",
		Format:                spec.SSAArtifactFormatSegmentsManifestV1,
		TaskID:                strings.TrimSpace(taskID),
		ProgramName:           strings.TrimSpace(programName),
		ReportType:            strings.TrimSpace(reportType),
		TotalSegments:         len(segments),
		TotalCompressedSize:   totalCompressed,
		TotalUncompressedSize: totalRawBytes,
		Segments:              segments,
		ProducedAt:            time.Now().Unix(),
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	if _, err := session.uploadBytes(ctx, manifestRaw, manifestKey); err != nil {
		return nil, err
	}
	log.Infof("ssa artifact manifest uploaded task=%s key=%s segments=%d total_raw=%d total_compressed=%d",
		taskID, manifestKey, len(segments), totalRawBytes, totalCompressed)
	manifestSHA := sha256.Sum256(manifestRaw)
	return &SSAArtifactBuildResult{
		ObjectKey:        manifestKey,
		Codec:            "identity",
		ArtifactPath:     "",
		ArtifactFormat:   spec.SSAArtifactFormatSegmentsManifestV1,
		UncompressedSize: int64(len(manifestRaw)),
		CompressedSize:   int64(len(manifestRaw)),
		SHA256:           hex.EncodeToString(manifestSHA[:]),
	}, nil
}

func deriveSSAContinuousObjectKeys(baseObjectKey string) (segmentPrefix string, manifestKey string) {
	key := strings.Trim(strings.TrimSpace(baseObjectKey), "/")
	if key == "" {
		key = "ssa/tasks/unknown/ssa_result_parts.ndjson.zst"
	}
	dir := ppath.Dir(key)
	if dir == "." || dir == "/" {
		dir = ""
	}
	return ppath.Join(dir, "segments"), ppath.Join(dir, "manifest.json")
}

func compressSSAArtifactFile(rawPath, compressedPath, codec string) (int64, string, error) {
	in, err := os.Open(rawPath)
	if err != nil {
		return 0, "", err
	}
	defer in.Close()

	out, err := os.OpenFile(compressedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, "", err
	}
	defer out.Close()

	hasher := sha256.New()
	dst := io.MultiWriter(out, hasher)
	switch normalizeArtifactCodec(codec) {
	case "zstd":
		zw, err := zstd.NewWriter(dst, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			return 0, "", err
		}
		if _, err := io.Copy(zw, in); err != nil {
			_ = zw.Close()
			return 0, "", err
		}
		if err := zw.Close(); err != nil {
			return 0, "", err
		}
	case "gzip":
		gw := gzip.NewWriter(dst)
		if _, err := io.Copy(gw, in); err != nil {
			_ = gw.Close()
			return 0, "", err
		}
		if err := gw.Close(); err != nil {
			return 0, "", err
		}
	case "identity":
		if _, err := io.Copy(dst, in); err != nil {
			return 0, "", err
		}
	default:
		return 0, "", utils.Errorf("unsupported artifact codec: %s", codec)
	}
	if err := out.Sync(); err != nil {
		return 0, "", err
	}
	st, err := out.Stat()
	if err != nil {
		return 0, "", err
	}
	return st.Size(), hex.EncodeToString(hasher.Sum(nil)), nil
}

func uploadSSAArtifactFileWithObjectKey(ctx context.Context, path string, size int64, objectKey string, provider ssaUploadConfigProvider) error {
	tmp := &SSAArtifactCollector{}
	return tmp.UploadBySTSWithProviderContext(ctx, path, size, func(force bool) (*SSAArtifactUploadConfig, error) {
		cfg, err := provider(force)
		if err != nil {
			return nil, err
		}
		cp := *cfg
		cp.ObjectKey = strings.TrimSpace(objectKey)
		return &cp, nil
	})
}

func uploadSSAArtifactBytesWithObjectKey(ctx context.Context, payload []byte, objectKey string, provider ssaUploadConfigProvider) error {
	tmpFile, err := os.CreateTemp("", "ssa-manifest-*.json")
	if err != nil {
		return err
	}
	path := tmpFile.Name()
	defer os.Remove(path)
	if _, err := tmpFile.Write(payload); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return uploadSSAArtifactFileWithObjectKey(ctx, path, int64(len(payload)), objectKey, provider)
}

func (c *SSAArtifactCollector) BuildCompressedArtifact(codec string) (*SSAArtifactBuildResult, error) {
	c.mu.Lock()
	if c.initErr != nil {
		err := c.initErr
		c.mu.Unlock()
		return nil, err
	}
	if err := c.initSpoolLocked(); err != nil {
		c.initErr = err
		c.mu.Unlock()
		return nil, err
	}
	if c.partsFile != nil {
		_ = c.partsFile.Sync()
	}
	partsPath := c.partsPath
	spoolDir := c.spoolDir
	programName := c.programName
	reportType := c.reportType
	riskCount := c.riskCount
	fileCount := c.fileCount
	flowCount := c.flowCount
	if strings.TrimSpace(reportType) == "" {
		reportType = string(sfreport.IRifyFullReportType)
	}
	c.mu.Unlock()

	in, err := os.Open(partsPath)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	stat, err := in.Stat()
	if err != nil {
		return nil, err
	}
	uncompressedSize := stat.Size()

	artifactPath := filepath.Join(spoolDir, fmt.Sprintf("artifact.%s", codecExt(codec)))
	out, err := os.OpenFile(artifactPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	defer out.Close()

	h := sha256.New()
	dst := io.MultiWriter(out, h)
	actualCodec := normalizeArtifactCodec(codec)

	switch actualCodec {
	case "zstd":
		zw, err := zstd.NewWriter(dst, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(zw, in); err != nil {
			_ = zw.Close()
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
	case "gzip":
		gw := gzip.NewWriter(dst)
		if _, err := io.Copy(gw, in); err != nil {
			_ = gw.Close()
			return nil, err
		}
		if err := gw.Close(); err != nil {
			return nil, err
		}
	case "identity":
		if _, err := io.Copy(dst, in); err != nil {
			return nil, err
		}
	default:
		return nil, utils.Errorf("unsupported artifact codec: %s", codec)
	}

	if err := out.Sync(); err != nil {
		return nil, err
	}
	outStat, err := out.Stat()
	if err != nil {
		return nil, err
	}

	return &SSAArtifactBuildResult{
		ObjectKey:        "",
		Codec:            actualCodec,
		ArtifactPath:     artifactPath,
		ArtifactFormat:   spec.SSAArtifactFormatPartsNDJSONV1,
		UncompressedSize: uncompressedSize,
		CompressedSize:   outStat.Size(),
		SHA256:           hex.EncodeToString(h.Sum(nil)),
		ProgramName:      programName,
		ReportType:       reportType,
		RiskCount:        riskCount,
		FileCount:        fileCount,
		FlowCount:        flowCount,
	}, nil
}

func (c *SSAArtifactCollector) BuildAndUploadCompressedArtifactWithProvider(codec string, provider ssaUploadConfigProvider) (*SSAArtifactBuildResult, error) {
	return c.BuildAndUploadCompressedArtifactWithProviderContext(context.Background(), codec, provider)
}

func (c *SSAArtifactCollector) BuildAndUploadCompressedArtifactWithProviderContext(ctx context.Context, codec string, provider ssaUploadConfigProvider) (*SSAArtifactBuildResult, error) {
	if provider == nil {
		return nil, utils.Errorf("empty upload config provider")
	}
	if ctx == nil {
		ctx = c.uploadContext()
	}
	c.mu.Lock()
	if c.initErr != nil {
		err := c.initErr
		c.mu.Unlock()
		return nil, err
	}
	if err := c.initSpoolLocked(); err != nil {
		c.initErr = err
		c.mu.Unlock()
		return nil, err
	}
	if c.partsFile != nil {
		_ = c.partsFile.Sync()
	}
	partsPath := c.partsPath
	programName := c.programName
	reportType := c.reportType
	riskCount := c.riskCount
	fileCount := c.fileCount
	flowCount := c.flowCount
	if strings.TrimSpace(reportType) == "" {
		reportType = string(sfreport.IRifyFullReportType)
	}
	c.mu.Unlock()

	in, err := os.Open(partsPath)
	if err != nil {
		return nil, err
	}
	stat, err := in.Stat()
	if err != nil {
		_ = in.Close()
		return nil, err
	}
	uncompressedSize := stat.Size()

	cfg, err := provider(false)
	if err != nil {
		_ = in.Close()
		return nil, err
	}
	if _, err := validateSSAUploadConfig(cfg); err != nil {
		_ = in.Close()
		return nil, err
	}
	objectKey := strings.TrimSpace(cfg.ObjectKey)
	actualCodec := normalizeArtifactCodec(codec)
	session, err := newSSAObjectStoreUploadSession(provider, c.recordRetry)
	if err != nil {
		_ = in.Close()
		return nil, err
	}
	defer session.Close()

	pipeReader, pipeWriter := io.Pipe()
	compressErrCh := make(chan error, 1)
	go func() {
		defer in.Close()
		var copyErr error
		switch actualCodec {
		case "zstd":
			writer, writerErr := zstd.NewWriter(pipeWriter, zstd.WithEncoderLevel(zstd.SpeedDefault))
			if writerErr != nil {
				_ = pipeWriter.CloseWithError(writerErr)
				compressErrCh <- writerErr
				return
			}
			_, copyErr = io.Copy(writer, in)
			if closeErr := writer.Close(); copyErr == nil {
				copyErr = closeErr
			}
		case "gzip":
			writer := gzip.NewWriter(pipeWriter)
			_, copyErr = io.Copy(writer, in)
			if closeErr := writer.Close(); copyErr == nil {
				copyErr = closeErr
			}
		case "identity":
			_, copyErr = io.Copy(pipeWriter, in)
		default:
			copyErr = utils.Errorf("unsupported artifact codec: %s", actualCodec)
		}
		if copyErr != nil {
			_ = pipeWriter.CloseWithError(copyErr)
		} else {
			_ = pipeWriter.Close()
		}
		compressErrCh <- copyErr
	}()

	uploadStart := time.Now()
	stats, uploadErr := session.upload(ctx, pipeReader, -1, objectKey)
	_ = pipeReader.CloseWithError(uploadErr)
	compressErr := <-compressErrCh
	uploadMS := time.Since(uploadStart).Milliseconds()
	c.recordUploadMs(uploadMS)
	if uploadErr != nil {
		return nil, uploadErr
	}
	if compressErr != nil {
		return nil, compressErr
	}
	log.Infof("ssa artifact stream uploaded task=%s key=%s raw=%d stored=%d sha256=%s parts=%d request_id=%s duration_ms=%d",
		c.taskID, objectKey, uncompressedSize, stats.Bytes, stats.SHA256, stats.Parts, stats.RequestID, uploadMS)

	return &SSAArtifactBuildResult{
		ObjectKey:        objectKey,
		Codec:            actualCodec,
		ArtifactPath:     "",
		ArtifactFormat:   spec.SSAArtifactFormatPartsNDJSONV1,
		UncompressedSize: uncompressedSize,
		CompressedSize:   stats.Bytes,
		SHA256:           stats.SHA256,
		ProgramName:      programName,
		ReportType:       reportType,
		RiskCount:        riskCount,
		FileCount:        fileCount,
		FlowCount:        flowCount,
	}, nil
}

func readSSAMultipartPartSize() int64 {
	raw := strings.TrimSpace(os.Getenv("SCANNODE_SSA_MULTIPART_PART_SIZE_MB"))
	if raw == "" {
		return defaultSSAMultipartPartSizeBytes
	}
	mb, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || mb <= 0 {
		return defaultSSAMultipartPartSizeBytes
	}
	if mb >= maxSSAMultipartPartSizeBytes/(1024*1024) {
		return maxSSAMultipartPartSizeBytes
	}
	if mb <= minSSAMultipartPartSizeBytes/(1024*1024) {
		return minSSAMultipartPartSizeBytes
	}
	size := mb * 1024 * 1024
	return size
}

func readSSAMultipartConcurrency() int {
	raw := strings.TrimSpace(os.Getenv("SCANNODE_SSA_MULTIPART_CONCURRENCY"))
	if raw == "" {
		return defaultSSAMultipartConcurrency
	}
	concurrency, err := strconv.Atoi(raw)
	if err != nil {
		return defaultSSAMultipartConcurrency
	}
	if concurrency < 1 {
		return 1
	}
	if concurrency > maxSSAMultipartConcurrency {
		return maxSSAMultipartConcurrency
	}
	return concurrency
}

func (c *SSAArtifactCollector) UploadBySTS(cfg *SSAArtifactUploadConfig, artifactPath string, size int64) error {
	return c.UploadBySTSContext(context.Background(), cfg, artifactPath, size)
}

func (c *SSAArtifactCollector) UploadBySTSContext(ctx context.Context, cfg *SSAArtifactUploadConfig, artifactPath string, size int64) error {
	return c.UploadBySTSWithProviderContext(ctx, artifactPath, size, func(force bool) (*SSAArtifactUploadConfig, error) {
		_ = force
		return cfg, nil
	})
}

func (c *SSAArtifactCollector) UploadBySTSWithProvider(artifactPath string, size int64, provider ssaUploadConfigProvider) error {
	return c.UploadBySTSWithProviderContext(context.Background(), artifactPath, size, provider)
}

func (c *SSAArtifactCollector) UploadBySTSWithProviderContext(ctx context.Context, artifactPath string, size int64, provider ssaUploadConfigProvider) error {
	if provider == nil {
		return utils.Errorf("empty upload config provider")
	}
	if ctx == nil {
		ctx = c.uploadContext()
	}
	cfg, err := provider(false)
	if err != nil {
		return err
	}
	if _, err := validateSSAUploadConfig(cfg); err != nil {
		return err
	}
	session, err := newSSAObjectStoreUploadSession(provider, c.recordRetry)
	if err != nil {
		return err
	}
	defer session.Close()

	uploadStart := time.Now()
	stats, err := session.uploadFile(ctx, artifactPath, size, strings.TrimSpace(cfg.ObjectKey))
	uploadMS := time.Since(uploadStart).Milliseconds()
	c.recordUploadMs(uploadMS)
	if err != nil {
		log.Warnf("upload_attempt_failed task=%s key=%s duration_ms=%d error=%q", c.taskID, cfg.ObjectKey, uploadMS, err.Error())
		return err
	}
	log.Infof("upload_attempt task=%s key=%s bytes=%d sha256=%s parts=%d request_id=%s duration_ms=%d",
		c.taskID, cfg.ObjectKey, stats.Bytes, stats.SHA256, stats.Parts, stats.RequestID, uploadMS)
	return nil
}

func (c *SSAArtifactCollector) BuildReadyEvent(result *SSAArtifactBuildResult, totalLines int64, riskCountHint int64) *spec.SSAArtifactReadyEvent {
	if c == nil || result == nil {
		return nil
	}
	codec := strings.TrimSpace(result.Codec)
	objectKey := strings.TrimSpace(result.ObjectKey)
	if strings.TrimSpace(codec) == "" {
		codec = "identity"
	}
	if riskCountHint <= 0 {
		riskCountHint = result.RiskCount
	}
	metrics := c.snapshotUploadMetrics()
	metricsJSON, _ := json.Marshal(metrics)
	return &spec.SSAArtifactReadyEvent{
		ObjectKey:        objectKey,
		Codec:            codec,
		ArtifactFormat:   result.ArtifactFormat,
		CompressedSize:   result.CompressedSize,
		UncompressedSize: result.UncompressedSize,
		SHA256:           result.SHA256,
		ProgramName:      result.ProgramName,
		ReportType:       result.ReportType,
		TotalLines:       totalLines,
		RiskCount:        riskCountHint,
		FileCount:        result.FileCount,
		FlowCount:        result.FlowCount,
		ProducedAt:       time.Now().Unix(),
		Metrics:          metricsJSON,
	}
}

func (c *SSAArtifactCollector) BuildUploadFailedEvent(errorCode, errorMessage string, uploadedBytes uint64) *spec.SSAArtifactUploadFailedEvent {
	if c == nil {
		return nil
	}
	metrics := c.snapshotUploadMetrics()
	metricsJSON, _ := json.Marshal(metrics)
	return &spec.SSAArtifactUploadFailedEvent{
		ErrorCode:     errorCode,
		ErrorMessage:  errorMessage,
		UploadedBytes: uploadedBytes,
		Metrics:       metricsJSON,
	}
}

func (c *SSAArtifactCollector) Cleanup() {
	if c == nil {
		return
	}
	c.mu.Lock()
	input := c.continuousInput
	done := c.continuousDone
	needClose := c.continuousStarted && !c.continuousClosed && input != nil
	if needClose {
		close(input)
		c.continuousClosed = true
	}
	c.mu.Unlock()

	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.partsFile != nil {
		_ = c.partsFile.Close()
		c.partsFile = nil
	}
	if c.spoolDir != "" {
		_ = os.RemoveAll(c.spoolDir)
		c.spoolDir = ""
		c.partsPath = ""
	}
}

func sanitizePathComponent(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func codecExt(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "", "zstd":
		return "zst"
	case "gzip":
		return "gz"
	case "identity":
		return "ndjson"
	default:
		return "bin"
	}
}
