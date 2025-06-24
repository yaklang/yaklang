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

type ChunkedResultHandler func(id int, chunkRaw []byte, totalTime time.Duration, chunkSendTime time.Duration, isEnd bool)

type RandomChunkedSender struct {
	ctx            context.Context
	requestPacket  []byte
	maxChunkLength int
	minChunkLength int
	maxDelay       time.Duration
	minDelay       time.Duration
	handler        ChunkedResultHandler
}

type RandomChunkedHTTPOption func(*RandomChunkedSender)

func NewRandomChunkedSender(
	ctx context.Context,
	minChunkLength, maxChunkLength int,
	minDelay, maxDelay time.Duration,
	handler ...ChunkedResultHandler,
) (*RandomChunkedSender, error) {
	sender := &RandomChunkedSender{
		ctx:            context.Background(),
		maxChunkLength: maxChunkLength,
		minChunkLength: minChunkLength,
		maxDelay:       maxDelay,
		minDelay:       minDelay,
		handler:        nil,
	}

	if ctx != nil {
		sender.ctx = ctx
	}
	if len(handler) > 0 {
		sender.handler = handler[0]
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

func (r *RandomChunkedSender) Send(rawPacket []byte, writer io.Writer) error {
	// header换成chunked的 body保持不变
	r.requestPacket = HTTPHeaderForceChunked(rawPacket)
	headers, body := SplitHTTPHeadersAndBodyFromPacket(r.requestPacket)

	// 发送HTTP头部
	if _, err := writer.Write([]byte(headers)); err != nil {
		return utils.Errorf("send headers failed: %s", err)
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
			r.handler(chunkCount, buffer[:n], totalDuration, chunkDuration, false)
		}
	}

	// 发送结束分块标记
	endChunkNow := time.Now()
	endChunk := fmt.Sprintf("0%s", DoubleCRLF)
	if _, err := writer.Write([]byte(endChunk)); err != nil {
		return utils.Errorf("send end chunk failed: %s", err)
	}
	if r.handler != nil {
		r.handler(chunkCount+1, []byte(endChunk), time.Since(startTime), time.Since(endChunkNow), true)
	}
	return nil
}
