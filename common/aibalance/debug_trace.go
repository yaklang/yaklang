// debug_trace.go - aibalance 上下游字节流抓包基础设施
//
// 用途: 给 aibalance 加一对"上游 raw HTTP request/response 字节流" + "下行
// 发给客户端的 SSE 字节流" 的对照抓包, 用来定位"直连 deepseek OK / 经 aibalance
// 中转却老中断"这类协议级故障. 不抓不分析、靠看 log 推测的诊断方式已经被现实
// 证明不够 — 这里把字节真正落盘.
//
// 触发:
//   - env AIBALANCE_DEBUG_TRACE=1  (开关, 不指定目录时落 $TMPDIR/aibalance-trace)
//   - env AIBALANCE_TRACE_DIR=/path (指定目录, 等价于隐式打开开关)
//   - utils.Debug(func() { ... }) 已经在 InDebugMode() 下生效, 这里复用同套语义
//
// 默认关闭, 不会污染生产 IO. 文件失败仅记 log, 绝不阻塞主流程.
//
// 每个请求一个目录, 目录名 = ${ts}-${reqID}, 内含:
//
//	00.meta.json               请求元信息 (model, provider, route, tool mode 等)
//	01.client_request.txt      客户端 POST /v1/chat/completions 的 raw HTTP body
//	02.upstream_request.raw    aibalance -> 上游 wrapper 的 raw HTTP request bytes
//	03.upstream_response.raw   上游 -> aibalance 的 raw HTTP response (header + 完整 body)
//	04.downstream_response.sse aibalance -> 客户端 的 raw bytes (HTTP header + chunked SSE body)
//	05.summary.txt             汇总 (开始/结束时间, 上下游字节数, finish_reason, tool_calls 数)
//
// 关键词: aibalance debug trace, 上下游字节对比, AIBALANCE_DEBUG_TRACE,
//
//	RawHTTPRequestResponseCallback 接线, conn wrap dump
package aibalance

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// DebugTraceEnabled 全局开关. 通过 env AIBALANCE_DEBUG_TRACE / AIBALANCE_TRACE_DIR
// 任一非空触发. 不强制要求 utils.InDebugMode() (那是更宽松的全局 debug 开关,
// 但 aibalance trace 涉及落盘, 必须有显式独立 env 防止生产意外开启).
//
// 关键词: aibalance trace env 开关, 默认关闭, 显式独立 env 防误开
func DebugTraceEnabled() bool {
	if strings.TrimSpace(os.Getenv("AIBALANCE_DEBUG_TRACE")) != "" {
		return true
	}
	if strings.TrimSpace(os.Getenv("AIBALANCE_TRACE_DIR")) != "" {
		return true
	}
	return false
}

// DebugTraceRootDir 决定 trace 文件落盘根目录.
//   - 优先 env AIBALANCE_TRACE_DIR 指定路径
//   - 否则 $TMPDIR/aibalance-trace 或 /tmp/aibalance-trace
//
// 关键词: aibalance trace dir 解析
func DebugTraceRootDir() string {
	if d := strings.TrimSpace(os.Getenv("AIBALANCE_TRACE_DIR")); d != "" {
		return d
	}
	base := os.TempDir()
	if base == "" {
		base = "/tmp"
	}
	return filepath.Join(base, "aibalance-trace")
}

// DebugTraceSession 单次 chat completion 请求的全链路抓包会话.
//
// 生命周期: 在 serveChatCompletions 入口处 NewDebugTraceSession 创建,
// defer (*DebugTraceSession).Close 关闭. env 关闭时 NewDebugTraceSession
// 返回 nil, 所有方法对 nil receiver 安全 (no-op), 调用方无需到处 if s != nil.
//
// 并发: 同一 session 的所有 Write* 串行化 (mu 保护). 并发 OK 但语义不保序.
//
// 关键词: DebugTraceSession 抓包会话, nil-safe receiver, 上下游字节对比
type DebugTraceSession struct {
	reqID   string
	rootDir string

	startedAt time.Time

	metaFile             *os.File
	clientRequestFile    *os.File
	upstreamRequestFile  *os.File
	upstreamResponseFile *os.File
	downstreamFile       *os.File

	downstreamBytes atomic.Int64
	upstreamBytes   atomic.Int64

	closed atomic.Bool
	mu     sync.Mutex
}

