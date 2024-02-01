package node

import (
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec"
)

func (n *NodeBase) Notify(key string, msg *spec.Message) {
	body, err := json.Marshal(msg)
	if err != nil {
		log.Error("marshal [%v] failed: %v", spew.Sdump(msg), err)
		return
	}
	err = n.publisher.PublishTo("palm-backend", fmt.Sprintf("server.backend.%v", key), amqp.Publishing{
		Body: body,
	})
	if err != nil {
		log.Errorf("publish palm-backend %v failed: %v", key, err)
		return
	}
}

func (n *NodeBase) NotifyHeartbeat(key string, msg *spec.Message) {
	body, err := json.Marshal(msg)
	if err != nil {
		log.Error("marshal [%v] failed: %v", spew.Sdump(msg), err)
		return
	}
	err = n.publisher.PublishTo(
		"palm-backend",
		fmt.Sprintf("heartbeat.%v", key),
		amqp.Publishing{
			Body: body,
		})
	if err != nil {
		log.Errorf("publish palm-backend %v failed: %v", key, err)
		return
	}
}
