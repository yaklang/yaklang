package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const maxExtensionAuthorizationTaskBytes = 256 << 10

type extensionAuthorizationClientTaskParams struct {
	Schema  string          `json:"schema"`
	Payload json.RawMessage `json:"payload"`
}

type extensionAuthorizationClientIdentityInput struct {
	DeviceID     string `json:"deviceId,omitempty"`
	TabID        int    `json:"tabId"`
	FrameID      int    `json:"frameId"`
	AccountLabel string `json:"accountLabel,omitempty"`
}

type extensionAuthorizationClientWorkspaceInput struct {
	Mode  string                                    `json:"mode"`
	Left  extensionAuthorizationClientIdentityInput `json:"left"`
	Right extensionAuthorizationClientIdentityInput `json:"right"`
}

type extensionAuthorizationInstance struct {
	DeviceID string                              `json:"deviceId"`
	Badge    string                              `json:"badge"`
	Current  bool                                `json:"current"`
	Tabs     []extensionAuthorizationInstanceTab `json:"tabs"`
	Error    string                              `json:"error,omitempty"`
}

type extensionAuthorizationInstanceTab struct {
	ID           int     `json:"id"`
	WindowID     int     `json:"windowId"`
	Active       bool    `json:"active,omitempty"`
	Title        string  `json:"title"`
	URL          string  `json:"url"`
	Incognito    bool    `json:"incognito"`
	LastAccessed float64 `json:"lastAccessed,omitempty"`
}

func extensionAuthorizationInstances(connections []ExtensionBridgeConnection, currentDeviceID string) []extensionAuthorizationInstance {
	instances := make([]extensionAuthorizationInstance, 0, len(connections))
	for _, connection := range connections {
		if connection.ManagedInstance == nil || connection.ManagedInstance.Manager != "ytray" {
			continue
		}
		instances = append(instances, extensionAuthorizationInstance{
			DeviceID: connection.DeviceID,
			Badge:    connection.ManagedInstance.Badge,
			Current:  connection.DeviceID == currentDeviceID,
			Tabs:     []extensionAuthorizationInstanceTab{},
		})
	}
	return instances
}

func (s *ExtensionBridgeServer) listExtensionAuthorizationInstances(
	ctx context.Context,
	deviceID string,
) (interface{}, *ExtensionBridgeError) {
	if s.manager == nil {
		return nil, &ExtensionBridgeError{Code: "unavailable", Message: "browser extension bridge manager is not available"}
	}
	instances := extensionAuthorizationInstances(s.Connections(), deviceID)
	for index := range instances {
		raw, err := s.manager.CallDevice(ctx, instances[index].DeviceID, "browser.tabs", map[string]interface{}{})
		if err != nil {
			instances[index].Error = err.Error()
			continue
		}
		var tabs []extensionAuthorizationInstanceTab
		if err := json.Unmarshal(raw, &tabs); err != nil {
			instances[index].Error = "browser returned an invalid tab list"
			continue
		}
		for _, tab := range tabs {
			if len(instances[index].Tabs) >= 256 {
				break
			}
			if tab.ID <= 0 || (!strings.HasPrefix(tab.URL, "http://") && !strings.HasPrefix(tab.URL, "https://")) {
				continue
			}
			instances[index].Tabs = append(instances[index].Tabs, tab)
		}
	}
	return map[string]interface{}{
		"instances": instances,
	}, nil
}

