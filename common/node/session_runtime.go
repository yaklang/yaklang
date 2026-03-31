package node

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

func buildSessionTransport(cfg BaseConfig) (SessionTransport, error) {
	if cfg.TransportClient != nil {
		return cfg.TransportClient, nil
	}
	return NewHTTPTransport(HTTPTransportConfig{
		BaseURL: cfg.PlatformAPIBaseURL,
		Client:  cfg.HTTPClient,
	})
}

func (n *NodeBase) bootstrapSession() error {
	ctx, cancel := context.WithTimeout(n.rootCtx, n.requestTimeout)
	defer cancel()

	session, err := n.transport.Bootstrap(ctx, BootstrapRequest{
		EnrollmentToken: n.enrollmentToken,
		NodeID:          n.NodeId,
		NodeType:        string(n.NodeType),
		Version:         n.version,
		Labels:          cloneStringMap(n.labels),
		CapabilityKeys:  cloneStringSlice(n.capabilityKeys),
	})
	if err != nil {
		return err
	}

	n.sessionMu.Lock()
	n.session = session
	n.sessionMu.Unlock()
	log.Infof("node session established: node_id=%s session_id=%s", n.NodeId, session.SessionID)
	return nil
}

func (n *NodeBase) sleepWithContext(duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-n.rootCtx.Done():
		return n.rootCtx.Err()
	case <-timer.C:
		return nil
	}
}

func (n *NodeBase) runTickerFuncs() {
	n.WalkTickerFunc(func(_ string, f *tickerFunc) {
		if f.first && !f.firstExecuted.IsSet() {
			f.firstExecuted.Set()
			f.F()
			return
		}
		f.currentMod = (f.currentMod + 1) % f.IntervalSeconds
		if f.currentMod == 0 {
			f.F()
		}
	})
}

func (n *NodeBase) currentSession() (SessionState, bool) {
	n.sessionMu.RLock()
	defer n.sessionMu.RUnlock()

	if n.session.SessionID == "" || n.session.SessionToken == "" {
		return SessionState{}, false
	}
	return n.session, true
}

func (n *NodeBase) GetSessionState() (SessionState, bool) {
	return n.currentSession()
}

func (n *NodeBase) clearSession() {
	n.sessionMu.Lock()
	n.session = SessionState{}
	n.sessionMu.Unlock()
	n.isRegistered.UnSet()
}
