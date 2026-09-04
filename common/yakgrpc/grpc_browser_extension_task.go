package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	browserTaskDefaultTimeout = 30 * time.Second
	browserTaskMaxTimeout     = 2 * time.Minute
	browserTaskMaxPayload     = 256 << 10
	browserTaskMaxOutput      = 8 << 20
	browserTaskMaxEvent       = 1 << 20
)

func browserTaskRequiresConnectedDevice(schema string) bool {
	switch schema {
	case "authorization.evidence.inspect",
		"authorization.evidence.packet",
		"authorization.evidence.diff",
		"authorization.evidence.validate":
		return false
	default:
		return true
	}
}

type browserTaskEmitter struct {
	mu         sync.Mutex
	stream     ypb.Yak_ExecuteBrowserExtensionTaskServer
	taskID     string
	deviceID   string
	sequence   uint64
	outputSize int
	truncated  bool
}

func (e *browserTaskEmitter) send(eventType, message string, data []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	terminal := eventType == "completed" || eventType == "error" || eventType == "cancelled"
	if len(data) > browserTaskMaxEvent {
		data = append([]byte(nil), data[:browserTaskMaxEvent]...)
		message = strings.TrimSpace(message + " (event data truncated)")
	}
	if !terminal && e.outputSize+len(data)+len(message) > browserTaskMaxOutput {
		if e.truncated {
			return nil
		}
		e.truncated = true
		e.sequence++
		return e.stream.Send(&ypb.BrowserExtensionTaskEvent{
			TaskId: e.taskID, DeviceId: e.deviceID, Type: "warning",
			Message:   "task output limit reached; further log events were discarded",
			Timestamp: time.Now().UnixMilli(), Sequence: e.sequence,
		})
	}
	e.outputSize += len(data) + len(message)
	e.sequence++
	return e.stream.Send(&ypb.BrowserExtensionTaskEvent{
		TaskId: e.taskID, DeviceId: e.deviceID, Type: eventType,
		Message: message, Data: data, Timestamp: time.Now().UnixMilli(), Sequence: e.sequence,
	})
}

