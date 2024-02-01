package mq

import (
	uuid "github.com/google/uuid"
	"github.com/streadway/amqp"
	"time"
)

func (b *Broker) CreatePublisherFunc(exchange string, routingKey string) func(msg amqp.Publishing) error {
	var pub = b.defaultPublisher
	if pub == nil {
		b.defaultPublisher = b.GetPublisher()
	}

	return func(msg amqp.Publishing) error {
		return pub.PublishTo(exchange, routingKey, msg)
	}
}

func (b *Broker) CreateRPCClient(exchange string, id string) (*RPCClient, error) {
	if id == "" {
		id = uuid.New().String()
	}
	return NewRPCClientWithBroker(b, exchange, id)
}

func NewPublishingMessageFromRPC(rpcId, responseTo string) *amqp.Publishing {
	return &amqp.Publishing{
		CorrelationId: rpcId,
		ReplyTo:       responseTo,
		Timestamp:     time.Now(),
	}
}