// NewDebugTraceSession 工厂. env 关闭时返回 nil (no-op 模式).
//
//	reqID 用调用方现有 trace_id / request_id; 为空时自动生成 8 字符随机串.
//
// 关键词: NewDebugTraceSession 工厂, env-gated 默认 no-op
func NewDebugTraceSession(reqID string) *DebugTraceSession {
	if !DebugTraceEnabled() {
		return nil
	}
	if strings.TrimSpace(reqID) == "" {
		reqID = utils.RandStringBytes(8)
	}
	startedAt := time.Now()
	dir := filepath.Join(DebugTraceRootDir(), startedAt.Format("20060102-150405.000")+"-"+reqID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Errorf("debug_trace: mkdir %s failed: %v", dir, err)
		return nil
	}
	open := func(name string) *os.File {
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			log.Errorf("debug_trace: open %s failed: %v", path, err)
			return nil
		}
		return f
	}
	s := &DebugTraceSession{
		reqID:                reqID,
		rootDir:              dir,
		startedAt:            startedAt,
		metaFile:             open("00.meta.json"),
		clientRequestFile:    open("01.client_request.txt"),
		upstreamRequestFile:  open("02.upstream_request.raw"),
		upstreamResponseFile: open("03.upstream_response.raw"),
		downstreamFile:       open("04.downstream_response.sse"),
	}
	log.Infof("debug_trace: session started req=%s dir=%s", reqID, dir)
	return s
}

// ReqID 返回本 session 的 request id, 用于跨日志关联.
func (s *DebugTraceSession) ReqID() string {
	if s == nil {
		return ""
	}
	return s.reqID
}

// RootDir 返回本 session 的落盘目录, 便于 log + 测试断言.
func (s *DebugTraceSession) RootDir() string {
	if s == nil {
		return ""
	}
	return s.rootDir
}

// WriteMeta 写 00.meta.json (一次性). meta 是任意可 JSON 化的元信息,
// 推荐传 map[string]any 包含 model / provider 身份 / route / tool mode 等.
//
// 关键词: WriteMeta 元信息落盘, JSON 序列化
func (s *DebugTraceSession) WriteMeta(meta any) {
	if s == nil || s.metaFile == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		log.Errorf("debug_trace: marshal meta failed: %v", err)
		return
	}
	if _, err := s.metaFile.Write(raw); err != nil {
		log.Errorf("debug_trace: write meta failed: %v", err)
	}
}

// WriteClientRequest 写 01.client_request.txt (一次性), 内容是客户端原始请求体.
// 关键词: WriteClientRequest 客户端请求体落盘
func (s *DebugTraceSession) WriteClientRequest(body []byte) {
	if s == nil || s.clientRequestFile == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.clientRequestFile.Write(body); err != nil {
		log.Errorf("debug_trace: write client_request failed: %v", err)
	}
}

// WriteUpstreamRequestResponse 接 aispec.WithRawHTTPRequestResponseCallback,
// 在 stream 路径下会在 body 消费完后一次性触发, 包含完整上游字节流.
// 写入 02.upstream_request.raw + 03.upstream_response.raw.
//
//	requestBytes:        aibalance -> upstream wrapper 的 raw HTTP request bytes
//	responseHeaderBytes: upstream -> aibalance 的 HTTP response 起始行 + header
//	bodyPreview:         在 stream 模式下是 aispec TeeReader 累积下来的**完整** body
//	                     (流读完时再回调, 见 base.go::executeChatBaseRequest io.TeeReader)
//	usage:               aispec 解析到的 ChatUsage (可为 nil)
//
// 关键词: WriteUpstreamRequestResponse, aispec raw callback 接线, stream 完整 body
func (s *DebugTraceSession) WriteUpstreamRequestResponse(requestBytes, responseHeaderBytes, bodyPreview []byte, usage *aispec.ChatUsage) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.upstreamRequestFile != nil {
		if _, err := s.upstreamRequestFile.Write(requestBytes); err != nil {
			log.Errorf("debug_trace: write upstream_request failed: %v", err)
		}
	}
	if s.upstreamResponseFile != nil {
		if _, err := s.upstreamResponseFile.Write(responseHeaderBytes); err != nil {
			log.Errorf("debug_trace: write upstream_response header failed: %v", err)
		}
		if _, err := s.upstreamResponseFile.Write(bodyPreview); err != nil {
			log.Errorf("debug_trace: write upstream_response body failed: %v", err)
		}
		s.upstreamBytes.Add(int64(len(responseHeaderBytes) + len(bodyPreview)))
	}
	if usage != nil {
		raw, _ := json.Marshal(usage)
		log.Infof("debug_trace: upstream usage req=%s usage=%s", s.reqID, string(raw))
	}
}

// teeDownstreamWrite 是 dumpConn.Write 内部的同步 tee 路径.
// 把下行字节写到 04.downstream_response.sse, 失败仅 log.
func (s *DebugTraceSession) teeDownstreamWrite(p []byte) {
	if s == nil || s.downstreamFile == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.downstreamFile.Write(p); err != nil {
		log.Errorf("debug_trace: write downstream failed: %v", err)
	}
	s.downstreamBytes.Add(int64(len(p)))
}