func (s *Server) ExecuteBrowserExtensionTask(req *ypb.BrowserExtensionTaskRequest, stream ypb.Yak_ExecuteBrowserExtensionTaskServer) error {
	if req == nil {
		return errors.New("browser extension task request is required")
	}
	if s.browserBridge == nil {
		return errors.New("browser extension bridge is not running")
	}
	taskID := strings.TrimSpace(req.GetTaskId())
	if taskID == "" {
		taskID = uuid.NewString()
	}
	deviceID := strings.TrimSpace(req.GetDeviceId())
	schema := strings.ToLower(strings.TrimSpace(req.GetSchema()))
	emitter := &browserTaskEmitter{stream: stream, taskID: taskID, deviceID: deviceID}
	if deviceID == "" {
		return emitter.send("error", "browser extension device id is required", nil)
	}
	if len(req.GetPayload()) > browserTaskMaxPayload {
		return emitter.send("error", fmt.Sprintf("task payload exceeds %d bytes", browserTaskMaxPayload), nil)
	}
	if !json.Valid(req.GetPayload()) {
		return emitter.send("error", "task payload must be valid JSON", nil)
	}
	if schema != "capability.call" &&
		schema != "yak.script" &&
		schema != "authorization.workspace.create" &&
		schema != "authorization.workspace.inspect" &&
		schema != "authorization.baseline.bind" &&
		schema != "authorization.logical.bind" &&
		schema != "authorization.plan.create" &&
		schema != "authorization.plan.execute" &&
		schema != "authorization.evidence.inspect" &&
		schema != "authorization.evidence.packet" &&
		schema != "authorization.evidence.diff" &&
		schema != "authorization.evidence.validate" {
		return emitter.send("error", fmt.Sprintf("unsupported browser extension task schema %q", schema), nil)
	}
	paired, connected := false, false
	for _, device := range s.browserBridge.Snapshot().Devices {
		if device.ID == deviceID {
			paired = true
			break
		}
	}
	for _, connection := range s.browserBridge.Snapshot().Connections {
		if connection.DeviceID == deviceID {
			connected = true
			break
		}
	}
	if !paired {
		return emitter.send("error", "paired browser extension device not found", nil)
	}
	if browserTaskRequiresConnectedDevice(schema) && !connected {
		return emitter.send("error", "browser extension device is not connected", nil)
	}

	timeout := time.Duration(req.GetTimeoutMilliseconds()) * time.Millisecond
	if timeout <= 0 {
		timeout = browserTaskDefaultTimeout
	}
	if timeout > browserTaskMaxTimeout {
		timeout = browserTaskMaxTimeout
	}
	ctx, cancel := context.WithTimeout(stream.Context(), timeout)
	defer cancel()
	if err := emitter.send("queued", "waiting for an execution slot", nil); err != nil {
		return err
	}
	select {
	case s.browserTasks <- struct{}{}:
		defer func() { <-s.browserTasks }()
	case <-ctx.Done():
		return emitter.send(browserTaskContextEvent(ctx), ctx.Err().Error(), nil)
	}
	if err := emitter.send("running", schema, nil); err != nil {
		return err
	}

	var err error
	switch schema {
	case "capability.call":
		err = s.executeBrowserCapabilityTask(ctx, req.GetPayload(), emitter)
	case "yak.script":
		err = s.executeBrowserYakTask(ctx, req.GetPayload(), emitter)
	case "authorization.workspace.create":
		err = s.executeBrowserAuthorizationWorkspaceCreate(ctx, req.GetPayload(), emitter)
	case "authorization.workspace.inspect":
		err = s.executeBrowserAuthorizationWorkspaceInspect(ctx, req.GetPayload(), emitter)
	case "authorization.baseline.bind":
		err = s.executeBrowserAuthorizationBaselineBind(ctx, req.GetPayload(), emitter)
	case "authorization.logical.bind":
		err = s.executeBrowserAuthorizationLogicalBind(ctx, req.GetPayload(), emitter)
	case "authorization.plan.create":
		err = s.executeBrowserAuthorizationPlanCreate(ctx, req.GetPayload(), emitter)
	case "authorization.plan.execute":
		err = s.executeBrowserAuthorizationPlanExecute(ctx, req.GetPayload(), emitter)
	case "authorization.evidence.inspect":
		err = s.executeBrowserAuthorizationEvidenceInspect(ctx, req.GetPayload(), emitter)
	case "authorization.evidence.packet":
		err = s.executeBrowserAuthorizationEvidencePacket(ctx, req.GetPayload(), emitter)
	case "authorization.evidence.diff":
		err = s.executeBrowserAuthorizationEvidenceDiff(ctx, req.GetPayload(), emitter)
	case "authorization.evidence.validate":
		err = s.executeBrowserAuthorizationEvidenceValidate(ctx, req.GetPayload(), emitter)
	}
	if err != nil {
		if ctx.Err() != nil {
			return emitter.send(browserTaskContextEvent(ctx), ctx.Err().Error(), nil)
		}
		var lifecycle *browser.ExtensionAuthorizationWorkspaceLifecycleError
		if errors.As(err, &lifecycle) {
			data, _ := json.Marshal(lifecycle)
			return emitter.send("error", lifecycle.Error(), data)
		}
		return emitter.send("error", err.Error(), nil)
	}
	return emitter.send("completed", "task completed", nil)
}

func (s *Server) authorizationEvidenceWorkspaceForTask(
	ctx context.Context,
	workspaceID string,
	emitter *browserTaskEmitter,
) error {
	workspace, err := s.browserBridge.GetExtensionAuthorizationWorkspace(
		ctx,
		workspaceID,
		false,
	)
	if err != nil {
		return err
	}
	if workspace.Left.DeviceID != emitter.deviceID {
		return errors.New("authorization workspace does not belong to the task device")
	}
	return nil
}

