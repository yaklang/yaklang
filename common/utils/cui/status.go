package cui

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"
)

// 专门用来更新状态的 Frame
type StatusText struct {
	io.Reader

	interval time.Duration

	lastData []byte
	data     []byte

	rIndex int64

	readMux *sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

func NewStatusTextWithContext(interval time.Duration, initData []byte, baseCtx context.Context) *StatusText {
	ctx, cancel := context.WithCancel(baseCtx)

	return &StatusText{
		interval: interval,
		data:     initData,
		readMux:  new(sync.Mutex),
		ctx:      ctx,
		cancel:   cancel,
	}
}

func NewStatusText(interval time.Duration, initData []byte) *StatusText {
	return NewStatusTextWithContext(interval, initData, context.Background())
}

func (s *StatusText) Read(p []byte) (n int, err error) {

	s.readMux.Lock()
	defer s.readMux.Unlock()

	if bytes.Equal(s.data, s.lastData) {
		time.Sleep(s.interval)
		s.rIndex = 0
	}

	if s.rIndex >= int64(len(s.data)) {
		time.Sleep(s.interval)
		s.rIndex = 0
		s.lastData = s.data
		return
	}

	n = copy(p, s.data[s.rIndex:])
	s.rIndex += int64(n)
	return
}

func (s *StatusText) Update(data []byte) {
	s.readMux.Lock()
	defer s.readMux.Unlock()

	s.lastData = s.data
	s.data = data
}

func (s *StatusText) Close() {
	s.cancel()
}
