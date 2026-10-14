package scannode

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const aiTrafficProtocolVersion = 1
const aiTrafficCapabilityV1 = "ai.traffic.capture.v1"
const aiTrafficDefaultPacketLimit = 10 << 20
const aiTrafficDefaultSessionLimit = 1 << 30
const aiTrafficMaxPacketLimit = 64 << 20
const aiTrafficMaxSessionLimit = 64 << 30

var aiTrafficCollectors sync.Map

type aiTrafficCollector struct {
	binding     aiSessionBinding
	policy      *aiv1.AITrafficCapturePolicy
	emitter     aiSessionRuntimeEmitter
	dir         string
	client      *http.Client
	wake        chan struct{}
	stop        chan struct{}
	stopped     sync.Once
	finished    sync.Once
	uploadMu    sync.Mutex
	mu          sync.Mutex
	currentTurn string
	accepting   bool
	active      uint64
	used        uint64
	failed      bool
}

func newAITrafficCollector(binding aiSessionBinding, emitter aiSessionRuntimeEmitter) (*aiTrafficCollector, error) {
	policy := binding.TrafficCapture
	if policy == nil {
		return nil, nil
	}
	if policy.GetProtocolVersion() != aiTrafficProtocolVersion {
		return nil, fmt.Errorf("unsupported AI traffic protocol version")
	}
	endpoint, err := url.Parse(policy.GetUploadBaseUrl())
	platform, platformErr := url.Parse(binding.PlatformAPIBaseURL)
	if err != nil || platformErr != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Host != platform.Host || endpoint.Scheme != platform.Scheme || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, fmt.Errorf("traffic upload endpoint must use the authenticated platform origin")
	}
	if binding.PlatformBearerToken == "" || binding.NodeSessionID == "" {
		return nil, fmt.Errorf("traffic capture requires authenticated node session")
	}
	policy = proto.Clone(policy).(*aiv1.AITrafficCapturePolicy)
	policy.PacketLimitBytes = trafficCaptureLimit(policy.PacketLimitBytes, aiTrafficDefaultPacketLimit, aiTrafficMaxPacketLimit)
	policy.SessionLimitBytes = trafficCaptureLimit(policy.SessionLimitBytes, aiTrafficDefaultSessionLimit, aiTrafficMaxSessionLimit)
	identity := trafficSHA([]byte(fmt.Sprintf("%s/%d/%s", binding.Ref.SessionID, binding.Ref.BindEpoch, binding.NodeSessionID)))
	root, err := filepath.Abs(consts.GetDefaultYakitBaseDir())
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(root, "legion-ai-traffic", identity)
	relative, err := filepath.Rel(root, directory)
	if err != nil || !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("traffic spool must remain within the local root")
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	client := binding.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	requestClient := *client
	requestClient.Timeout = 10 * time.Second
	requestClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	// Cumulative local admission survives retries/restarts; the platform remains
	// the authoritative quota and may reject bytes that a concurrent node admitted.
	var used uint64
	if raw, readErr := os.ReadFile(filepath.Join(directory, "admitted-bytes")); readErr == nil {
		fmt.Sscan(string(raw), &used)
	}
	_, markerErr := os.Stat(filepath.Join(directory, "capture-incomplete"))
	c := &aiTrafficCollector{
		binding: binding, policy: policy, emitter: emitter,
		dir: directory, client: &requestClient,
		wake: make(chan struct{}, 1), stop: make(chan struct{}),
		accepting: true, used: used, failed: !errors.Is(markerErr, os.ErrNotExist),
	}
	aiTrafficCollectors.Store(c.key(), c)
	go c.run()
	c.signal()
	return c, nil
}

func trafficCaptureLimit(configured, defaultLimit, hardLimit uint64) uint64 {
	if configured == 0 {
		return defaultLimit
	}
	if configured > hardLimit {
		return hardLimit
	}
	return configured
}

