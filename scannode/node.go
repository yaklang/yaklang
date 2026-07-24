package scannode

import (
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/spec"
)

type ScanNode struct {
	node              *node.NodeBase
	manager           *TaskManager
	capabilityManager *CapabilityManager
	ruleSyncClient    ruleSyncer
	httpClient        *http.Client
	invokeLimiter     *invokeLimiter
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
	cfg.CapabilityKeys = normalizeScanNodeCapabilityKeys(cfg.CapabilityKeys)
	if cfg.StatusProvider == nil {
		cfg.StatusProvider = agent
	}

	// Register a post-bootstrap hook so the rule sync client gets valid node
	// session credentials as soon as the node registers with the platform.
	// The node-accessible snapshot endpoints authenticate via node_session_id
	// query param + Bearer session token; without this hook the client would
	// send empty credentials and get 401 on every sync. The hook is set before
	// NewNodeBase so it is captured into the NodeBase lifecycle.
	agent.ruleSyncClient = NewRuleSyncClient(&RuleSyncConfig{
		ServerURL:   cfg.PlatformAPIBaseURL,
		SyncEnabled: true,
		Client:      cfg.HTTPClient,
	})
	existingHook := cfg.PostBootstrapHook
	cfg.PostBootstrapHook = func(session node.SessionState) {
		agent.updateRuleSyncCredentials(session)
		if existingHook != nil {
			existingHook(session)
		}
	}

	base, err := node.NewNodeBase(cfg)
	if err != nil {
		return nil, err
	}

	agent.node = base
	agent.capabilityManager = newCapabilityManager(CapabilityManagerConfig{
		NodeIDProvider: base.CurrentNodeID,
		BaseDir:        base.BaseDir(),
		RootContext:    base.GetRootContext(),
	})
	agent.httpClient = cfg.HTTPClient
	agent.initInvokeLimiter()
	agent.bridge = newLegionJobBridge(agent)
	return agent, nil
}

func (s *ScanNode) Run() {
	if s.bridge != nil {
		go s.bridge.Run(s.node.GetRootContext())
	}
	s.node.Serve()
}

// updateRuleSyncCredentials feeds the node session id + session token into the
// rule sync client so it can authenticate against the node-accessible snapshot
// endpoints. Called from the PostBootstrapHook after the node registers.
func (s *ScanNode) updateRuleSyncCredentials(session node.SessionState) {
	client, ok := s.ruleSyncClient.(*RuleSyncClient)
	if !ok || client == nil {
		return
	}
	client.UpdateCredentials(session.SessionID, session.SessionToken)
	log.Infof("rule sync client credentials updated: node_session_id=%s", session.SessionID)
}

func (s *ScanNode) Shutdown() {
	if s == nil || s.node == nil {
		return
	}
	if s.capabilityManager != nil {
		if err := s.capabilityManager.Close(); err != nil {
			log.Errorf("shutdown capability manager failed: %v", err)
		}
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
