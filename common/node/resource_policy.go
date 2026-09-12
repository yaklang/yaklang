package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// ResourcePolicy is a complete replacement. Explicit zero reserves are valid;
// a zero maximum means unlimited concurrent jobs.
type ResourcePolicy struct {
	SystemReservedCPUMillicores uint64 `json:"system_reserved_cpu_millicores"`
	SystemReservedMemoryBytes   uint64 `json:"system_reserved_memory_bytes"`
	MaxRunningJobs              uint32 `json:"max_running_jobs"`
}
type HeartbeatResponse struct {
	ResourcePolicy *ResourcePolicy `json:"resource_policy,omitempty"`
}
type ResourcePolicyApplier interface{ ApplyResourcePolicy(ResourcePolicy) error }

// ResourcePolicyTransport extends the legacy transport without changing mocks.
type ResourcePolicyTransport interface {
	HeartbeatWithResponse(context.Context, SessionState, HeartbeatRequest) (HeartbeatResponse, error)
}

func (t *httpTransport) HeartbeatWithResponse(ctx context.Context, session SessionState, request HeartbeatRequest) (HeartbeatResponse, error) {
	var response HeartbeatResponse
	endpoint := fmt.Sprintf(heartbeatEndpointFmt, url.PathEscape(session.SessionID))
	err := t.postJSON(ctx, endpoint, session.SessionToken, request, &response)
	return response, err
}

// UnmarshalJSON requires the entire policy; absent fields must not silently
// erase a managed reserve or concurrency ceiling.
func (p *ResourcePolicy) UnmarshalJSON(raw []byte) error {
	var fields struct {
		CPU     *uint64 `json:"system_reserved_cpu_millicores"`
		Memory  *uint64 `json:"system_reserved_memory_bytes"`
		Maximum *uint32 `json:"max_running_jobs"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if fields.CPU == nil || fields.Memory == nil || fields.Maximum == nil {
		return fmt.Errorf("resource policy requires CPU, memory and maximum")
	}
	*p = ResourcePolicy{SystemReservedCPUMillicores: *fields.CPU, SystemReservedMemoryBytes: *fields.Memory, MaxRunningJobs: *fields.Maximum}
	return nil
}

type resourcePolicyResponseError struct{ err error }

func (e *resourcePolicyResponseError) Error() string {
	return fmt.Sprintf("decode heartbeat resource policy: %v", e.err)
}
func (e *resourcePolicyResponseError) Unwrap() error { return e.err }