func (s *Server) executeBrowserAuthorizationEvidenceInspect(
	ctx context.Context,
	payload []byte,
	emitter *browserTaskEmitter,
) error {
	var input browser.ExtensionAuthorizationEvidenceInspectInput
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode authorization.evidence.inspect payload: %w", err)
	}
	if err := s.authorizationEvidenceWorkspaceForTask(ctx, input.WorkspaceID, emitter); err != nil {
		return err
	}
	bundle, err := s.browserBridge.InspectExtensionAuthorizationEvidence(ctx, input)
	if err != nil {
		return err
	}
	result, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("encode authorization evidence bundle: %w", err)
	}
	return emitter.send("result", "authorization.evidence.inspect", result)
}

func (s *Server) executeBrowserAuthorizationEvidencePacket(
	ctx context.Context,
	payload []byte,
	emitter *browserTaskEmitter,
) error {
	var input browser.ExtensionAuthorizationEvidencePacketInput
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode authorization.evidence.packet payload: %w", err)
	}
	if err := s.authorizationEvidenceWorkspaceForTask(ctx, input.WorkspaceID, emitter); err != nil {
		return err
	}
	packet, err := s.browserBridge.ReadExtensionAuthorizationEvidencePacket(ctx, input)
	if err != nil {
		return err
	}
	result, err := json.Marshal(packet)
	if err != nil {
		return fmt.Errorf("encode authorization evidence packet: %w", err)
	}
	return emitter.send("result", "authorization.evidence.packet", result)
}

func (s *Server) executeBrowserAuthorizationEvidenceDiff(
	ctx context.Context,
	payload []byte,
	emitter *browserTaskEmitter,
) error {
	var input browser.ExtensionAuthorizationEvidenceDiffInput
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode authorization.evidence.diff payload: %w", err)
	}
	if err := s.authorizationEvidenceWorkspaceForTask(ctx, input.WorkspaceID, emitter); err != nil {
		return err
	}
	diff, err := s.browserBridge.DiffExtensionAuthorizationEvidence(ctx, input)
	if err != nil {
		return err
	}
	result, err := json.Marshal(diff)
	if err != nil {
		return fmt.Errorf("encode authorization evidence diff: %w", err)
	}
	return emitter.send("result", "authorization.evidence.diff", result)
}

func (s *Server) executeBrowserAuthorizationEvidenceValidate(
	ctx context.Context,
	payload []byte,
	emitter *browserTaskEmitter,
) error {
	var input browser.ExtensionAuthorizationEvidenceValidationInput
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode authorization.evidence.validate payload: %w", err)
	}
	if err := s.authorizationEvidenceWorkspaceForTask(ctx, input.WorkspaceID, emitter); err != nil {
		return err
	}
	validation, err := s.browserBridge.ValidateExtensionAuthorizationEvidence(ctx, input)
	if err != nil {
		return err
	}
	result, err := json.Marshal(validation)
	if err != nil {
		return fmt.Errorf("encode authorization evidence validation: %w", err)
	}
	return emitter.send("result", "authorization.evidence.validate", result)
}

func (s *Server) executeBrowserAuthorizationLogicalBind(
	ctx context.Context,
	payload []byte,
	emitter *browserTaskEmitter,
) error {
	var input browser.ExtensionAuthorizationLogicalBindingInput
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode authorization.logical.bind payload: %w", err)
	}
	workspace, err := s.browserBridge.GetExtensionAuthorizationWorkspace(
		ctx,
		input.WorkspaceID,
		false,
	)
	if err != nil {
		return err
	}
	if workspace.Left.DeviceID != emitter.deviceID {
		return errors.New("authorization workspace does not belong to the task device")
	}
	workspace, err = s.browserBridge.BindExtensionAuthorizationLogicalRequests(ctx, input)
	if err != nil {
		return err
	}
	result, err := json.Marshal(workspace)
	if err != nil {
		return fmt.Errorf("encode authorization workspace: %w", err)
	}
	return emitter.send("result", "authorization.logical.bind", result)
}

