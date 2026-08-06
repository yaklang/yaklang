package browser

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

func normalizeAuthorizationIdentity(
	input ExtensionAuthorizationIdentityInput,
) (ExtensionAuthorizationIdentityInput, error) {
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.AccountLabel = strings.TrimSpace(input.AccountLabel)
	if input.DeviceID == "" || len(input.DeviceID) > 320 {
		return input, errors.New("authorization identity deviceId is required")
	}
	if input.TabID <= 0 || input.FrameID < 0 {
		return input, errors.New("authorization identity target is invalid")
	}
	if len([]rune(input.AccountLabel)) > 80 {
		return input, errors.New("authorization identity accountLabel exceeds 80 characters")
	}
	return input, nil
}

func extensionAuthorizationHasCapability(
	connection ExtensionBridgeConnection,
	method string,
) bool {
	for _, capability := range connection.Capabilities {
		if capability == method {
			return true
		}
	}
	return false
}

func authorizationDevice(
	snapshot ExtensionBridgeManagerSnapshot,
	deviceID string,
	required ...string,
) (ExtensionBridgeDevice, ExtensionBridgeConnection, error) {
	var device ExtensionBridgeDevice
	var connection ExtensionBridgeConnection
	foundDevice := false
	foundConnection := false
	for _, candidate := range snapshot.Devices {
		if candidate.ID == deviceID {
			device = candidate
			foundDevice = true
			break
		}
	}
	for _, candidate := range snapshot.Connections {
		if candidate.DeviceID == deviceID {
			connection = candidate
			foundConnection = true
			break
		}
	}
	if !foundDevice {
		return device, connection, fmt.Errorf("paired browser extension device %q was not found", deviceID)
	}
	if !foundConnection {
		return device, connection, fmt.Errorf("browser extension device %q is offline", deviceID)
	}
	if device.InstallationID == "" || connection.InstallationID != device.InstallationID {
		return device, connection, fmt.Errorf("browser extension device %q installation identity changed", deviceID)
	}
	for _, method := range required {
		if !extensionAuthorizationHasCapability(connection, method) {
			return device, connection, fmt.Errorf(
				"browser extension device %q does not declare %s",
				deviceID,
				method,
			)
		}
	}
	return device, connection, nil
}

func decodeAuthorizationResult(raw json.RawMessage, output interface{}) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("browser extension returned an empty authorization result")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("decode browser authorization result: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("browser extension returned trailing authorization data")
	}
	return nil
}

func validateAuthorizationContext(
	context extensionAuthorizationContextBase,
	device ExtensionBridgeDevice,
	input ExtensionAuthorizationIdentityInput,
) error {
	if context.Version != 1 || context.ID == "" || context.DeviceID != input.DeviceID {
		return errors.New("browser authorization context identity does not match the selected device")
	}
	if context.InstallationID != device.InstallationID {
		return errors.New("browser authorization context installation identity does not match the paired device")
	}
	if context.Target.TabID != input.TabID || context.Target.FrameID != input.FrameID || context.Target.DocumentID == "" {
		return errors.New("browser authorization context target does not match the selected document")
	}
	if context.IsolationContextID == "" || context.CookieStoreID == "" || context.Origin == "" ||
		context.GrantID == "" || context.Fingerprint == "" {
		return errors.New("browser authorization context is incomplete")
	}
	if context.ExpiresAt <= time.Now().UnixMilli() || context.ExpiresAt <= context.CreatedAt {
		return errors.New("browser authorization context is already expired")
	}
	switch context.Authentication.Status {
	case "authenticated", "unauthenticated", "unknown":
	default:
		return errors.New("browser authorization context has an invalid authentication status")
	}
	return nil
}

func authorizationSlotFromContext(
	side string,
	label string,
	kind string,
	context extensionAuthorizationContextBase,
) ExtensionAuthorizationIdentitySlot {
	return ExtensionAuthorizationIdentitySlot{
		Side:               side,
		AccountLabel:       label,
		DeviceID:           context.DeviceID,
		InstallationID:     context.InstallationID,
		IsolationContextID: context.IsolationContextID,
		CookieStoreID:      context.CookieStoreID,
		Origin:             context.Origin,
		GrantID:            context.GrantID,
		Target:             context.Target,
		ContextReference: ExtensionAuthorizationContextReference{
			Kind: kind,
			ID:   context.ID,
		},
		Fingerprint:    context.Fingerprint,
		Authentication: context.Authentication,
		ExpiresAt:      context.ExpiresAt,
	}
}

