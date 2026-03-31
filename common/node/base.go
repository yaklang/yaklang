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
	n.cancel()
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
				log.Errorf("heartbeat failed, rebuilding session: %v", err)
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
