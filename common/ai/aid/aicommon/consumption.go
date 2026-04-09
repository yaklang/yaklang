package aicommon

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai/ytoken"
)

// CreateConsumptionReader creates a Reader that periodically reports accumulated token count via callback.
// An optional externalTokenCounter can be provided to also accumulate tokens into a shared atomic counter
// (useful when multiple streams feed into a single total).
func CreateConsumptionReader(r io.Reader, callback func(current int), externalTokenCounter ...*atomic.Int64) io.Reader {
	cr := &consumptionReader{
		reader:   r,
		callback: callback,
		ctx:      context.Background(),
		cancel:   func() {},
	}
	if len(externalTokenCounter) > 0 {
		cr.externalTokenCounter = externalTokenCounter[0]
	}
	return cr
}

type consumptionReader struct {
	reader               io.Reader
	callback             func(current int)
	count                atomic.Int64
	once                 atomic.Bool
	ctx                  context.Context
	cancel               context.CancelFunc
	externalTokenCounter *atomic.Int64
}

// estimateTokens calculates token count using Qwen BPE vocabulary.
func estimateTokens(data []byte) int {
	return ytoken.CalcTokenCount(string(data))
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
		tokens := int64(estimateTokens(p[:n]))
		t.count.Add(tokens)
		if t.externalTokenCounter != nil {
			t.externalTokenCounter.Add(tokens)
		}
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