func lowerAuthorizationLevel(level string) string {
	switch level {
	case "strong", "conditional", "none":
		return level
	default:
		return "none"
	}
}

func authorizationWorkspaceState(level string) string {
	switch level {
	case "strong":
		return "ready"
	case "conditional":
		return "conditional"
	default:
		return "blocked"
	}
}

func evaluateAuthorizationProof(
	source string,
	sourceProofID string,
	baseLevel string,
	cookieStoreRelation string,
	refreshCheck string,
	baseReasons []string,
	left ExtensionAuthorizationIdentitySlot,
	right ExtensionAuthorizationIdentitySlot,
	now int64,
	expiresAt int64,
) ExtensionAuthorizationProof {
	level := lowerAuthorizationLevel(baseLevel)
	sameOrigin := left.Origin != "" && left.Origin == right.Origin
	reasons := append([]string(nil), baseReasons...)
	accountRelation := "unknown"
	credentialRelation := "unknown"
	if !sameOrigin {
		level = "none"
		reasons = append(reasons, "两个身份页面来源不同，不能建立默认授权差异工作区")
	}
	if left.Authentication.Status == "unauthenticated" || right.Authentication.Status == "unauthenticated" {
		level = "none"
		reasons = append(reasons, "至少一个身份明确未登录")
	} else if (left.Authentication.Status == "unknown" || right.Authentication.Status == "unknown") && level == "strong" {
		level = "conditional"
		reasons = append(reasons, "至少一个身份的页面登录信号仍需用户确认")
	}
	if left.InstallationID == right.InstallationID {
		if left.Fingerprint == right.Fingerprint {
			accountRelation = "same"
			credentialRelation = "same"
			level = "none"
			reasons = append(reasons, "两个身份的认证指纹相同，可能仍是同一登录态")
		} else {
			credentialRelation = "different"
			reasons = append(reasons, "同一插件安装下的认证指纹不同")
		}
	} else if source == "separate-installations" {
		reasons = append(reasons, "认证指纹使用各安装独立 HMAC，不能跨设备直接比较账号是否相同")
	}
	if expiresAt <= now {
		level = "none"
		reasons = append(reasons, "身份上下文已经过期")
	}
	return ExtensionAuthorizationProof{
		ID:                        "authorization-proof-" + uuid.NewString(),
		Source:                    source,
		SourceProofID:             sourceProofID,
		Level:                     level,
		SameOrigin:                sameOrigin,
		CookieStoreRelation:       cookieStoreRelation,
		AccountEvidenceRelation:   accountRelation,
		RequestCredentialRelation: credentialRelation,
		RefreshCheck:              refreshCheck,
		Reasons:                   reasons,
		CreatedAt:                 now,
		ExpiresAt:                 expiresAt,
	}
}

func minAuthorizationExpiry(values ...int64) int64 {
	result := time.Now().Add(extensionAuthorizationWorkspaceTTL).UnixMilli()
	for _, value := range values {
		if value > 0 && value < result {
			result = value
		}
	}
	return result
}

func newAuthorizationComparisonKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate authorization baseline comparison key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}

type authorizationCallResult struct {
	side string
	raw  json.RawMessage
	err  error
}

func (m *ExtensionBridgeManager) callAuthorizationPair(
	ctx context.Context,
	leftDeviceID string,
	leftMethod string,
	leftParams interface{},
	rightDeviceID string,
	rightMethod string,
	rightParams interface{},
) (json.RawMessage, json.RawMessage, error) {
	results := make(chan authorizationCallResult, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		raw, err := m.CallDevice(ctx, leftDeviceID, leftMethod, leftParams)
		results <- authorizationCallResult{side: "left", raw: raw, err: err}
	}()
	go func() {
		defer wait.Done()
		raw, err := m.CallDevice(ctx, rightDeviceID, rightMethod, rightParams)
		results <- authorizationCallResult{side: "right", raw: raw, err: err}
	}()
	wait.Wait()
	close(results)
	var leftRaw, rightRaw json.RawMessage
	for result := range results {
		if result.err != nil {
			return nil, nil, fmt.Errorf("%s authorization context: %w", result.side, result.err)
		}
		if result.side == "left" {
			leftRaw = result.raw
		} else {
			rightRaw = result.raw
		}
	}
	return leftRaw, rightRaw, nil
}

