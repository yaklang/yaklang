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
		StartOffset:    kafka.LastOffset,
		CommitInterval: time.Second * 1,
		GroupID:        groupId,
		MaxAttempts:    3,
		//ErrorLogger:    golog.New().SetOutput(os.Stdout),
		//Logger:         golog.New().SetOutput(os.Stdout),
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
				if errorCount < r.config.Retry {
					log.Infof("read message fail: %s,retry count: %v", err, errorCount)
					continue
				} else {
					log.Errorf("reader read message fail: %s", err)
					break
				}
			}
			marshal, err := json.Marshal(message)
			if err != nil {
				continue
			}
			log.Debug(string(marshal))
			var msg T
			if err = json.Unmarshal(message.Value, &msg); err != nil {
				log.Errorf("unmarshal msg fail: %s", err)
				continue
			}
			select {
			case <-r.ctx.Done():
				break
			case msgChannel <- msg:
				continue
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
func (r *AgentReader[T]) Close() {
	r.reader.Close()
}
