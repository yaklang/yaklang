package browser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

func validateAuthorizationBaselineRequestMetadata(
	request ExtensionAuthorizationBaselineRequest,
) error {
	if request.Method == "" ||
		len(request.Method) > 32 ||
		request.URL == "" ||
		request.Path == "" ||
		!validAuthorizationFingerprint(request.ActionFingerprint, "sha256:") ||
		len(request.ContentType) > 512 ||
		len(request.URL) > 8192 ||
		len(request.Path) > 8192 ||
		len(request.HeaderNames) > 256 ||
		len(request.Fields) > 300 {
		return errors.New("browser authorization baseline request metadata is invalid")
	}
	switch request.Protocol {
	case "":
		if request.OperationFingerprint != "" || len(request.OperationNames) != 0 {
			return errors.New("browser authorization baseline protocol metadata is incomplete")
		}
	case "graphql":
		if !validAuthorizationFingerprint(request.OperationFingerprint, "sha256:") ||
			len(request.OperationNames) == 0 ||
			len(request.OperationNames) > 16 {
			return errors.New("browser authorization GraphQL operation metadata is invalid")
		}
		for _, name := range request.OperationNames {
			if len([]rune(name)) == 0 ||
				len([]rune(name)) > 128 ||
				(!authorizationGraphQLNamePattern.MatchString(name) &&
					!authorizationGraphQLFallbackNamePattern.MatchString(name)) {
				return errors.New("browser authorization GraphQL operation name is invalid")
			}
			for _, character := range name {
				if character < 0x20 || character == 0x7f {
					return errors.New("browser authorization GraphQL operation name is invalid")
				}
			}
		}
	default:
		return errors.New("browser authorization baseline protocol is unsupported")
	}
	for _, character := range request.Method {
		if character < 'A' || character > 'Z' {
			return errors.New("browser authorization baseline request method is invalid")
		}
	}
	for _, field := range request.Fields {
		if field.Path == "" ||
			len(field.Path) > 512 ||
			field.ByteLength < 0 ||
			field.ByteLength > 2*1024*1024 ||
			!validAuthorizationFingerprint(field.ValueFingerprint, "workspace-hmac-sha256:") {
			return errors.New("browser authorization baseline field metadata is invalid")
		}
		switch field.Location {
		case "header", "path", "query", "body":
		default:
			return errors.New("browser authorization baseline field location is invalid")
		}
		switch field.Category {
		case "authentication", "csrf", "signature", "nonce", "timestamp", "resource", "unknown":
		default:
			return errors.New("browser authorization baseline field category is invalid")
		}
		switch field.ValueType {
		case "string", "number", "boolean", "null", "binary":
		default:
			return errors.New("browser authorization baseline field type is invalid")
		}
	}
	return nil
}

func validateAuthorizationLogicalRequestBinding(
	binding *ExtensionAuthorizationLogicalRequestBinding,
	baseline ExtensionAuthorizationBaseline,
) error {
	if binding == nil {
		return nil
	}
	if binding.Version != 1 ||
		binding.Source != "local-replay-draft" ||
		binding.BaselineID != baseline.ID ||
		binding.IsolationContextID != baseline.IsolationContextID ||
		binding.CookieStoreID != baseline.CookieStoreID ||
		binding.Target != baseline.Target ||
		binding.Origin != baseline.Origin ||
		binding.ProfileID == "" ||
		len([]rune(binding.ProfileName)) == 0 ||
		len([]rune(binding.ProfileName)) > 120 ||
		binding.ProfileUpdatedAt <= 0 ||
		binding.ReplayUpdatedAt <= 0 ||
		binding.CreatedAt <= 0 ||
		binding.ExpiresAt != baseline.ExpiresAt ||
		!validAuthorizationFingerprint(binding.BindingFingerprint, "sha256:") {
		return errors.New("browser authorization logical request binding identity is invalid")
	}
	if err := validateAuthorizationBaselineRequestMetadata(binding.Request); err != nil {
		return fmt.Errorf("browser authorization logical request: %w", err)
	}
	if binding.Validation.ProofLevel != "structure" ||
		len(binding.Validation.Summary) > 500 ||
		len(binding.Validation.Warnings) > 16 {
		return errors.New("browser authorization logical request validation proof is invalid")
	}
	for _, warning := range binding.Validation.Warnings {
		if len([]rune(warning)) > 200 {
			return errors.New("browser authorization logical request validation warning is invalid")
		}
	}
	if len(binding.OutputDestinations) == 0 || len(binding.OutputDestinations) > 32 {
		return errors.New("browser authorization logical request outputs are invalid")
	}
	seen := make(map[string]struct{}, len(binding.OutputDestinations))
	for _, destination := range binding.OutputDestinations {
		lower := strings.ToLower(destination)
		if len(destination) > 512 ||
			(destination != "body" &&
				!strings.HasPrefix(destination, "body.") &&
				!strings.HasPrefix(lower, "header.") &&
				!strings.HasPrefix(destination, "query.")) ||
			lower == "header.authorization" ||
			lower == "header.cookie" ||
			lower == "header.host" ||
			lower == "header.proxy-authorization" {
			return errors.New("browser authorization logical request contains an unsupported output")
		}
		if _, ok := seen[destination]; ok {
			return errors.New("browser authorization logical request contains duplicate outputs")
		}
		seen[destination] = struct{}{}
	}
	return nil
}

func validateAuthorizationBaseline(
	baseline ExtensionAuthorizationBaseline,
	slot ExtensionAuthorizationIdentitySlot,
) error {
	if baseline.Version != 1 || baseline.ID == "" || baseline.NetworkRequestID == "" {
		return errors.New("browser authorization baseline identity is incomplete")
	}
	if baseline.DeviceID != slot.DeviceID ||
		baseline.InstallationID != slot.InstallationID ||
		baseline.IsolationContextID != slot.IsolationContextID ||
		baseline.CookieStoreID != slot.CookieStoreID ||
		baseline.Origin != slot.Origin ||
		baseline.GrantID != slot.GrantID ||
		baseline.Target != slot.Target ||
		baseline.AuthContextReference != slot.ContextReference {
		return errors.New("browser authorization baseline does not match the selected identity slot")
	}
	if err := validateAuthorizationBaselineRequestMetadata(baseline.Request); err != nil {
		return err
	}
	if baseline.ExpiresAt <= time.Now().UnixMilli() || baseline.ExpiresAt > slot.ExpiresAt {
		return errors.New("browser authorization baseline expiry is invalid")
	}
	return validateAuthorizationLogicalRequestBinding(baseline.LogicalRequest, baseline)
}