func (c *aiTrafficCollector) key() string {
	return fmt.Sprintf("%s/%d/%s", c.binding.Ref.SessionID, c.binding.Ref.BindEpoch, c.binding.NodeSessionID)
}
func (c *aiTrafficCollector) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}
func (c *aiTrafficCollector) emit(ctx context.Context, kind string, message proto.Message) bool {
	if c.emitter == nil {
		return false
	}
	raw, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(message)
	if err != nil {
		return false
	}
	if emitter, ok := c.emitter.(aiSessionRuntimeContextRefEmitter); ok {
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		ref := c.binding.Ref
		c.mu.Lock()
		if c.currentTurn != "" {
			ref.CommandID = c.currentTurn
		}
		c.mu.Unlock()
		return emitter.EmitForRefContext(ctx, ref, kind, raw)
	}
	return false
}
func trafficSHA(raw []byte) string { hash := sha256.Sum256(raw); return hex.EncodeToString(hash[:]) }

type aiTrafficContextKey struct{}
type aiTrafficContext struct {
	collector *aiTrafficCollector
	turnID    string
}

func withAITrafficContext(ctx context.Context, collector *aiTrafficCollector, turnID string) context.Context {
	return context.WithValue(ctx, aiTrafficContextKey{}, aiTrafficContext{collector, turnID})
}

func init() {
	lowhttp.RegisterSaveHTTPFlowHandler(saveAITrafficHTTPFlow)
	yakit.RegisterPostInitDatabaseFunction(func() error {
		lowhttp.RegisterSaveHTTPFlowHandler(saveAITrafficHTTPFlow)
		return nil
	})
}

func saveAITrafficHTTPFlow(r *lowhttp.LowhttpResponse, syncSave bool) {
	if r == nil {
		return
	}
	if r.RequestInstance != nil {
		if scope, ok := r.RequestInstance.Context().Value(aiTrafficContextKey{}).(aiTrafficContext); ok && scope.collector != nil {
			scope.collector.recordHTTPFlow(r, scope.turnID)
		}
	}
	yakit.SaveLowHTTPFlow(r, syncSave)
}

func (c *aiTrafficCollector) recordHTTPFlow(r *lowhttp.LowhttpResponse, turn string) {
	if r == nil || len(r.RawPacket) == 0 {
		return
	}
	c.mu.Lock()
	if !c.accepting {
		c.failed = true
		c.mu.Unlock()
		return
	}
	c.active++
	if turn == "" {
		turn = c.currentTurn
	}
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.active--; c.mu.Unlock(); c.signal() }()
	finished := time.Now()
	started := finished
	if r.TraceInfo != nil {
		started = finished.Add(-r.TraceInfo.TotalTime)
	}
	terminal := &aiv1.AITrafficRecord{ProtocolVersion: 1, FlowId: uuid.NewString(), SessionId: c.binding.Ref.SessionID, TurnId: turn, RuntimeId: r.RuntimeId, NodeId: c.binding.NodeID, NodeSessionId: c.binding.NodeSessionID, BindEpoch: c.binding.Ref.BindEpoch, Phase: "terminal", Outcome: "success", StartedAtMs: started.UnixMilli(), FinishedAtMs: finished.UnixMilli(), Scheme: "http"}
	if r.Https {
		terminal.Scheme = "https"
	}
	trafficRequestMetadata(terminal, r.RawRequest)
	var requestHeaderFile, requestBodyFile, responseHeaderFile, responseBodyFile string
	if r.RequestInstance != nil {
		requestHeaderFile = httpctx.GetRequestTooLargeHeaderFile(r.RequestInstance)
		requestBodyFile = httpctx.GetRequestTooLargeBodyFile(r.RequestInstance)
		responseHeaderFile = httpctx.GetResponseTooLargeHeaderFile(r.RequestInstance)
		responseBodyFile = httpctx.GetResponseTooLargeBodyFile(r.RequestInstance)
	}
	request, requestSize, requestTruncated, reqErr := trafficPacket(r.RawRequest, requestHeaderFile, requestBodyFile, int64(c.policy.PacketLimitBytes))
	response, responseSize, responseTruncated, rspErr := trafficPacket(r.RawPacket, responseHeaderFile, responseBodyFile, int64(c.policy.PacketLimitBytes))
	request, response, packetLimitReason := trafficBoundPacketPair(request, response, int(c.policy.PacketLimitBytes))
	requestTruncated = requestTruncated || int64(len(request)) < requestSize
	responseTruncated = responseTruncated || int64(len(response)) < responseSize
	if (requestTruncated && len(request) > 0 && !trafficHeadersComplete(request)) || (responseTruncated && len(response) > 0 && !trafficHeadersComplete(response)) {
		terminal.CaptureError = "packet_headers_limit"
	} else if packetLimitReason != "" {
		terminal.CaptureError = packetLimitReason
	} else if requestTruncated || responseTruncated {
		terminal.CaptureError = "packet_limit"
	}
	trafficRequestMetadata(terminal, request)
	terminal.RequestSizeBytes = uint64(requestSize)
	terminal.ResponseSizeBytes = uint64(responseSize)
	terminal.RequestTruncated = requestTruncated
	terminal.ResponseTruncated = responseTruncated
	if reqErr != nil || rspErr != nil {
		terminal.CaptureError = "local_packet_unavailable"
	}
	if rsp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(response)), nil); err == nil {
		terminal.StatusCode = int32(rsp.StatusCode)
		rsp.Body.Close()
	}
	terminal.RequestSha256 = trafficSHA(request)
	terminal.ResponseSha256 = trafficSHA(response)
	c.enqueue(&aiv1.AITrafficUpload{Record: terminal, RawRequest: request, RawResponse: response})
}

