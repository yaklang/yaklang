package aicommon

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

// DebugStreamPrinter 把 AI 输出流事件以"人类友好"的方式打印到调试终端。
//
// 之前的实现是：来一个 delta 就立刻 fmt.Println 一次。当存在两条并发流
// （比如 system raw stream 和用户可见的 stream 一起产出 token）时, 每个
// token 都触发一次"流切换 + 换行 + 头部"，结果 DEBUG=1 的终端被切成
// 一行一个 token 的碎片，还会被 log.Info 从中间打断造成"夹心"。
//
// 新实现：
//
//   - 每条流（按 EventUUID 归属）有独立的累积缓冲区, 接收 delta 时只追加
//     到缓冲, 不立刻写出。
//   - 后台 flusher 每隔 `flushInterval` 把所有非空缓冲一次性写出, 每条流
//     输出一行: `[stream|node|uuid] <累积内容>\n`。
//   - 单次 Fprintln 调用对应一次原子的 stderr 写, 保证一行不被其他写入
//     从中间切断。
//   - 日志 / 非流事件触发 FlushIfActive 时, 立即 flush 所有缓冲, 再让日志
//     从新行开始, 避免"夹心"。
//   - 缓冲长时间没新内容（默认 5s）就被回收, 防止泄漏。
//
// 关键词: DEBUG=1 流式输出, AI stream delta coalesce, per-stream buffer flush
type DebugStreamPrinter struct {
	mu sync.Mutex

	// out 是真正的写出端, 默认 os.Stderr, 与 log 包同侧。
	out io.Writer

	// 每条流的累积缓冲, key = streamKeyOf(event)。
	buffers map[string]*streamLineBuf

	// 后台 flusher 的间隔, 0 表示禁用后台 flusher（仅靠 FlushIfActive）。
	flushInterval time.Duration

	// 缓冲多久没新增内容就被回收, 0 表示禁用。
	idleEvictAfter time.Duration

	// flusher 生命周期控制。
	flusherOnce sync.Once
	stopCh      chan struct{}
	stopped     bool
}

// streamLineBuf 单条流的累积状态。
type streamLineBuf struct {
	header     string
	content    bytes.Buffer
	lastUpdate time.Time
}

// defaultDebugStreamPrinter 进程级共享 printer, 所有调试入口都通过它写出,
// 这样跨包并发的流也能被同一把 mutex 串行化。
var defaultDebugStreamPrinter = &DebugStreamPrinter{
	out:            os.Stderr,
	buffers:        make(map[string]*streamLineBuf),
	flushInterval:  250 * time.Millisecond,
	idleEvictAfter: 5 * time.Second,
	stopCh:         make(chan struct{}),
}

// GetDefaultDebugStreamPrinter 返回进程级默认 printer。
func GetDefaultDebugStreamPrinter() *DebugStreamPrinter {
	return defaultDebugStreamPrinter
}

// NewDebugStreamPrinter 创建一个独立 printer, 一般只用于测试。
// flushInterval=0 时禁用后台 flusher, 全部依赖 FlushIfActive 手动触发。
func NewDebugStreamPrinter(out io.Writer) *DebugStreamPrinter {
	if out == nil {
		out = os.Stderr
	}
	return &DebugStreamPrinter{
		out:            out,
		buffers:        make(map[string]*streamLineBuf),
		flushInterval:  250 * time.Millisecond,
		idleEvictAfter: 5 * time.Second,
		stopCh:         make(chan struct{}),
	}
}

// streamKeyOf 用于把 AiOutputEvent 归到一条"逻辑流"。
// 优先 EventUUID（同一次 AI 调用的所有 delta 共享）, 其次回退到
// CoordinatorId + NodeId, 最差情况只用 NodeId。
func streamKeyOf(e *schema.AiOutputEvent) string {
	if e == nil {
		return ""
	}
	if e.EventUUID != "" {
		return "u:" + e.EventUUID
	}
	parts := make([]string, 0, 2)
	if e.CoordinatorId != "" {
		parts = append(parts, e.CoordinatorId)
	}
	if e.NodeId != "" {
		parts = append(parts, e.NodeId)
	}
	if len(parts) == 0 {
		return ""
	}
	return "c:" + strings.Join(parts, "|")
}