func authorizationFieldKey(source string, field ExtensionAuthorizationBaselineField) string {
	return source + "\x00" + field.Location + "\x00" + field.Path
}

func authorizationResourceCandidateID(
	actionFingerprint string,
	source string,
	location string,
	path string,
) string {
	sum := sha256.Sum256([]byte(actionFingerprint + "\x00" + source + "\x00" + location + "\x00" + path))
	return "authorization-resource-" + hex.EncodeToString(sum[:])
}

func authorizationOperationCandidateID(
	actionFingerprint string,
	templateSide string,
	authContextSide string,
) string {
	sum := sha256.Sum256([]byte(
		actionFingerprint + "\x00" + templateSide + "\x00" + authContextSide,
	))
	return "authorization-operation-" + hex.EncodeToString(sum[:])
}

func authorizationStructuredBodyContentType(contentType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return strings.Contains(normalized, "json") ||
		normalized == "application/x-www-form-urlencoded"
}

func authorizationPrimitiveResourceType(valueType string) bool {
	return valueType == "string" || valueType == "number" || valueType == "boolean"
}

func inferAuthorizationBaselinePair(
	left *ExtensionAuthorizationBaseline,
	right *ExtensionAuthorizationBaseline,
) ExtensionAuthorizationBaselinePair {
	if left == nil || right == nil {
		return ExtensionAuthorizationBaselinePair{
			State:              "waiting",
			Reasons:            []string{"还需要为另一个身份选择正常请求"},
			ResourceCandidates: []ExtensionAuthorizationResourceCandidate{},
		}
	}
	if left.Request.ActionFingerprint != right.Request.ActionFingerprint {
		return ExtensionAuthorizationBaselinePair{
			State: "mismatch",
			Reasons: []string{
				"A/B 请求的方法、路径、Content-Type 或字段结构不同，不能直接构造交叉矩阵",
			},
			ResourceCandidates: []ExtensionAuthorizationResourceCandidate{},
		}
	}
	candidates := make([]ExtensionAuthorizationResourceCandidate, 0)
	appendCandidates := func(
		source string,
		actionFingerprint string,
		leftFields []ExtensionAuthorizationBaselineField,
		rightFieldsInput []ExtensionAuthorizationBaselineField,
		logicalBodyOnly bool,
		leftContentType string,
		rightContentType string,
	) {
		rightFields := make(map[string]ExtensionAuthorizationBaselineField, len(rightFieldsInput))
		for _, field := range rightFieldsInput {
			rightFields[authorizationFieldKey(source, field)] = field
		}
		for _, leftField := range leftFields {
			rightField, ok := rightFields[authorizationFieldKey(source, leftField)]
			if !ok || leftField.ValueFingerprint == rightField.ValueFingerprint {
				continue
			}
			if leftField.Category == "authentication" ||
				leftField.Category == "csrf" ||
				leftField.Category == "signature" ||
				leftField.Category == "nonce" ||
				leftField.Category == "timestamp" ||
				(leftField.Location == "header" &&
					leftField.Category != "resource" &&
					rightField.Category != "resource") ||
				leftField.Path == "body" ||
				leftField.ValueType != rightField.ValueType ||
				!authorizationPrimitiveResourceType(leftField.ValueType) ||
				(logicalBodyOnly && leftField.Location != "body") {
				continue
			}
			confidence := "medium"
			reasons := []string{"A/B 正常请求在同一字段位置使用不同值"}
			if source == "logical" {
				reasons = []string{"A/B 已验证明文网关在同一逻辑字段使用不同值"}
			}
			if leftField.Category == "resource" || rightField.Category == "resource" {
				confidence = "high"
				reasons = append(reasons, "字段名具有资源标识语义")
			}
			requiresLogicalBinding := source == "wire" &&
				leftField.Location == "body" &&
				(!authorizationStructuredBodyContentType(leftContentType) ||
					!authorizationStructuredBodyContentType(rightContentType) ||
					leftField.Category != "resource" ||
					rightField.Category != "resource")
			if source == "wire" && leftField.Location == "body" {
				if requiresLogicalBinding {
					reasons = append(
						reasons,
						"线上 Body 不是双方均确认的结构化资源字段，必须先绑定逻辑明文",
					)
				} else {
					reasons = append(
						reasons,
						"JSON/Form 中双方均确认、类型一致的原始资源值可进行确定性结构化替换",
					)
				}
			}
			candidates = append(candidates, ExtensionAuthorizationResourceCandidate{
				ID: authorizationResourceCandidateID(
					actionFingerprint,
					source,
					leftField.Location,
					leftField.Path,
				),
				Source:                 source,
				Location:               leftField.Location,
				Path:                   leftField.Path,
				Category:               leftField.Category,
				Confidence:             confidence,
				RequiresLogicalBinding: requiresLogicalBinding,
				Reasons:                reasons,
			})
		}
	}
	appendCandidates(
		"wire",
		left.Request.ActionFingerprint,
		left.Request.Fields,
		right.Request.Fields,
		false,
		left.Request.ContentType,
		right.Request.ContentType,
	)
	if left.LogicalRequest != nil || right.LogicalRequest != nil {
		if left.LogicalRequest == nil || right.LogicalRequest == nil {
			return ExtensionAuthorizationBaselinePair{
				State: "mismatch",
				Reasons: []string{
					"A/B 只有一侧绑定了逻辑明文请求，不能构造加密 Body 交叉矩阵",
				},
				ResourceCandidates: []ExtensionAuthorizationResourceCandidate{},
			}
		}
		if left.LogicalRequest.Request.ActionFingerprint != right.LogicalRequest.Request.ActionFingerprint {
			return ExtensionAuthorizationBaselinePair{
				State: "mismatch",
				Reasons: []string{
					"A/B 逻辑明文请求的方法、路径、Content-Type 或字段结构不同",
				},
				ResourceCandidates: []ExtensionAuthorizationResourceCandidate{},
			}
		}
		appendCandidates(
			"logical",
			left.LogicalRequest.Request.ActionFingerprint,
			left.LogicalRequest.Request.Fields,
			right.LogicalRequest.Request.Fields,
			true,
			left.LogicalRequest.Request.ContentType,
			right.LogicalRequest.Request.ContentType,
		)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Confidence != candidates[j].Confidence {
			return candidates[i].Confidence == "high"
		}
		if candidates[i].Source != candidates[j].Source {
			return candidates[i].Source == "logical"
		}
		if candidates[i].RequiresLogicalBinding != candidates[j].RequiresLogicalBinding {
			return !candidates[i].RequiresLogicalBinding
		}
		if candidates[i].Location != candidates[j].Location {
			return candidates[i].Location < candidates[j].Location
		}
		return candidates[i].Path < candidates[j].Path
	})
	if len(candidates) > 20 {
		candidates = candidates[:20]
	}
	reasons := []string{"A/B 线上请求具有相同业务结构"}
	if left.LogicalRequest != nil && right.LogicalRequest != nil {
		reasons = append(reasons, "A/B 逻辑明文与线上密文的结构绑定均已验证")
	}
	if len(candidates) == 0 {
		reasons = append(reasons, "尚未发现可用于交叉测试的差异资源字段")
	} else {
		reasons = append(reasons, fmt.Sprintf("发现 %d 个非认证差异字段候选", len(candidates)))
	}
	return ExtensionAuthorizationBaselinePair{
		State:              "matched",
		ActionFingerprint:  left.Request.ActionFingerprint,
		Reasons:            reasons,
		ResourceCandidates: candidates,
	}
}

