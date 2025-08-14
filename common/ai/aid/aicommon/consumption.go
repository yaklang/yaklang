package aicommon

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

// CreateConsumptionReader 创建一个新的 Reader，每秒回调当前的 token 数量
func CreateConsumptionReader(r io.Reader, callback func(current int)) io.Reader {
	return &consumptionReader{
		reader:   r,
		callback: callback,
		ctx:      context.Background(),
		cancel:   func() {},
	}
}

type consumptionReader struct {
	reader   io.Reader
	callback func(current int)
	count    atomic.Int64
	once     atomic.Bool
	ctx      context.Context
	cancel   context.CancelFunc
}

// estimateTokens 估算内容的 token 数量
func estimateTokens(data []byte) int {
	tokens := 0
	isASCII := true
	asciiCount := 0

	for _, b := range data {
		if b >= 128 {
			// 非 ASCII 字符（如中文），每个字节都计算
			isASCII = false
			tokens++
		} else {
			if isASCII {
				// ASCII 字符，每 4 个字符算一个 token
				asciiCount++
				if asciiCount >= 4 {
					tokens++
					asciiCount = 0
				}
			} else {
				// 重置状态
				isASCII = true
				asciiCount = 1
			}
		}
	}

	// 处理剩余的 ASCII 字符
	if asciiCount > 0 {
		tokens++
	}

	return tokens
}

func (t *consumptionReader) Read(p []byte) (n int, err error) {
	// 启动计数器，确保只启动一次
	if !t.once.Swap(true) {
		t.ctx, t.cancel = context.WithCancel(context.Background())
		go t.startCounter()
	}

	// 读取数据
	n, err = t.reader.Read(p)
	if n > 0 {
		// 估算 token 数量并累加
		tokens := estimateTokens(p[:n])
		t.count.Add(int64(tokens))
	}

	// 如果读取结束或出错，取消计数器
	if err != nil {
		t.cancel()
	}

	return n, err
}

func (t *consumptionReader) startCounter() {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			current := t.count.Load()
			if t.callback != nil {
				t.callback(int(current))
			}
		case <-t.ctx.Done():
			if t.callback != nil {
				t.callback(int(t.count.Load()))
			}
			return
		}
	}
}