func extensionAuthorizationWorkspaceInputForDevice(
	deviceID string,
	input extensionAuthorizationClientWorkspaceInput,
) (ExtensionAuthorizationWorkspaceInput, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return ExtensionAuthorizationWorkspaceInput{}, errors.New("connected browser identity is missing")
	}
	input.Left.DeviceID = strings.TrimSpace(input.Left.DeviceID)
	input.Right.DeviceID = strings.TrimSpace(input.Right.DeviceID)
	if input.Left.DeviceID != "" && input.Left.DeviceID != deviceID {
		return ExtensionAuthorizationWorkspaceInput{}, errors.New(
			"authorization workspace left identity must be the calling browser instance",
		)
	}
	input.Left.DeviceID = deviceID
	if input.Right.DeviceID == "" {
		input.Right.DeviceID = deviceID
	}
	return ExtensionAuthorizationWorkspaceInput{
		Mode: input.Mode,
		Left: ExtensionAuthorizationIdentityInput{
			DeviceID: input.Left.DeviceID, TabID: input.Left.TabID, FrameID: input.Left.FrameID,
			AccountLabel: input.Left.AccountLabel,
		},
		Right: ExtensionAuthorizationIdentityInput{
			DeviceID: input.Right.DeviceID, TabID: input.Right.TabID, FrameID: input.Right.FrameID,
			AccountLabel: input.Right.AccountLabel,
		},
	}, nil
}

func decodeExtensionAuthorizationClientJSON(raw json.RawMessage, output interface{}) error {
	if len(raw) == 0 {
		return errors.New("JSON payload is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON payload contains trailing data")
	}
	return nil
}

func extensionAuthorizationClientError(err error) *ExtensionBridgeError {
	if err == nil {
		return nil
	}
	var lifecycle *ExtensionAuthorizationWorkspaceLifecycleError
	if errors.As(err, &lifecycle) {
		data, _ := json.Marshal(lifecycle)
		return &ExtensionBridgeError{
			Code:    "authorization_workspace_" + string(lifecycle.Reason),
			Message: lifecycle.Error(),
			Data:    data,
		}
	}
	return &ExtensionBridgeError{
		Code:    "authorization_task_failed",
		Message: err.Error(),
	}
}

func extensionAuthorizationClientSlot(
	workspace ExtensionAuthorizationWorkspace,
	side string,
) (ExtensionAuthorizationIdentitySlot, error) {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "left":
		return workspace.Left, nil
	case "right":
		return workspace.Right, nil
	default:
		return ExtensionAuthorizationIdentitySlot{}, errors.New("authorization side must be left or right")
	}
}

func (s *ExtensionBridgeServer) extensionAuthorizationWorkspaceForDevice(
	ctx context.Context,
	deviceID, workspaceID string,
	revalidate bool,
) (ExtensionAuthorizationWorkspace, error) {
	if s.manager == nil {
		return ExtensionAuthorizationWorkspace{}, errors.New("browser extension bridge manager is not available")
	}
	deviceID = strings.TrimSpace(deviceID)
	workspaceID = strings.TrimSpace(workspaceID)
	if deviceID == "" {
		return ExtensionAuthorizationWorkspace{}, errors.New("connected browser identity is missing")
	}
	if workspaceID == "" {
		return ExtensionAuthorizationWorkspace{}, errors.New("authorization workspaceId is required")
	}
	workspace, err := s.manager.GetExtensionAuthorizationWorkspace(ctx, workspaceID, revalidate)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if workspace.Left.DeviceID != deviceID {
		return ExtensionAuthorizationWorkspace{}, errors.New(
			"authorization workspace does not belong to this browser extension",
		)
	}
	return workspace, nil
}

