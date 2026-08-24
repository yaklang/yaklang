package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
)

type ScanNode struct {
	node              *node.NodeBase
	manager           *TaskManager
	capabilityManager *CapabilityManager
	ruleSyncClient    ruleSyncer
	pluginBundles     pluginBundleInstaller
	httpClient        *http.Client
	invokeLimiter     *invokeLimiter
	maxRunningJobs    uint32
	bridge            *legionJobBridge
	ssaGitOwnerScope  string
	ssaGitScopeLock   *ssagitworkdir.OwnerScopeLock
	runtimeHost       *runtimeHostExecutor
}

var scanNodeTaskDrainTimeout = 30 * time.Second

type ScanNodeOption func(*scanNodeOptions)

type scanNodeOptions struct {
	runtimeHost RuntimeHostConfig
}

func WithRuntimeHost(cfg RuntimeHostConfig) ScanNodeOption {
	return func(options *scanNodeOptions) {
		options.runtimeHost = cfg
	}
}

func NewScanNode(cfg node.BaseConfig, options ...ScanNodeOption) (*ScanNode, error) {
	var resolvedOptions scanNodeOptions
	for _, option := range options {
		if option != nil {
			option(&resolvedOptions)
		}
	}
	if resolvedOptions.runtimeHost.Enabled {
		cfg.CapabilityKeys = append(cfg.CapabilityKeys, AIRuntimeHostCapabilityKey)
	}
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
	agent.pluginBundles, err = NewPluginBundleManager(PluginBundleManagerConfig{
		BaseDir: base.BaseDir(),
		Client:  cfg.HTTPClient,
	})
	if err != nil {
		base.Shutdown()
		return nil, utils.Errorf("initialize plugin bundle manager: %v", err)
	}
	agent.ssaGitOwnerScope, agent.ssaGitScopeLock, err = recoverSSAGitWorkspacesForInstallation(base.AgentInstallationID())
	if err != nil {
		base.Shutdown()
		return nil, utils.Errorf("recover stale SSA Git workspaces for Scan Node installation: %v", err)
	}
	agent.capabilityManager = newCapabilityManager(CapabilityManagerConfig{
		NodeIDProvider: base.CurrentNodeID,
		BaseDir:        base.BaseDir(),
		RootContext:    base.GetRootContext(),
	})
	if resolvedOptions.runtimeHost.Enabled {
		resolvedOptions.runtimeHost.BaseDir = base.BaseDir()
		resolvedOptions.runtimeHost.AgentInstallationID = base.AgentInstallationID()
		resolvedOptions.runtimeHost.NodeIDProvider = base.CurrentNodeID
		resolvedOptions.runtimeHost.SessionProvider = base.GetSessionState
		agent.runtimeHost, err = newRuntimeHostExecutor(resolvedOptions.runtimeHost)
		if err != nil {
			base.Shutdown()
			return nil, utils.Errorf("initialize AI Runtime Host: %v", err)
		}
		agent.capabilityManager.runtimeStatusProviders = append(
			agent.capabilityManager.runtimeStatusProviders,
			agent.runtimeHost,
		)
	}
	agent.httpClient = cfg.HTTPClient
	agent.initInvokeLimiter()
	agent.bridge = newLegionJobBridge(agent)
	return agent, nil
}

func ssaGitWorkspaceOwnerScope(agentInstallationID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(agentInstallationID)))
	return "node-" + hex.EncodeToString(sum[:16])
}

func recoverSSAGitWorkspacesForInstallation(agentInstallationID string) (string, *ssagitworkdir.OwnerScopeLock, error) {
	ownerScope := ssaGitWorkspaceOwnerScope(agentInstallationID)
	ownerLock, err := ssagitworkdir.AcquireOwnerScopeLock(ownerScope)
	if err != nil {
		return "", nil, err
	}
	recoveryLock, err := ssagitworkdir.AcquireOwnerScopeRecoveryLock(ownerScope)
	if err != nil {
		_ = ownerLock.Release()
		return "", nil, err
	}
	defer recoveryLock.Release()
	if err := ssagitworkdir.CleanupForOwnerScope(ownerScope); err != nil {
		_ = ownerLock.Release()
		return "", nil, err
	}
	return ownerScope, ownerLock, nil
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
	s.manager.BeginShutdown()
	if s.capabilityManager != nil {
		if err := s.capabilityManager.Close(); err != nil {
			log.Errorf("shutdown capability manager failed: %v", err)
		}
	}
	if s.runtimeHost != nil {
		_ = s.runtimeHost.Close()
	}
	s.node.Shutdown()
	s.releaseSSAGitScopeLockAfterTasks()
}

func (s *ScanNode) releaseSSAGitScopeLockAfterTasks() {
	drainCtx, cancelDrain := context.WithTimeout(context.Background(), scanNodeTaskDrainTimeout)
	defer cancelDrain()
	if err := s.manager.WaitForEmpty(drainCtx); err != nil {
		log.Errorf("wait for Scan Node tasks before releasing SSA Git workspace owner scope lock failed: %v", err)
		go func() {
			_ = s.manager.WaitForEmpty(context.Background())
			if releaseErr := s.ssaGitScopeLock.Release(); releaseErr != nil {
				log.Errorf("release SSA Git workspace owner scope lock after delayed task drain failed: %v", releaseErr)
			}
		}()
		return
	}
	if err := s.ssaGitScopeLock.Release(); err != nil {
		log.Errorf("release SSA Git workspace owner scope lock failed: %v", err)
	}
}

func (s *ScanNode) Snapshot() node.RuntimeStatus {
	return node.RuntimeStatus{
		LifecycleState: node.DefaultLifecycleState,
		RunningJobs:    uint32(s.manager.Count()),
		MaxRunningJobs: s.maxRunningJobs,
		ActiveAttempts: s.manager.ActiveAttemptHeartbeats(time.Now().UTC()),
	}
}
