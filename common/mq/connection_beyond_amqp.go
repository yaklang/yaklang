package mq

import (
	"context"
	"encoding/json"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/streadway/amqp"
	"net"
	"sync"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Connection Models
type ConnectionFrame struct {
	From string `json:"from"`
	Buf  []byte `json:"buf"`

	// 服务端和客户端都应该处理，客户端收到这个，就直接取消全部上下文
	// 服务端收到这个，马上关闭文件，删除缓存
	Closed bool `json:"closed"`

	// 只有服务端处理这个，创建会话
	First bool `json:"first"`
}

type Listener struct {
	net.Listener

	broker     *Broker
	addr       string
	conns      *sync.Map
	accepts    chan *Connection
	concurrent int

	ctx    context.Context
	cancel context.CancelFunc
}

func NewListener(b *Broker, addr string, concurrent int) (*Listener, error) {
	c, cancel := context.WithCancel(context.Background())
	l := &Listener{
		concurrent: concurrent,
		broker:     b,
		addr:       addr,
		conns:      new(sync.Map),
		accepts:    make(chan *Connection),
		ctx:        c,
		cancel:     cancel,
	}
	err := l.init()
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Listener) newConnection(from string) *Connection {
	c := &Connection{
		local:  l.addr,
		remote: from,
		broker: l.broker,
		rChan:  make(chan []byte),
	}
	c.ctx, c.cancel = context.WithCancel(l.ctx)
	c.writeCtx, c.writeCancel = context.WithCancel(l.ctx)
	c.readCtx, c.readCancel = context.WithCancel(l.ctx)
	return c
}

func (l *Listener) init() error {
	swg := utils.NewSizedWaitGroup(100)
	l.broker.DoConfigure(
		WithQueueDeclare(&QueueDeclaringParam{
			Name:       remoteQueue(l.addr),
			AutoDelete: true,
			Exclusive:  true,
		}),
		WithConsumingParam(&ConsumingParam{
			Queue:     remoteQueue(l.addr),
			AutoACK:   false,
			Exclusive: true,
			Handler: func(b *Broker, conn *amqp.Connection, channel *amqp.Channel, msg amqp.Delivery) {
				defer channel.Ack(msg.DeliveryTag, false)

				select {
				case <-l.ctx.Done():
					return
				default:
				}

				var frame ConnectionFrame
				err := json.Unmarshal(msg.Body, &frame)
				if err != nil {
					return
				}

				// 如果 closed 的话，就删除本地缓存
				if frame.Closed {
					log.Infof("recv close signal for [%s]", frame.From)
					raw, ok := l.conns.Load(frame.From)
					if !ok {
						return
					}
					_ = raw.(*Connection).Close()
					l.conns.Delete(frame.From)
					return
				}

				if frame.First {
					c := l.newConnection(frame.From)
					l.conns.Store(frame.From, c)
					swg.Add()
					go func() {
						defer swg.Done()

						l.accepts <- c
					}()
					return
				}

				raw, ok := l.conns.Load(frame.From)
				if !ok {
					return
				}

				c := raw.(*Connection)

				swg.Add()
				go func() {
					swg.Done()

					c.rChan <- frame.Buf
				}()
			},
		}),
		BeforeChannelExit(func(broker *Broker, a *amqp.Channel) error {
			_, _ = a.QueueDelete(remoteQueue(l.addr), false, false, false)
			return nil
		}),
	)
	return nil
}

func (l *Listener) Accept() (*Connection, error) {
	select {
	case <-l.ctx.Done():
		return nil, utils.Errorf("context done")
	case c, ok := <-l.accepts:
		if !ok {
			return nil, utils.Errorf("accepts closed")
		}
		return c, nil
	}
}

func (l *Listener) Close() error {
	close(l.accepts)
	l.cancel()
	l.conns.Range(func(key, value interface{}) bool {
		_ = value.(*Connection).Close()
		return true
	})
	return nil
}

func (l *Listener) Addr() net.Addr {
	return &ConnectionAddr{
		addr: l.addr,
	}
}

// client connection
type Connection struct {
	net.Conn

	local, remote string

	broker    *Broker
	publisher *Publisher

	rChan chan []byte
	rbuf  []byte

	ctx    context.Context
	cancel context.CancelFunc

	readCtx, writeCtx       context.Context
	readCancel, writeCancel context.CancelFunc
}

func (c *Connection) Read(b []byte) (n int, err error) {
	if len(c.rbuf) > 0 {
		n = copy(b, c.rbuf)
		c.rbuf = c.rbuf[n:]
		return n, nil
	}

	select {
	case buf, ok := <-c.rChan:
		if !ok {
			return 0, utils.Errorf("read chan closed")
		}

		n = copy(b, buf)
		c.rbuf = buf[n:]
		return n, nil
	case <-c.readCtx.Done():
		return 0, utils.Errorf("read context done")
	case <-c.ctx.Done():
		return 0, utils.Errorf("core context done")
	}
}

func (c *Connection) Write(b []byte) (n int, err error) {
	select {
	case <-c.writeCtx.Done():
		return 0, utils.Errorf("write context done")
	case <-c.ctx.Done():
		return 0, utils.Errorf("core context done")
	default:
	}

SPLIT_BY_LEN:
	var buf = b
	if len(buf) > 4096 {
		buf = b[:4096]
	}
	n = len(buf)
	if len(buf) > 0 {
		log.Infof("sent: %v", len(buf))

		f := &ConnectionFrame{
			From: c.local,
			Buf:  b,
		}

		raw, err := json.Marshal(f)
		if err != nil {
			return 0, utils.Errorf("marshal failed: %s", err)
		}
		p := amqp.Publishing{
			Body: raw,
		}
		err = c.GetPublisher().PublishToQueue(remoteQueue(c.remote), p)
		if err != nil {
			return 0, utils.Errorf("publish to queue %v failed: %s", remoteQueue(c.remote), err)
		}
	}

	if n < len(b) {
		b = b[n-1:]
		log.Infof("body toooo long   split ......... sent: %v", n)
		goto SPLIT_BY_LEN
	}

	return len(b), nil
}

func (c *Connection) GetPublisher() *Publisher {
	if c.publisher != nil {
		return c.publisher
	}
	c.publisher = c.broker.GetPublisher()
	return c.GetPublisher()
}

func NewConnection(local, remote string, ctx context.Context, options ...BrokerConfigHandler) (*Connection, error) {
	if ctx == nil {
		log.Info("empty context for client connection...")
		ctx = context.Background()
	}
	rc, rcancel := context.WithCancel(ctx)
	wc, wcancel := context.WithCancel(ctx)
	ctx, cancel := context.WithCancel(ctx)

	broker, err := NewBroker(ctx, options...)
	if err != nil {
		return nil, err
	}

	var c = &Connection{
		local:   local,
		remote:  remote,
		broker:  broker,
		rChan:   make(chan []byte),
		readCtx: rc, readCancel: rcancel,
		writeCtx: wc, writeCancel: wcancel,
		ctx: ctx, cancel: cancel,
	}
	err = c.init()
	if err != nil {
		return nil, err
	}

	err = c.broker.RunBackground()
	if err != nil {
		return nil, err
	}

	err = c.helo()
	if err != nil {
		return nil, err
	}

	return c, nil
}

func NewConnectionWithBroker(local, remote string, broker *Broker) (*Connection, error) {
	return NewConnection(local, remote, broker.ctx, broker.GetAuthBrokerConfigHandlers()...)
}

func localQueue(s string) string {
	return fmt.Sprintf("amqp-fakeconn-local-queue-%v", s)
}

func remoteQueue(s string) string {
	return fmt.Sprintf("amqp-fakeconn-remote-core-queue-%v", s)
}

func (c *Connection) init() error {
	c.broker.DoConfigure(
		WithQueueDeclare(&QueueDeclaringParam{
			Name:       localQueue(c.local),
			AutoDelete: true,
			Exclusive:  true,
		}),
		WithConsumingParam(&ConsumingParam{
			Queue:     localQueue(c.local),
			Consumer:  uuid.NewV4().String(),
			AutoACK:   false,
			Exclusive: true,
			Handler: func(b *Broker, conn *amqp.Connection, channel *amqp.Channel, msg amqp.Delivery) {
				defer channel.Ack(msg.DeliveryTag, false)

				var m ConnectionFrame
				err := json.Unmarshal(msg.Body, &m)
				if err != nil {
					log.Error("delivery body parsing failed: %s", err)
					return
				}

				if m.Closed {
					log.Info("closed from another pear")
					_ = c.Close()
					return
				}

				c.rChan <- m.Buf
			},
		}),
		BeforeChannelExit(func(broker *Broker, a *amqp.Channel) error {
			_, _ = a.QueueDelete(localQueue(c.local), false, false, false)
			return nil
		}),
	)

	return nil
}

func (c *Connection) helo() error {
	firstMsg := &ConnectionFrame{
		From:  c.local,
		First: true,
	}
	raw, err := json.Marshal(firstMsg)
	if err != nil {
		return utils.Errorf("marshal helo msg failed: %s", err)
	}
	return c.GetPublisher().PublishToQueue(
		remoteQueue(c.remote), amqp.Publishing{Body: raw},
	)
}

func (c *Connection) SetDeadline(t time.Time) error {
	if t.Sub(time.Now()) > 0 {
		c.readCtx, c.readCancel = context.WithDeadline(c.writeCtx, t)
	}

	if t.Sub(time.Now()) > 0 {
		c.writeCtx, c.writeCancel = context.WithDeadline(c.writeCtx, t)
	}

	return utils.Errorf("time: %s is larger than now[%s]", t, time.Now())
}

func (c *Connection) SetReadDeadline(t time.Time) error {
	if t.Sub(time.Now()) > 0 {
		c.readCtx, c.readCancel = context.WithDeadline(c.writeCtx, t)
	}
	return utils.Errorf("time: %s is larger than now[%s]", t, time.Now())
}

func (c *Connection) SetWriteDeadline(t time.Time) error {
	if t.Sub(time.Now()) > 0 {
		c.writeCtx, c.writeCancel = context.WithDeadline(c.writeCtx, t)
	}
	return utils.Errorf("time: %s is larger than now[%s]", t, time.Now())
}

func (c *Connection) Close() error {
	c.cancel()
	c.readCancel()
	c.writeCancel()

	raw, err := json.Marshal(&ConnectionFrame{
		From:   c.local,
		Closed: true,
	})
	if err != nil {
		return err
	}

	return c.GetPublisher().PublishToQueue(
		remoteQueue(c.remote), amqp.Publishing{
			Body: raw,
		},
	)
}

// 初始化网络地址 API
type ConnectionAddr struct {
	net.Addr

	addr string
}

func (a *ConnectionAddr) Network() string {
	return "amqp"
}

func (a *ConnectionAddr) String() string {
	return a.addr
}

func (c *Connection) LocalAddr() net.Addr {
	return &ConnectionAddr{
		addr: c.local,
	}
}

func (c *Connection) RemoteAddr() net.Addr {
	return &ConnectionAddr{
		addr: c.remote,
	}
}

// 假装这是 conn/listen pair
//     1. 构建 GRPC Client
