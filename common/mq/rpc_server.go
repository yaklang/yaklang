package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"strings"
	"sync"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type serverProcessing struct {
	CorrelationId string
	Context       context.Context
	Cancel        context.CancelFunc
}

func rpcServerConfig(
	exchange, node string, funcNames []string,
	cb func(
		broker *Broker, ctx context.Context, // 基础信息
		f, node string, // 消息来源
		delivery *amqp.Delivery) (interface{}, error),
) BrokerConfigHandler {
	getRoutingKey := func(f, n string) string {
		return fmt.Sprintf("rpc.%v.%v", f, n)
	}
	processing := new(sync.Map)

	param := &QueueDeclaringParam{
		Name:       fmt.Sprintf("node-queue.%v.%v", node, utils.CalcSha1(funcNames)),
		Durable:    false,
		AutoDelete: true,
		Exclusive:  true,
	}
	return func(b *Broker) {
		WithQueueDeclare(param)(b)
		for _, f := range funcNames {
			rK := getRoutingKey(f, node)
			//log.Infof("bind routine key: %s to queue: %s", rK, param.Name)
			WithQueueBind(&QueueBindingParam{
				Name:     param.Name,
				Key:      rK,
				Exchange: exchange,
			})(b)
		}
		BeforeChannelExit(func(broker *Broker, a *amqp.Channel) error {
			_, err := a.QueueDelete(param.Name, false, false, false)
			if err != nil {
				return err
			}
			return nil
		})
		WithConsumingParam(&ConsumingParam{
			Queue:     param.Name,
			Exclusive: true,
			Handler: func(b *Broker, conn *amqp.Connection, channel *amqp.Channel, msg amqp.Delivery) {
				_ = channel.Ack(msg.DeliveryTag, false)

				//log.Infof("recv %v", param.Name)
				if !strings.HasPrefix(msg.RoutingKey, "rpc.") {
					return
				}

				result := strings.Split(msg.RoutingKey, ".")
				if len(result) != 3 {
					log.Errorf("error rpc key in rpc server: %v", msg.RoutingKey)
					return
				}
				f, node := result[1], result[2]

				if f == "" || node == "" {
					log.Error("routingkey error, func and node cannot be empty")
					return
				}
				if msg.CorrelationId == "" {
					log.Error("rpcId(amqp.Delivery.CorrelationId) cannot be empty")
					return
				}

				if msg.AppId == "" {
					log.Error("clientId(amqp.Delivery.AppId) cannot be empty")
					return
				}

				// 设置上下文, 然后如果遇到了取消事件, 就应该结束
				if raw, ok := processing.Load(msg.CorrelationId); ok {
					if msg.Type == RPC_MessageType_Cancel {
						defer processing.Delete(msg.CorrelationId)
						p, valid := raw.(*serverProcessing)
						if !valid {
							log.Error("internal error: BUG! serverProcessing error!")
							return
						}
						p.Cancel()
						return
					} else {
						log.Errorf("repeated request: %v", spew.Sdump(msg))
						return
					}
				} else {
					ctx, cancel := context.WithCancel(b.ctx)
					actualCancel := func() {
						cancel()
						processing.Delete(msg.CorrelationId)
					}
					processing.Store(msg.CorrelationId, &serverProcessing{
						CorrelationId: msg.CorrelationId,
						Context:       ctx,
						Cancel:        actualCancel,
					})
					toRKey := fmt.Sprintf("rpc-response.%v", msg.AppId)

					go func() {
						defer actualCancel()

						err := b.defaultPublisher.PublishTo(exchange, toRKey, amqp.Publishing{
							CorrelationId: msg.CorrelationId,
							ReplyTo:       msg.AppId,
							Type:          RPC_MessageType_RequestReceived,
						})
						if err != nil {
							log.Errorf("request recv signal send failed: %s", err)
							return
						}

						buf, err := cb(b, ctx, f, node, &msg)
						if err != nil {
							msg := amqp.Publishing{
								CorrelationId: msg.CorrelationId,
								ReplyTo:       msg.AppId,
								Timestamp:     time.Now(),
								Type:          RPC_MessageType_Error,
							}
							msg.Body = []byte(err.Error())
							err = b.defaultPublisher.PublishTo(exchange, toRKey, msg)
							if err != nil {
								log.Error(err)
							}
							return
						}

						body, err := json.Marshal(buf)
						if err != nil {
							msg := amqp.Publishing{
								CorrelationId: msg.CorrelationId,
								ReplyTo:       msg.AppId,
								Timestamp:     time.Now(),
								Type:          RPC_MessageType_Error,
							}
							msg.Body = []byte(err.Error())
							err = b.defaultPublisher.PublishTo(exchange, toRKey, msg)
							if err != nil {
								log.Error(err)
							}
							return
						}

						err = b.defaultPublisher.PublishTo(
							exchange, toRKey,
							amqp.Publishing{
								CorrelationId: msg.CorrelationId,
								ReplyTo:       msg.AppId,
								Timestamp:     time.Now(),
								Type:          RPC_MessageType_Response,
								Body:          body,
							})
						if err != nil {
							log.Error(err)
							return
						}
					}()
				}
			},
		})(b)
	}
}

type RPCServer struct {
	NodeId   string
	Exchange string
	ctx      context.Context
	cancel   context.CancelFunc
	broker   *Broker
}

func (r *RPCServer) RegisterServices(funcNames []string,
	cb func(broker *Broker, ctx context.Context, f, node string, delivery *amqp.Delivery) (message interface{}, e error)) {
	r.broker.DoConfigure(
		rpcServerConfig(r.Exchange, r.NodeId, funcNames, cb),
	)
}

func (r *RPCServer) RegisterService(funcName string,
	cb func(broker *Broker, ctx context.Context, f, node string, delivery *amqp.Delivery) (message interface{}, e error)) {
	r.broker.DoConfigure(rpcServerConfig(r.Exchange, r.NodeId, []string{funcName}, cb))
}

func NewRPCServer(ctx context.Context, exchange, nodeId string, options ...BrokerConfigHandler) (*RPCServer, error) {
	rCtx, cancel := context.WithCancel(ctx)

	broker, err := NewBroker(rCtx, options...)
	if err != nil {
		return nil, errors.Errorf("build broker failed: %v", err)
	}

	return &RPCServer{
		NodeId:   nodeId,
		Exchange: exchange,
		broker:   broker,
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

func (r *RPCServer) RunBackground() error {
	return r.broker.RunBackground()
}

func (r *RPCServer) Serve() {
	r.broker.Serve()
}

func (r *RPCServer) IsServing() bool {
	return r.broker.IsServing()
}

func (r *RPCServer) DoConfigure(options ...BrokerConfigHandler) {
	r.broker.DoConfigure(options...)
}

// 一定用在 Serve 之前
func (r *RPCServer) GetRPCClient(id string) (*RPCClient, error) {
	if id == "" {
		return nil, errors.New("id cannot be empty")
	}
	return NewRPCClientWithBroker(r.broker, r.Exchange, id)
}

func (r *RPCServer) GetBroker() *Broker {
	return r.broker
}