// trafficBoundPacketPair uses one budget for the complete flow. Reserve both
// headers before dividing the remaining bytes between request/response bodies.
// If the headers alone exceed the budget, preserve bounded prefixes of both
// sides and explicitly report that complete headers could not be retained.
func trafficBoundPacketPair(request, response []byte, limit int) ([]byte, []byte, string) {
	if len(request)+len(response) <= limit {
		return request, response, ""
	}
	requestHeader, responseHeader := trafficHeaderLength(request), trafficHeaderLength(response)
	if requestHeader+responseHeader > limit {
		requestLimit, responseLimit := trafficSplitBudget(limit, requestHeader, responseHeader)
		return request[:requestLimit], response[:responseLimit], "packet_headers_limit"
	}
	requestBody, responseBody := trafficSplitBudget(limit-requestHeader-responseHeader, len(request)-requestHeader, len(response)-responseHeader)
	return request[:requestHeader+requestBody], response[:responseHeader+responseBody], "packet_limit"
}

func trafficHeadersComplete(packet []byte) bool {
	return bytes.Contains(packet, []byte("\r\n\r\n")) || bytes.Contains(packet, []byte("\n\n"))
}

func trafficHeaderLength(packet []byte) int {
	if end := bytes.Index(packet, []byte("\r\n\r\n")); end >= 0 {
		return end + 4
	}
	if end := bytes.Index(packet, []byte("\n\n")); end >= 0 {
		return end + 2
	}
	// A partial or malformed header must never be mistaken for body space.
	return len(packet)
}

func trafficSplitBudget(limit, leftSize, rightSize int) (int, int) {
	left := min(leftSize, limit/2)
	right := min(rightSize, limit-left)
	left += min(leftSize-left, limit-left-right)
	return left, right
}

func trafficRequestMetadata(record *aiv1.AITrafficRecord, packet []byte) {
	// Preserve the attempt's start URL even if terminal packets are truncated,
	// unavailable, or dropped by the local/platform raw-byte quota.
	if record.RedactedUrl != nil {
		return
	}
	if req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(packet))); err == nil {
		record.Method = req.Method
		record.Host = req.Host
		if !trafficValidURLHost(record.Scheme, record.Host) {
			record.Host = ""
		}
		if safeURL := trafficRedactedURL(req.URL, record.Scheme, record.Host); safeURL != "" {
			record.RedactedUrl = proto.String(safeURL)
		}
		req.Body.Close()
	}
	if len(record.Method) > 16 {
		record.Method = ""
	}
}

