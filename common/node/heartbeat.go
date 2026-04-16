package node

import (
	"context"
	"fmt"
	"time"
)

func (n *NodeBase) heartbeat() error {
	session, ok := n.currentSession()
	if !ok {
		return fmt.Errorf("node session not established")
	}
	status := n.runtimeStatus()

	ctx, cancel := context.WithTimeout(n.rootCtx, n.requestTimeout)
	defer cancel()

	return n.transport.Heartbeat(ctx, session, HeartbeatRequest{
		LifecycleState:           status.LifecycleState,
		Version:                  n.version,
		RunningJobs:              status.RunningJobs,
		MaxRunningJobs:           status.MaxRunningJobs,
		CapabilityKeys:           cloneStringSlice(n.capabilityKeys),
		Labels:                   cloneStringMap(n.labels),
		ObservedAt:               time.Now().UTC(),
		HeartbeatIntervalSeconds: durationToWholeSeconds(n.heartbeatInterval),
		ActiveAttempts:           cloneActiveAttemptHeartbeats(status.ActiveAttempts),
		HostInfo:                 n.hostInfoSnapshot(),
	})
}

func (n *NodeBase) runtimeStatus() RuntimeStatus {
	status := RuntimeStatus{
		LifecycleState: n.lifecycleState,
		MaxRunningJobs: n.maxRunningJobs,
		ActiveAttempts: []ActiveAttemptHeartbeat{},
	}
	if n.statusProvider == nil {
		return status
	}

	snapshot := n.statusProvider.Snapshot()
	if snapshot.LifecycleState != "" {
		status.LifecycleState = snapshot.LifecycleState
	}
	status.RunningJobs = snapshot.RunningJobs
	if snapshot.MaxRunningJobs != 0 || status.MaxRunningJobs == 0 {
		status.MaxRunningJobs = snapshot.MaxRunningJobs
	}
	status.ActiveAttempts = cloneActiveAttemptHeartbeats(snapshot.ActiveAttempts)
	return status
}
