package node

import (
	"encoding/json"
	"github.com/streadway/amqp"
	"yaklang/common/log"
	"yaklang/common/spec"
)

func (n *NodeBase) onScriptTask(msg *amqp.Delivery) {
	key := spec.ParseServerPushKey(msg.RoutingKey)
	switch spec.ServerPushKey(key) {
	case spec.ServerPush_ScriptTask:
		task := &spec.ScriptTask{}
		err := json.Unmarshal(msg.Body, task)
		if err != nil {
			log.Warn("unmarshal script task failed: ", err)
			return
		}

		log.Infof("start to execute with task-id: %v", task.TaskID)
		go func() {
			err = n.ScriptExecutor.ExecuteWithTaskID(task.TaskID, task.ScriptCode)
			if err != nil {
				log.Errorf("execute with task id failed: %s", err)
			}
			return
		}()
	default:
		return
	}
}
