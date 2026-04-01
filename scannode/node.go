package scannode

import (
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/spec"
)

type ScanNode struct {
	node              *node.NodeBase
	manager           *TaskManager
	capabilityManager *CapabilityManager
	maxRunningJobs    uint32
	bridge            *legionJobBridge
}

func NewScanNode(cfg node.BaseConfig) (*ScanNode, error) {
	agent := &ScanNode{
		manager:        newTaskManager(),
		maxRunningJobs: cfg.MaxRunningJobs,
	}
	if cfg.NodeType == "" {
		cfg.NodeType = spec.NodeType_Scanner
	}
	if len(cfg.CapabilityKeys) == 0 {
		cfg.CapabilityKeys = []string{"yak.execute"}
	}
	if cfg.StatusProvider == nil {
		cfg.StatusProvider = agent
	}

	base, err := node.NewNodeBase(cfg)
	if err != nil {
		return nil, err
	}

	agent.node = base
	agent.capabilityManager = newCapabilityManager(CapabilityManagerConfig{
		NodeID:  base.NodeId,
		BaseDir: consts.GetDefaultYakitBaseDir(),
	})
	agent.bridge = newLegionJobBridge(agent)
	return agent, nil
}

func (s *ScanNode) Run() {
	if s.bridge != nil {
		go s.bridge.Run(s.node.GetRootContext())
	}
	s.node.Serve()
}

func (s *ScanNode) Shutdown() {
	if s == nil || s.node == nil {
		return
	}
	s.node.Shutdown()
}

func (s *ScanNode) Snapshot() node.RuntimeStatus {
	return node.RuntimeStatus{
		LifecycleState: node.DefaultLifecycleState,
		RunningJobs:    uint32(s.manager.Count()),
		MaxRunningJobs: s.maxRunningJobs,
		ActiveAttempts: s.manager.ActiveAttemptHeartbeats(time.Now().UTC()),
	}
}

// RuleSyncClient 规则同步客户端（当前 session transport 重构阶段默认不初始化）。
var globalRuleSyncClient *RuleSyncClient

// GetRuleSyncClient 获取规则同步客户端
func GetRuleSyncClient() *RuleSyncClient {
	return globalRuleSyncClient
}