func inferVerticalAuthorizationBaselinePair(
	left *ExtensionAuthorizationBaseline,
	right *ExtensionAuthorizationBaseline,
) ExtensionAuthorizationBaselinePair {
	if left == nil || right == nil {
		return ExtensionAuthorizationBaselinePair{
			State: "waiting",
			Reasons: []string{
				"还需要低权限身份 A 的正常控制请求和高权限身份 B 的特权操作",
			},
			ResourceCandidates:  []ExtensionAuthorizationResourceCandidate{},
			OperationCandidates: []ExtensionAuthorizationOperationCandidate{},
		}
	}
	leftAuth := make(map[string]ExtensionAuthorizationBaselineField)
	for _, field := range left.Request.Fields {
		if field.Category != "authentication" && field.Category != "csrf" {
			continue
		}
		leftAuth[authorizationFieldKey("wire", field)] = field
	}
	authenticationPaths := make([]string, 0)
	missingAuthPaths := make([]string, 0)
	dynamicPaths := make([]string, 0)
	for _, field := range right.Request.Fields {
		switch field.Category {
		case "authentication", "csrf":
			authenticationPaths = append(authenticationPaths, field.Path)
			source, ok := leftAuth[authorizationFieldKey("wire", field)]
			if !ok || source.ValueType != field.ValueType {
				missingAuthPaths = append(missingAuthPaths, field.Path)
			}
		case "signature", "nonce", "timestamp":
			dynamicPaths = append(dynamicPaths, field.Path)
		}
	}
	sort.Strings(authenticationPaths)
	sort.Strings(missingAuthPaths)
	sort.Strings(dynamicPaths)
	methodSupported := false
	switch right.Request.Method {
	case "GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE":
		methodSupported = true
	}
	reasons := []string{
		"高权限身份 B 的请求将作为不可变操作模板",
		"低权限探测只从身份 A 的正常控制请求移植同路径认证与 CSRF 字段",
	}
	if len(authenticationPaths) == 0 {
		reasons = append(reasons, "高权限操作没有可验证的认证字段，不能证明认证上下文已被替换")
	}
	if len(missingAuthPaths) > 0 {
		reasons = append(
			reasons,
			fmt.Sprintf("低权限控制请求缺少 %d 个高权限模板所需的认证字段", len(missingAuthPaths)),
		)
	}
	if len(dynamicPaths) > 0 {
		reasons = append(
			reasons,
			fmt.Sprintf("高权限模板包含 %d 个身份相关动态字段，执行前需要低权限页面重算", len(dynamicPaths)),
		)
	}
	if !methodSupported {
		reasons = append(reasons, "高权限操作使用了当前受控数据面不支持的方法")
	}
	candidate := ExtensionAuthorizationOperationCandidate{
		ID: authorizationOperationCandidateID(
			right.Request.ActionFingerprint,
			"right",
			"left",
		),
		TemplateSide:           "right",
		AuthContextSide:        "left",
		LowControlSide:         "left",
		Method:                 right.Request.Method,
		Path:                   right.Request.Path,
		ActionFingerprint:      right.Request.ActionFingerprint,
		Eligible:               methodSupported && len(authenticationPaths) > 0 && len(missingAuthPaths) == 0,
		SideEffect:             right.Request.Method != "GET" && right.Request.Method != "HEAD" && right.Request.Method != "OPTIONS",
		RequiresDynamicRebuild: len(dynamicPaths) > 0,
		AuthenticationPaths:    authenticationPaths,
		MissingAuthPaths:       missingAuthPaths,
		DynamicPaths:           dynamicPaths,
		Reasons:                reasons,
	}
	return ExtensionAuthorizationBaselinePair{
		State:               "matched",
		ActionFingerprint:   right.Request.ActionFingerprint,
		Reasons:             reasons,
		ResourceCandidates:  []ExtensionAuthorizationResourceCandidate{},
		OperationCandidates: []ExtensionAuthorizationOperationCandidate{candidate},
	}
}

func inferAuthorizationBaselinePairForMode(
	mode string,
	left *ExtensionAuthorizationBaseline,
	right *ExtensionAuthorizationBaseline,
) ExtensionAuthorizationBaselinePair {
	if mode == "vertical" {
		return inferVerticalAuthorizationBaselinePair(left, right)
	}
	pair := inferAuthorizationBaselinePair(left, right)
	if pair.ResourceCandidates == nil {
		pair.ResourceCandidates = []ExtensionAuthorizationResourceCandidate{}
	}
	pair.OperationCandidates = []ExtensionAuthorizationOperationCandidate{}
	return pair
}

