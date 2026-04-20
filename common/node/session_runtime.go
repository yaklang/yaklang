package node

import (
	"context"
	"fmt"
	"math"
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
		EnrollmentToken:          n.enrollmentToken,
		NodeID:                   n.LegacyNodeID(),
		ClaimedName:              n.DisplayName(),
		AgentInstallationID:      n.AgentInstallationID(),
		HostIdentity:             n.hostIdentitySnapshot(),
		NodeType:                 string(n.NodeType),
		Version:                  n.version,
		Labels:                   cloneStringMap(n.labels),
		CapabilityKeys:           cloneStringSlice(n.capabilityKeys),
		HeartbeatIntervalSeconds: durationToWholeSeconds(n.heartbeatInterval),
		HostInfo:                 n.hostInfoSnapshot(),
	})
	if err != nil {
		return err
	}
	if session.NodeID == "" {
		return fmt.Errorf("bootstrap response node_id is required")
	}
	n.setCurrentNodeID(session.NodeID)

	n.sessionMu.Lock()
	n.session = session
	n.sessionMu.Unlock()
	log.Infof("node session established: node_id=%s session_id=%s", n.CurrentNodeID(), session.SessionID)
	return nil
}

func (n *NodeBase) hostInfoSnapshot() HostInfo {
	if n == nil || n.hostInfoProvider == nil {
		return HostInfo{}
	}
	return normalizeHostInfo(n.hostInfoProvider.Snapshot())
}

func (n *NodeBase) hostIdentitySnapshot() HostIdentity {
	if n == nil || n.hostIdentityProvider == nil {
		return HostIdentity{}
	}
	return normalizeHostIdentity(n.hostIdentityProvider.Snapshot())
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

func durationToWholeSeconds(value time.Duration) uint32 {
	if value <= 0 {
		return 0
	}

	seconds := uint32(math.Ceil(value.Seconds()))
	if seconds == 0 {
		return 1
	}
	return seconds
}