func trafficValidURLHost(scheme, host string) bool {
	if (scheme != "http" && scheme != "https") || host == "" || len(host) > 300 {
		return false
	}
	parsed, err := url.Parse(scheme + "://" + host)
	return err == nil && parsed.Host == host && parsed.Hostname() != "" && parsed.User == nil && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

// Keep these rules aligned with Legion internal/aitraffic.RedactURL. The node
// cannot import the platform; no raw URL may enter event or spool metadata.
func trafficSensitiveURLKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("-", "", "_", "", ".", "").Replace(key))
	for _, value := range []string{"authorization", "cookie", "token", "password", "passwd", "secret", "apikey", "credential"} {
		if strings.Contains(key, value) {
			return true
		}
	}
	return key == "auth" || key == "pwd" || key == "sessionid"
}

func trafficRedactedURL(target *url.URL, scheme, host string) string {
	if target == nil || target.Opaque != "" || !trafficValidURLHost(scheme, host) {
		return ""
	}
	parsed := *target
	if parsed.Scheme == "" {
		parsed.Scheme = scheme
	}
	if parsed.Host == "" {
		parsed.Host = host
	}
	if parsed.Scheme != scheme || !strings.EqualFold(parsed.Host, host) || len(parsed.String()) > 16<<10 {
		return ""
	}
	// ParseQuery errors must fail closed instead of silently retaining partial
	// query fields. URL credentials and fragments are never metadata.
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return ""
	}
	parsed.User, parsed.Fragment, parsed.RawFragment = nil, "", ""
	const mask = "[REDACTED]"
	segments := strings.Split(parsed.Path, "/")
	for i := 1; i < len(segments); i++ {
		if trafficSensitiveURLKey(segments[i-1]) {
			segments[i] = mask
		}
	}
	parsed.Path, parsed.RawPath = strings.Join(segments, "/"), ""
	for key := range values {
		if trafficSensitiveURLKey(key) {
			values.Set(key, mask)
		}
	}
	parsed.RawQuery = values.Encode()
	redacted := parsed.String()
	if len(redacted) > 16<<10 {
		return ""
	}
	return redacted
}

// Oversize HTTPFlow implementations store headers/body separately. Read the
// actual bytes synchronously before their owner can remove these local files.
func trafficPacket(raw []byte, headerPath, bodyPath string, limit int64) ([]byte, int64, bool, error) {
	if headerPath == "" && bodyPath == "" {
		size := int64(len(raw))
		if size > limit {
			return append([]byte(nil), raw[:limit]...), size, true, nil
		}
		return append([]byte(nil), raw...), size, false, nil
	}
	var readers []io.Reader
	var files []*os.File
	var size int64
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()
	for _, path := range []string{headerPath, bodyPath} {
		if path == "" {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, size, true, err
		}
		files = append(files, f)
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return nil, size, true, fmt.Errorf("packet evidence is not a regular file")
		}
		size += info.Size()
		readers = append(readers, f)
	}
	packet, err := io.ReadAll(io.LimitReader(io.MultiReader(readers...), limit))
	return packet, size, size > limit, err
}