func inferAuthorizationRequestCredentialRelation(
	left *ExtensionAuthorizationBaseline,
	right *ExtensionAuthorizationBaseline,
) string {
	if left == nil || right == nil {
		return "unknown"
	}
	rightAuthentication := make(map[string]ExtensionAuthorizationBaselineField)
	for _, field := range right.Request.Fields {
		if field.Category == "authentication" {
			rightAuthentication[authorizationFieldKey("wire", field)] = field
		}
	}
	matched := 0
	for _, field := range left.Request.Fields {
		if field.Category != "authentication" {
			continue
		}
		rightField, ok := rightAuthentication[authorizationFieldKey("wire", field)]
		if !ok {
			continue
		}
		matched++
		if field.ValueFingerprint != rightField.ValueFingerprint {
			return "different"
		}
	}
	if matched > 0 {
		return "same"
	}
	return "unknown"
}

func appendAuthorizationReason(reasons []string, reason string) []string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return reasons
	}
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	if len(reasons) >= 32 {
		return reasons
	}
	return append(reasons, reason)
}

func validateVerticalAuthorizationVerificationBaseline(
	baseline *ExtensionAuthorizationBaseline,
) error {
	if baseline == nil {
		return errors.New("post-state verification baseline is missing")
	}
	switch baseline.Request.Method {
	case "GET", "HEAD", "OPTIONS":
	default:
		return errors.New(
			"post-state verification baseline must use a read-only request",
		)
	}
	if authorizationBaselineRequiresDynamicRebuild(baseline) {
		return errors.New(
			"post-state verification baseline cannot require dynamic or logical rebuilding",
		)
	}
	return nil
}

func (m *ExtensionBridgeManager) BindExtensionAuthorizationBaseline(
	ctx context.Context,
	input ExtensionAuthorizationBaselineInput,
) (ExtensionAuthorizationWorkspace, error) {
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.Side = strings.ToLower(strings.TrimSpace(input.Side))
	input.NetworkRequestID = strings.TrimSpace(input.NetworkRequestID)
	if input.WorkspaceID == "" || (!input.Clear && input.NetworkRequestID == "") {
		return ExtensionAuthorizationWorkspace{}, errors.New("authorization baseline workspaceId and networkRequestId are required")
	}
	if input.Side != "left" && input.Side != "right" && input.Side != "verification" {
		return ExtensionAuthorizationWorkspace{}, errors.New(
			"authorization baseline side must be left, right, or verification",
		)
	}
	if input.Clear {
		if input.Side != "verification" {
			return ExtensionAuthorizationWorkspace{}, errors.New(
				"only the post-state verification baseline can be cleared directly",
			)
		}
		workspace, err := m.GetExtensionAuthorizationWorkspace(
			ctx,
			input.WorkspaceID,
			true,
		)
		if err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		if workspace.Mode != "vertical" {
			return ExtensionAuthorizationWorkspace{}, errors.New(
				"post-state verification baseline is only available in vertical authorization mode",
			)
		}
		workspace.Baselines.Verification = nil
		workspace.Plan = nil
		workspace.Execution = nil
		if err := m.updateExtensionAuthorizationWorkspace(workspace); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		return workspace, nil
	}
	workspace, err := m.GetExtensionAuthorizationWorkspace(ctx, input.WorkspaceID, true)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if workspace.State == "stale" || workspace.State == "blocked" {
		return ExtensionAuthorizationWorkspace{}, errors.New("authorization workspace is not eligible for baseline capture")
	}
	if input.Side == "verification" && workspace.Mode != "vertical" {
		return ExtensionAuthorizationWorkspace{}, errors.New(
			"post-state verification baseline is only available in vertical authorization mode",
		)
	}
	slot := workspace.Left
	if input.Side == "right" || input.Side == "verification" {
		slot = workspace.Right
	}
	snapshot := m.Snapshot()
	if _, _, err := authorizationDevice(
		snapshot,
		slot.DeviceID,
		"browser.authorization.baseline.capture",
		"browser.authorization.baseline.get",
	); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	raw, err := m.CallDevice(
		ctx,
		slot.DeviceID,
		"browser.authorization.baseline.capture",
		map[string]interface{}{
			"tabId":            slot.Target.TabID,
			"frameId":          slot.Target.FrameID,
			"documentId":       slot.Target.DocumentID,
			"authContextKind":  slot.ContextReference.Kind,
			"authContextId":    slot.ContextReference.ID,
			"networkRequestId": input.NetworkRequestID,
			"comparisonKey":    workspace.comparisonKey,
		},
	)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, fmt.Errorf("capture %s authorization baseline: %w", input.Side, err)
	}
	var baseline ExtensionAuthorizationBaseline
	if err := decodeAuthorizationResult(raw, &baseline); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if err := validateAuthorizationBaseline(baseline, slot); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if input.Side == "verification" {
		if err := validateVerticalAuthorizationVerificationBaseline(&baseline); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		workspace.Baselines.Verification = &baseline
		workspace.Plan = nil
		workspace.Execution = nil
		if err := m.updateExtensionAuthorizationWorkspace(workspace); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		return workspace, nil
	}
	if input.Side == "left" {
		workspace.Baselines.Left = &baseline
	} else {
		workspace.Baselines.Right = &baseline
	}
	workspace.BaselinePair = inferAuthorizationBaselinePairForMode(
		workspace.Mode,
		workspace.Baselines.Left,
		workspace.Baselines.Right,
	)
	credentialRelation := inferAuthorizationRequestCredentialRelation(
		workspace.Baselines.Left,
		workspace.Baselines.Right,
	)
	if credentialRelation != "unknown" {
		workspace.Proof.RequestCredentialRelation = credentialRelation
	}
	if workspace.Proof.Level == "conditional" &&
		workspace.Baselines.Left != nil &&
		workspace.Baselines.Right != nil {
		switch credentialRelation {
		case "different":
			workspace.Proof.Reasons = appendAuthorizationReason(
				workspace.Proof.Reasons,
				"A/B 正常请求证明实际发送的认证字段不同，Tab-local 条件隔离可用于只读矩阵",
			)
		case "same":
			workspace.State = "blocked"
			workspace.Proof.Level = "none"
			workspace.Proof.Reasons = appendAuthorizationReason(
				workspace.Proof.Reasons,
				"A/B 正常请求使用相同认证字段，两个普通 Tab 不能作为不同身份",
			)
		default:
			workspace.Proof.Reasons = appendAuthorizationReason(
				workspace.Proof.Reasons,
				"A/B 正常请求没有提供可比较的认证字段，Tab-local 条件隔离仍未完成",
			)
		}
	}
	workspace.Plan = nil
	workspace.Execution = nil
	if err := m.updateExtensionAuthorizationWorkspace(workspace); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	return workspace, nil
}

