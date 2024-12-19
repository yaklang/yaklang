package kafka

import (
	"context"
	"encoding/json"
	"github.com/segmentio/kafka-go"
	"time"
)

type Topic string

const (
	heartTopic Topic = "heart"
	LogTopic   Topic = "log"
	//use pulic mode,use token

	ManagerTopic Topic = "manager"

	TaskTopic        Topic = "task"
	AgentBackMessage Topic = "agent_back_message"
)

type WriterConfig struct {
	retry   int
	timeout int
}

type WriterOptions func(config *WriterConfig)

func WithWriterConfigRetry(retry int) WriterOptions {
	return func(config *WriterConfig) {
		config.retry = retry
	}
}
func defaultWriterConfig() *WriterConfig {
	return &WriterConfig{
		retry:   3,
		timeout: 5,
	}
}

type AgentWriter struct {
	ctx     context.Context
	cancel  context.CancelFunc
	writer  kafka.Writer
	config  *WriterConfig
	address string
}

func NewWriter(ctx context.Context, address string, opts ...WriterOptions) *AgentWriter {
	config := defaultWriterConfig()
	for _, opt := range opts {
		opt(config)
	}
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

func (w *AgentWriter) WriteHeartRequest(request *Request) error {
	return w.writeMessage(request, heartTopic)
}
func (w *AgentWriter) WriteTaskRequest(request *Request) error {
	return w.writeMessage(request, TaskTopic)
}
func (w *AgentWriter) WriteRequest(request *Request, topic Topic) error {
	return w.writeMessage(request, topic)
}

// WriteAgentResponse 只有Agent返回扫描任务的结果 需要进行type的区分
func (w *AgentWriter) WriteAgentResponse(response *Response) error {
	return w.WriteResponse(response, AgentBackMessage)
}
func (w *AgentWriter) WriteResponse(response *Response, topic Topic) error {
	return w.writeMessage(response, topic)
}
func (w *AgentWriter) WriteLog(response *Request) error {
	return w.writeMessage(response, LogTopic)
}
