package scannode

import (
	"bytes"
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
	"github.com/minio/minio-go/v7"
	minioCreds "github.com/minio/minio-go/v7/pkg/credentials"
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

	STSAccessKey    string
	STSSecretKey    string
	STSSessionToken string
	STSExpiresAt    int64
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

	continuousEnabled  bool
	continuousCodec    string
	continuousProvider ssaUploadConfigProvider
	continuousStarted  bool
	continuousInput    chan []byte
	continuousDone     chan struct{}
	continuousClosed   bool
	continuousErr      error
	continuousBuild    *SSAArtifactBuildResult
}

const (
	defaultSSAMultipartPartSizeBytes = 16 * 1024 * 1024
	minSSAMultipartPartSizeBytes     = 5 * 1024 * 1024
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
		build, err := runContinuousSegmentedUpload(codec, provider, taskID, programName, reportType, input)
		c.mu.Lock()
		c.continuousBuild = build
		c.continuousErr = err
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

	select {
	case input <- payload:
		return nil
	case <-done:
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.continuousErr != nil {
			return c.continuousErr
		}
		return utils.Errorf("continuous upload exited unexpectedly")
	}
}

func NewSSAArtifactCollector(taskID, runtimeID, subTaskID string) *SSAArtifactCollector {
	c := &SSAArtifactCollector{
		taskID:    taskID,
		runtimeID: runtimeID,
		subTaskID: subTaskID,
		startedAt: time.Now(),
	}
	if err := c.initSpoolLocked(); err != nil {
		c.initErr = err
	}
	return c
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
	if c == nil {
		return nil, utils.Errorf("collector is nil")
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
		return c.BuildAndUploadCompressedArtifactWithProvider(codec, provider)
	}

	if continuousEnabled && started && done != nil {
		<-done
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

	return c.BuildAndUploadCompressedArtifactWithProvider(codec, provider)
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

func runContinuousSegmentedUpload(codec string, provider ssaUploadConfigProvider, taskID string, programName string, reportType string, input <-chan []byte) (*SSAArtifactBuildResult, error) {
	if provider == nil {
		return nil, utils.Errorf("empty upload config provider")
	}
	baseCfg, err := provider(false)
	if err != nil {
		return nil, err
	}
	if err := validateSSAUploadConfig(baseCfg); err != nil {
		return nil, err
	}
	segmentPrefix, manifestKey := deriveSSAContinuousObjectKeys(strings.TrimSpace(baseCfg.ObjectKey))
	segmentMaxBytes := readSSASegmentMaxBytes()
	if segmentMaxBytes <= 0 {
		segmentMaxBytes = defaultSSASegmentMaxBytes
	}
	flushInterval := readSSASegmentFlushInterval()
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
		if err := uploadSSAArtifactFileWithObjectKey(compPath, compressedSize, segmentKey, provider); err != nil {
			return err
		}
		uploadMS := time.Since(uploadStart).Milliseconds()
		log.Infof("ssa artifact segment uploaded task=%s seq=%d key=%s codec=%s raw=%d compressed=%d",
			taskID, seq, segmentKey, uploadCodec, rawBytes, compressedSize)
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
	if err := uploadSSAArtifactBytesWithObjectKey(manifestRaw, manifestKey, provider); err != nil {
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

func uploadSSAArtifactFileWithObjectKey(path string, size int64, objectKey string, provider ssaUploadConfigProvider) error {
	tmp := &SSAArtifactCollector{}
	return tmp.UploadBySTSWithProvider(path, size, func(force bool) (*SSAArtifactUploadConfig, error) {
		cfg, err := provider(force)
		if err != nil {
			return nil, err
		}
		cp := *cfg
		cp.ObjectKey = strings.TrimSpace(objectKey)
		return &cp, nil
	})
}

func uploadSSAArtifactBytesWithObjectKey(payload []byte, objectKey string, provider ssaUploadConfigProvider) error {
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
	return uploadSSAArtifactFileWithObjectKey(path, int64(len(payload)), objectKey, provider)
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
	if provider == nil {
		return nil, utils.Errorf("empty upload config provider")
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
	if err := validateSSAUploadConfig(cfg); err != nil {
		_ = in.Close()
		return nil, err
	}
	baseBucket := strings.TrimSpace(cfg.Bucket)
	baseObjectKey := strings.TrimSpace(cfg.ObjectKey)
	uploadOpts := minio.PutObjectOptions{ContentType: "application/octet-stream"}
	partSize := readSSAMultipartPartSize()
	actualCodec := normalizeArtifactCodec(codec)

	var (
		uploadID string
		core     *minio.Core
	)
	for attempt := 0; attempt < 2; attempt++ {
		curCfg, e := provider(attempt == 1)
		if e != nil {
			_ = in.Close()
			return nil, e
		}
		if strings.TrimSpace(curCfg.Bucket) != baseBucket || strings.TrimSpace(curCfg.ObjectKey) != baseObjectKey {
			_ = in.Close()
			return nil, utils.Errorf("upload target changed during sts refresh")
		}
		client, e := buildSSAUploadClient(curCfg)
		if e != nil {
			_ = in.Close()
			return nil, e
		}
		tmpCore := &minio.Core{Client: client}
		uid, e := tmpCore.NewMultipartUpload(context.Background(), baseBucket, baseObjectKey, uploadOpts)
		if e == nil {
			core = tmpCore
			uploadID = uid
			break
		}
		if !isSSACredentialError(e) || attempt > 0 {
			_ = in.Close()
			return nil, e
		}
	}
	if uploadID == "" || core == nil {
		_ = in.Close()
		return nil, utils.Errorf("new multipart upload failed")
	}
	abort := true
	defer func() {
		if abort {
			_ = core.AbortMultipartUpload(context.Background(), baseBucket, baseObjectKey, uploadID)
		}
	}()

	pr, pw := io.Pipe()
	compressErrCh := make(chan error, 1)
	go func() {
		defer close(compressErrCh)
		defer func() {
			_ = in.Close()
		}()
		var copyErr error
		switch actualCodec {
		case "zstd":
			zw, err := zstd.NewWriter(pw, zstd.WithEncoderLevel(zstd.SpeedDefault))
			if err != nil {
				_ = pw.CloseWithError(err)
				compressErrCh <- err
				return
			}
			_, copyErr = io.Copy(zw, in)
			if closeErr := zw.Close(); copyErr == nil {
				copyErr = closeErr
			}
		case "gzip":
			gw := gzip.NewWriter(pw)
			_, copyErr = io.Copy(gw, in)
			if closeErr := gw.Close(); copyErr == nil {
				copyErr = closeErr
			}
		case "identity":
			_, copyErr = io.Copy(pw, in)
		default:
			copyErr = utils.Errorf("unsupported artifact codec: %s", actualCodec)
		}
		if copyErr != nil {
			_ = pw.CloseWithError(copyErr)
			compressErrCh <- copyErr
			return
		}
		compressErrCh <- nil
		_ = pw.Close()
	}()

	defer pr.Close()
	buf := make([]byte, partSize)
	partNo := 1
	var (
		completeParts    []minio.CompletePart
		compressedSize   int64
		compressedHasher = sha256.New()
	)

	for {
		n, readErr := io.ReadFull(pr, buf)
		if n > 0 {
			chunk := buf[:n]
			_, _ = compressedHasher.Write(chunk)
			compressedSize += int64(n)

			var part minio.ObjectPart
			var partErr error
			for attempt := 0; attempt < 2; attempt++ {
				curCfg, e := provider(attempt == 1)
				if e != nil {
					return nil, e
				}
				if strings.TrimSpace(curCfg.Bucket) != baseBucket || strings.TrimSpace(curCfg.ObjectKey) != baseObjectKey {
					return nil, utils.Errorf("upload target changed during sts refresh")
				}
				client, e := buildSSAUploadClient(curCfg)
				if e != nil {
					return nil, e
				}
				core = &minio.Core{Client: client}
				part, partErr = core.PutObjectPart(
					context.Background(),
					baseBucket,
					baseObjectKey,
					uploadID,
					partNo,
					bytes.NewReader(chunk),
					int64(n),
					minio.PutObjectPartOptions{},
				)
				if partErr == nil {
					break
				}
				if !isSSACredentialError(partErr) || attempt > 0 {
					return nil, partErr
				}
			}
			completeParts = append(completeParts, minio.CompletePart{
				PartNumber:     part.PartNumber,
				ETag:           part.ETag,
				ChecksumCRC32:  part.ChecksumCRC32,
				ChecksumCRC32C: part.ChecksumCRC32C,
				ChecksumSHA1:   part.ChecksumSHA1,
				ChecksumSHA256: part.ChecksumSHA256,
			})
			partNo++
		}

		if readErr == io.EOF {
			break
		}
		if readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	if compressErr := <-compressErrCh; compressErr != nil {
		return nil, compressErr
	}

	if len(completeParts) == 0 {
		return nil, utils.Errorf("no multipart parts uploaded")
	}

	for attempt := 0; attempt < 2; attempt++ {
		curCfg, e := provider(attempt == 1)
		if e != nil {
			return nil, e
		}
		if strings.TrimSpace(curCfg.Bucket) != baseBucket || strings.TrimSpace(curCfg.ObjectKey) != baseObjectKey {
			return nil, utils.Errorf("upload target changed during sts refresh")
		}
		client, e := buildSSAUploadClient(curCfg)
		if e != nil {
			return nil, e
		}
		core = &minio.Core{Client: client}
		_, err = core.CompleteMultipartUpload(context.Background(), baseBucket, baseObjectKey, uploadID, completeParts, uploadOpts)
		if err == nil {
			abort = false
			return &SSAArtifactBuildResult{
				ObjectKey:        baseObjectKey,
				Codec:            actualCodec,
				ArtifactPath:     "",
				ArtifactFormat:   spec.SSAArtifactFormatPartsNDJSONV1,
				UncompressedSize: uncompressedSize,
				CompressedSize:   compressedSize,
				SHA256:           hex.EncodeToString(compressedHasher.Sum(nil)),
				ProgramName:      programName,
				ReportType:       reportType,
				RiskCount:        riskCount,
				FileCount:        fileCount,
				FlowCount:        flowCount,
			}, nil
		}
		if !isSSACredentialError(err) || attempt > 0 {
			return nil, err
		}
	}
	return nil, utils.Errorf("complete multipart upload failed")
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
	size := mb * 1024 * 1024
	if size < minSSAMultipartPartSizeBytes {
		return minSSAMultipartPartSizeBytes
	}
	return size
}

func isSSACredentialError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	keys := []string{
		"expiredtoken",
		"token expired",
		"signaturedoesnotmatch",
		"invalidtoken",
		"accessdenied",
		"sts credentials expired",
	}
	for _, key := range keys {
		if strings.Contains(msg, key) {
			return true
		}
	}
	return false
}

func validateSSAUploadConfig(cfg *SSAArtifactUploadConfig) error {
	if cfg == nil {
		return utils.Errorf("empty upload config")
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	bucket := strings.TrimSpace(cfg.Bucket)
	objectKey := strings.TrimSpace(cfg.ObjectKey)
	if endpoint == "" || bucket == "" || objectKey == "" {
		return utils.Errorf("upload config missing endpoint/bucket/object_key")
	}
	accessKey := strings.TrimSpace(cfg.STSAccessKey)
	secretKey := strings.TrimSpace(cfg.STSSecretKey)
	if accessKey == "" || secretKey == "" {
		return utils.Errorf("sts credentials missing")
	}
	return nil
}

func buildSSAUploadClient(cfg *SSAArtifactUploadConfig) (*minio.Client, error) {
	if err := validateSSAUploadConfig(cfg); err != nil {
		return nil, err
	}
	return minio.New(strings.TrimSpace(cfg.Endpoint), &minio.Options{
		Creds:  minioCreds.NewStaticV4(strings.TrimSpace(cfg.STSAccessKey), strings.TrimSpace(cfg.STSSecretKey), strings.TrimSpace(cfg.STSSessionToken)),
		Secure: cfg.UseSSL,
		Region: strings.TrimSpace(cfg.Region),
	})
}

func (c *SSAArtifactCollector) UploadBySTS(cfg *SSAArtifactUploadConfig, artifactPath string, size int64) error {
	return c.UploadBySTSWithProvider(artifactPath, size, func(force bool) (*SSAArtifactUploadConfig, error) {
		_ = force
		return cfg, nil
	})
}

func (c *SSAArtifactCollector) UploadBySTSWithProvider(artifactPath string, size int64, provider ssaUploadConfigProvider) error {
	if provider == nil {
		return utils.Errorf("empty upload config provider")
	}
	cfg, err := provider(false)
	if err != nil {
		return err
	}
	if err := validateSSAUploadConfig(cfg); err != nil {
		return err
	}
	f, err := os.Open(artifactPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if size <= 0 {
		if st, err := f.Stat(); err == nil {
			size = st.Size()
		}
	}

	if size <= 0 {
		return utils.Errorf("artifact size invalid")
	}
	partSize := readSSAMultipartPartSize()
	if size <= partSize {
		for attempt := 0; attempt < 2; attempt++ {
			cfg, err := provider(attempt == 1)
			if err != nil {
				return err
			}
			client, err := buildSSAUploadClient(cfg)
			if err != nil {
				return err
			}
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			info, err := client.PutObject(context.Background(),
				strings.TrimSpace(cfg.Bucket),
				strings.TrimSpace(cfg.ObjectKey),
				f,
				size,
				minio.PutObjectOptions{ContentType: "application/octet-stream"})
			if err == nil {
				if info.Size > 0 && info.Size != size {
					return utils.Errorf("uploaded size mismatch expect=%d got=%d", size, info.Size)
				}
				return nil
			}
			if !isSSACredentialError(err) || attempt > 0 {
				return err
			}
		}
		return utils.Errorf("upload failed")
	}

	baseBucket := strings.TrimSpace(cfg.Bucket)
	baseObjectKey := strings.TrimSpace(cfg.ObjectKey)
	uploadOpts := minio.PutObjectOptions{ContentType: "application/octet-stream"}
	var (
		uploadID string
		core     *minio.Core
	)
	for attempt := 0; attempt < 2; attempt++ {
		curCfg, e := provider(attempt == 1)
		if e != nil {
			return e
		}
		if strings.TrimSpace(curCfg.Bucket) != baseBucket || strings.TrimSpace(curCfg.ObjectKey) != baseObjectKey {
			return utils.Errorf("upload target changed during sts refresh")
		}
		client, e := buildSSAUploadClient(curCfg)
		if e != nil {
			return e
		}
		tmpCore := &minio.Core{Client: client}
		uid, e := tmpCore.NewMultipartUpload(context.Background(), baseBucket, baseObjectKey, uploadOpts)
		if e == nil {
			core = tmpCore
			uploadID = uid
			break
		}
		if !isSSACredentialError(e) || attempt > 0 {
			return e
		}
	}
	if uploadID == "" || core == nil {
		return utils.Errorf("new multipart upload failed")
	}
	abort := true
	defer func() {
		if abort {
			_ = core.AbortMultipartUpload(context.Background(), baseBucket, baseObjectKey, uploadID)
		}
	}()

	buf := make([]byte, partSize)
	partNo := 1
	var completeParts []minio.CompletePart
	for {
		n, readErr := io.ReadFull(f, buf)
		if readErr == io.EOF {
			break
		}
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			return readErr
		}
		if n <= 0 {
			if readErr == io.ErrUnexpectedEOF {
				break
			}
			continue
		}

		var part minio.ObjectPart
		var partErr error
		for attempt := 0; attempt < 2; attempt++ {
			curCfg, e := provider(attempt == 1)
			if e != nil {
				return e
			}
			if strings.TrimSpace(curCfg.Bucket) != baseBucket || strings.TrimSpace(curCfg.ObjectKey) != baseObjectKey {
				return utils.Errorf("upload target changed during sts refresh")
			}
			client, e := buildSSAUploadClient(curCfg)
			if e != nil {
				return e
			}
			core = &minio.Core{Client: client}
			part, partErr = core.PutObjectPart(
				context.Background(),
				baseBucket,
				baseObjectKey,
				uploadID,
				partNo,
				bytes.NewReader(buf[:n]),
				int64(n),
				minio.PutObjectPartOptions{},
			)
			if partErr == nil {
				break
			}
			if !isSSACredentialError(partErr) || attempt > 0 {
				return partErr
			}
		}
		completeParts = append(completeParts, minio.CompletePart{
			PartNumber:     part.PartNumber,
			ETag:           part.ETag,
			ChecksumCRC32:  part.ChecksumCRC32,
			ChecksumCRC32C: part.ChecksumCRC32C,
			ChecksumSHA1:   part.ChecksumSHA1,
			ChecksumSHA256: part.ChecksumSHA256,
		})
		partNo++
		if readErr == io.ErrUnexpectedEOF {
			break
		}
	}
	if len(completeParts) == 0 {
		return utils.Errorf("no multipart parts uploaded")
	}

	for attempt := 0; attempt < 2; attempt++ {
		curCfg, e := provider(attempt == 1)
		if e != nil {
			return e
		}
		if strings.TrimSpace(curCfg.Bucket) != baseBucket || strings.TrimSpace(curCfg.ObjectKey) != baseObjectKey {
			return utils.Errorf("upload target changed during sts refresh")
		}
		client, e := buildSSAUploadClient(curCfg)
		if e != nil {
			return e
		}
		core = &minio.Core{Client: client}
		_, err = core.CompleteMultipartUpload(context.Background(), baseBucket, baseObjectKey, uploadID, completeParts, uploadOpts)
		if err == nil {
			abort = false
			return nil
		}
		if !isSSACredentialError(err) || attempt > 0 {
			return err
		}
	}
	return utils.Errorf("complete multipart upload failed")
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
