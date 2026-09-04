package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const extensionAuthorizationWorkspaceIDPrefix = "authorization-workspace"

func newExtensionAuthorizationWorkspaceID(engineInstanceID string) string {
	encodedInstance := base64.RawURLEncoding.EncodeToString([]byte(engineInstanceID))
	return extensionAuthorizationWorkspaceIDPrefix + "." + encodedInstance + "." + uuid.NewString()
}

func extensionAuthorizationWorkspaceEngineInstance(id string) (string, bool) {
	parts := strings.Split(id, ".")
	if len(parts) != 3 || parts[0] != extensionAuthorizationWorkspaceIDPrefix || parts[1] == "" || parts[2] == "" {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(decoded) == 0 || len(decoded) > 256 {
		return "", false
	}
	return string(decoded), true
}

func (m *ExtensionBridgeManager) rememberExtensionAuthorizationWorkspaceLifecycleLocked(
	workspace ExtensionAuthorizationWorkspace,
	reason ExtensionAuthorizationWorkspaceLifecycleReason,
	replacementWorkspaceID string,
	now int64,
) {
	if m.authorizationTombstones == nil {
		m.authorizationTombstones = make(map[string]extensionAuthorizationWorkspaceTombstone)
	}
	engineInstanceID := workspace.EngineInstanceID
	if engineInstanceID == "" {
		engineInstanceID = m.engineInstanceID
	}
	m.authorizationTombstones[workspace.ID] = extensionAuthorizationWorkspaceTombstone{
		Reason:                 reason,
		WorkspaceID:            workspace.ID,
		EngineInstanceID:       engineInstanceID,
		ExpiresAt:              workspace.ExpiresAt,
		ReplacementWorkspaceID: replacementWorkspaceID,
		RemovedAt:              now,
	}
}

func (m *ExtensionBridgeManager) cleanupExtensionAuthorizationTombstonesLocked(now int64) {
	for id, tombstone := range m.authorizationTombstones {
		if tombstone.RemovedAt <= now-extensionAuthorizationTombstoneTTL.Milliseconds() {
			delete(m.authorizationTombstones, id)
		}
	}
	for len(m.authorizationTombstones) > maxAuthorizationWorkspaceTombstones {
		var oldestID string
		var oldestAt int64
		for id, tombstone := range m.authorizationTombstones {
			if oldestID == "" || tombstone.RemovedAt < oldestAt ||
				(tombstone.RemovedAt == oldestAt && id < oldestID) {
				oldestID = id
				oldestAt = tombstone.RemovedAt
			}
		}
		if oldestID == "" {
			break
		}
		delete(m.authorizationTombstones, oldestID)
	}
}

func (m *ExtensionBridgeManager) cleanupExpiredExtensionAuthorizationWorkspacesLocked(now int64) {
	for id, workspace := range m.authorization {
		if workspace.ExpiresAt > now {
			continue
		}
		delete(m.authorization, id)
		m.rememberExtensionAuthorizationWorkspaceLifecycleLocked(
			workspace,
			ExtensionAuthorizationWorkspaceExpired,
			"",
			now,
		)
	}
	m.cleanupExtensionAuthorizationTombstonesLocked(now)
}

func sameExtensionAuthorizationWorkspaceIdentityPair(
	left ExtensionAuthorizationWorkspace,
	right ExtensionAuthorizationWorkspace,
) bool {
	if left.Mode != right.Mode {
		return false
	}
	sameSlot := func(a, b ExtensionAuthorizationIdentitySlot) bool {
		return a.DeviceID == b.DeviceID &&
			a.InstallationID == b.InstallationID &&
			a.IsolationContextID == b.IsolationContextID &&
			a.CookieStoreID == b.CookieStoreID &&
			a.Origin == b.Origin &&
			a.Target.TabID == b.Target.TabID &&
			a.Target.FrameID == b.Target.FrameID
	}
	return sameSlot(left.Left, right.Left) && sameSlot(left.Right, right.Right)
}

func (m *ExtensionBridgeManager) evictExtensionAuthorizationWorkspaceForCapacityLocked(now int64) {
	for len(m.authorization) >= maxExtensionAuthorizationWorkspaces {
		var oldestID string
		var oldestAt int64
		for id, workspace := range m.authorization {
			if oldestID == "" || workspace.CreatedAt < oldestAt ||
				(workspace.CreatedAt == oldestAt && id < oldestID) {
				oldestID = id
				oldestAt = workspace.CreatedAt
			}
		}
		if oldestID == "" {
			break
		}
		workspace := m.authorization[oldestID]
		delete(m.authorization, oldestID)
		m.rememberExtensionAuthorizationWorkspaceLifecycleLocked(
			workspace,
			ExtensionAuthorizationWorkspaceEvicted,
			"",
			now,
		)
	}
}

func (m *ExtensionBridgeManager) insertExtensionAuthorizationWorkspace(
	workspace ExtensionAuthorizationWorkspace,
) {
	m.authorizationMu.Lock()
	defer m.authorizationMu.Unlock()
	if m.authorization == nil {
		m.authorization = make(map[string]ExtensionAuthorizationWorkspace)
	}
	now := time.Now().UnixMilli()
	m.cleanupExpiredExtensionAuthorizationWorkspacesLocked(now)
	workspace.EngineInstanceID = m.engineInstanceID
	if _, updating := m.authorization[workspace.ID]; !updating {
		for id, candidate := range m.authorization {
			if id == workspace.ID || !sameExtensionAuthorizationWorkspaceIdentityPair(candidate, workspace) {
				continue
			}
			delete(m.authorization, id)
			m.rememberExtensionAuthorizationWorkspaceLifecycleLocked(
				candidate,
				ExtensionAuthorizationWorkspaceReplaced,
				workspace.ID,
				now,
			)
		}
		m.evictExtensionAuthorizationWorkspaceForCapacityLocked(now)
	}
	delete(m.authorizationTombstones, workspace.ID)
	m.authorization[workspace.ID] = workspace
	m.cleanupExtensionAuthorizationTombstonesLocked(now)
}

func (m *ExtensionBridgeManager) extensionAuthorizationWorkspaceLifecycleErrorLocked(
	id string,
) *ExtensionAuthorizationWorkspaceLifecycleError {
	if tombstone, ok := m.authorizationTombstones[id]; ok {
		return &ExtensionAuthorizationWorkspaceLifecycleError{
			Reason:                 tombstone.Reason,
			WorkspaceID:            tombstone.WorkspaceID,
			EngineInstanceID:       tombstone.EngineInstanceID,
			ExpiresAt:              tombstone.ExpiresAt,
			ReplacementWorkspaceID: tombstone.ReplacementWorkspaceID,
		}
	}
	if engineInstanceID, encoded := extensionAuthorizationWorkspaceEngineInstance(id); encoded && engineInstanceID != m.engineInstanceID {
		return &ExtensionAuthorizationWorkspaceLifecycleError{
			Reason:           ExtensionAuthorizationWorkspaceEngineInstanceChanged,
			WorkspaceID:      id,
			EngineInstanceID: m.engineInstanceID,
		}
	}
	return &ExtensionAuthorizationWorkspaceLifecycleError{
		Reason:           ExtensionAuthorizationWorkspaceNotFound,
		WorkspaceID:      id,
		EngineInstanceID: m.engineInstanceID,
	}
}

func (m *ExtensionBridgeManager) updateExtensionAuthorizationWorkspace(
	workspace ExtensionAuthorizationWorkspace,
) error {
	m.authorizationMu.Lock()
	defer m.authorizationMu.Unlock()
	now := time.Now().UnixMilli()
	m.cleanupExpiredExtensionAuthorizationWorkspacesLocked(now)
	current, ok := m.authorization[workspace.ID]
	if !ok {
		return m.extensionAuthorizationWorkspaceLifecycleErrorLocked(workspace.ID)
	}
	workspace.EngineInstanceID = current.EngineInstanceID
	m.authorization[workspace.ID] = workspace
	return nil
}

func authorizationContextMatches(
	slot ExtensionAuthorizationIdentitySlot,
	context extensionAuthorizationContextBase,
) bool {
	return slot.DeviceID == context.DeviceID &&
		slot.InstallationID == context.InstallationID &&
		slot.IsolationContextID == context.IsolationContextID &&
		slot.CookieStoreID == context.CookieStoreID &&
		slot.Origin == context.Origin &&
		slot.GrantID == context.GrantID &&
		slot.Target == context.Target &&
		slot.Fingerprint == context.Fingerprint &&
		slot.ContextReference.ID == context.ID
}

func authorizationContextBoundaryMatches(
	slot ExtensionAuthorizationIdentitySlot,
	context extensionAuthorizationContextBase,
) bool {
	return slot.DeviceID == context.DeviceID &&
		slot.InstallationID == context.InstallationID &&
		slot.IsolationContextID == context.IsolationContextID &&
		slot.CookieStoreID == context.CookieStoreID &&
		slot.Origin == context.Origin &&
		slot.GrantID == context.GrantID &&
		slot.Target.TabID == context.Target.TabID &&
		slot.Target.FrameID == context.Target.FrameID
}

func (m *ExtensionBridgeManager) revalidateExtensionAuthorizationWorkspace(
	ctx context.Context,
	workspace ExtensionAuthorizationWorkspace,
) (ExtensionAuthorizationWorkspace, error) {
	type validationReference struct {
		method   string
		id       string
		kind     string
		slot     ExtensionAuthorizationIdentitySlot
		baseline *ExtensionAuthorizationBaseline
	}
	methodFor := func(reference ExtensionAuthorizationContextReference) (string, error) {
		switch reference.Kind {
		case "handle":
			return "browser.authorization.context.get", nil
		case "attestation":
			return "browser.authorization.context.attest", nil
		default:
			return "", errors.New("authorization workspace contains an unknown context reference")
		}
	}
	referenceFor := func(
		slot ExtensionAuthorizationIdentitySlot,
		baseline *ExtensionAuthorizationBaseline,
	) (validationReference, error) {
		if baseline != nil {
			return validationReference{
				method:   "browser.authorization.baseline.get",
				id:       baseline.ID,
				kind:     "baseline",
				slot:     slot,
				baseline: baseline,
			}, nil
		}
		method, err := methodFor(slot.ContextReference)
		return validationReference{
			method: method,
			id:     slot.ContextReference.ID,
			kind:   slot.ContextReference.Kind,
			slot:   slot,
		}, err
	}
	leftReference, err := referenceFor(workspace.Left, workspace.Baselines.Left)
	if err != nil {
		return workspace, err
	}
	rightReference, err := referenceFor(workspace.Right, workspace.Baselines.Right)
	if err != nil {
		return workspace, err
	}
	paramsFor := func(reference validationReference) map[string]interface{} {
		if reference.kind == "attestation" {
			return map[string]interface{}{
				"tabId": reference.slot.Target.TabID, "frameId": reference.slot.Target.FrameID,
			}
		}
		return map[string]interface{}{"id": reference.id}
	}
	leftRaw, rightRaw, err := m.callAuthorizationPair(
		ctx,
		workspace.Left.DeviceID,
		leftReference.method,
		paramsFor(leftReference),
		workspace.Right.DeviceID,
		rightReference.method,
		paramsFor(rightReference),
	)
	if err != nil {
		return workspace, err
	}
	type validationOutcome struct {
		baseline       *ExtensionAuthorizationBaseline
		slot           *ExtensionAuthorizationIdentitySlot
		logicalCleared bool
	}
	validateResult := func(
		reference validationReference,
		raw json.RawMessage,
	) (validationOutcome, error) {
		if reference.kind == "baseline" {
			var current ExtensionAuthorizationBaseline
			if err := decodeAuthorizationResult(raw, &current); err != nil {
				return validationOutcome{}, err
			}
			if err := validateAuthorizationBaseline(current, reference.slot); err != nil {
				return validationOutcome{}, err
			}
			reconciled, logicalCleared, err := reconcileAuthorizationBaselineRefresh(
				*reference.baseline,
				current,
			)
			if err != nil {
				return validationOutcome{}, err
			}
			return validationOutcome{
				baseline:       &reconciled,
				logicalCleared: logicalCleared,
			}, nil
		}
		if reference.kind == "handle" {
			var handle extensionAuthorizationHandle
			if err := decodeAuthorizationResult(raw, &handle); err != nil {
				return validationOutcome{}, err
			}
			if !authorizationContextMatches(reference.slot, handle.extensionAuthorizationContextBase) {
				return validationOutcome{}, errors.New("authorization identity context changed")
			}
			return validationOutcome{}, nil
		}
		var attestation extensionAuthorizationAttestation
		if err := decodeAuthorizationResult(raw, &attestation); err != nil {
			return validationOutcome{}, err
		}
		if !authorizationContextBoundaryMatches(reference.slot, attestation.extensionAuthorizationContextBase) {
			return validationOutcome{}, errors.New("authorization identity context changed")
		}
		refreshed := authorizationSlotFromContext(
			reference.slot.Side,
			reference.slot.AccountLabel,
			"attestation",
			attestation.extensionAuthorizationContextBase,
		)
		return validationOutcome{slot: &refreshed}, nil
	}
	leftOutcome, err := validateResult(leftReference, leftRaw)
	if err != nil {
		return workspace, err
	}
	rightOutcome, err := validateResult(rightReference, rightRaw)
	if err != nil {
		return workspace, err
	}
	if leftOutcome.slot != nil || rightOutcome.slot != nil {
		if leftOutcome.slot != nil {
			workspace.Left = *leftOutcome.slot
		}
		if rightOutcome.slot != nil {
			workspace.Right = *rightOutcome.slot
		}
		workspace.Proof = evaluateAuthorizationProof(
			"separate-installations",
			"",
			"strong",
			"unknown",
			"not-required",
			[]string{"两个身份来自不同的已配对插件安装与浏览器 Profile"},
			workspace.Left,
			workspace.Right,
			time.Now().UnixMilli(),
			minAuthorizationExpiry(workspace.ExpiresAt, workspace.Left.ExpiresAt, workspace.Right.ExpiresAt),
		)
		workspace.State = authorizationWorkspaceState(workspace.Proof.Level)
		workspace.StaleReason = ""
		workspace.Recovery = nil
		if err := m.updateExtensionAuthorizationWorkspace(workspace); err != nil {
			return workspace, err
		}
	}
	if workspace.Baselines.Verification != nil {
		verificationReference, err := referenceFor(
			workspace.Right,
			workspace.Baselines.Verification,
		)
		if err != nil {
			return workspace, err
		}
		verificationRaw, err := m.CallDevice(
			ctx,
			workspace.Right.DeviceID,
			verificationReference.method,
			map[string]interface{}{"id": verificationReference.id},
		)
		if err != nil {
			return workspace, err
		}
		verificationOutcome, err := validateResult(
			verificationReference,
			verificationRaw,
		)
		if err != nil {
			return workspace, err
		}
		if err := validateVerticalAuthorizationVerificationBaseline(
			verificationOutcome.baseline,
		); err != nil {
			return workspace, err
		}
		workspace.Baselines.Verification = verificationOutcome.baseline
	}
	if leftOutcome.logicalCleared || rightOutcome.logicalCleared {
		if workspace.Baselines.Left == nil || workspace.Baselines.Right == nil {
			return workspace, errors.New(
				"authorization logical binding changed without a complete A/B baseline pair",
			)
		}
		left := *workspace.Baselines.Left
		right := *workspace.Baselines.Right
		if leftOutcome.baseline != nil {
			left = *leftOutcome.baseline
		}
		if rightOutcome.baseline != nil {
			right = *rightOutcome.baseline
		}
		left.LogicalRequest = nil
		right.LogicalRequest = nil
		workspace.Baselines.Left = &left
		workspace.Baselines.Right = &right
		workspace.BaselinePair = inferAuthorizationBaselinePairForMode(
			workspace.Mode,
			&left,
			&right,
		)
		workspace.BaselinePair.Reasons = appendAuthorizationReason(
			workspace.BaselinePair.Reasons,
			"明文网关、回放草稿或页面绑定已变化；线上 A/B 基线仍有效，请重新绑定逻辑明文",
		)
		workspace.Plan = nil
		workspace.Execution = nil
		if err := m.updateExtensionAuthorizationWorkspace(workspace); err != nil {
			return workspace, err
		}
	}
	return workspace, nil
}

func (m *ExtensionBridgeManager) GetExtensionAuthorizationWorkspace(
	ctx context.Context,
	id string,
	revalidate bool,
) (ExtensionAuthorizationWorkspace, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ExtensionAuthorizationWorkspace{}, errors.New("authorization workspace id is required")
	}
	m.authorizationMu.Lock()
	now := time.Now().UnixMilli()
	m.cleanupExpiredExtensionAuthorizationWorkspacesLocked(now)
	workspace, ok := m.authorization[id]
	lifecycleError := m.extensionAuthorizationWorkspaceLifecycleErrorLocked(id)
	m.authorizationMu.Unlock()
	if !ok {
		return ExtensionAuthorizationWorkspace{}, lifecycleError
	}
	if !revalidate || workspace.State == "stale" {
		return workspace, nil
	}
	validated, err := m.revalidateExtensionAuthorizationWorkspace(ctx, workspace)
	if err == nil {
		return validated, nil
	}
	var lifecycle *ExtensionAuthorizationWorkspaceLifecycleError
	if errors.As(err, &lifecycle) {
		return ExtensionAuthorizationWorkspace{}, lifecycle
	}
	workspace.State = "stale"
	workspace.Recovery = extensionAuthorizationRecoveryForError(err)
	workspace.StaleReason = workspace.Recovery.Message
	workspace.Proof.Level = "none"
	workspace.Proof.Reasons = append(
		workspace.Proof.Reasons,
		"实时复核失败："+workspace.Recovery.Message,
	)
	if err := m.updateExtensionAuthorizationWorkspace(workspace); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	return workspace, nil
}