// Close 关闭所有文件, 写 05.summary.txt. 幂等.
//
// 关键词: DebugTraceSession Close, summary 汇总, 幂等
func (s *DebugTraceSession) Close() {
	if s == nil {
		return
	}
	if !s.closed.CompareAndSwap(false, true) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	endedAt := time.Now()
	summary := fmt.Sprintf(
		"req_id:          %s\n"+
			"root_dir:        %s\n"+
			"started_at:      %s\n"+
			"ended_at:        %s\n"+
			"duration_ms:    %d\n"+
			"upstream_bytes:  %d\n"+
			"downstream_bytes: %d\n",
		s.reqID, s.rootDir,
		s.startedAt.Format(time.RFC3339Nano),
		endedAt.Format(time.RFC3339Nano),
		endedAt.Sub(s.startedAt).Milliseconds(),
		s.upstreamBytes.Load(),
		s.downstreamBytes.Load(),
	)
	if path := filepath.Join(s.rootDir, "05.summary.txt"); path != "" {
		if err := os.WriteFile(path, []byte(summary), 0o644); err != nil {
			log.Errorf("debug_trace: write summary failed: %v", err)
		}
	}
	closers := []*os.File{
		s.metaFile, s.clientRequestFile,
		s.upstreamRequestFile, s.upstreamResponseFile,
		s.downstreamFile,
	}
	for _, f := range closers {
		if f != nil {
			_ = f.Close()
		}
	}
	log.Infof("debug_trace: session closed req=%s up=%dB down=%dB dir=%s",
		s.reqID, s.upstreamBytes.Load(), s.downstreamBytes.Load(), s.rootDir)
}

// dumpConn wrap net.Conn 把所有 Write 字节 tee 到 trace session 的 04 文件.
// 对 Read 透明; 不破坏 net.Conn 其它语义. trace=nil 时直接退化为原 conn 行为
// (WrapConnForTrace 会跳过 wrap, 这里不会被构造).
//
// 关键词: dumpConn net.Conn wrap, 下行字节 tee, 不破坏 deadline/Close/LocalAddr
type dumpConn struct {
	net.Conn
	trace *DebugTraceSession
}

// Write 同步 tee 到 trace 文件, 然后真实写出.
// 关键词: dumpConn Write tee, 下行字节抓包
func (d *dumpConn) Write(p []byte) (int, error) {
	if d.trace != nil {
		d.trace.teeDownstreamWrite(p)
	}
	return d.Conn.Write(p)
}

// WrapConnForTrace 把 conn 用 dumpConn 包一层. trace=nil 时直接返回原 conn,
// 零开销; 否则返回 *dumpConn 实例.
//
// 关键词: WrapConnForTrace conn 包装, 零开销 fallback
func WrapConnForTrace(conn net.Conn, trace *DebugTraceSession) net.Conn {
	if trace == nil || conn == nil {
		return conn
	}
	return &dumpConn{Conn: conn, trace: trace}
}

// debugTraceCallbackForAispec 构造一个 aispec.RawHTTPRequestResponseCallback
// 闭包接到给定 trace session, 用于 provider.GetAIClientWithRawMessagesAndTrace.
// trace=nil 时返回 nil callback (aispec 会按现有行为不调用).
//
// 关键词: debug trace -> aispec 接线 helper, RawHTTPRequestResponseCallback 闭包
func debugTraceCallbackForAispec(trace *DebugTraceSession) aispec.RawHTTPRequestResponseCallback {
	if trace == nil {
		return nil
	}
	return func(requestBytes, responseHeaderBytes, bodyPreview []byte, usage *aispec.ChatUsage) {
		trace.WriteUpstreamRequestResponse(requestBytes, responseHeaderBytes, bodyPreview, usage)
	}
}

// debugTraceLazyMeta 是 utils.Debug 风格的"惰性求值 + 仅在 trace 开启时执行".
// 调用方传一个 func() any, 仅在 trace != nil 时调用, 把返回值序列化进 meta.json.
//
// 关键词: utils.Debug 风格惰性求值, trace meta 落盘
func debugTraceLazyMeta(trace *DebugTraceSession, f func() any) {
	if trace == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("debug_trace: lazy meta panic: %v", r)
		}
	}()
	meta := f()
	if meta == nil {
		return
	}
	trace.WriteMeta(meta)
}

// 编译期接口断言: dumpConn 必须满足 net.Conn 接口.
var _ net.Conn = (*dumpConn)(nil)

// 编译期接口断言: io.Writer 兼容性 (downstreamFile 等都是 *os.File 隐式 io.Writer).
var _ io.Writer = (*os.File)(nil)