func (m *ExtensionBridgeManager) CreateExtensionAuthorizationWorkspace(
	ctx context.Context,
	input ExtensionAuthorizationWorkspaceInput,
) (ExtensionAuthorizationWorkspace, error) {
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode == "" {
		mode = "horizontal"
	}
	if mode != "horizontal" && mode != "vertical" {
		return ExtensionAuthorizationWorkspace{}, errors.New(
			"authorization workspace mode must be horizontal or vertical",
		)
	}
	leftInput, err := normalizeAuthorizationIdentity(input.Left)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	rightInput, err := normalizeAuthorizationIdentity(input.Right)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if leftInput.DeviceID == rightInput.DeviceID && leftInput.TabID == rightInput.TabID {
		return ExtensionAuthorizationWorkspace{}, errors.New("authorization identities cannot use the same browser tab")
	}
	snapshot := m.Snapshot()
	now := time.Now().UnixMilli()
	var leftSlot, rightSlot ExtensionAuthorizationIdentitySlot
	var proof ExtensionAuthorizationProof

	if leftInput.DeviceID == rightInput.DeviceID {
		device, _, deviceErr := authorizationDevice(
			snapshot,
			leftInput.DeviceID,
			"browser.isolation.proof",
			"browser.authorization.context.capture",
			"browser.authorization.context.get",
		)
		if deviceErr != nil {
			return ExtensionAuthorizationWorkspace{}, deviceErr
		}
		rawProof, callErr := m.CallDevice(ctx, leftInput.DeviceID, "browser.isolation.proof", map[string]interface{}{
			"leftTabId":  leftInput.TabID,
			"rightTabId": rightInput.TabID,
		})
		if callErr != nil {
			return ExtensionAuthorizationWorkspace{}, fmt.Errorf("create extension isolation proof: %w", callErr)
		}
		var sourceProof extensionAuthorizationIsolationProof
		if err := decodeAuthorizationResult(rawProof, &sourceProof); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		leftRaw, rightRaw, callErr := m.callAuthorizationPair(
			ctx,
			leftInput.DeviceID,
			"browser.authorization.context.capture",
			map[string]interface{}{
				"tabId": leftInput.TabID, "frameId": leftInput.FrameID,
				"isolationProofId": sourceProof.ID, "slotId": "left", "accountLabel": leftInput.AccountLabel,
			},
			rightInput.DeviceID,
			"browser.authorization.context.capture",
			map[string]interface{}{
				"tabId": rightInput.TabID, "frameId": rightInput.FrameID,
				"isolationProofId": sourceProof.ID, "slotId": "right", "accountLabel": rightInput.AccountLabel,
			},
		)
		if callErr != nil {
			return ExtensionAuthorizationWorkspace{}, callErr
		}
		var leftHandle, rightHandle extensionAuthorizationHandle
		if err := decodeAuthorizationResult(leftRaw, &leftHandle); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		if err := decodeAuthorizationResult(rightRaw, &rightHandle); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		if leftHandle.SlotID != "left" || rightHandle.SlotID != "right" ||
			leftHandle.IsolationProofID != sourceProof.ID || rightHandle.IsolationProofID != sourceProof.ID {
			return ExtensionAuthorizationWorkspace{}, errors.New("extension authorization handles do not match the isolation proof")
		}
		if err := validateAuthorizationContext(leftHandle.extensionAuthorizationContextBase, device, leftInput); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		if err := validateAuthorizationContext(rightHandle.extensionAuthorizationContextBase, device, rightInput); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		leftSlot = authorizationSlotFromContext(
			"left", leftInput.AccountLabel, "handle", leftHandle.extensionAuthorizationContextBase,
		)
		rightSlot = authorizationSlotFromContext(
			"right", rightInput.AccountLabel, "handle", rightHandle.extensionAuthorizationContextBase,
		)
		expiresAt := minAuthorizationExpiry(
			sourceProof.ExpiresAt,
			leftHandle.ExpiresAt,
			rightHandle.ExpiresAt,
		)
		proof = evaluateAuthorizationProof(
			"extension-cookie-store",
			sourceProof.ID,
			sourceProof.Level,
			sourceProof.CookieStoreRelation,
			sourceProof.RefreshCheck,
			sourceProof.Reasons,
			leftSlot,
			rightSlot,
			now,
			expiresAt,
		)
	} else {
		leftDevice, _, leftErr := authorizationDevice(
			snapshot,
			leftInput.DeviceID,
			"browser.authorization.context.attest",
			"browser.authorization.context.attestation.get",
		)
		if leftErr != nil {
			return ExtensionAuthorizationWorkspace{}, leftErr
		}
		rightDevice, _, rightErr := authorizationDevice(
			snapshot,
			rightInput.DeviceID,
			"browser.authorization.context.attest",
			"browser.authorization.context.attestation.get",
		)
		if rightErr != nil {
			return ExtensionAuthorizationWorkspace{}, rightErr
		}
		if leftDevice.InstallationID == rightDevice.InstallationID {
			return ExtensionAuthorizationWorkspace{}, errors.New(
				"different device records share the same installation identity; isolation is not proven",
			)
		}
		leftRaw, rightRaw, callErr := m.callAuthorizationPair(
			ctx,
			leftInput.DeviceID,
			"browser.authorization.context.attest",
			map[string]interface{}{"tabId": leftInput.TabID, "frameId": leftInput.FrameID},
			rightInput.DeviceID,
			"browser.authorization.context.attest",
			map[string]interface{}{"tabId": rightInput.TabID, "frameId": rightInput.FrameID},
		)
		if callErr != nil {
			return ExtensionAuthorizationWorkspace{}, callErr
		}
		var leftAttestation, rightAttestation extensionAuthorizationAttestation
		if err := decodeAuthorizationResult(leftRaw, &leftAttestation); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		if err := decodeAuthorizationResult(rightRaw, &rightAttestation); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		if err := validateAuthorizationContext(leftAttestation.extensionAuthorizationContextBase, leftDevice, leftInput); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		if err := validateAuthorizationContext(rightAttestation.extensionAuthorizationContextBase, rightDevice, rightInput); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		leftSlot = authorizationSlotFromContext(
			"left", leftInput.AccountLabel, "attestation", leftAttestation.extensionAuthorizationContextBase,
		)
		rightSlot = authorizationSlotFromContext(
			"right", rightInput.AccountLabel, "attestation", rightAttestation.extensionAuthorizationContextBase,
		)
		expiresAt := minAuthorizationExpiry(leftAttestation.ExpiresAt, rightAttestation.ExpiresAt)
		proof = evaluateAuthorizationProof(
			"separate-installations",
			"",
			"strong",
			"unknown",
			"not-required",
			[]string{"两个身份来自不同的已配对插件安装与浏览器 Profile"},
			leftSlot,
			rightSlot,
			now,
			expiresAt,
		)
	}

	workspace := ExtensionAuthorizationWorkspace{
		Version:          1,
		ID:               newExtensionAuthorizationWorkspaceID(m.engineInstanceID),
		EngineInstanceID: m.engineInstanceID,
		Mode:             mode,
		State:            authorizationWorkspaceState(proof.Level),
		Left:             leftSlot,
		Right:            rightSlot,
		Proof:            proof,
		BaselinePair: ExtensionAuthorizationBaselinePair{
			State: "waiting",
			Reasons: func() []string {
				if mode == "vertical" {
					return []string{"为低权限身份 A 选择正常控制请求，并为高权限身份 B 选择待验证的特权操作"}
				}
				return []string{"分别为身份 A 与身份 B 选择语义相同的正常请求"}
			}(),
			ResourceCandidates:  []ExtensionAuthorizationResourceCandidate{},
			OperationCandidates: []ExtensionAuthorizationOperationCandidate{},
		},
		CreatedAt: now,
		ExpiresAt: proof.ExpiresAt,
	}
	workspace.comparisonKey, err = newAuthorizationComparisonKey()
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	m.insertExtensionAuthorizationWorkspace(workspace)
	return workspace, nil
}
