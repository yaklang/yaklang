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
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const aiTrafficProtocolVersion = 1
const aiTrafficCapabilityV1 = "ai.traffic.capture.v1"
const aiTrafficDefaultPacketLimit = 10 << 20
const aiTrafficDefaultSessionLimit = 1 << 30

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
	uploadMu    sync.Mutex
	mu          sync.Mutex
	currentTurn string
	accepting   bool
	active      uint64
	used        uint64
	failed      bool
}

func newAITrafficCollector(binding aiSessionBinding, emitter aiSessionRuntimeEmitter, directory string) (*aiTrafficCollector, error) {
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
	if policy.PacketLimitBytes == 0 || policy.PacketLimitBytes > aiTrafficDefaultPacketLimit {
		policy.PacketLimitBytes = aiTrafficDefaultPacketLimit
	}
	if policy.SessionLimitBytes == 0 || policy.SessionLimitBytes > aiTrafficDefaultSessionLimit {
		policy.SessionLimitBytes = aiTrafficDefaultSessionLimit
	}
	if directory == "" {
		// Neither remote session IDs nor tool IDs are allowed to select local paths.
		identity := trafficSHA([]byte(fmt.Sprintf("%s/%d/%s", binding.Ref.SessionID, binding.Ref.BindEpoch, binding.NodeSessionID)))
		directory = filepath.Join(consts.GetDefaultYakitBaseDir(), "legion-ai-traffic", identity)
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
	c := &aiTrafficCollector{binding: binding, policy: policy, emitter: emitter, dir: directory, client: &requestClient, wake: make(chan struct{}, 1), stop: make(chan struct{}), accepting: true}
	// Cumulative local admission survives retries/restarts; the platform remains
	// the authoritative quota and may reject bytes that a concurrent node admitted.
	if raw, readErr := os.ReadFile(filepath.Join(directory, "admitted-bytes")); readErr == nil {
		fmt.Sscan(string(raw), &c.used)
	}
	aiTrafficCollectors.Store(c.key(), c)
	go c.run()
	c.signal()
	return c, nil
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
func (c *aiTrafficCollector) emit(kind string, message proto.Message) {
	if c.emitter != nil {
		raw, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(message)
		if err == nil {
			c.emitter.Emit(kind, raw)
		}
	}
}
func trafficSHA(raw []byte) string { hash := sha256.Sum256(raw); return hex.EncodeToString(hash[:]) }

func (c *aiTrafficCollector) observe(start lowhttp.HTTPAttempt) func(lowhttp.HTTPAttempt) {
	c.mu.Lock()
	if !c.accepting {
		c.failed = true
		c.mu.Unlock()
		return nil
	}
	c.active++
	turn := start.TurnID
	if turn == "" {
		turn = c.currentTurn
	}
	c.mu.Unlock()
	record := &aiv1.AITrafficRecord{ProtocolVersion: 1, FlowId: uuid.NewString(), SessionId: c.binding.Ref.SessionID, TurnId: turn, ToolCallId: start.ToolCallID, AgentId: start.AgentID, NodeId: c.binding.NodeID, NodeSessionId: c.binding.NodeSessionID, BindEpoch: c.binding.Ref.BindEpoch, Phase: "started", Outcome: "pending", StartedAtMs: start.StartedAt.UnixMilli(), Scheme: "http"}
	if start.HTTPS {
		record.Scheme = "https"
	}
	trafficRequestMetadata(record, start.Request)
	c.enqueue(&aiv1.AITrafficUpload{Record: record})
	var once sync.Once
	return func(end lowhttp.HTTPAttempt) {
		once.Do(func() {
			defer func() { c.mu.Lock(); c.active--; c.mu.Unlock(); c.signal() }()
			terminal := proto.Clone(record).(*aiv1.AITrafficRecord)
			terminal.Phase = "terminal"
			terminal.FinishedAtMs = end.FinishedAt.UnixMilli()
			terminal.Outcome = "success"
			if end.Error != nil {
				terminal.Outcome = "network_failed"
				if errors.Is(end.Error, context.Canceled) || errors.Is(end.Error, context.DeadlineExceeded) {
					terminal.Outcome = "cancelled"
				}
			}
			request, requestSize, requestTruncated, reqErr := trafficPacket(end.Request, end.RequestHeaderFile, end.RequestBodyFile, int64(c.policy.PacketLimitBytes))
			response, responseSize, responseTruncated, rspErr := trafficPacket(end.Response, end.ResponseHeaderFile, end.ResponseBodyFile, int64(c.policy.PacketLimitBytes))
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
		})
	}
}

func trafficRequestMetadata(record *aiv1.AITrafficRecord, packet []byte) {
	if req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(packet))); err == nil {
		record.Method = req.Method
		record.Host = req.Host
		if strings.ContainsAny(record.Host, "\r\n@") || len(record.Host) > 300 {
			record.Host = ""
		}
		req.Body.Close()
	}
	if len(record.Method) > 16 {
		record.Method = ""
	}
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
		if err := trafficAtomicWrite(filepath.Join(c.dir, "admitted-bytes"), []byte(fmt.Sprint(c.used))); err != nil {
			c.failed = true
		}
	}
	raw, err := proto.Marshal(upload)
	if err == nil {
		err = trafficAtomicWrite(filepath.Join(c.dir, upload.Record.FlowId+"."+upload.Record.Phase+".pb"), raw)
	}
	if err != nil {
		c.failed = true
	}
	c.mu.Unlock()
	if err != nil {
		failure := proto.Clone(upload.Record).(*aiv1.AITrafficRecord)
		failure.CaptureError = "spool_write_failed"
		c.emit("ai_traffic_batch", &aiv1.AITrafficBatch{ProtocolVersion: 1, Records: []*aiv1.AITrafficRecord{failure}})
	}
	c.signal()
}

