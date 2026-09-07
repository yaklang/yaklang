package node

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"time"
)

func (n *NodeBase) heartbeat() error {
	session, ok := n.currentSession()
	if !ok {
		return fmt.Errorf("node session not established")
	}
	status := n.runtimeStatus()
	if !session.RuntimeHostCapacityAccepted {
		status.RuntimeHostCapacity = nil
	}

	ctx, cancel := context.WithTimeout(n.rootCtx, n.requestTimeout)
	defer cancel()

	request := HeartbeatRequest{
		LifecycleState:           status.LifecycleState,
		Version:                  n.version,
		RunningJobs:              status.RunningJobs,
		MaxRunningJobs:           status.MaxRunningJobs,
		CapabilityKeys:           cloneStringSlice(n.capabilityKeys),
		Labels:                   cloneStringMap(n.labels),
		ObservedAt:               time.Now().UTC(),
		HeartbeatIntervalSeconds: durationToWholeSeconds(n.heartbeatInterval),
		ActiveAttempts:           cloneActiveAttemptHeartbeats(status.ActiveAttempts),
		RuntimeHostCapacity:      cloneRuntimeHostCapacity(status.RuntimeHostCapacity),
		HostInfo:                 n.hostInfoSnapshot(),
	}
	if transport, ok := n.transport.(ResourcePolicyTransport); ok {
		response, err := transport.HeartbeatWithResponse(ctx, session, request)
		if err != nil {
			var policyErr *resourcePolicyResponseError
			if errors.As(err, &policyErr) {
				return n.rejectHeartbeatPolicy(err)
			}
			return err
		}
		if response.ResourcePolicy != nil {
			applier, ok := n.statusProvider.(ResourcePolicyApplier)
			if !ok {
				return fmt.Errorf("resource policy consumer unavailable")
			}
			if err := applier.ApplyResourcePolicy(*response.ResourcePolicy); err != nil {
				return n.rejectHeartbeatPolicy(fmt.Errorf("apply resource policy: %w", err))
			}
		} else if session.ResourcePolicyRequired {
			return n.rejectHeartbeatPolicy(fmt.Errorf("required resource policy missing from heartbeat"))
		}
		return nil
	}
	if session.ResourcePolicyRequired {
		return fmt.Errorf("resource policy transport unavailable")
	}
	return n.transport.Heartbeat(ctx, session, request)
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
	if _, managed := n.statusProvider.(ResourcePolicyApplier); managed || snapshot.MaxRunningJobs != 0 || status.MaxRunningJobs == 0 {
		status.MaxRunningJobs = snapshot.MaxRunningJobs
	}
	status.ActiveAttempts = cloneActiveAttemptHeartbeats(snapshot.ActiveAttempts)
	status.RuntimeHostCapacity = cloneRuntimeHostCapacity(snapshot.RuntimeHostCapacity)
	return status
}

// A bad configuration response must not replace the session and cancel active
// attempts. Bootstrap still fails closed until its first valid managed policy.
func (n *NodeBase) rejectHeartbeatPolicy(err error) error {
	if n.isRegistered != nil && n.isRegistered.IsSet() {
		log.Errorf("retain previous node resource policy: %v", err)
		return nil
	}
	return err
}
