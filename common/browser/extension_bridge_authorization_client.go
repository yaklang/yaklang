package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const maxExtensionAuthorizationTaskBytes = 256 << 10

type extensionAuthorizationClientTaskParams struct {
	Schema  string          `json:"schema"`
	Payload json.RawMessage `json:"payload"`
}

type extensionAuthorizationClientIdentityInput struct {
	TabID        int    `json:"tabId"`
	FrameID      int    `json:"frameId"`
	AccountLabel string `json:"accountLabel,omitempty"`
}

type extensionAuthorizationClientWorkspaceInput struct {
	Mode  string                                    `json:"mode"`
	Left  extensionAuthorizationClientIdentityInput `json:"left"`
	Right extensionAuthorizationClientIdentityInput `json:"right"`
}

type extensionAuthorizationYakitOpenInput struct {
	WorkspaceID string `json:"workspaceId,omitempty"`
	TabID       int    `json:"tabId,omitempty"`
	Mode        string `json:"mode,omitempty"`
}

func decodeExtensionAuthorizationYakitOpen(raw json.RawMessage) (extensionAuthorizationYakitOpenInput, error) {
	var input extensionAuthorizationYakitOpenInput
	if err := decodeExtensionAuthorizationClientJSON(raw, &input); err != nil {
		return input, err
	}
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	if input.WorkspaceID != "" {
		if input.TabID != 0 || input.Mode != "" {
			return input, errors.New("workspaceId cannot be combined with tabId or mode")
		}
		return input, nil
	}
	if input.TabID <= 0 {
		return input, errors.New("workspaceId or tabId is required")
	}
	if input.Mode == "" {
		input.Mode = "horizontal"
	}
	if input.Mode != "horizontal" && input.Mode != "vertical" {
		return input, errors.New("mode must be horizontal or vertical")
	}
	return input, nil
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
	if workspace.Left.DeviceID != deviceID || workspace.Right.DeviceID != deviceID {
		return ExtensionAuthorizationWorkspace{}, errors.New(
			"authorization workspace does not belong exclusively to this browser extension",
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
		workspace, err := s.manager.CreateExtensionAuthorizationWorkspace(ctx, ExtensionAuthorizationWorkspaceInput{
			Mode: input.Mode,
			Left: ExtensionAuthorizationIdentityInput{
				DeviceID: deviceID, TabID: input.Left.TabID, FrameID: input.Left.FrameID,
				AccountLabel: input.Left.AccountLabel,
			},
			Right: ExtensionAuthorizationIdentityInput{
				DeviceID: deviceID, TabID: input.Right.TabID, FrameID: input.Right.FrameID,
				AccountLabel: input.Right.AccountLabel,
			},
		})
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
		side := strings.ToLower(strings.TrimSpace(input.Side))
		var slot ExtensionAuthorizationIdentitySlot
		switch side {
		case "left":
			slot = workspace.Left
		case "right":
			slot = workspace.Right
		default:
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "baseline side must be left or right"}
		}
		limit := input.Limit
		if limit == 0 {
			limit = 50
		}
		if limit < 1 || limit > 200 {
			return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "baseline candidate limit must be between 1 and 200"}
		}
		raw, err := s.manager.CallDevice(ctx, deviceID, "browser.authorization.baseline.candidates", map[string]interface{}{
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

func (s *ExtensionBridgeServer) openExtensionAuthorizationWorkspaceInYakit(
	ctx context.Context,
	deviceID string,
	params json.RawMessage,
) (interface{}, *ExtensionBridgeError) {
	input, err := decodeExtensionAuthorizationYakitOpen(params)
	if err != nil {
		return nil, &ExtensionBridgeError{
			Code:    "invalid_params",
			Message: "invalid authorization workspace handoff: " + err.Error(),
		}
	}
	handoff := map[string]interface{}{
		"event":    "authorization.workspace.open",
		"deviceId": deviceID,
	}
	result := map[string]interface{}{"opened": true}
	if input.WorkspaceID != "" {
		workspace, workspaceErr := s.extensionAuthorizationWorkspaceForDevice(
			ctx,
			deviceID,
			input.WorkspaceID,
			false,
		)
		if workspaceErr != nil {
			return nil, extensionAuthorizationClientError(workspaceErr)
		}
		handoff["workspaceId"] = workspace.ID
		handoff["mode"] = workspace.Mode
		result["workspaceId"] = workspace.ID
	} else {
		handoff["tabId"] = input.TabID
		handoff["mode"] = input.Mode
		result["tabId"] = input.TabID
	}
	yakit.BroadcastData(yakit.ServerPushType_BrowserExtension, handoff)
	return result, nil
}