func trafficAtomicWrite(path string, raw []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".traffic-")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
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
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
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
	c.uploadMu.Lock()
	defer c.uploadMu.Unlock()
	paths, err := c.pending()
	if err != nil {
		return
	}
	if len(paths) > 64 {
		paths = paths[:64]
	}
	var batch []*aiv1.AITrafficRecord
	for _, path := range paths {
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
		batch = append(batch, upload.Record)
		if len(batch) == 32 {
			c.emit("ai_traffic_batch", &aiv1.AITrafficBatch{ProtocolVersion: 1, Records: batch})
			batch = nil
		}
	}
	if len(batch) > 0 {
		c.emit("ai_traffic_batch", &aiv1.AITrafficBatch{ProtocolVersion: 1, Records: batch})
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
			if err = trafficAtomicWrite(receiptPath, receiptRaw); err != nil {
				return
			}
		}
		// Receipt is fsynced before queue removal; restart replays this step safely.
		if err = os.Remove(path); err == nil {
			if dir, e := os.Open(c.dir); e == nil {
				dir.Sync()
				dir.Close()
			}
		}
	}
}

func validAITrafficReceipt(receipt *aiv1.AITrafficReceipt, upload *aiv1.AITrafficUpload, raw []byte) bool {
	return receipt.GetProtocolVersion() == 1 && receipt.GetDurable() && receipt.GetFlowId() == upload.GetRecord().GetFlowId() && receipt.GetPhase() == upload.GetRecord().GetPhase() && receipt.GetUploadSha256() == trafficSHA(raw) && (receipt.GetStorageStatus() == "stored" || receipt.GetStorageStatus() == "quota_dropped")
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
		if result.Complete || ctx.Err() != nil {
			c.emit("ai_traffic_drain", result)
			return result
		}
		c.signal()
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h.collector.drain(ctx, "")
	h.collector.stopped.Do(func() { close(h.collector.stop) })
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
