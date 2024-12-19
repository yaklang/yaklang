package kafka

import (
	"context"
	"encoding/json"
	"github.com/segmentio/kafka-go"
	"github.com/yaklang/yaklang/common/log"
	"time"
)

type ReaderConfig struct {
	timeout  int
	maxBytes int64
	retry    int
}

func defaultReaderConfig() *ReaderConfig {
	return &ReaderConfig{
		timeout:  3,
		maxBytes: 1024 * 1024 * 10,
		retry:    3,
	}
}

type ReaderOptions func(config *ReaderConfig)

func WithReadMaxBytes(maxBytes int64) ReaderOptions {
	return func(config *ReaderConfig) {
		config.maxBytes = maxBytes
	}
}
func WithRetry(retry int) ReaderOptions {
	return func(config *ReaderConfig) {
		config.retry = retry
	}
}
func WithTimeout(timeout int) ReaderOptions {
	return func(config *ReaderConfig) {
		config.timeout = timeout
	}
}

type AgentReader[T any] struct {
	ctx     context.Context
	cancel  context.CancelFunc
	reader  *kafka.Reader
	message chan T
	config  *ReaderConfig
}

func NewReader[T any](ctx context.Context, address string, groupId string, topic string, opts ...ReaderOptions) *AgentReader[T] {
	config := defaultReaderConfig()
	childCtx, cancelFunc := context.WithCancel(ctx)
	for _, i := range opts {
		i(config)
	}
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{address},
		Topic:          topic,
		StartOffset:    kafka.FirstOffset,
		CommitInterval: 1 * time.Second,
		GroupID:        groupId,
	})
	r := &AgentReader[T]{
		ctx:     childCtx,
		cancel:  cancelFunc,
		reader:  reader,
		message: make(chan T, 1024),
		config:  config,
	}
	return r
}
func (r *AgentReader[T]) ReadMessage(ctx context.Context) <-chan T {
	go func() {
		defer func() {
			close(r.message)
		}()
		var errorCount int
		for {
			message, err := r.reader.ReadMessage(ctx)
			if err != nil {
				errorCount++
				if errorCount < r.config.retry {
					log.Infof("read message fail: %s,retry count: %v", err, errorCount)
					continue
				} else {
					log.Errorf("reader read message fail: %s", err)
					break
				}
			}
			var msg T
			if err = json.Unmarshal(message.Value, &msg); err != nil {
				log.Errorf("unmarshal msg fail: %s", err)
				continue
			}
			select {
			case <-r.ctx.Done():
				return
			case r.message <- msg:
			}
		}
	}()
	return r.message
}