func sameAuthorizationWireBaseline(
	left ExtensionAuthorizationBaseline,
	right ExtensionAuthorizationBaseline,
) bool {
	left.LogicalRequest = nil
	right.LogicalRequest = nil
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return bytes.Equal(leftJSON, rightJSON)
}

func reconcileAuthorizationBaselineRefresh(
	expected ExtensionAuthorizationBaseline,
	current ExtensionAuthorizationBaseline,
) (ExtensionAuthorizationBaseline, bool, error) {
	if !sameAuthorizationWireBaseline(expected, current) {
		return ExtensionAuthorizationBaseline{}, false, errors.New(
			"authorization wire baseline changed",
		)
	}
	if expected.LogicalRequest == nil {
		current.LogicalRequest = nil
		return current, false, nil
	}
	if current.LogicalRequest == nil {
		return current, true, nil
	}
	expectedLogical, _ := json.Marshal(expected.LogicalRequest)
	currentLogical, _ := json.Marshal(current.LogicalRequest)
	if !bytes.Equal(expectedLogical, currentLogical) {
		return ExtensionAuthorizationBaseline{}, false, errors.New(
			"authorization logical binding changed",
		)
	}
	return current, false, nil
}

type extensionAuthorizationTransformProfileListItem struct {
	ID                 string                       `json:"id"`
	Name               string                       `json:"name"`
	Enabled            bool                         `json:"enabled"`
	Target             ExtensionAuthorizationTarget `json:"target"`
	IsolationContextID string                       `json:"isolationContextId"`
	CookieStoreID      string                       `json:"cookieStoreId"`
	Origin             string                       `json:"origin"`
	Match              struct {
		Methods    []string `json:"methods"`
		URLPattern string   `json:"urlPattern"`
	} `json:"match"`
	Request struct {
		Enabled bool              `json:"enabled"`
		Nodes   []json.RawMessage `json:"nodes"`
	} `json:"request"`
	Response       json.RawMessage `json:"response"`
	FailMode       string          `json:"failMode"`
	MaxConcurrency int             `json:"maxConcurrency"`
	Recovery       json.RawMessage `json:"recovery,omitempty"`
	CreatedAt      int64           `json:"createdAt"`
	UpdatedAt      int64           `json:"updatedAt"`
}

func authorizationProfileRecoveryState(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	var recovery struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(raw, &recovery); err != nil {
		return "", err
	}
	return strings.TrimSpace(recovery.State), nil
}

func authorizationWildcardURLMatches(pattern string, rawURL string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	patterns := []string{pattern}
	if normalized := normalizeAuthorizationProfileRoutePattern(pattern); normalized != pattern {
		patterns = append(patterns, normalized)
	}
	parsed, parsedErr := url.Parse(rawURL)
	for _, candidate := range patterns {
		expression := strings.ReplaceAll(regexp.QuoteMeta(candidate), `\*`, `.*`)
		matcher, err := regexp.Compile(`(?i)^` + expression + `$`)
		if err != nil {
			continue
		}
		if matcher.MatchString(rawURL) ||
			(parsedErr == nil && matcher.MatchString(parsed.Path)) {
			return true
		}
	}
	return false
}

func normalizeAuthorizationProfileRoutePattern(pattern string) string {
	pathStart := strings.Index(pattern, "/")
	if scheme := strings.Index(pattern, "://"); scheme >= 0 {
		hostStart := scheme + 3
		hostPath := strings.Index(pattern[hostStart:], "/")
		if hostPath < 0 {
			return pattern
		}
		pathStart = hostStart + hostPath
	}
	if pathStart < 0 {
		return pattern
	}
	pathEnd := len(pattern)
	if suffix := strings.IndexAny(pattern[pathStart:], "?#"); suffix >= 0 {
		pathEnd = pathStart + suffix
	}
	path := pattern[pathStart:pathEnd]
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if segment == "" || strings.Contains(segment, "*") || segment == ":resource" {
			continue
		}
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			decoded = segment
		}
		if authorizationNumericPathSegmentPattern.MatchString(decoded) ||
			authorizationUUIDPathSegmentPattern.MatchString(decoded) ||
			authorizationHexPathSegmentPattern.MatchString(decoded) ||
			authorizationOpaquePathSegmentPattern.MatchString(decoded) {
			segments[index] = ":resource"
		}
	}
	return pattern[:pathStart] + strings.Join(segments, "/") + pattern[pathEnd:]
}