func (s *Server) executeBrowserAuthorizationPlanExecute(
	ctx context.Context,
	payload []byte,
	emitter *browserTaskEmitter,
) error {
	var input browser.ExtensionAuthorizationExecutionInput
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode authorization.plan.execute payload: %w", err)
	}
	workspace, err := s.browserBridge.GetExtensionAuthorizationWorkspace(
		ctx,
		input.WorkspaceID,
		false,
	)
	if err != nil {
		return err
	}
	if workspace.Left.DeviceID != emitter.deviceID {
		return errors.New("authorization workspace does not belong to the task device")
	}
	workspace, err = s.browserBridge.ExecuteExtensionAuthorizationPlan(ctx, input)
	if err != nil {
		return err
	}
	result, err := json.Marshal(workspace)
	if err != nil {
		return fmt.Errorf("encode authorization workspace: %w", err)
	}
	return emitter.send("result", "authorization.plan.execute", result)
}

func (s *Server) executeBrowserAuthorizationPlanCreate(
	ctx context.Context,
	payload []byte,
	emitter *browserTaskEmitter,
) error {
	var input browser.ExtensionAuthorizationPlanInput
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode authorization.plan.create payload: %w", err)
	}
	workspace, err := s.browserBridge.GetExtensionAuthorizationWorkspace(
		ctx,
		input.WorkspaceID,
		false,
	)
	if err != nil {
		return err
	}
	if workspace.Left.DeviceID != emitter.deviceID {
		return errors.New("authorization workspace does not belong to the task device")
	}
	workspace, err = s.browserBridge.CreateExtensionAuthorizationPlan(ctx, input)
	if err != nil {
		return err
	}
	result, err := json.Marshal(workspace)
	if err != nil {
		return fmt.Errorf("encode authorization workspace: %w", err)
	}
	return emitter.send("result", "authorization.plan.create", result)
}

func (s *Server) executeBrowserAuthorizationBaselineBind(
	ctx context.Context,
	payload []byte,
	emitter *browserTaskEmitter,
) error {
	var input browser.ExtensionAuthorizationBaselineInput
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode authorization.baseline.bind payload: %w", err)
	}
	workspace, err := s.browserBridge.GetExtensionAuthorizationWorkspace(
		ctx,
		input.WorkspaceID,
		false,
	)
	if err != nil {
		return err
	}
	if workspace.Left.DeviceID != emitter.deviceID {
		return errors.New("authorization workspace does not belong to the task device")
	}
	workspace, err = s.browserBridge.BindExtensionAuthorizationBaseline(ctx, input)
	if err != nil {
		return err
	}
	result, err := json.Marshal(workspace)
	if err != nil {
		return fmt.Errorf("encode authorization workspace: %w", err)
	}
	return emitter.send("result", "authorization.baseline.bind", result)
}

func decodeBrowserTaskPayload(payload []byte, output interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("browser task payload contains trailing data")
	}
	return nil
}

func (s *Server) executeBrowserAuthorizationWorkspaceCreate(
	ctx context.Context,
	payload []byte,
	emitter *browserTaskEmitter,
) error {
	var input browser.ExtensionAuthorizationWorkspaceInput
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode authorization.workspace.create payload: %w", err)
	}
	if strings.TrimSpace(input.Left.DeviceID) != emitter.deviceID {
		return errors.New("authorization workspace left device must match the task device")
	}
	workspace, err := s.browserBridge.CreateExtensionAuthorizationWorkspace(ctx, input)
	if err != nil {
		return err
	}
	result, err := json.Marshal(workspace)
	if err != nil {
		return fmt.Errorf("encode authorization workspace: %w", err)
	}
	return emitter.send("result", "authorization.workspace.create", result)
}

