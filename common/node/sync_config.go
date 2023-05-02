package node

import (
	"github.com/streadway/amqp"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/mq"
	"yaklang.io/yaklang/common/spec"
	"yaklang.io/yaklang/common/utils"
)

func (c *NodeBase) getSyncConfigMqHandler() []mq.BrokerConfigHandler {
	queueName := spec.GetNodeBaseNotificationQueueByNodeId(c.NodeId)
	rKey := spec.GetNodeBaseNotificationRoutingKeyByNodeId(c.NodeId)
	return []mq.BrokerConfigHandler{
		mq.WithExchangeDeclare(&mq.ExchangeDeclaringParam{
			Name: spec.CommonServerPushExchange,
			Kind: "topic",
		}),
		mq.WithQueueDeclare(&mq.QueueDeclaringParam{
			Name:      queueName,
			Exclusive: true,
		}),
		mq.WithQueueBind(&mq.QueueBindingParam{
			Name:     queueName,
			Key:      rKey,
			Exchange: spec.CommonServerPushExchange,
		}),
		mq.WithQueueBind(&mq.QueueBindingParam{
			Name:     queueName,
			Key:      spec.CommonServerPushDefaultKey,
			Exchange: spec.CommonServerPushExchange,
		}),
		mq.WithConsumingParam(&mq.ConsumingParam{
			Queue:     queueName,
			Exclusive: true,
			Handler:   c.onNotificationFromServer,
		}),
		mq.BeforeChannelExit(func(broker *mq.Broker, a *amqp.Channel) error {
			_, _ = a.QueueDelete(queueName, false, false, false)
			return nil
		}),
	}
}

func (c *NodeBase) onNotificationFromServer(b *mq.Broker, conn *amqp.Connection, channel *amqp.Channel, msg amqp.Delivery) {
	_ = channel.Ack(msg.DeliveryTag, false)

	if utils.InDebugMode() {
		log.Infof("notification recv from server: k:(%v) body:%v", msg.RoutingKey, string(msg.Body))
	}

	for _, f := range c.onNotificationComingFuncs {
		f(&msg)
	}
}