func boundedAuthorizationProfileText(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

func authorizationTransformProfileCandidates(
	profiles []extensionAuthorizationTransformProfileListItem,
	slot ExtensionAuthorizationIdentitySlot,
	baseline *ExtensionAuthorizationBaseline,
) []ExtensionAuthorizationTransformProfileCandidate {
	return authorizationTransformProfileCandidatesForRoute(
		profiles,
		slot,
		baseline,
		baseline,
	)
}

func authorizationTransformProfileCandidatesForRoute(
	profiles []extensionAuthorizationTransformProfileListItem,
	slot ExtensionAuthorizationIdentitySlot,
	identityBaseline *ExtensionAuthorizationBaseline,
	routeBaseline *ExtensionAuthorizationBaseline,
) []ExtensionAuthorizationTransformProfileCandidate {
	if identityBaseline == nil || routeBaseline == nil {
		return nil
	}
	capacity := len(profiles)
	if capacity > maxAuthorizationTransformProfiles {
		capacity = maxAuthorizationTransformProfiles
	}
	output := make([]ExtensionAuthorizationTransformProfileCandidate, 0, capacity)
	for profileIndex, profile := range profiles {
		if profileIndex >= maxAuthorizationTransformProfiles {
			break
		}
		if profile.Target != slot.Target ||
			profile.Target != identityBaseline.Target ||
			profile.IsolationContextID != slot.IsolationContextID ||
			profile.IsolationContextID != identityBaseline.IsolationContextID ||
			profile.CookieStoreID != slot.CookieStoreID ||
			profile.CookieStoreID != identityBaseline.CookieStoreID ||
			profile.Origin != slot.Origin ||
			profile.Origin != identityBaseline.Origin ||
			profile.Origin != routeBaseline.Origin {
			continue
		}
		reasons := make([]string, 0, 5)
		profileID := boundedAuthorizationProfileText(profile.ID, 256)
		profileName := boundedAuthorizationProfileText(profile.Name, 120)
		if profileID == "" ||
			profileID != strings.TrimSpace(profile.ID) ||
			profileName == "" ||
			profileName != strings.TrimSpace(profile.Name) {
			reasons = append(reasons, "Profile identity is invalid")
		}
		if !profile.Enabled || !profile.Request.Enabled {
			reasons = append(reasons, "request transform is disabled")
		}
		recoveryState, recoveryErr := authorizationProfileRecoveryState(
			profile.Recovery,
		)
		if recoveryErr != nil {
			reasons = append(reasons, "document recovery metadata is invalid")
		} else if recoveryState != "" && recoveryState != "ready" {
			reasons = append(reasons, "document recovery is not ready")
		}
		methodCapacity := len(profile.Match.Methods)
		if methodCapacity > 32 {
			methodCapacity = 32
			reasons = append(reasons, "Profile declares too many HTTP methods")
		}
		methods := make([]string, 0, methodCapacity)
		methodCompatible := len(profile.Match.Methods) == 0
		for methodIndex, method := range profile.Match.Methods {
			if methodIndex >= 32 {
				break
			}
			normalized := strings.ToUpper(strings.TrimSpace(method))
			if normalized == "" {
				continue
			}
			methods = append(methods, normalized)
			if normalized == strings.ToUpper(routeBaseline.Request.Method) {
				methodCompatible = true
			}
		}
		sort.Strings(methods)
		urlPattern := boundedAuthorizationProfileText(profile.Match.URLPattern, 2_048)
		routeCompatible := methodCompatible &&
			urlPattern == strings.TrimSpace(profile.Match.URLPattern) &&
			authorizationWildcardURLMatches(urlPattern, routeBaseline.Request.URL)
		if !routeCompatible {
			reasons = append(reasons, "route does not match the captured baseline")
		}
		destinations := make([]string, 0)
		seenDestinations := make(map[string]struct{})
		hasBodyOutput := false
		hasForbiddenOutput := false
		if len(profile.Request.Nodes) > 64 {
			reasons = append(reasons, "Profile request Pipeline is too large")
		}
		for nodeIndex, rawNode := range profile.Request.Nodes {
			if nodeIndex >= 64 {
				break
			}
			var node struct {
				Kind        string `json:"kind"`
				Destination string `json:"destination"`
			}
			if err := json.Unmarshal(rawNode, &node); err != nil {
				reasons = append(reasons, "Profile contains an invalid request node")
				continue
			}
			if node.Kind != "output.write" {
				continue
			}
			destination := boundedAuthorizationProfileText(node.Destination, 512)
			if destination != strings.TrimSpace(node.Destination) {
				reasons = append(reasons, "Profile output destination is too long")
				continue
			}
			lower := strings.ToLower(destination)
			if strings.HasPrefix(lower, "header.") {
				destination = "header." + strings.TrimSpace(lower[7:])
				lower = destination
			}
			if destination == "" {
				continue
			}
			if lower == "body" || strings.HasPrefix(lower, "body.") {
				hasBodyOutput = true
			}
			if lower == "header.authorization" ||
				lower == "header.cookie" ||
				lower == "header.host" ||
				lower == "header.proxy-authorization" {
				hasForbiddenOutput = true
			}
			if _, exists := seenDestinations[destination]; exists {
				continue
			}
			seenDestinations[destination] = struct{}{}
			destinations = append(destinations, destination)
		}
		sort.Strings(destinations)
		if len(destinations) > 32 {
			reasons = append(reasons, "Profile declares too many output destinations")
			destinations = destinations[:32]
		}
		if len(destinations) == 0 {
			reasons = append(reasons, "Profile does not declare a request output")
		}
		if hasForbiddenOutput {
			reasons = append(reasons, "Profile attempts to produce an authentication Header")
		}
		logicalBodyReasons := append([]string{}, reasons...)
		if !hasBodyOutput {
			logicalBodyReasons = append(
				logicalBodyReasons,
				"Profile does not produce an encrypted Body output",
			)
		}
		dynamicFields := make(map[string]string)
		requiredDynamicFields := make(map[string]struct{})
		for _, field := range routeBaseline.Request.Fields {
			if field.Category != "signature" &&
				field.Category != "nonce" &&
				field.Category != "timestamp" &&
				field.Category != "csrf" {
				continue
			}
			path := field.Path
			if strings.HasPrefix(strings.ToLower(path), "header.") {
				path = "header." + strings.ToLower(strings.TrimSpace(path[7:]))
			}
			dynamicFields[path] = field.Category
			if field.Category != "csrf" {
				requiredDynamicFields[path] = struct{}{}
			}
		}
		dynamicFieldReasons := append([]string{}, reasons...)
		if len(requiredDynamicFields) == 0 {
			dynamicFieldReasons = append(
				dynamicFieldReasons,
				"baseline has no required Header/Query dynamic field",
			)
		} else {
			producedDynamicFields := make(map[string]struct{})
			for _, destination := range destinations {
				if destination == "body" ||
					strings.HasPrefix(destination, "body.") ||
					(!strings.HasPrefix(destination, "header.") &&
						!strings.HasPrefix(destination, "query.")) {
					dynamicFieldReasons = append(
						dynamicFieldReasons,
						"Profile contains an output unsupported by Header/Query reconstruction",
					)
					continue
				}
				if _, exists := dynamicFields[destination]; !exists {
					dynamicFieldReasons = append(
						dynamicFieldReasons,
						"Profile output does not correspond to a dynamic baseline field",
					)
					continue
				}
				producedDynamicFields[destination] = struct{}{}
			}
			for required := range requiredDynamicFields {
				if _, exists := producedDynamicFields[required]; !exists {
					dynamicFieldReasons = append(
						dynamicFieldReasons,
						"Profile does not cover every required dynamic baseline field",
					)
					break
				}
			}
		}
		output = append(output, ExtensionAuthorizationTransformProfileCandidate{
			ID:                    profileID,
			Name:                  profileName,
			Methods:               methods,
			URLPattern:            urlPattern,
			OutputDestinations:    destinations,
			Eligible:              len(reasons) == 0,
			Reasons:               reasons,
			DynamicFieldsEligible: len(dynamicFieldReasons) == 0,
			DynamicFieldsReasons:  dynamicFieldReasons,
			LogicalBodyEligible:   len(logicalBodyReasons) == 0,
			LogicalBodyReasons:    logicalBodyReasons,
			UpdatedAt:             profile.UpdatedAt,
		})
	}
	sort.Slice(output, func(left int, right int) bool {
		if output[left].Eligible != output[right].Eligible {
			return output[left].Eligible
		}
		if output[left].Name != output[right].Name {
			return output[left].Name < output[right].Name
		}
		return output[left].ID < output[right].ID
	})
	return output
}

func (m *ExtensionBridgeManager) ListExtensionAuthorizationTransformProfiles(
	ctx context.Context,
	workspaceID string,
) (ExtensionAuthorizationTransformProfileCandidates, error) {
	workspace, err := m.GetExtensionAuthorizationWorkspace(
		ctx,
		strings.TrimSpace(workspaceID),
		true,
	)
	if err != nil {
		return ExtensionAuthorizationTransformProfileCandidates{}, err
	}
	if workspace.Baselines.Left == nil ||
		workspace.Baselines.Right == nil ||
		workspace.BaselinePair.State != "matched" {
		return ExtensionAuthorizationTransformProfileCandidates{}, errors.New(
			"authorization transform profiles require a matched A/B baseline pair",
		)
	}
	snapshot := m.Snapshot()
	params := func(target ExtensionAuthorizationTarget) map[string]interface{} {
		return map[string]interface{}{
			"tabId":      target.TabID,
			"frameId":    target.FrameID,
			"documentId": target.DocumentID,
		}
	}
	if workspace.Mode == "vertical" {
		if _, _, err := authorizationDevice(
			snapshot,
			workspace.Left.DeviceID,
			"browser.transform.profile.list",
		); err != nil {
			return ExtensionAuthorizationTransformProfileCandidates{}, err
		}
		raw, err := m.CallDevice(
			ctx,
			workspace.Left.DeviceID,
			"browser.transform.profile.list",
			params(workspace.Left.Target),
		)
		if err != nil {
			return ExtensionAuthorizationTransformProfileCandidates{}, fmt.Errorf(
				"list low-privilege operation transform profiles: %w",
				err,
			)
		}
		var profiles []extensionAuthorizationTransformProfileListItem
		if err := decodeAuthorizationResult(raw, &profiles); err != nil {
			return ExtensionAuthorizationTransformProfileCandidates{}, err
		}
		return ExtensionAuthorizationTransformProfileCandidates{
			Left: authorizationTransformProfileCandidatesForRoute(
				profiles,
				workspace.Left,
				workspace.Baselines.Left,
				workspace.Baselines.Right,
			),
			Right: []ExtensionAuthorizationTransformProfileCandidate{},
		}, nil
	}
	for _, slot := range []ExtensionAuthorizationIdentitySlot{
		workspace.Left,
		workspace.Right,
	} {
		if _, _, err := authorizationDevice(
			snapshot,
			slot.DeviceID,
			"browser.transform.profile.list",
		); err != nil {
			return ExtensionAuthorizationTransformProfileCandidates{}, err
		}
	}
	leftRaw, rightRaw, err := m.callAuthorizationPair(
		ctx,
		workspace.Left.DeviceID,
		"browser.transform.profile.list",
		params(workspace.Left.Target),
		workspace.Right.DeviceID,
		"browser.transform.profile.list",
		params(workspace.Right.Target),
	)
	if err != nil {
		return ExtensionAuthorizationTransformProfileCandidates{}, fmt.Errorf(
			"list A/B authorization transform profiles: %w",
			err,
		)
	}
	var leftProfiles, rightProfiles []extensionAuthorizationTransformProfileListItem
	if err := decodeAuthorizationResult(leftRaw, &leftProfiles); err != nil {
		return ExtensionAuthorizationTransformProfileCandidates{}, err
	}
	if err := decodeAuthorizationResult(rightRaw, &rightProfiles); err != nil {
		return ExtensionAuthorizationTransformProfileCandidates{}, err
	}
	return ExtensionAuthorizationTransformProfileCandidates{
		Left: authorizationTransformProfileCandidates(
			leftProfiles,
			workspace.Left,
			workspace.Baselines.Left,
		),
		Right: authorizationTransformProfileCandidates(
			rightProfiles,
			workspace.Right,
			workspace.Baselines.Right,
		),
	}, nil
}

func authorizationOperationTransformFingerprint(
	binding ExtensionAuthorizationOperationTransformBinding,
) string {
	canonical := struct {
		Version            int                          `json:"version"`
		AuthBaselineID     string                       `json:"authBaselineId"`
		TemplateBaselineID string                       `json:"templateBaselineId"`
		ProfileID          string                       `json:"profileId"`
		ProfileUpdatedAt   int64                        `json:"profileUpdatedAt"`
		IsolationContextID string                       `json:"isolationContextId"`
		CookieStoreID      string                       `json:"cookieStoreId"`
		Target             ExtensionAuthorizationTarget `json:"target"`
		Origin             string                       `json:"origin"`
		ActionFingerprint  string                       `json:"actionFingerprint"`
		DynamicPaths       []string                     `json:"dynamicPaths"`
	}{
		Version:            binding.Version,
		AuthBaselineID:     binding.AuthBaselineID,
		TemplateBaselineID: binding.TemplateBaselineID,
		ProfileID:          binding.ProfileID,
		ProfileUpdatedAt:   binding.ProfileUpdatedAt,
		IsolationContextID: binding.IsolationContextID,
		CookieStoreID:      binding.CookieStoreID,
		Target:             binding.Target,
		Origin:             binding.Origin,
		ActionFingerprint:  binding.ActionFingerprint,
		DynamicPaths:       binding.DynamicPaths,
	}
	encoded, _ := json.Marshal(canonical)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (m *ExtensionBridgeManager) inspectVerticalAuthorizationOperationTransform(
	ctx context.Context,
	workspace ExtensionAuthorizationWorkspace,
	candidateID string,
	profileID string,
) (*ExtensionAuthorizationOperationTransformBinding, error) {
	profileID = strings.TrimSpace(profileID)
	if workspace.Mode != "vertical" ||
		profileID == "" ||
		workspace.Baselines.Left == nil ||
		workspace.Baselines.Right == nil {
		return nil, errors.New("vertical authorization operation transform input is invalid")
	}
	var selected *ExtensionAuthorizationOperationCandidate
	for index := range workspace.BaselinePair.OperationCandidates {
		candidate := &workspace.BaselinePair.OperationCandidates[index]
		if candidate.ID == candidateID {
			selected = candidate
			break
		}
	}
	if selected == nil {
		return nil, errors.New("vertical authorization operation candidate is missing")
	}
	candidates, err := m.ListExtensionAuthorizationTransformProfiles(ctx, workspace.ID)
	if err != nil {
		return nil, err
	}
	var profile *ExtensionAuthorizationTransformProfileCandidate
	for index := range candidates.Left {
		candidate := &candidates.Left[index]
		if candidate.ID == profileID {
			profile = candidate
			break
		}
	}
	if profile == nil ||
		!profile.Eligible ||
		!profile.DynamicFieldsEligible ||
		!sameAuthorizationDynamicPaths(profile.OutputDestinations, selected.DynamicPaths) {
		return nil, errors.New(
			"low-privilege operation transform does not exactly cover the required dynamic fields",
		)
	}
	now := time.Now().UnixMilli()
	binding := &ExtensionAuthorizationOperationTransformBinding{
		Version:            1,
		AuthBaselineID:     workspace.Baselines.Left.ID,
		TemplateBaselineID: workspace.Baselines.Right.ID,
		ProfileID:          profile.ID,
		ProfileName:        profile.Name,
		ProfileUpdatedAt:   profile.UpdatedAt,
		IsolationContextID: workspace.Left.IsolationContextID,
		CookieStoreID:      workspace.Left.CookieStoreID,
		Target:             workspace.Left.Target,
		Origin:             workspace.Left.Origin,
		ActionFingerprint:  workspace.Baselines.Right.Request.ActionFingerprint,
		DynamicPaths:       append([]string(nil), selected.DynamicPaths...),
		CreatedAt:          now,
		ExpiresAt:          workspace.ExpiresAt,
	}
	binding.BindingFingerprint = authorizationOperationTransformFingerprint(*binding)
	return binding, nil
}

func (m *ExtensionBridgeManager) BindExtensionAuthorizationLogicalRequests(
	ctx context.Context,
	input ExtensionAuthorizationLogicalBindingInput,
) (ExtensionAuthorizationWorkspace, error) {
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.TransformProfiles.Left = strings.TrimSpace(input.TransformProfiles.Left)
	input.TransformProfiles.Right = strings.TrimSpace(input.TransformProfiles.Right)
	if input.WorkspaceID == "" ||
		input.TransformProfiles.Left == "" ||
		input.TransformProfiles.Right == "" {
		return ExtensionAuthorizationWorkspace{}, errors.New(
			"authorization logical binding requires workspaceId and A/B transform profiles",
		)
	}
	workspace, err := m.GetExtensionAuthorizationWorkspace(ctx, input.WorkspaceID, false)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if workspace.Mode == "vertical" {
		return ExtensionAuthorizationWorkspace{}, errors.New(
			"vertical authorization operation templates do not use horizontal logical resource binding",
		)
	}
	if workspace.State == "stale" ||
		workspace.State == "blocked" ||
		workspace.Baselines.Left == nil ||
		workspace.Baselines.Right == nil {
		return ExtensionAuthorizationWorkspace{}, errors.New(
			"authorization logical binding requires a current matched A/B baseline pair",
		)
	}
	snapshot := m.Snapshot()
	for _, slot := range []ExtensionAuthorizationIdentitySlot{workspace.Left, workspace.Right} {
		if _, _, err := authorizationDevice(
			snapshot,
			slot.DeviceID,
			"browser.authorization.baseline.logical.bind",
			"browser.authorization.baseline.get",
		); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
	}
	leftRaw, rightRaw, err := m.callAuthorizationPair(
		ctx,
		workspace.Left.DeviceID,
		"browser.authorization.baseline.logical.bind",
		map[string]interface{}{
			"id":            workspace.Baselines.Left.ID,
			"profileId":     input.TransformProfiles.Left,
			"comparisonKey": workspace.comparisonKey,
		},
		workspace.Right.DeviceID,
		"browser.authorization.baseline.logical.bind",
		map[string]interface{}{
			"id":            workspace.Baselines.Right.ID,
			"profileId":     input.TransformProfiles.Right,
			"comparisonKey": workspace.comparisonKey,
		},
	)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, fmt.Errorf(
			"bind A/B logical authorization requests: %w",
			err,
		)
	}
	var left, right ExtensionAuthorizationBaseline
	if err := decodeAuthorizationResult(leftRaw, &left); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if err := decodeAuthorizationResult(rightRaw, &right); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if !sameAuthorizationWireBaseline(left, *workspace.Baselines.Left) ||
		!sameAuthorizationWireBaseline(right, *workspace.Baselines.Right) {
		return ExtensionAuthorizationWorkspace{}, errors.New(
			"authorization logical binding changed the immutable wire baseline",
		)
	}
	if err := validateAuthorizationBaseline(left, workspace.Left); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if err := validateAuthorizationBaseline(right, workspace.Right); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if left.LogicalRequest == nil ||
		right.LogicalRequest == nil ||
		left.LogicalRequest.ProfileID != input.TransformProfiles.Left ||
		right.LogicalRequest.ProfileID != input.TransformProfiles.Right {
		return ExtensionAuthorizationWorkspace{}, errors.New(
			"browser authorization logical binding profile identity is invalid",
		)
	}
	workspace.Baselines.Left = &left
	workspace.Baselines.Right = &right
	workspace.BaselinePair = inferAuthorizationBaselinePairForMode(
		workspace.Mode,
		&left,
		&right,
	)
	workspace.Plan = nil
	workspace.Execution = nil
	if err := m.updateExtensionAuthorizationWorkspace(workspace); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	return workspace, nil
}
