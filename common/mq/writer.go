package mq

import (
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"io"
)

type AmqpWriter struct {
	io.Writer
	ch *amqp.Channel

	exchange, key string
}

func (a *AmqpWriter) Write(p []byte) (n int, err error) {
	err = a.ch.Publish(a.exchange, a.key, false, false, amqp.Publishing{Body: p})
	if err != nil {
		return 0, errors.Errorf("publish failed: %s", err)
	}
	return len(p), nil
}

func NewAmqpWriter(c *amqp.Channel, exchange, key string) *AmqpWriter {
	return &AmqpWriter{
		ch:       c,
		exchange: exchange,
		key:      key,
	}
}