func (c *aiTrafficCollector) enqueue(upload *aiv1.AITrafficUpload) {
	c.mu.Lock()
	bytesCount := uint64(len(upload.RawRequest) + len(upload.RawResponse))
	if bytesCount > 0 && (c.used >= c.policy.SessionLimitBytes || bytesCount > c.policy.SessionLimitBytes-c.used) {
		upload.RawRequest = nil
		upload.RawResponse = nil
		upload.Record.RequestTruncated = true
		upload.Record.ResponseTruncated = true
		upload.Record.CaptureError = "local_session_limit"
		upload.Record.RequestSha256 = trafficSHA(nil)
		upload.Record.ResponseSha256 = trafficSHA(nil)
	} else if bytesCount > 0 {
		c.used += bytesCount
		if err := trafficAtomicWrite(c.dir, "admitted-bytes", []byte(fmt.Sprint(c.used))); err != nil {
			c.failed = true
		}
	}
	if upload.Record.CaptureError != "" {
		_ = c.markIncompleteLocked()
	}
	raw, err := proto.Marshal(upload)
	if err == nil {
		err = trafficAtomicWrite(c.dir, upload.Record.FlowId+"."+upload.Record.Phase+".pb", raw)
	}
	if err != nil {
		c.failed = true
	}
	c.mu.Unlock()
	if err != nil {
		failure := proto.Clone(upload.Record).(*aiv1.AITrafficRecord)
		failure.CaptureError = "spool_write_failed"
		c.emit(context.Background(), "ai_traffic_batch", &aiv1.AITrafficBatch{ProtocolVersion: 1, Records: []*aiv1.AITrafficRecord{failure}})
	}
	c.signal()
}

func (c *aiTrafficCollector) markIncompleteLocked() error {
	c.failed = true
	// Keep evidence coverage separate from whether the upload queue is empty.
	// A durable metadata-only receipt cannot restore missing/truncated packets.
	return trafficAtomicWrite(c.dir, "capture-incomplete", []byte("incomplete\n"))
}

