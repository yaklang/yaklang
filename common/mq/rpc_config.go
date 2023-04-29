package mq

import (
	"fmt"
	"github.com/streadway/amqp"
)

func RPCClientConfig(exchange, id string, cb func(requestId string, msg *amqp.Delivery)) BrokerConfigHandler {
	queueName := fmt.Sprintf("rpc-client-queue-%v", id)
	rKey := fmt.Sprintf("rpc-response.%v", id)
	return func(b *Broker) {
		WithQueueDeclare(&QueueDeclaringParam{
			Name:       queueName,
			AutoDelete: true,
			Exclusive:  true,
		})(b)

		WithQueueBind(&QueueBindingParam{
			Name:     queueName,
			Key:      rKey,
			Exchange: exchange,
		})(b)

		WithConsumingParam(&ConsumingParam{
			Queue:     queueName,
			Exclusive: true,
			Handler: func(b *Broker, conn *amqp.Connection, channel *amqp.Channel, msg amqp.Delivery) {
				_ = channel.Ack(msg.DeliveryTag, false)
				cb(msg.CorrelationId, &msg)
			},
		})(b)

		BeforeChannelExit(func(broker *Broker, a *amqp.Channel) error {
			_, _ = a.QueuePurge(queueName, false)
			_ = a.QueueUnbind(queueName, rKey, exchange, nil)
			_, err := a.QueueDelete(queueName, false, false, false)
			if err != nil {
				return err
			}
			return nil
		})
	}
}