func (s *ExtensionBridgeServer) handleExtensionAuthorizationClientTask(
	ctx context.Context,
	deviceID string,
	params json.RawMessage,
) (interface{}, *ExtensionBridgeError) {
	if s.manager == nil {
		return nil, &ExtensionBridgeError{
			Code:    "bridge_unavailable",
			Message: "browser extension bridge manager is not available",
		}
	}
	if len(params) == 0 || len(params) > maxExtensionAuthorizationTaskBytes {
		return nil, &ExtensionBridgeError{
			Code:    "invalid_params",
			Message: fmt.Sprintf("authorization task must be between 1 byte and %d bytes", maxExtensionAuthorizationTaskBytes),
		}
	}
	var task extensionAuthorizationClientTaskParams
	if err := decodeExtensionAuthorizationClientJSON(params, &task); err != nil {
		return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid authorization task: " + err.Error()}
	}
	task.Schema = strings.ToLower(strings.TrimSpace(task.Schema))
	if len(task.Payload) == 0 || len(task.Payload) > maxExtensionAuthorizationTaskBytes {
		return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "authorization task payload is missing or too large"}
	}

	switch task.Schema {
	case "authorization.workspace.create":
		var input extensionAuthorizationClientWorkspaceInput
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid authorization workspace: " + err.Error()}
		}
		workspaceInput, err := extensionAuthorizationWorkspaceInputForDevice(deviceID, input)
		if err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: err.Error()}
		}
		workspace, err := s.manager.CreateExtensionAuthorizationWorkspace(ctx, workspaceInput)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		return workspace, nil

	case "authorization.workspace.inspect":
		var input struct {
			WorkspaceID string `json:"workspaceId"`
			Revalidate  *bool  `json:"revalidate,omitempty"`
		}
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid workspace inspection: " + err.Error()}
		}
		revalidate := true
		if input.Revalidate != nil {
			revalidate = *input.Revalidate
		}
		workspace, err := s.extensionAuthorizationWorkspaceForDevice(ctx, deviceID, input.WorkspaceID, revalidate)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		return workspace, nil

	case "authorization.baseline.candidates":
		var input struct {
			WorkspaceID string `json:"workspaceId"`
			Side        string `json:"side"`
			Limit       int    `json:"limit,omitempty"`
		}
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid baseline candidate query: " + err.Error()}
		}
		workspace, err := s.extensionAuthorizationWorkspaceForDevice(ctx, deviceID, input.WorkspaceID, false)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		slot, err := extensionAuthorizationClientSlot(workspace, input.Side)
		if err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "baseline side must be left or right"}
		}
		limit := input.Limit
		if limit == 0 {
			limit = 50
		}
		if limit < 1 || limit > 200 {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "baseline candidate limit must be between 1 and 200"}
		}
		raw, err := s.manager.CallDevice(ctx, slot.DeviceID, "browser.authorization.baseline.candidates", map[string]interface{}{
			"tabId":           slot.Target.TabID,
			"frameId":         slot.Target.FrameID,
			"documentId":      slot.Target.DocumentID,
			"authContextKind": slot.ContextReference.Kind,
			"authContextId":   slot.ContextReference.ID,
			"limit":           limit,
		})
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		var candidates []map[string]interface{}
		if err := decodeExtensionAuthorizationClientJSON(raw, &candidates); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_device_result", Message: "invalid baseline candidates: " + err.Error()}
		}
		return candidates, nil

	case "authorization.capture.start", "authorization.capture.status", "authorization.capture.stop":
		var input struct {
			WorkspaceID string `json:"workspaceId"`
			Side        string `json:"side"`
		}
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid authorization capture task: " + err.Error()}
		}
		workspace, err := s.extensionAuthorizationWorkspaceForDevice(ctx, deviceID, input.WorkspaceID, false)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		if task.Schema == "authorization.capture.start" && workspace.State != "ready" && workspace.State != "conditional" {
			return nil, &ExtensionBridgeError{Code: "workspace_not_ready", Message: "authorization workspace is not ready for capture"}
		}
		slot, err := extensionAuthorizationClientSlot(workspace, input.Side)
		if err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "capture side must be left or right"}
		}
		method := "browser.network.status"
		params := map[string]interface{}{
			"tabId": slot.Target.TabID, "frameId": slot.Target.FrameID, "documentId": slot.Target.DocumentID,
		}
		if task.Schema == "authorization.capture.start" {
			method = "browser.network.start"
			params["captureHeaders"] = true
			params["captureBody"] = true
			params["maxEntries"] = 200
			params["maxBodyBytes"] = 64 * 1024
		} else if task.Schema == "authorization.capture.stop" {
			method = "browser.network.stop"
		}
		raw, err := s.manager.CallDevice(ctx, slot.DeviceID, method, params)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		var result interface{}
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_device_result", Message: "invalid authorization capture result: " + err.Error()}
		}
		return result, nil

	case "authorization.baseline.bind":
		var input ExtensionAuthorizationBaselineInput
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid baseline binding: " + err.Error()}
		}
		if _, err := s.extensionAuthorizationWorkspaceForDevice(ctx, deviceID, input.WorkspaceID, false); err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		workspace, err := s.manager.BindExtensionAuthorizationBaseline(ctx, input)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		return workspace, nil

	case "authorization.logical.bind":
		var input ExtensionAuthorizationLogicalBindingInput
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid logical request binding: " + err.Error()}
		}
		if _, err := s.extensionAuthorizationWorkspaceForDevice(ctx, deviceID, input.WorkspaceID, false); err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		workspace, err := s.manager.BindExtensionAuthorizationLogicalRequests(ctx, input)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		return workspace, nil

	case "authorization.plan.create":
		var input ExtensionAuthorizationPlanInput
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid authorization plan: " + err.Error()}
		}
		if _, err := s.extensionAuthorizationWorkspaceForDevice(ctx, deviceID, input.WorkspaceID, false); err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		workspace, err := s.manager.CreateExtensionAuthorizationPlan(ctx, input)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		return workspace, nil

	case "authorization.plan.execute":
		var input ExtensionAuthorizationExecutionInput
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid authorization execution: " + err.Error()}
		}
		if _, err := s.extensionAuthorizationWorkspaceForDevice(ctx, deviceID, input.WorkspaceID, true); err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		workspace, err := s.manager.ExecuteExtensionAuthorizationPlan(ctx, input)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		return workspace, nil

	case "authorization.evidence.inspect":
		var input ExtensionAuthorizationEvidenceInspectInput
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid authorization evidence inspection: " + err.Error()}
		}
		if _, err := s.extensionAuthorizationWorkspaceForDevice(ctx, deviceID, input.WorkspaceID, false); err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		bundle, err := s.manager.InspectExtensionAuthorizationEvidence(ctx, input)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		return bundle, nil

	case "authorization.evidence.packet":
		var input ExtensionAuthorizationEvidencePacketInput
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid authorization evidence packet query: " + err.Error()}
		}
		if _, err := s.extensionAuthorizationWorkspaceForDevice(ctx, deviceID, input.WorkspaceID, false); err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		packet, err := s.manager.ReadExtensionAuthorizationEvidencePacket(ctx, input)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		return packet, nil

	case "authorization.evidence.diff":
		var input ExtensionAuthorizationEvidenceDiffInput
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid authorization evidence diff query: " + err.Error()}
		}
		if _, err := s.extensionAuthorizationWorkspaceForDevice(ctx, deviceID, input.WorkspaceID, false); err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		diff, err := s.manager.DiffExtensionAuthorizationEvidence(ctx, input)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		return diff, nil

	case "authorization.evidence.validate":
		var input ExtensionAuthorizationEvidenceValidationInput
		if err := decodeExtensionAuthorizationClientJSON(task.Payload, &input); err != nil {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid authorization evidence validation: " + err.Error()}
		}
		if _, err := s.extensionAuthorizationWorkspaceForDevice(ctx, deviceID, input.WorkspaceID, false); err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		validation, err := s.manager.ValidateExtensionAuthorizationEvidence(ctx, input)
		if err != nil {
			return nil, extensionAuthorizationClientError(err)
		}
		return validation, nil

	default:
		return nil, &ExtensionBridgeError{
			Code:    "invalid_params",
			Message: "unsupported authorization task schema: " + task.Schema,
		}
	}
}