func (s *Server) executeBrowserAuthorizationWorkspaceInspect(
	ctx context.Context,
	payload []byte,
	emitter *browserTaskEmitter,
) error {
	var input struct {
		WorkspaceID string `json:"workspaceId"`
		Revalidate  *bool  `json:"revalidate,omitempty"`
	}
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode authorization.workspace.inspect payload: %w", err)
	}
	revalidate := true
	if input.Revalidate != nil {
		revalidate = *input.Revalidate
	}
	workspace, err := s.browserBridge.GetExtensionAuthorizationWorkspace(ctx, input.WorkspaceID, revalidate)
	if err != nil {
		return err
	}
	if workspace.Left.DeviceID != emitter.deviceID {
		return errors.New("authorization workspace does not belong to the task device")
	}
	result, err := json.Marshal(workspace)
	if err != nil {
		return fmt.Errorf("encode authorization workspace: %w", err)
	}
	return emitter.send("result", "authorization.workspace.inspect", result)
}

func (s *Server) executeBrowserCapabilityTask(ctx context.Context, payload []byte, emitter *browserTaskEmitter) error {
	var input struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode capability.call payload: %w", err)
	}
	input.Method = strings.TrimSpace(input.Method)
	if input.Method == "" {
		return errors.New("capability.call method is required")
	}
	var params interface{} = map[string]interface{}{}
	if len(input.Params) > 0 && string(input.Params) != "null" {
		if err := json.Unmarshal(input.Params, &params); err != nil {
			return fmt.Errorf("decode capability.call params: %w", err)
		}
	}
	result, err := s.browserBridge.CallDevice(ctx, emitter.deviceID, input.Method, params)
	if err != nil {
		return err
	}
	return emitter.send("result", input.Method, result)
}

func (s *Server) executeBrowserYakTask(ctx context.Context, payload []byte, emitter *browserTaskEmitter) error {
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeBrowserTaskPayload(payload, &input); err != nil {
		return fmt.Errorf("decode yak.script payload: %w", err)
	}
	if strings.TrimSpace(input.Code) == "" {
		return errors.New("yak.script code is required")
	}

	feedbackClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		data, _ := protojson.MarshalOptions{UseProtoNames: true}.Marshal(result)
		message := strings.TrimSpace(string(result.GetRaw()))
		if message == "" {
			message = strings.TrimSpace(result.GetOutputJson())
		}
		if message == "" {
			message = strings.TrimSpace(string(result.GetMessage()))
		}
		return emitter.send("log", message, data)
	}, emitter.taskID)
	engine := yak.NewYakitVirtualClientScriptEngine(feedbackClient)
	engine.HookOsExit()
	engine.RegisterEngineHooks(func(ae *antlr4yak.Engine) error {
		ae.SetVars(map[string]interface{}{
			"RUNTIME_ID": emitter.taskID,
			"CTX":        ctx,
			"browser": map[string]interface{}{
				"ExtensionCall": func(method string, params interface{}, timeoutSeconds ...float64) (interface{}, error) {
					callCtx := ctx
					var cancel context.CancelFunc
					if len(timeoutSeconds) > 0 && timeoutSeconds[0] > 0 {
						callCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds[0]*float64(time.Second)))
						defer cancel()
					}
					result, err := s.browserBridge.CallDevice(callCtx, emitter.deviceID, method, params)
					if err != nil {
						return nil, err
					}
					return decodeBrowserTaskResult(result), nil
				},
				"ExtensionStatus": func() interface{} {
					for _, connection := range s.browserBridge.Snapshot().Connections {
						if connection.DeviceID == emitter.deviceID {
							return connection
						}
					}
					return map[string]interface{}{"deviceId": emitter.deviceID, "connected": false}
				},
			},
		})
		return nil
	})
	_, err := engine.ExecuteExWithContext(ctx, input.Code, map[string]interface{}{
		"RUNTIME_ID": emitter.taskID,
		"CTX":        ctx,
	})
	return err
}

func decodeBrowserTaskResult(result json.RawMessage) interface{} {
	if len(result) == 0 || string(result) == "null" {
		return nil
	}
	var value interface{}
	if err := json.Unmarshal(result, &value); err != nil {
		return string(result)
	}
	return value
}

func browserTaskContextEvent(ctx context.Context) string {
	if errors.Is(ctx.Err(), context.Canceled) {
		return "cancelled"
	}
	return "error"
}