func trafficAtomicWrite(directory, name string, raw []byte) error {
	if !filepath.IsAbs(directory) {
		return fmt.Errorf("traffic spool directory must be absolute")
	}
	if !filepath.IsLocal(name) || filepath.Clean(name) != name || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("traffic spool file must have a plain basename")
	}
	directory = filepath.Clean(directory)
	path := filepath.Clean(filepath.Join(directory, name))
	relative, err := filepath.Rel(directory, path)
	if err != nil || relative != name || relative == "." {
		return fmt.Errorf("traffic spool file must be a direct child of its directory")
	}
	file, err := os.CreateTemp(directory, ".traffic-")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer func() {
		if err := os.Remove(temp); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Warnf("AI traffic temporary file cleanup failed: %v", err)
		}
	}()
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(temp, path); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (c *aiTrafficCollector) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-c.wake:
		case <-ticker.C:
		}
		c.flush(context.Background())
	}
}
func (c *aiTrafficCollector) pending() ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(c.dir, "*.pb"))
	sort.Strings(paths)
	return paths, err
}
func (c *aiTrafficCollector) flush(ctx context.Context) {
	if !c.uploadMu.TryLock() {
		return
	}
	defer c.uploadMu.Unlock()
	paths, err := c.pending()
	if err != nil {
		return
	}
	if len(paths) > 64 {
		paths = paths[:64]
	}
	var batch []*aiv1.AITrafficRecord
	var markers []string
	var hashes []string
	emitBatch := func() {
		if c.emit(ctx, "ai_traffic_batch", &aiv1.AITrafficBatch{ProtocolVersion: 1, Records: batch}) {
			for i, marker := range markers {
				_ = trafficAtomicWrite(c.dir, filepath.Base(marker), []byte(hashes[i]))
			}
		}
		batch, markers, hashes = nil, nil, nil
	}
	for _, path := range paths {
		if ctx.Err() != nil {
			return
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		upload := new(aiv1.AITrafficUpload)
		if proto.Unmarshal(raw, upload) != nil || upload.Record == nil {
			c.mu.Lock()
			c.failed = true
			c.mu.Unlock()
			continue
		}
		marker := strings.TrimSuffix(path, ".pb") + ".metadata"
		hash := trafficSHA(raw)
		if acknowledged, err := os.ReadFile(marker); err == nil && string(acknowledged) == hash {
			continue
		}
		batch = append(batch, upload.Record)
		markers = append(markers, marker)
		hashes = append(hashes, hash)
		if len(batch) == 32 {
			emitBatch()
		}
	}
	if len(batch) > 0 {
		emitBatch()
	}
	for _, path := range paths {
		if ctx.Err() != nil {
			return
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		upload := new(aiv1.AITrafficUpload)
		if proto.Unmarshal(raw, upload) != nil || upload.Record == nil {
			continue
		}
		receiptPath := strings.TrimSuffix(path, ".pb") + ".receipt"
		receiptRaw, receiptErr := os.ReadFile(receiptPath)
		receipt := new(aiv1.AITrafficReceipt)
		if receiptErr != nil || proto.Unmarshal(receiptRaw, receipt) != nil || !validAITrafficReceipt(receipt, upload, raw) {
			receipt, err = c.upload(ctx, upload, raw)
			if err != nil {
				return
			}
			receiptRaw, _ = proto.Marshal(receipt)
			if err = trafficAtomicWrite(c.dir, filepath.Base(receiptPath), receiptRaw); err != nil {
				return
			}
		}
		if upload.Record.CaptureError != "" || (upload.Record.Phase == "terminal" && receipt.StorageStatus != "stored") {
			c.mu.Lock()
			markerErr := c.markIncompleteLocked()
			c.mu.Unlock()
			if markerErr != nil {
				return
			}
		}
		// Receipt is fsynced before queue removal; restart replays this step safely.
		if err = os.Remove(path); err == nil {
			marker := strings.TrimSuffix(path, ".pb") + ".metadata"
			if acknowledged, e := os.ReadFile(marker); e == nil && string(acknowledged) == trafficSHA(raw) {
				if removeErr := os.Remove(marker); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
					log.Warnf("AI traffic metadata acknowledgement cleanup failed: %v", removeErr)
				}
			}
			if dir, e := os.Open(c.dir); e == nil {
				dir.Sync()
				dir.Close()
			}
		}
	}
}

func validAITrafficReceipt(receipt *aiv1.AITrafficReceipt, upload *aiv1.AITrafficUpload, raw []byte) bool {
	return receipt.GetProtocolVersion() == 1 && receipt.GetDurable() && receipt.GetFlowId() == upload.GetRecord().GetFlowId() && receipt.GetPhase() == upload.GetRecord().GetPhase() && receipt.GetUploadSha256() == trafficSHA(raw) && (receipt.GetStorageStatus() == "stored" || receipt.GetStorageStatus() == "quota_dropped" || receipt.GetStorageStatus() == "metadata_only")
}
func (c *aiTrafficCollector) upload(ctx context.Context, upload *aiv1.AITrafficUpload, raw []byte) (*aiv1.AITrafficReceipt, error) {
	endpoint := strings.TrimRight(c.policy.UploadBaseUrl, "/") + "/" + url.PathEscape(c.binding.Ref.SessionID) + "/records"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("Authorization", "Bearer "+c.binding.PlatformBearerToken)
	request.Header.Set("X-Node-Session-ID", c.binding.NodeSessionID)
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("traffic upload status %d", response.StatusCode)
	}
	receiptRaw, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	receipt := new(aiv1.AITrafficReceipt)
	if err = proto.Unmarshal(receiptRaw, receipt); err != nil {
		return nil, err
	}
	if !validAITrafficReceipt(receipt, upload, raw) {
		return nil, fmt.Errorf("traffic receipt is not durable or does not match upload")
	}
	return receipt, nil
}

// A start receipt with no terminal queue/receipt survives a process restart.
// It remains unconfirmed even after all upload files have been acknowledged.
func (c *aiTrafficCollector) unresolvedStarts() uint64 {
	paths, _ := filepath.Glob(filepath.Join(c.dir, "*.started.*"))
	unresolved := make(map[string]bool)
	for _, path := range paths {
		base := path[:strings.LastIndex(path, ".started.")]
		if _, err := os.Stat(base + ".terminal.receipt"); err == nil {
			continue
		}
		if _, err := os.Stat(base + ".terminal.pb"); err == nil {
			continue
		}
		unresolved[base] = true
	}
	return uint64(len(unresolved))
}