// shortUUID 取 EventUUID 前 8 个字符做头部展示, 避免占用过宽。
func shortUUID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	if s == "" {
		return "--------"
	}
	return s
}

// buildHeader 构造一条流的行首标记, 格式 [stream|node|uuid8] 。
// system / reason 流会替换 stream 标签, 便于区分思考流与正文流。
// 关键词: stream header label system reason
func buildHeader(e *schema.AiOutputEvent) string {
	label := "stream"
	switch {
	case e.IsReason:
		label = "reason"
	case e.IsSystem:
		label = "system"
	}
	node := e.NodeId
	if node == "" {
		node = "-"
	}
	return fmt.Sprintf("[%s|%s|%s] ", label, node, shortUUID(e.EventUUID))
}

// sanitizeDelta 把 delta 中的 \r / \n 转义为可视字面量, 保证行不被破坏。
// 关键词: sanitize delta escape newline
func sanitizeDelta(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	s := string(b)
	s = strings.ReplaceAll(s, "\r\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// PrintStreamDelta 把一个流 delta 追加到对应流的缓冲, 不立即写出。
// 真正写出由 flusher 周期性或 FlushIfActive 触发。
// 关键词: PrintStreamDelta append buffer
func (p *DebugStreamPrinter) PrintStreamDelta(e *schema.AiOutputEvent) {
	if e == nil || !e.IsStream || len(e.StreamDelta) == 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.ensureFlusherStartedLocked()

	key := streamKeyOf(e)
	buf, ok := p.buffers[key]
	if !ok {
		buf = &streamLineBuf{header: buildHeader(e)}
		p.buffers[key] = buf
	}
	buf.content.WriteString(sanitizeDelta(e.StreamDelta))
	buf.lastUpdate = time.Now()
}

// ensureFlusherStartedLocked 在 mutex 内启动后台 flusher（仅一次）。
// 关键词: lazy start flusher goroutine
func (p *DebugStreamPrinter) ensureFlusherStartedLocked() {
	if p.flushInterval <= 0 {
		return
	}
	p.flusherOnce.Do(func() {
		go p.flushLoop()
	})
}

// flushLoop 后台周期性 flush 所有累积超过半个 interval 的缓冲。
func (p *DebugStreamPrinter) flushLoop() {
	t := time.NewTicker(p.flushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			p.flushAll(false)
		case <-p.stopCh:
			p.flushAll(true)
			return
		}
	}
}

// flushAll 写出所有满足条件的缓冲。
//   - force=true: 全部写出, 不看新鲜度。
//   - force=false: 只写出在过去半个 interval 内"没有继续追加"的缓冲, 给
//     高频流一点合并时间, 避免一个 token 一行的退化。
//
// 关键词: flushAll force interval merge
func (p *DebugStreamPrinter) flushAll(force bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.flushAllLocked(force)
}

func (p *DebugStreamPrinter) flushAllLocked(force bool) {
	if len(p.buffers) == 0 {
		return
	}
	now := time.Now()
	staleAfter := p.flushInterval / 2
	for key, buf := range p.buffers {
		if buf.content.Len() == 0 {
			if p.idleEvictAfter > 0 && now.Sub(buf.lastUpdate) > p.idleEvictAfter {
				delete(p.buffers, key)
			}
			continue
		}
		if !force && staleAfter > 0 && now.Sub(buf.lastUpdate) < staleAfter {
			// 还在快速追加, 让它再攒一会
			continue
		}
		line := buf.header + buf.content.String()
		buf.content.Reset()
		// Fprintln 把 header+content+\n 作为单次 Write, 在多数操作系统下
		// stderr 的 write 是原子的, 这样可以避免和其他 goroutine 的 Write
		// 在字节层面交错。
		_, _ = fmt.Fprintln(p.out, line)
	}
}

// FlushIfActive 立即把所有非空缓冲全部写出, 让后续日志 / 普通事件
// 从干净的新行开始。命名沿用旧 API 是为了让调用方代码无感升级。
// 关键词: FlushIfActive immediate flush
func (p *DebugStreamPrinter) FlushIfActive() {
	p.flushAll(true)
}

// Reset 清空所有缓冲, 主要用于测试。
func (p *DebugStreamPrinter) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buffers = make(map[string]*streamLineBuf)
}

