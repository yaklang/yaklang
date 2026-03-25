package aicommon

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils/chanx"
)

type MemoryFlushBufferConfig struct {
	MaxPendingIterations int
	MaxPendingBytes      int
}

type MemoryFlushSignal struct {
	Iteration          int
	Task               AIStatefulTask
	IsDone             bool
	Reason             any
	ShouldEndIteration bool
}

type MemoryFlushPayload struct {
	FlushReason       string
	ContextualInput   string
	PendingIterations int
	PendingBytes      int
}

type memoryFlushAsyncJob struct {
	signal   MemoryFlushSignal
	callback func(*MemoryFlushPayload, error)
}

type MemoryFlushBuffer struct {
	label  string
	differ *TimelineDiffer
	config MemoryFlushBufferConfig

	mu                sync.Mutex
	pendingDiffs      []string
	pendingBytes      int
	pendingIterations int
	firstIteration    int
	lastIteration     int

	jobs      *chanx.UnlimitedChan[memoryFlushAsyncJob]
	workerOnce sync.Once
	closeOnce sync.Once
}

func DefaultMemoryFlushBufferConfig() MemoryFlushBufferConfig {
	return MemoryFlushBufferConfig{
		MaxPendingIterations: 3,
		MaxPendingBytes:      4096,
	}
}

func NewMemoryFlushBuffer(label string, differ *TimelineDiffer, config *MemoryFlushBufferConfig) *MemoryFlushBuffer {
	resolved := DefaultMemoryFlushBufferConfig()
	if config != nil {
		if config.MaxPendingIterations > 0 {
			resolved.MaxPendingIterations = config.MaxPendingIterations
		}
		if config.MaxPendingBytes > 0 {
			resolved.MaxPendingBytes = config.MaxPendingBytes
		}
	}
	return &MemoryFlushBuffer{
		label:  label,
		differ: differ,
		config: resolved,
		jobs:   chanx.NewUnlimitedChan[memoryFlushAsyncJob](context.Background(), 32),
	}
}

func (b *MemoryFlushBuffer) ProcessAsync(signal MemoryFlushSignal, callback func(*MemoryFlushPayload, error)) {
	if b == nil || b.differ == nil {
		if callback != nil {
			go callback(nil, nil)
		}
		return
	}

	b.startWorker()
	b.jobs.SafeFeed(memoryFlushAsyncJob{signal: signal, callback: callback})
}

func (b *MemoryFlushBuffer) Close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() {
		if b.jobs != nil {
			b.jobs.Close()
		}
	})
}

func (b *MemoryFlushBuffer) startWorker() {
	b.workerOnce.Do(func() {
		go func() {
			for job := range b.jobs.OutputChannel() {
				payload, err := b.Capture(job.signal)
				if job.callback != nil {
					job.callback(payload, err)
				}
			}
		}()
	})
}

func (b *MemoryFlushBuffer) Capture(signal MemoryFlushSignal) (*MemoryFlushPayload, error) {
	if b == nil || b.differ == nil {
		return nil, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	diffStr, err := b.differ.Diff()
	if err != nil {
		return nil, err
	}

	trimmedDiff := strings.TrimSpace(diffStr)
	if trimmedDiff != "" {
		if b.firstIteration == 0 {
			b.firstIteration = signal.Iteration
		}
		b.lastIteration = signal.Iteration
		b.pendingDiffs = append(b.pendingDiffs, diffStr)
		b.pendingBytes += len(diffStr)
		b.pendingIterations++
	}

	if len(b.pendingDiffs) == 0 {
		return nil, nil
	}

	flushReason := b.resolveFlushReason(signal)
	if flushReason == "" {
		return nil, nil
	}

	payload := &MemoryFlushPayload{
		FlushReason:       flushReason,
		ContextualInput:   b.buildContextualInput(signal, flushReason),
		PendingIterations: b.pendingIterations,
		PendingBytes:      b.pendingBytes,
	}
	b.reset()
	return payload, nil
}

func (b *MemoryFlushBuffer) resolveFlushReason(signal MemoryFlushSignal) string {
	if signal.Task != nil && signal.Task.IsAsyncMode() {
		return "milestone_async_mode"
	}
	if signal.IsDone {
		return "task_done"
	}
	if signal.ShouldEndIteration {
		return "milestone_end_iteration"
	}
	if b.config.MaxPendingIterations > 0 && b.pendingIterations >= b.config.MaxPendingIterations {
		return "batch_iteration_limit"
	}
	if b.config.MaxPendingBytes > 0 && b.pendingBytes >= b.config.MaxPendingBytes {
		return "batch_byte_limit"
	}
	return ""
}

func (b *MemoryFlushBuffer) buildContextualInput(signal MemoryFlushSignal, flushReason string) string {
	taskID := ""
	taskStatus := ""
	if signal.Task != nil {
		taskID = signal.Task.GetId()
		taskStatus = string(signal.Task.GetStatus())
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("记忆批处理[%s/%s]\n", b.label, flushReason))
	builder.WriteString(fmt.Sprintf("ReAct迭代范围: %d-%d/%s\n", b.firstIteration, b.lastIteration, taskID))
	builder.WriteString(fmt.Sprintf("累计轮次: %d\n", b.pendingIterations))
	builder.WriteString(fmt.Sprintf("累计字节: %d\n", b.pendingBytes))
	builder.WriteString(fmt.Sprintf("任务状态: %s\n", taskStatus))
	builder.WriteString(fmt.Sprintf("完成状态: %v\n", signal.IsDone))
	builder.WriteString(fmt.Sprintf("原因: %v\n", signal.Reason))
	builder.WriteString("---\n")
	builder.WriteString(strings.Join(b.pendingDiffs, "\n\n--- pending diff ---\n\n"))
	return builder.String()
}

func (b *MemoryFlushBuffer) reset() {
	b.pendingDiffs = nil
	b.pendingBytes = 0
	b.pendingIterations = 0
	b.firstIteration = 0
	b.lastIteration = 0
}
