package lowhttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type ChunkedResultHandler func(id int, chunkRaw []byte, totalTime time.Duration, chunkSendTime time.Duration)

type randomChunkedSender struct {
	ctx            context.Context
	requestPacket  []byte
	maxChunkLength int
	minChunkLength int
	maxDelay       time.Duration
	minDelay       time.Duration
	handler        ChunkedResultHandler
}

type RandomChunkedHTTPOption func(*randomChunkedSender)

func WithRandomChunkedContext(ctx context.Context) RandomChunkedHTTPOption {
	return func(r *randomChunkedSender) {
		if ctx != nil {
			r.ctx = ctx
		}
	}
}

func WithRandomChunkedLength(minChunk, maxChunk int) RandomChunkedHTTPOption {
	return func(r *randomChunkedSender) {
		if maxChunk > 0 && minChunk > 0 && maxChunk >= minChunk {
			r.maxChunkLength = maxChunk
			r.minChunkLength = minChunk
		}
	}
}

func WithRandomChunkedDelay(minDelay, maxDelay time.Duration) RandomChunkedHTTPOption {
	return func(r *randomChunkedSender) {
		if maxDelay > 0 && minDelay > 0 && maxDelay >= minDelay {
			r.maxDelay = maxDelay
			r.minDelay = minDelay
		}
	}
}

func WithRandomChunkedHandler(handler ChunkedResultHandler) RandomChunkedHTTPOption {
	return func(r *randomChunkedSender) {
		r.handler = handler
	}
}

func newRandomChunkedSender(requestPacket []byte, opts ...RandomChunkedHTTPOption) (*randomChunkedSender, error) {
	// 设置默认值
	sender := &randomChunkedSender{
		ctx:            context.Background(),
		requestPacket:  HTTPHeaderForceChunked(requestPacket), // header换成chunked的 body保持不变
		maxChunkLength: 1024,                                  // 默认最大1KB
		minChunkLength: 256,                                   // 默认最小256B
		maxDelay:       100 * time.Millisecond,                // 默认最大100ms
		minDelay:       50 * time.Millisecond,                 // 默认最小50ms
		handler:        nil,
	}

	for _, opt := range opts {
		opt(sender)
	}

	// 验证配置
	if sender.maxChunkLength <= 0 || sender.minChunkLength <= 0 {
		return nil, utils.Error("chunked config error: chunk length should greater than zero")
	}
	if sender.maxChunkLength < sender.minChunkLength {
		return nil, utils.Error("chunked config error: max chunk length should greater than min chunk length")
	}
	if sender.maxDelay <= 0 || sender.minDelay <= 0 {
		return nil, utils.Error("chunked config error: delay should greater than zero")
	}
	if sender.maxDelay < sender.minDelay {
		return nil, utils.Error("chunked config error: max delay should greater than min delay")
	}

	return sender, nil
}

func (r *randomChunkedSender) getRandomDelayTime() time.Duration {
	delayRange := r.maxDelay - r.minDelay
	if delayRange <= 0 {
		return r.minDelay
	}
	randomDelay := time.Duration(rand.Int63n(int64(delayRange)))
	return r.minDelay + randomDelay
}

func (r *randomChunkedSender) calcRandomChunkedLen() int {
	lenRange := r.maxChunkLength - r.minChunkLength
	if lenRange <= 0 {
		return r.minChunkLength
	}
	randomLength := rand.Intn(lenRange)
	return randomLength + r.minChunkLength
}

func (r *randomChunkedSender) send(writer io.Writer) error {
	headers, body := SplitHTTPHeadersAndBodyFromPacket(r.requestPacket)

	// 发送HTTP头部
	if _, err := writer.Write([]byte(headers)); err != nil {
		return utils.Errorf("send headers failed: %s", err)
	}

	// 处理空body的情况
	if len(body) == 0 {
		endChunk := []byte(fmt.Sprintf("0%s", DoubleCRLF))
		if _, err := writer.Write(endChunk); err != nil {
			return utils.Errorf("send empty end chunk failed: %s", err)
		}
		return nil
	}

	reader := bytes.NewReader(body)
	totalSize := len(body)
	sentBytes := 0
	chunkCount := 0

	// 记录发送开始时间
	startTime := time.Now()
	for sentBytes < totalSize {
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		default:
		}
		chunkStartTime := time.Now()

		chunkSize := r.calcRandomChunkedLen()
		remainingBytes := totalSize - sentBytes
		if chunkSize > remainingBytes {
			chunkSize = remainingBytes
		}

		// 读取当前分块数据
		buffer := make([]byte, chunkSize)
		n, err := reader.Read(buffer)
		if err != nil {
			return utils.Errorf("read chunk %d failed: %s", chunkCount, err)
		}
		// 没有更多数据
		if n == 0 {
			break
		}

		// 发送分块长度头 (十六进制格式)
		chunkHeader := fmt.Sprintf("%x\r\n", n)
		if _, err := writer.Write([]byte(chunkHeader)); err != nil {
			return utils.Errorf("send chunk %d header failed: %s", chunkCount, err)
		}

		// 发送分块内容
		chunkData := append(buffer[:n], []byte(CRLF)...)
		if _, err := writer.Write(chunkData); err != nil {
			return utils.Errorf("send chunk %d content failed: %s", chunkCount, err)
		}

		sentBytes += n
		chunkCount++

		// 添加随机延迟
		if sentBytes < totalSize {
			delay := r.getRandomDelayTime()
			if delay > 0 {
				select {
				case <-r.ctx.Done():
					return r.ctx.Err()
				case <-time.After(delay):
					// 延迟完成，继续下一个分块
				}
			}
		}

		// 每个块的结果
		totalDuration := time.Since(startTime)
		chunkDuration := time.Since(chunkStartTime)
		if r.handler != nil {
			r.handler(chunkCount, buffer[:n], totalDuration, chunkDuration)
		}
	}

	// 发送结束分块标记
	endChunk := fmt.Sprintf("0%s", DoubleCRLF)
	if _, err := writer.Write([]byte(endChunk)); err != nil {
		return utils.Errorf("send end chunk failed: %s", err)
	}

	return nil
}

func SendRandomChunkedHTTP(writer io.Writer, req []byte, opts ...RandomChunkedHTTPOption) error {
	chunker, err := newRandomChunkedSender(req, opts...)
	if err != nil {
		return utils.Errorf("create chunked sender failed: %s", err)
	}
	return chunker.send(writer)
}
