package scannode

import (
	"context"
	"encoding/json"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/mq"
	"yaklang.io/yaklang/common/node"
	"yaklang.io/yaklang/common/spec"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/scannode/scanrpc"
)

type ScanNode struct {
	node    *node.NodeBase
	helper  *scanrpc.SCANServerHelper
	manager *TaskManager
}

type WebServerConfig struct {
	WebServerPort string `json:"web_server_port"`
}

func NewScanNodeWithAMQPUrl(id string, amqpUrl string, serverIp string) (*ScanNode, error) {
	base, err := node.NewNodeBase(
		spec.NodeType_Scanner,
		spec.CommonRPCExchange,
		id, "",
		mq.WithAMQPUrl(amqpUrl),
	)
	if err != nil {
		return nil, err
	}

	node := &ScanNode{node: base}
	agent := node
	agent.node.HookAfterRegisteringFinished(
		func() {
			node.GetIpecho(serverIp, node.node.WebServerPort)
		},
	)
	// 回传日志信息
	agent.node.HookAfterRegisteringFinished(
		func() {
			go func() {
				err := utils.HandleStdout(context.Background(), func(i string) {
					msg := agent.node.NewBaseMessage(spec.MessageType_NodeLog)
					raw, err := json.Marshal(i)
					if err != nil {
						log.Errorf("marshal log failed: %v", err)
					}
					msg.Content = raw
					agent.node.Notify(spec.BackendKey_NodeLog, msg)
				})
				if err != nil {
					log.Errorf("handle stdout failed: %v", err)
				}
			}()
		},
	)
	node.initScanRPC()
	return node, nil
}

func NewScanNode(id string, amqpConfig *spec.AMQPConfig) (*ScanNode, error) {
	return NewScanNodeWithAMQPUrl(id, amqpConfig.GetAMQPUrl(), amqpConfig.Host)
}

func (s *ScanNode) Run() {
	s.node.Serve()
}

func (s *ScanNode) GetServerHelper() *scanrpc.SCANServerHelper {
	return s.helper
}
