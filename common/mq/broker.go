package mq

import (
	"context"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"github.com/tevino/abool"
	"io"
	"sync"
	"time"
	"yaklang.io/yaklang/common/log"
)

type ChannelHandler func(broker *Broker, a *amqp.Channel) error
type ConnectionHandler func(broker *Broker, a *amqp.Connection) error

type Broker struct {
	ctx    context.Context
	cancel context.CancelFunc

	amqpUrl                        string
	onExchangeDeclare              []ChannelHandler
	onExchangeBind                 []ChannelHandler
	onQueueDeclare                 []ChannelHandler
	onQueueBind                    []ChannelHandler
	afterChannelCreated            []ChannelHandler
	afterConnectionCreated         []ConnectionHandler
	afterQueueAndExchangeDeclaring []ChannelHandler
	beforeChannelExit              []ChannelHandler
	beforeConnectionExit           []ConnectionHandler

	// dialConfig for amqp
	dialConfig amqp.Config

	// runtime
	consumingParams []*ConsumingParam

	// 是否正在运行中
	wg        *sync.WaitGroup
	isServing *abool.AtomicBool

	conn    *amqp.Connection
	channel *amqp.Channel

	defaultPublisher *Publisher
}

func (b *Broker) GetAuthBrokerConfigHandlers() []BrokerConfigHandler {
	return []BrokerConfigHandler{
		WithAMQPUrl(b.amqpUrl),
	}
}

func NewBroker(ctx context.Context, options ...BrokerConfigHandler) (*Broker, error) {
	rootCtx, cancel := context.WithCancel(ctx)
	broker := &Broker{
		amqpUrl:   "amqp://guest:guest@127.0.0.1:5672/",
		cancel:    cancel,
		ctx:       rootCtx,
		wg:        new(sync.WaitGroup),
		isServing: abool.New(),
	}
	for _, option := range options {
		option(broker)
	}

	broker.defaultPublisher = broker.GetPublisher()

	return broker, nil
}

func (b *Broker) DoConfigure(handlers ...BrokerConfigHandler) {
	for _, p := range handlers {
		p(b)
	}
}

func (b *Broker) Serve() {
	for {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			defer cancel()
			err := b.serve()
			if err != nil {
				log.Error("serve mq.Broker failed: %s", err)
			}
		}()

		select {
		case <-b.ctx.Done():
			log.Error("cancel mq.Broker: context done")
			return
		case <-ctx.Done():

		}

		log.Warn("retry in 3 sec")
		time.Sleep(3 * time.Second)
	}
}

func (b *Broker) serve() (err error) {
	//log.Infof("ampqUrl: %s", b.amqpUrl)
	conn, err := amqp.DialConfig(b.amqpUrl, b.dialConfig)
	if err != nil {
		return errors.Errorf("dial failed: %s", err)
	}
	b.conn = conn
	defer func() {
		for _, h := range b.beforeConnectionExit {
			err = h(b, conn)
			if err != nil {
				log.Error(err)
			}
		}
		err = conn.Close()
		if err != nil {
			log.Errorf("close conn failed: %s", err)
		}
	}()

	for _, h := range b.afterConnectionCreated {
		err = h(b, conn)
		if err != nil {
			return errors.Errorf("before conn create failed: %s", err)
		}
	}

	c, err := conn.Channel()
	if err != nil {
		return errors.Errorf("create channel failed: %s", err)
	}
	b.channel = c
	defer func() {
		for _, h := range b.beforeChannelExit {
			err = h(b, c)
			if err != nil {
				log.Error(err)
			}
		}

		err = c.Close()
		if err != nil {
			log.Errorf("exit channel failed: %s", err)
		}
	}()

	for _, h := range b.afterChannelCreated {
		err = h(b, c)
		if err != nil {
			return errors.Errorf("on channel created failed: %s", err)
		}
	}

	err = b.initDeclareAndBinding(c)
	if err != nil {
		log.Infof("init declaring and binding failed: %v", err)
		return err
	}

	for _, h := range b.afterQueueAndExchangeDeclaring {
		err = h(b, c)
		if err != nil {
			return errors.Errorf("after queue n exchange declaring failed: %s", err)
		}
	}

	b.wg = new(sync.WaitGroup)

	if len(b.consumingParams) <= 0 {
		log.Error("consuming failed for 0 consumers")
	}

	for _, p := range b.consumingParams {
		b.wg.Add(1)
		p := p
		go func() {
			defer b.wg.Done()
			err = b.consume(conn, c, p)
		}()
	}
	b.isServing.Set()
	defer b.isServing.UnSet()

	b.wg.Wait()

	return errors.New("normal exit / all consumers down / no consumers")
}

func (b *Broker) initDeclareAndBinding(c *amqp.Channel) (err error) {
	for _, h := range b.onExchangeDeclare {
		err = h(b, c)
		if err != nil {
			return errors.Errorf("declare exchange failed: %s", err)
		}
	}

	for _, h := range b.onQueueDeclare {
		err = h(b, c)
		if err != nil {
			return errors.Errorf("queue declare failed: %s", err)
		}
	}

	for _, h := range b.onQueueBind {
		err = h(b, c)
		if err != nil {
			return errors.Errorf("queue binding failed: %s", err)
		}
	}

	for _, h := range b.onExchangeBind {
		err = h(b, c)
		if err != nil {
			return errors.Errorf("exchange binding failed: %s", err)
		}
	}

	return nil
}

func (b *Broker) consume(conn *amqp.Connection, channel *amqp.Channel, p *ConsumingParam) error {
	ch, err := channel.Consume(p.Queue, p.Consumer, p.AutoACK, p.Exclusive, p.NoLocal, p.NoWait, p.Args)
	if err != nil {
		return errors.Errorf("consume failed: %s", err)
	}

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return errors.New("exit, channel error")
			}
			p.Handler(b, conn, channel, msg)
		case <-b.ctx.Done():
			return errors.Errorf("root context is done")
		}
	}
}

func (b *Broker) Close() {
	b.cancel()
}

func (b *Broker) CreateReader(p *ConsumingParam) (io.Reader, error) {
	b.wg.Add(1)

	data, err := b.channel.Consume(p.Queue, p.Consumer, p.AutoACK, p.Exclusive, p.NoLocal, p.NoWait, p.Args)
	if err != nil {
		return nil, errors.Errorf("amqp consume failed: %s", err)
	}

	r, w := io.Pipe()
	go func() {
		b.wg.Done()
		defer w.Close()

		for {
			select {
			case msg, ok := <-data:
				if !ok {
					return
				}

				_, err := w.Write(msg.Body)
				if err != nil {
					return
				}
			}
		}
	}()

	return r, nil
}

func (b *Broker) CreateWriter(exchange, key string) io.Writer {
	return NewAmqpWriter(b.channel, exchange, key)
}

func (b *Broker) IsServing() bool {
	return b.isServing.IsSet()
}

func (b *Broker) RunBackground() error {
	ticker := time.Tick(200 * time.Millisecond)

	go func() {
		b.Serve()
	}()

	var (
		count       = 0
		failedCount = 0
	)
	for {
		select {
		case <-ticker:
			if b.IsServing() {
				count++
			} else {
				failedCount++
			}

			if count >= 3 {
				return nil
			}

			if failedCount > 30 {
				return errors.New("failed to serve")
			}
		}
	}
}
