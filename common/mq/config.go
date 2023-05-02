package mq

import (
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"yaklang.io/yaklang/common/log"
)

type BrokerConfigHandler func(b *Broker)

func WithAMQPUrl(url string) BrokerConfigHandler {
	return func(b *Broker) {
		b.amqpUrl = url
	}
}

type ExchangeDeclaringParam struct {
	Name       string
	Kind       string
	Durable    bool
	AutoDelete bool
	Internel   bool
	NoWait     bool
	Args       amqp.Table
}

func WithExchangeDeclare(p *ExchangeDeclaringParam) BrokerConfigHandler {
	return func(b *Broker) {
		b.onExchangeDeclare = append(b.onExchangeDeclare, func(b *Broker, a *amqp.Channel) error {
			return a.ExchangeDeclare(p.Name, p.Kind, p.Durable, p.AutoDelete, p.Internel, p.NoWait, p.Args)
		})
	}
}

type ExchangeBindingParam struct {
	Destination string
	Key         string
	Source      string
	NoWait      bool
	Args        amqp.Table
}

func WithExchangeBind(p *ExchangeBindingParam) BrokerConfigHandler {
	return func(b *Broker) {
		b.onExchangeBind = append(b.onExchangeBind, func(b *Broker, a *amqp.Channel) error {
			return a.ExchangeBind(p.Destination, p.Key, p.Source, p.NoWait, p.Args)
		})
	}
}

type QueueDeclaringParam struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool
	Args       amqp.Table
}

func WithQueueDeclare(p *QueueDeclaringParam) BrokerConfigHandler {
	return func(b *Broker) {
		b.onQueueDeclare = append(b.onQueueDeclare, func(b *Broker, a *amqp.Channel) error {
			queue, err := a.QueueDeclare(p.Name, p.Durable, p.AutoDelete, p.Exclusive, p.NoWait, p.Args)
			if err != nil {
				return errors.Errorf("queue[%s] declare failed: %s", p.Name, err)
			}
			log.Infof("queue: %s is declared: ", queue.Name)
			return nil
		})
	}
}

func WithQueueDeclarePassive(p *QueueDeclaringParam) BrokerConfigHandler {
	return func(b *Broker) {
		b.onQueueDeclare = append(b.onQueueDeclare, func(b *Broker, a *amqp.Channel) error {
			queue, err := a.QueueDeclarePassive(p.Name, p.Durable, p.AutoDelete, p.Exclusive, p.NoWait, p.Args)
			if err != nil {
				return errors.Errorf("queue[%s] declare failed: %s", p.Name, err)
			}
			log.Infof("queue: %s is declared: ", queue.Name)
			return nil
		})
	}
}

type QueueBindingParam struct {
	Name     string
	Key      string
	Exchange string
	NoWait   bool
	Args     amqp.Table
}

func WithQueueBind(p *QueueBindingParam) BrokerConfigHandler {
	return func(b *Broker) {
		b.onQueueBind = append(b.onQueueBind, func(b *Broker, a *amqp.Channel) error {
			return a.QueueBind(p.Name, p.Key, p.Exchange, p.NoWait, p.Args)
		})
	}
}

func HookAfterChannelCreated(c ChannelHandler) BrokerConfigHandler {
	return func(b *Broker) {
		b.afterChannelCreated = append(b.afterChannelCreated, c)
	}
}

func HookAfterConnectionCreated(c ConnectionHandler) BrokerConfigHandler {
	return func(b *Broker) {
		b.afterConnectionCreated = append(b.afterConnectionCreated, c)
	}
}

func HookAfterQueueAndExchangeDeclaring(c ChannelHandler) BrokerConfigHandler {
	return func(b *Broker) {
		b.afterQueueAndExchangeDeclaring = append(b.afterQueueAndExchangeDeclaring, c)
	}
}

func BeforeChannelExit(c ChannelHandler) BrokerConfigHandler {
	return func(b *Broker) {
		b.beforeChannelExit = append(b.beforeChannelExit, c)
	}
}

func BeforeConnectionExit(c ConnectionHandler) BrokerConfigHandler {
	return func(b *Broker) {
		b.beforeConnectionExit = append(b.beforeConnectionExit, c)
	}
}

func WithDialConfig(c amqp.Config) BrokerConfigHandler {
	return func(b *Broker) {
		b.dialConfig = c
	}
}

type ConsumingParam struct {
	Queue     string
	Consumer  string
	AutoACK   bool
	Exclusive bool
	NoLocal   bool
	NoWait    bool
	Args      amqp.Table
	Handler   func(b *Broker, conn *amqp.Connection, channel *amqp.Channel, msg amqp.Delivery)
}

func WithConsumingParam(p *ConsumingParam) BrokerConfigHandler {
	return func(b *Broker) {
		b.consumingParams = append(b.consumingParams, p)
	}
}
