package node

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tevino/abool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec"
)

const sessionRetryInterval = 3 * time.Second

// NodeBase owns the node lifecycle and session transport.
type NodeBase struct {
	rootCtx context.Context
	cancel  context.CancelFunc

	NodeType spec.NodeType
	NodeId   string

	enrollmentToken string
	version         string
	labels          map[string]string
	capabilityKeys  []string
	maxRunningJobs  uint32
	lifecycleState  string
	requestTimeout  time.Duration

	transport         SessionTransport
	statusProvider    RuntimeStatusProvider
	heartbeatInterval time.Duration
	tickerInterval    time.Duration

	tickerFuncs *sync.Map

	isRegistered *abool.AtomicBool

	sessionMu sync.RWMutex
	session   SessionState

	instanceLock *nodeInstanceLock
}

// NewNodeBase creates a node with session transport.
func NewNodeBase(cfg BaseConfig) (*NodeBase, error) {
	normalized, err := normalizeBaseConfig(cfg)
	if err != nil {
		return nil, err
	}
	transport, err := buildSessionTransport(normalized)
	if err != nil {
		return nil, err
	}
	instanceLock, err := acquireNodeInstanceLock(normalized.NodeID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	node := &NodeBase{
		rootCtx:           ctx,
		cancel:            cancel,
		NodeType:          normalized.NodeType,
		NodeId:            normalized.NodeID,
		enrollmentToken:   normalized.EnrollmentToken,
		version:           normalized.Version,
		labels:            cloneStringMap(normalized.Labels),
		capabilityKeys:    cloneStringSlice(normalized.CapabilityKeys),
		maxRunningJobs:    normalized.MaxRunningJobs,
		lifecycleState:    normalized.LifecycleState,
		requestTimeout:    normalized.RequestTimeout,
		transport:         transport,
		statusProvider:    normalized.StatusProvider,
		heartbeatInterval: normalized.HeartbeatInterval,
		tickerInterval:    normalized.TickerInterval,
		tickerFuncs:       new(sync.Map),
		isRegistered:      abool.NewBool(false),
		instanceLock:      instanceLock,
	}
	return node, nil
}

func (n *NodeBase) IsRegistered() bool {
	return n.isRegistered.IsSet()
}

func (n *NodeBase) WithCancelContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(n.rootCtx)
}

func (n *NodeBase) Serve() {
	n.startDaemon()
}

func (n *NodeBase) Shutdown() {
	defer n.releaseInstanceLock()

	session, ok := n.currentSession()

	n.cancel()
	n.clearSession()
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.requestTimeout)
	defer cancel()

	err := n.transport.Shutdown(ctx, session, ShutdownRequest{
		ObservedAt: time.Now().UTC(),
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("shutdown node session failed: node_id=%s session_id=%s err=%v", n.NodeId, session.SessionID, err)
		return
	}
	log.Infof("node session ended: node_id=%s session_id=%s", n.NodeId, session.SessionID)
}

func (n *NodeBase) startDaemon() {
	if err := n.ensureSession(); err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Errorf("establish node session failed: %v", err)
		}
		return
	}
	n.runDaemonLoop()
}

func (n *NodeBase) ensureSession() error {
	for {
		if err := n.bootstrapSession(); err != nil {
			log.Errorf("bootstrap node session failed: %v", err)
		} else if err := n.heartbeat(); err != nil {
			log.Errorf("initial heartbeat failed: %v", err)
		} else {
			n.isRegistered.Set()
			return nil
		}
		if err := n.sleepWithContext(sessionRetryInterval); err != nil {
			return err
		}
	}
}

func (n *NodeBase) runDaemonLoop() {
	heartbeatTicker := time.NewTicker(n.heartbeatInterval)
	tickerLoop := time.NewTicker(n.tickerInterval)
	defer heartbeatTicker.Stop()
	defer tickerLoop.Stop()

	for {
		select {
		case <-n.rootCtx.Done():
			return
		case <-heartbeatTicker.C:
			if err := n.heartbeat(); err != nil {
				n.logHeartbeatFailure(err)
				n.clearSession()
				if err := n.ensureSession(); err != nil {
					if !errors.Is(err, context.Canceled) {
						log.Errorf("re-establish node session failed: %v", err)
					}
					return
				}
			}
		case <-tickerLoop.C:
			n.runTickerFuncs()
		}
	}
}

func (n *NodeBase) GetToken() string {
	session, ok := n.currentSession()
	if !ok {
		return ""
	}
	return session.SessionToken
}

func (n *NodeBase) GetRootContext() context.Context {
	return n.rootCtx
}

func (n *NodeBase) releaseInstanceLock() {
	if n == nil || n.instanceLock == nil {
		return
	}

	if err := n.instanceLock.Release(); err != nil {
		log.Errorf("release node instance lock failed: node_id=%s err=%v", n.NodeId, err)
	}
	n.instanceLock = nil
}

func (n *NodeBase) logHeartbeatFailure(err error) {
	session, _ := n.currentSession()
	if IsSessionInactiveTransportError(err) {
		log.Errorf(
			"heartbeat rejected because node session is no longer active: node_id=%s session_id=%s err=%v diagnosis=%q",
			n.NodeId,
			session.SessionID,
			err,
			"another process may be running with the same node_id and replaced this session",
		)
		return
	}

	log.Errorf(
		"heartbeat failed, rebuilding session: node_id=%s session_id=%s err=%v",
		n.NodeId,
		session.SessionID,
		err,
	)
}
