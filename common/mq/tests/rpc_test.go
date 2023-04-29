package tests

import (
	"context"
	"github.com/streadway/amqp"
	"github.com/stretchr/testify/assert"
	"yaklang/common/mq"
	"yaklang/common/thirdpartyservices"
	"testing"
	"time"
)

type HealthInfo struct {
	Timestamp int64 `json:"timestamp"`
}

type InspectNodeRequest struct {
	NodeId string `json:"node_id"`
}

func Test_RPC(t *testing.T) {
	test := assert.New(t)
	u := thirdpartyservices.GetAMQPUrl()

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	exchageName := "rpc-test"

	server, err := mq.NewRPCServer(
		ctx,
		exchageName, "testNode",
		mq.WithAMQPUrl(u), mq.WithExchangeDeclare(&mq.ExchangeDeclaringParam{
			Name: exchageName,
			Kind: "direct",
		}), mq.BeforeChannelExit(func(broker *mq.Broker, a *amqp.Channel) error {
			a.ExchangeDelete(exchageName, false, false)
			return nil
		}),
	)
	if !test.Nil(err) {
		return
	}

	server.RegisterService("testFunc", func(broker *mq.Broker, ctx context.Context, f, node string, delivery *amqp.Delivery) (message interface{}, e error) {
		time.Sleep(1 * time.Second)

		return &HealthInfo{
			Timestamp: time.Now().Unix(),
		}, nil
	})

	err = server.RunBackground()
	if !test.Nil(err) {
		return
	}

	// client
	b, err := mq.NewRPCClient(
		ctx, exchageName,
		mq.WithExchangeDeclare(&mq.ExchangeDeclaringParam{
			Name: exchageName,
			Kind: "direct",
		}),
		mq.WithAMQPUrl(u),
	)
	if !test.Nil(err) {
		return
	}
	err = b.Connect()
	if !test.Nil(err) {
		return
	}

	buf, err := b.Call(ctx, "testFunc", "testNode", &InspectNodeRequest{
		NodeId: "testNode",
	})
	if !test.Nil(err) {
		return
	}

	test.Greater(len(buf), 0)
}
