package kafka

import (
	"github.com/segmentio/kafka-go"
)

type messageType int

const (
	TopicHeart messageType = iota + 1
	TopicTask
	TopicResponse
)

type RpcClient struct {
	Name string
	*kafka.Reader
}
