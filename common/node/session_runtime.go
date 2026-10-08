package node

import (
	"context"
	"fmt"
	"math"
	"strings"
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
		ClaimedName:              n.DisplayName(),
		AgentInstallationID:      n.AgentInstallationID(),
		HostIdentity:             n.hostIdentitySnapshot(),
		NodeType:                 string(n.NodeType),
		Kind:                     n.kind,
		DockerEndpoint:           n.dockerEndpoint,
		Version:                  n.version,
		EngineReleaseID:          n.engineReleaseID,
		EngineDigest:             n.engineDigest,
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
	if err := validateSessionState(session, time.Now()); err != nil {
		return err
	}
	n.setCurrentNodeID(session.NodeID)

	n.sessionMu.Lock()
	if n.boundCompanyID != "" && n.boundCompanyID != session.CompanyID {
		n.sessionMu.Unlock()
		return fmt.Errorf("bootstrap cannot change or remove the bound company")
	}
	if session.CompanyID != "" {
		n.boundCompanyID = session.CompanyID
	}
	n.session = session
	n.sessionMu.Unlock()
	log.Infof("node session established: node_id=%s session_id=%s", n.CurrentNodeID(), session.SessionID)
	return nil
}

func validateSessionState(session SessionState, now time.Time) error {
	if strings.TrimSpace(session.SessionID) == "" {
		return fmt.Errorf("bootstrap response node_session_id is required")
	}
	if strings.TrimSpace(session.SessionToken) == "" {
		return fmt.Errorf("bootstrap response session_token is required")
	}
	if strings.TrimSpace(session.CompanyID) == "" {
		return nil
	}
	missing := ""
	switch {
	case session.SessionStartedAt.IsZero() || session.SessionStartedAt.After(now.Add(30*time.Second)):
		missing = "session_started_at"
	case session.ExpiresAt.IsZero() || !session.ExpiresAt.After(now):
		missing = "expires_at"
	case strings.TrimSpace(session.NATSURL) == "":
		missing = "nats_url"
	case strings.TrimSpace(session.NATSCredentials) == "":
		missing = "nats_credentials"
	case session.NATSCredentialsExpiresAt.IsZero() || !session.NATSCredentialsExpiresAt.After(now):
		missing = "nats_credentials_expires_at"
	case strings.TrimSpace(session.CommandStream) == "":
		missing = "command_stream"
	case strings.TrimSpace(session.CommandConsumer) == "":
		missing = "command_consumer"
	case strings.TrimSpace(session.InboxPrefix) == "":
		missing = "inbox_prefix"
	case strings.TrimSpace(session.CommandSubject) == "":
		missing = "command_subject"
	case strings.TrimSpace(session.EventSubjectPrefix) == "":
		missing = "event_subject_prefix"
	}
	if missing != "" {
		return fmt.Errorf("company node session requires %s", missing)
	}
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
	previous, hook := n.session, n.sessionInvalidatedHook
	n.session = SessionState{}
	n.sessionMu.Unlock()
	if n.isRegistered != nil {
		n.isRegistered.UnSet()
	}
	if previous.SessionID != "" && hook != nil {
		hook(previous)
	}
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

// SetSessionInvalidatedHook joins runtime revocation to HTTP session loss.
func (n *NodeBase) SetSessionInvalidatedHook(hook func(SessionState)) {
	n.sessionMu.Lock()
	defer n.sessionMu.Unlock()
	n.sessionInvalidatedHook = hook
}