// Stop 停止后台 flusher 并把残留缓冲写出, 主要用于测试 / 收尾。
// 关键词: stop printer flusher shutdown
func (p *DebugStreamPrinter) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	close(p.stopCh)
	p.mu.Unlock()
}

// WrapLogWriter 返回一个 io.Writer 包装层, 在转发任何写入之前先
// FlushIfActive。把 common/log 的默认输出 SetOutput 到这个包装上, 就能
// 让"日志一定从新行开始"，彻底消除"流尾被日志夹心"问题。
//
// 注意：传入的 inner 应当是真正的目的端（默认通常是 os.Stderr）。
// 关键词: WrapLogWriter log 与流共享 mutex 防夹心
func (p *DebugStreamPrinter) WrapLogWriter(inner io.Writer) io.Writer {
	if inner == nil {
		inner = os.Stderr
	}
	return &debugStreamFlushingWriter{printer: p, inner: inner}
}

// debugStreamFlushingWriter 是 WrapLogWriter 返回的包装类型。
type debugStreamFlushingWriter struct {
	printer *DebugStreamPrinter
	inner   io.Writer
	writeMu sync.Mutex
}

func (w *debugStreamFlushingWriter) Write(b []byte) (int, error) {
	// 先 flush 所有流缓冲, 让日志从新行开始
	w.printer.FlushIfActive()
	// 把日志写入串行化, 避免多个日志 goroutine 之间互相切片
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return w.inner.Write(b)
}

// installedLogWrapperOnce 保证整个进程内 log 包装只安装一次, 避免
// 不同入口（grpc, ai-focus, ...）重复 SetOutput 导致嵌套包装。
var installedLogWrapperOnce sync.Once

// EnsureLogFlushWrapperInstalled 在 DEBUG=1 时把 common/log 的默认输出
// 包装上一层 FlushIfActive, 让任何日志写入之前先把流缓冲刷出。
// 多次调用安全, 只生效一次。非 DEBUG 模式下是 no-op, 避免影响生产行为。
//
// 调用方应当只在"AI 调试入口"被命中时调用（比如 StartAIReAct, ai-focus）,
// 这样普通 yak 脚本不会被改默认日志输出。
//
// 关键词: EnsureLogFlushWrapperInstalled DEBUG=1 log 输出 flush
func EnsureLogFlushWrapperInstalled() {
	if !isDebugEnv() {
		return
	}
	installedLogWrapperOnce.Do(func() {
		// 默认 golog 写到 os.Stderr; 用 Stderr 作为内层目的端足够覆盖
		// 绝大多数 DEBUG 场景。如果有人后续 log.SetOutput 到文件, 那是
		// 显式选择, 我们不应破坏。
		setLogOutput(defaultDebugStreamPrinter.WrapLogWriter(os.Stderr))
	})
}

// setLogOutput 通过函数变量注入, 避免 aicommon 包硬依赖 common/log
// 的 SetOutput 签名。EnsureLogFlushWrapperInstalled 调用此变量;
// 真正的赋值在 init_log_hook.go 里完成, 由 common/log 导出的 SetOutput
// 绑定上来。
var setLogOutput func(io.Writer) = func(io.Writer) {}

// isDebugEnv 返回 DEBUG / YAKLANGDEBUG / PALMDEBUG 是否被设置。
// 不直接依赖 common/utils 避免潜在的导入循环。
// 关键词: isDebugEnv DEBUG YAKLANGDEBUG PALMDEBUG
func isDebugEnv() bool {
	return os.Getenv("DEBUG") != "" ||
		os.Getenv("YAKLANGDEBUG") != "" ||
		os.Getenv("PALMDEBUG") != ""
}
