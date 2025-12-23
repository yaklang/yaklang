package scannode

import (
	"context"
	"encoding/json"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mq"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/scannode/scanrpc"
)

type ScanNode struct {
	node     *node.NodeBase
	helper   *scanrpc.SCANServerHelper
	manager  *TaskManager
	serverIp string
}

type WebServerConfig struct {
	WebServerPort string `json:"web_server_port"`
}

func NewScanNodeWithAMQPUrl(id, serverPort string, amqpUrl string, serverIp string) (*ScanNode, error) {
	base, err := node.NewNodeBase(
		spec.NodeType_Scanner,
		spec.CommonRPCExchange,
		id, "",
		mq.WithAMQPUrl(amqpUrl),
	)
	if err != nil {
		return nil, err
	}

	node := &ScanNode{node: base, serverIp: serverIp}
	agent := node
	agent.node.HookAfterRegisteringFinished(
		func() {
			node.GetIpecho(serverIp, serverPort)
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
	// 注册完成后初始化规则同步（Token已通过注册获得）
	agent.node.HookAfterRegisteringFinished(
		func() {
			// 获取注册时返回的Token
			token := agent.node.GetToken()
			if token != "" {
				// 使用Token初始化规则同步客户端
				agent.initRuleSyncWithToken(token)
			}
		},
	)
	node.initScanRPC()
	return node, nil
}

func NewScanNode(id, serverPort string, amqpConfig *spec.AMQPConfig) (*ScanNode, error) {
	return NewScanNodeWithAMQPUrl(id, serverPort, amqpConfig.GetAMQPUrl(), amqpConfig.Host)
}

func (s *ScanNode) Run() {
	s.node.Serve()
}

func (s *ScanNode) GetServerHelper() *scanrpc.SCANServerHelper {
	return s.helper
}

// RuleSyncClient 规则同步客户端（节点持有以便后续使用）
var globalRuleSyncClient *RuleSyncClient

// initRuleSyncWithToken 使用Token初始化规则同步
func (s *ScanNode) initRuleSyncWithToken(token string) {
	serverURL := s.getServerHTTPURL()
	if serverURL == "" {
		log.Warnf("cannot determine server HTTP URL, rule sync disabled")
		return
	}

	config := &RuleSyncConfig{
		ServerURL:   serverURL,
		APIToken:    token,
		SyncEnabled: true,
	}
	globalRuleSyncClient = NewRuleSyncClient(config)
	log.Infof("rule sync client initialized with server: %s", serverURL)

	// 启动时同步一次规则
	go s.syncRulesOnStartup()
}

// getServerHTTPURL 获取Server的HTTP URL
func (s *ScanNode) getServerHTTPURL() string {
	if s.serverIp != "" && s.node.WebServerPort != "" {
		host := utils.HostPort(s.serverIp, s.node.WebServerPort)
		return "http://" + host
	}
	return ""
}

// syncRulesOnStartup 启动时同步规则
func (s *ScanNode) syncRulesOnStartup() {
	if globalRuleSyncClient == nil {
		return
	}

	log.Info("syncing rules on startup...")
	ruleCount, err := globalRuleSyncClient.SyncAndImportLatest()
	if err != nil {
		log.Errorf("sync rules failed: %v", err)
		return
	}
	log.Infof("synced %d rules on startup", ruleCount)
}

// GetRuleSyncClient 获取规则同步客户端
func GetRuleSyncClient() *RuleSyncClient {
	return globalRuleSyncClient
}
