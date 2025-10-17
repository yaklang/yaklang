package aicommon

import (
	"bytes"
	"io"
	"math/rand"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type streamEvent struct {
	startTime          time.Time
	isSystem           bool
	isReason           bool
	reader             io.Reader
	nodeId             string
	taskIndex          string
	disableMarkdown    bool
	contentType        string
	emitFinishCallback []func()
}

func newStreamAIOutputEventWriter(
	id string,
	emit BaseEmitter,
	timeStamp int64,
	eventWriterID string,
	event *streamEvent,
) *streamAIOutputEventWriter {
	nodeId := event.nodeId
	system := event.isSystem
	reason := event.isReason
	disableMarkdown := event.disableMarkdown
	taskIndex := event.taskIndex
	return &streamAIOutputEventWriter{
		coordinatorId:   id,
		nodeId:          nodeId,
		disableMarkdown: disableMarkdown,
		isSystem:        system,
		isReason:        reason,
		handler:         emit,
		timeStamp:       timeStamp,
		eventWriterID:   eventWriterID,
		taskIndex:       taskIndex,
		contentType:     event.contentType,
	}
}

type streamAIOutputEventWriter struct {
	isReason        bool
	isSystem        bool
	disableMarkdown bool
	coordinatorId   string
	nodeId          string
	contentType     string
	taskIndex       string
	handler         BaseEmitter
	timeStamp       int64
	eventWriterID   string
}

func (e *streamAIOutputEventWriter) Write(b []byte) (int, error) {
	if e.handler == nil {
		log.Error("eventWriteProducer: Event handler is nil")
		return 0, nil
	}

	if len(b) == 0 {
		return 0, nil
	}

	event := &schema.AiOutputEvent{
		CoordinatorId:   e.coordinatorId,
		NodeId:          e.nodeId,
		Type:            schema.EVENT_TYPE_STREAM,
		IsSystem:        e.isSystem,
		IsReason:        e.isReason,
		IsStream:        true,
		StreamDelta:     utils.CopyBytes(b),
		Timestamp:       e.timeStamp, // the event in same stream should have the same timestamp
		EventUUID:       e.eventWriterID,
		TaskIndex:       e.taskIndex,
		DisableMarkdown: e.disableMarkdown,
		ContentType:     e.contentType,
	}
	e.handler(event)
	return len(b), nil
}

func TypeWriterWrite(dst io.Writer, src string, bps int) (written int64, err error) {
	return TypeWriterCopy(dst, bytes.NewBufferString(src), bps)
}

// TypeWriterCopy 实现打字机模式的 Copy，以约 200 byte/sec 的速率随机延迟打印
// 模拟 AI 快速输出效果，提升用户体验
func TypeWriterCopy(dst io.Writer, src io.Reader, bytesPerSeconds int) (written int64, err error) {
	buf := make([]byte, 4)
	// 200 tokens/s => 平均每个 token 延迟 5ms
	// 为了模拟更自然的输出，使用随机延迟 (1-10ms)
	if bytesPerSeconds <= 0 {
		bytesPerSeconds = 200
	}
	var avgTokenTime = time.Second / time.Duration(bytesPerSeconds)

	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			// 将读取的数据写入目标
			wn, writeErr := dst.Write(buf[:n])
			written += int64(wn)
			if writeErr != nil {
				return written, writeErr
			}

			// 根据写入的字节数计算延迟
			// 假设平均每个 UTF-8 字符大约 1-3 字节，这里简单估算为 1-2 个 token
			byteCount := int64(n)
			estimatedTokens := byteCount / 2
			if estimatedTokens < 1 {
				estimatedTokens = 1
			}

			// 计算基础延迟时间
			baseDelay := time.Duration(estimatedTokens) * avgTokenTime

			// 添加随机波动 (±50%)，使输出看起来更自然
			randomFactor := 0.5 + (rand.Float64()) // 0.5 到 1.5
			delay := time.Duration(float64(baseDelay) * randomFactor)

			// 添加小的随机抖动 (0-3ms)
			jitter := time.Duration(rand.Intn(3)) * time.Millisecond
			finalDelay := delay + jitter

			time.Sleep(finalDelay)
		}

		if readErr != nil {
			if readErr != io.EOF {
				return written, readErr
			}
			break
		}
	}

	return written, nil
}
