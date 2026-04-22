package scannode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/log"
	hidsv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/hids/v1"
)

const (
	hidsResponseActionProcessTerminate = "process.terminate"

	hidsResponseActionStatusSucceeded = "succeeded"
	hidsResponseActionStatusFailed    = "failed"
)

var (
	ErrHIDSResponseActionUnsupported      = errors.New("hids response action is not supported on this build")
	ErrHIDSResponseActionProcessNotFound  = errors.New("target process is not running")
	ErrHIDSResponseActionIdentityMismatch = errors.New("target process identity no longer matches current state")
	ErrHIDSResponseActionProtectedProcess = errors.New("refusing to terminate protected process")
	ErrHIDSResponseActionSignalFailed     = errors.New("failed to terminate target process")
	ErrHIDSResponseActionStillRunning     = errors.New("target process is still running after termination")
)

type hidsResponseActionProcess struct {
	PID                 int
	BootID              string
	StartTimeUnixMillis int64
	ProcessName         string
	ProcessImage        string
	ProcessCommand      string
	Username            string
}

type hidsResponseActionCommandRef struct {
	capabilityCommandRef
	ActionType string
	Process    hidsResponseActionProcess
}

type HIDSResponseActionResultInput struct {
	CommandID     string
	CapabilityKey string
	SpecVersion   string
	ActionType    string
	Status        string
	ErrorCode     string
	ErrorMessage  string
	DetailJSON    []byte
	ObservedAt    time.Time
	Process       hidsResponseActionProcess
}

type hidsResponseActionExecutionResult struct {
	ObservedAt time.Time
	DetailJSON []byte
	Process    hidsResponseActionProcess
}

func (b *legionJobBridge) handleHIDSResponseActionExecute(
	ctx context.Context,
	raw []byte,
) error {
	var command hidsv1.ExecuteHIDSResponseActionCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal hids response action command: %w", err)
	}

	currentNodeID := b.agent.node.CurrentNodeID()
	ref := hidsResponseActionCommandRefFromCommand(currentNodeID, &command)
	if err := validateHIDSResponseActionCommand(currentNodeID, &command); err != nil {
		return b.capabilityPublisher.PublishResponseActionResult(ctx, HIDSResponseActionResultInput{
			CommandID:     ref.CommandID,
			CapabilityKey: ref.CapabilityKey,
			SpecVersion:   ref.SpecVersion,
			ActionType:    ref.ActionType,
			Status:        hidsResponseActionStatusFailed,
			ErrorCode:     "invalid_response_action_command",
			ErrorMessage:  err.Error(),
			ObservedAt:    time.Now().UTC(),
			Process:       ref.Process,
			DetailJSON:    mustMarshalHIDSResponseActionJSON(map[string]any{"reason": err.Error()}),
		})
	}

	result, err := executeHIDSResponseAction(ctx, ref.ActionType, ref.Process)
	if err == nil {
		return b.capabilityPublisher.PublishResponseActionResult(
			b.agent.node.GetRootContext(),
			HIDSResponseActionResultInput{
				CommandID:     ref.CommandID,
				CapabilityKey: ref.CapabilityKey,
				SpecVersion:   ref.SpecVersion,
				ActionType:    ref.ActionType,
				Status:        hidsResponseActionStatusSucceeded,
				ObservedAt:    result.ObservedAt,
				Process:       result.Process,
				DetailJSON:    cloneBytes(result.DetailJSON),
			},
		)
	}

	code, message := mapHIDSResponseActionError(err)
	log.Errorf(
		"execute hids response action failed: node_id=%s action=%s pid=%d command_id=%s code=%s err=%v",
		ref.NodeID,
		ref.ActionType,
		ref.Process.PID,
		ref.CommandID,
		code,
		err,
	)
	detailJSON := cloneBytes(result.DetailJSON)
	if len(detailJSON) == 0 {
		detailJSON = mustMarshalHIDSResponseActionJSON(map[string]any{"reason": message})
	}
	return b.capabilityPublisher.PublishResponseActionResult(
		b.agent.node.GetRootContext(),
		HIDSResponseActionResultInput{
			CommandID:     ref.CommandID,
			CapabilityKey: ref.CapabilityKey,
			SpecVersion:   ref.SpecVersion,
			ActionType:    ref.ActionType,
			Status:        hidsResponseActionStatusFailed,
			ErrorCode:     code,
			ErrorMessage:  message,
			ObservedAt:    responseActionObservedAt(result),
			Process:       effectiveResponseActionProcess(ref.Process, result.Process),
			DetailJSON:    detailJSON,
		},
	)
}

