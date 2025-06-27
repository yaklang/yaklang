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

type RandomChunkedSender struct {
	ctx            context.Context
	requestPacket  []byte
	maxChunkLength int
	minChunkLength int
	maxDelay       time.Duration
	minDelay       time.Duration
	handler        ChunkedResultHandler
}

type randomChunkedHTTPOption func(*RandomChunkedSender)

func _withRandomChunkCtx(ctx context.Context) randomChunkedHTTPOption {
	return func(r *RandomChunkedSender) {
		if ctx != nil {
			r.ctx = ctx
		}
	}
}

func _withRandomChunkChunkLength(min, max int) randomChunkedHTTPOption {
	return func(r *RandomChunkedSender) {
		r.maxChunkLength = max
		r.minChunkLength = min
	}
}

func _withRandomChunkDelay(min, max time.Duration) randomChunkedHTTPOption {
	return func(r *RandomChunkedSender) {
		r.maxDelay = max
		r.minDelay = min
	}
}

func _withRandomChunkResultHandler(f ChunkedResultHandler) randomChunkedHTTPOption {
	return func(r *RandomChunkedSender) {
		r.handler = f
	}
}

func newDefaultRandomChunkSender() *RandomChunkedSender {
	return &RandomChunkedSender{
		ctx:            context.Background(),
		maxChunkLength: 25,
		minChunkLength: 10,
		maxDelay:       time.Millisecond * 100,
		minDelay:       time.Millisecond * 50,
		handler:        nil,
	}
}

func NewRandomChunkedSender(
	options ...randomChunkedHTTPOption,
) (*RandomChunkedSender, error) {
	sender := newDefaultRandomChunkSender()
	for _, option := range options {
		option(sender)
	}
	// 验证配置
	if sender.maxChunkLength <= 0 || sender.minChunkLength <= 0 {
		return nil, utils.Error("chunked config error: chunk length should greater than zero")
	}
	if sender.maxChunkLength < sender.minChunkLength {
		return nil, utils.Error("chunked config error: max chunk length should greater than min chunk length")
	}
	if sender.maxDelay < 0 || sender.minDelay < 0 {
		return nil, utils.Error("chunked config error: delay should greater than zero")
	}
	if sender.maxDelay < sender.minDelay {
		return nil, utils.Error("chunked config error: max delay should greater than min delay")
	}

	return sender, nil
}

func (r *RandomChunkedSender) getRandomDelayTime() time.Duration {
	delayRange := r.maxDelay - r.minDelay
	if delayRange <= 0 {
		return r.minDelay
	}
	randomDelay := time.Duration(rand.Int63n(int64(delayRange)))
	return r.minDelay + randomDelay
}

func (r *RandomChunkedSender) calcRandomChunkedLen() int {
	lenRange := r.maxChunkLength - r.minChunkLength
	if lenRange <= 0 {
		return r.minChunkLength
	}
	randomLength := rand.Intn(lenRange)
	return randomLength + r.minChunkLength
}

// writeAndFlush 写入数据并立即flush，确保数据及时发送到服务端
func (r *RandomChunkedSender) writeAndFlush(writer io.Writer, data []byte, errMsg string) error {
	if _, err := writer.Write(data); err != nil {
		return utils.Errorf("%s: %s", errMsg, err)
	}

	// 检查writer是否支持Flush，如果支持则立即flush
	if flusher, ok := writer.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			return utils.Errorf("flush after %s: %s", errMsg, err)
		}
	}

	return nil
}

func (r *RandomChunkedSender) Send(rawPacket []byte, writer io.Writer) error {
	// header换成chunked的 body保持不变
	r.requestPacket = HTTPHeaderForceChunked(rawPacket)
	headers, body := SplitHTTPHeadersAndBodyFromPacket(r.requestPacket)

	// 发送HTTP头部
	if err := r.writeAndFlush(writer, []byte(headers), "send headers"); err != nil {
		return err
	}

	reader := bytes.NewReader(body)
	totalSize := len(body)
	sentBytes := 0
	chunkCount := 0

	// 记录发送开始时间
	startTime := time.Now()
	lastChunkTime := time.Now()
	for sentBytes < totalSize {
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		default:
		}

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

		// 发送分块长度头
		chunkHeader := fmt.Sprintf("%x\r\n", n)
		if err := r.writeAndFlush(writer, []byte(chunkHeader), "send chunk header"); err != nil {
			return err
		}

		// 发送分块内容
		chunkData := append(buffer[:n], []byte(CRLF)...)
		if err := r.writeAndFlush(writer, chunkData, "send chunk content"); err != nil {
			return err
		}

		sentBytes += n
		chunkCount++

		totalDuration := time.Since(startTime)
		chunkDuration := time.Since(lastChunkTime)
		lastChunkTime = time.Now()
		if r.handler != nil {
			r.handler(chunkCount, buffer[:n], totalDuration, chunkDuration)
		}

		// 添加随机延迟
		if sentBytes < totalSize {
			delay := r.getRandomDelayTime()
			if delay > 0 {
				select {
				case <-r.ctx.Done():
					return r.ctx.Err()
				case <-time.After(delay):
				}
			}
		}
	}

	// 发送结束分块标记
	endChunk := fmt.Sprintf("0%s", DoubleCRLF)
	if err := r.writeAndFlush(writer, []byte(endChunk), "send end chunk"); err != nil {
		return err
	}
	return nil
}
