package kafka

import (
	"context"
	"encoding/json"
	"github.com/segmentio/kafka-go"
	"time"
)

type Topic string

const (
	ManagerTopic Topic = "manager"
	TaskTopic    Topic = "task"

	CallBack Topic = "callback"
)

type AgentWriter struct {
	ctx     context.Context
	cancel  context.CancelFunc
	writer  kafka.Writer
	config  *KafkaConfig
	address string
}

func NewWriter(ctx context.Context, address string, config *KafkaConfig) *AgentWriter {
	tcp := kafka.TCP(address)
	chileCtx, cancelFunc := context.WithCancel(ctx)
	return &AgentWriter{
		ctx:     chileCtx,
		cancel:  cancelFunc,
		address: address,
		writer: kafka.Writer{
			Addr:         tcp,
			WriteTimeout: time.Second * time.Duration(config.timeout),
			ReadTimeout:  time.Second * time.Duration(config.timeout),
			MaxAttempts:  3,
		},
		config: config,
	}
}

func (w *AgentWriter) writeMessage(msg any, topic Topic) error {
	marshal, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	var currentRetry int
RETRY:
	err = w.writer.WriteMessages(w.ctx, kafka.Message{
		Topic: string(topic),
		Value: marshal,
	})
	if err != nil {
		currentRetry++
		if currentRetry < w.config.retry {
			goto RETRY
		} else {
			return err
		}
	}
	return nil
}