func validateHIDSResponseActionCommand(
	nodeID string,
	command *hidsv1.ExecuteHIDSResponseActionCommand,
) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("response action metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("response action command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("response action target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != strings.TrimSpace(nodeID):
		return fmt.Errorf("response action target_node_id mismatch: %s", command.GetTargetNodeId())
	case command.GetCapability() == nil:
		return fmt.Errorf("capability reference is required")
	case !isHIDSCapabilityKey(command.GetCapability().GetCapabilityKey()):
		return fmt.Errorf("response action capability_key must be hids")
	case strings.TrimSpace(command.GetAction()) != hidsResponseActionProcessTerminate:
		return fmt.Errorf("unsupported response action: %s", command.GetAction())
	case command.GetProcess() == nil:
		return fmt.Errorf("response action process is required")
	case command.GetProcess().GetPid() <= 0:
		return fmt.Errorf("response action process.pid must be positive")
	case strings.TrimSpace(command.GetProcess().GetBootId()) == "":
		return fmt.Errorf("response action process.boot_id is required")
	case command.GetProcess().GetStartTimeUnixMs() <= 0:
		return fmt.Errorf("response action process.start_time_unix_ms must be positive")
	default:
		return nil
	}
}

func hidsResponseActionCommandRefFromCommand(
	nodeID string,
	command *hidsv1.ExecuteHIDSResponseActionCommand,
) hidsResponseActionCommandRef {
	ref := hidsResponseActionCommandRef{
		capabilityCommandRef: capabilityCommandRef{NodeID: nodeID},
	}
	if command == nil {
		return ref
	}
	if command.GetMetadata() != nil {
		ref.CommandID = command.GetMetadata().GetCommandId()
	}
	if strings.TrimSpace(command.GetTargetNodeId()) != "" {
		ref.NodeID = strings.TrimSpace(command.GetTargetNodeId())
	}
	if command.GetCapability() != nil {
		ref.CapabilityKey = command.GetCapability().GetCapabilityKey()
		ref.SpecVersion = normalizeCapabilitySpecVersion(command.GetCapability().GetSpecVersion())
	}
	ref.ActionType = strings.TrimSpace(command.GetAction())
	ref.Process = hidsResponseActionProcessFromProto(command.GetProcess())
	return ref
}

func hidsResponseActionProcessFromProto(process *hidsv1.HIDSProcessRef) hidsResponseActionProcess {
	if process == nil {
		return hidsResponseActionProcess{}
	}
	return hidsResponseActionProcess{
		PID:                 int(process.GetPid()),
		BootID:              strings.TrimSpace(process.GetBootId()),
		StartTimeUnixMillis: process.GetStartTimeUnixMs(),
		ProcessName:         strings.TrimSpace(process.GetProcessName()),
		ProcessImage:        strings.TrimSpace(process.GetProcessImage()),
		ProcessCommand:      strings.TrimSpace(process.GetProcessCommand()),
		Username:            strings.TrimSpace(process.GetUsername()),
	}
}

func effectiveResponseActionProcess(
	fallback hidsResponseActionProcess,
	current hidsResponseActionProcess,
) hidsResponseActionProcess {
	if current.PID > 0 {
		return current
	}
	return fallback
}

func responseActionObservedAt(result hidsResponseActionExecutionResult) time.Time {
	if !result.ObservedAt.IsZero() {
		return result.ObservedAt.UTC()
	}
	return time.Now().UTC()
}

func mapHIDSResponseActionError(err error) (string, string) {
	switch {
	case errors.Is(err, ErrHIDSResponseActionUnsupported):
		return "action_not_supported", err.Error()
	case errors.Is(err, ErrHIDSResponseActionProcessNotFound):
		return "process_not_found", err.Error()
	case errors.Is(err, ErrHIDSResponseActionIdentityMismatch):
		return "process_identity_mismatch", err.Error()
	case errors.Is(err, ErrHIDSResponseActionProtectedProcess):
		return "protected_process", err.Error()
	case errors.Is(err, ErrHIDSResponseActionStillRunning):
		return "process_still_running", err.Error()
	case errors.Is(err, ErrHIDSResponseActionSignalFailed):
		return "process_signal_failed", err.Error()
	default:
		return "response_action_failed", err.Error()
	}
}

func mustMarshalHIDSResponseActionJSON(value any) []byte {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return raw
}

func stringOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
