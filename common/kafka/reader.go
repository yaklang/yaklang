package kafka

import (
	"context"
	"encoding/json"
	"github.com/segmentio/kafka-go"
	"github.com/yaklang/yaklang/common/log"
	"time"
)

type AgentReader[T any] struct {
	ctx    context.Context
	cancel context.CancelFunc
	reader *kafka.Reader
	config *KafkaConfig
}

func NewReader[T any](ctx context.Context, address string, groupId string, topic Topic, config *KafkaConfig) *AgentReader[T] {
	childCtx, cancelFunc := context.WithCancel(ctx)
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{address},
		Topic:          string(topic),
		StartOffset:    kafka.FirstOffset,
		CommitInterval: 1 * time.Second,
		GroupID:        groupId,
	})
	r := &AgentReader[T]{
		ctx:    childCtx,
		cancel: cancelFunc,
		reader: reader,
		config: config,
	}
	return r
}
func (r *AgentReader[T]) ReadMessage(ctx context.Context) <-chan T {
	var msgChannel = make(chan T, 1024)
	go func() {
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
				break
			case msgChannel <- msg:
			}
		}
		close(msgChannel)
		if err := r.reader.Close(); err != nil {
			log.Errorf("failed to closed reader: %s", err)
		}
		return
	}()
	return msgChannel
}
