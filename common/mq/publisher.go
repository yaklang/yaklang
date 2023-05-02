package mq

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"sync"
	"time"
	"github.com/yaklang/yaklang/common/log"
)

type Publisher struct {
	initConnFunc func() (*amqp.Connection, error)

	conn *amqp.Connection
	ch   *amqp.Channel
	mux  *sync.Mutex
	//confirmChan chan amqp.Confirmation
}

func (p *Publisher) PublishTo(exchange, routingKey string, msg amqp.Publishing) error {
	return p.Publish(3, exchange, routingKey, false, false, msg)
}

func (p *Publisher) PublishToQueue(queue string, msg amqp.Publishing) error {
	return p.Publish(3, "", queue, false, false, msg)
}

func (p *Publisher) init() (err error) {

	if p.conn == nil {
		p.conn, err = p.initConnFunc()
		if err != nil {
			return errors.Errorf("create amqp connection failed: %s", err)
		}
	}

	if p.ch == nil && p.conn != nil {
		p.ch, err = p.conn.Channel()
		if err != nil {
			return errors.Errorf("create channel failed: %s", err)
		}

		//err = p.ch.Confirm(false)
		//if err != nil {
		//	return errors.Errorf("enable confirm mode failed: %s", err)
		//}
		//
		//p.confirmChan = p.ch.NotifyPublish(make(chan amqp.Confirmation))
	}

	return nil
}

func (p *Publisher) Publish(failRetry int, exchange, routingKey string, mandatory, immediately bool, msg amqp.Publishing) (err error) {
	for i := 0; i < failRetry; i++ {
		if p.ch == nil {

			p.mux.Lock()
			err := p.init()
			p.mux.Unlock()

			if err != nil {
				log.Warnf("init publish failed: %s", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}
		}

		p.mux.Lock()
		err := p.ch.Publish(exchange, routingKey, mandatory, immediately, msg)
		p.mux.Unlock()

		if err != nil {
			log.Warnf("publish failed: %s", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		//select {
		//case confirmation, ok := <-p.confirmChan:
		//	if !ok {
		//		break
		//	}
		//	if !confirmation.Ack {
		//		continue
		//	}
		//}

		return nil
	}
	return errors.Errorf("retry failed to [%v]-[%v]: %v", exchange, routingKey, spew.Sdump(msg.Body))
}

func (b *Broker) createAMQPConnectionFunc() func() (*amqp.Connection, error) {
	return func() (connection *amqp.Connection, e error) {
		return amqp.DialConfig(b.amqpUrl, b.dialConfig)
	}
}

func (b *Broker) GetPublisher() *Publisher {
	createAMQPConnection := b.createAMQPConnectionFunc()
	pub := &Publisher{
		initConnFunc: createAMQPConnection,
		mux:          new(sync.Mutex),
	}
	return pub
}