func (c *aiTrafficCollector) drain(ctx context.Context, commandID string) *aiv1.AITrafficDrainResult {
	c.mu.Lock()
	c.accepting = false
	c.mu.Unlock()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		paths, err := c.pending()
		c.mu.Lock()
		active, failed := c.active, c.failed
		c.mu.Unlock()
		if unresolved := c.unresolvedStarts(); unresolved > active {
			active = unresolved
		}
		result := &aiv1.AITrafficDrainResult{ProtocolVersion: 1, Session: &aiv1.AISessionRef{SessionId: c.binding.Ref.SessionID, RunId: c.binding.Ref.RunID, BindEpoch: c.binding.Ref.BindEpoch}, NodeSessionId: c.binding.NodeSessionID, PendingRecords: uint64(len(paths)) + active, Complete: err == nil && len(paths) == 0 && active == 0 && !failed, CommandId: commandID}
		if (err == nil && len(paths) == 0 && active == 0) || ctx.Err() != nil {
			c.emit(ctx, "ai_traffic_drain", result)
			return result
		}
		select {
		case <-c.stop:
			c.flush(ctx)
		default:
			c.signal()
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

type aiTrafficRuntimeHandle struct {
	aiSessionRuntimeHandle
	collector *aiTrafficCollector
}

func (h *aiTrafficRuntimeHandle) activeTurnID() string {
	if p, ok := h.aiSessionRuntimeHandle.(aiSessionRuntimeTurnRefProvider); ok {
		return p.activeTurnID()
	}
	return ""
}
func (h *aiTrafficRuntimeHandle) SendInput(ctx context.Context, input aiSessionInput) error {
	return h.aiSessionRuntimeHandle.SendInput(ctx, input)
}
func (h *aiTrafficRuntimeHandle) Close(reason string) {
	h.aiSessionRuntimeHandle.Close(reason)
	h.finish()
}
func (h *aiTrafficRuntimeHandle) Cancel(reason string) {
	h.aiSessionRuntimeHandle.Cancel(reason)
	h.finish()
}
func (h *aiTrafficRuntimeHandle) finish() {
	h.collector.finish()
}
func (c *aiTrafficCollector) finish() {
	c.finished.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c.finishDrain(ctx)
	})
}
func (c *aiTrafficCollector) finishDrain(ctx context.Context) {
	c.drain(ctx, "")
	c.stopped.Do(func() { close(c.stop) })
}

// Automatic terminal events originate inside the engine. Drain evidence without
// calling engine.Close here: it may wait for that same engine goroutine to exit.
func (r *aiSessionRuntime) finishTrafficCapture() {
	r.mu.Lock()
	collector := r.trafficCollector
	r.mu.Unlock()
	if collector != nil {
		collector.finish()
	}
}

func (b *legionJobBridge) handleAITrafficDrain(ctx context.Context, raw []byte) error {
	command := new(aiv1.DrainAITrafficCommand)
	if err := proto.Unmarshal(raw, command); err != nil {
		return err
	}
	if command.GetSession().GetSessionId() == "" || command.GetMetadata().GetCommandId() == "" || command.GetSession().GetBindEpoch() == 0 {
		return fmt.Errorf("traffic drain requires fenced session and command ID")
	}
	session, _ := b.agent.node.GetSessionState()
	if command.GetExpectedNodeSessionId() != session.SessionID {
		return fmt.Errorf("traffic drain node session mismatch")
	}
	key := fmt.Sprintf("%s/%d/%s", command.Session.SessionId, command.Session.BindEpoch, session.SessionID)
	value, ok := aiTrafficCollectors.Load(key)
	if !ok {
		return fmt.Errorf("traffic collector unavailable")
	}
	timeout := time.Duration(command.TimeoutMs) * time.Millisecond
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	drainCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result := value.(*aiTrafficCollector).drain(drainCtx, command.GetMetadata().GetCommandId())
	payload, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(result)
	if err != nil {
		return err
	}
	ref := value.(*aiTrafficCollector).binding.Ref
	ref.CommandID = command.GetMetadata().GetCommandId()
	// The dedicated acknowledgement must survive the runtime entering terminal state.
	return b.ensureAIPublisher().PublishEvent(ctx, ref, 1, "ai_traffic_drain", payload)
}
