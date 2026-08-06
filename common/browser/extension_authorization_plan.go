package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

func authorizationBaselineRequiresDynamicRebuild(
	baseline *ExtensionAuthorizationBaseline,
) bool {
	if baseline == nil {
		return false
	}
	if baseline.LogicalRequest != nil {
		return true
	}
	for _, field := range baseline.Request.Fields {
		switch field.Category {
		case "signature", "nonce", "timestamp":
			return true
		}
	}
	return false
}

func authorizationPlanCase(
	id string,
	label string,
	requestSide string,
	authSide string,
	resourceSide string,
	baseline *ExtensionAuthorizationBaseline,
) ExtensionAuthorizationPlanCase {
	method := ""
	path := ""
	if baseline != nil {
		method = baseline.Request.Method
		path = baseline.Request.Path
	}
	return ExtensionAuthorizationPlanCase{
		ID:                  id,
		Label:               label,
		RequestBaselineSide: requestSide,
		AuthContextSide:     authSide,
		ResourceValueSide:   resourceSide,
		Method:              method,
		Path:                path,
		SideEffect:          method != "GET" && method != "HEAD" && method != "OPTIONS",
	}
}

func normalizeAuthorizationCanaryPaths(paths []string) ([]string, error) {
	if len(paths) > 8 {
		return nil, errors.New("authorization plan accepts at most 8 response canary paths")
	}
	output := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, value := range paths {
		path := strings.TrimSpace(value)
		segments := strings.Count(path, ".") + strings.Count(path, "[")
		if len(path) > 512 ||
			segments < 1 ||
			segments > 16 ||
			!extensionAuthorizationCanaryPathPattern.MatchString(path) {
			return nil, errors.New("authorization response canary path is invalid")
		}
		lower := strings.ToLower(path)
		if strings.Contains(lower, ".__proto__") ||
			strings.Contains(lower, ".prototype") ||
			strings.Contains(lower, ".constructor") {
			return nil, errors.New("authorization response canary path contains a reserved segment")
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		output = append(output, path)
	}
	return output, nil
}

const (
	authorizationIsolationStrong                   = "strong"
	authorizationIsolationSeparateStoreConditional = "separate-store-conditional"
	authorizationIsolationTabLocalConditional      = "tab-local-conditional"
)

func eligibleExtensionAuthorizationIsolationMode(
	workspace ExtensionAuthorizationWorkspace,
) string {
	if workspace.State == "ready" && workspace.Proof.Level == "strong" {
		return authorizationIsolationStrong
	}
	if workspace.State != "conditional" ||
		workspace.Proof.Level != "conditional" ||
		workspace.Proof.RequestCredentialRelation != "different" {
		return ""
	}
	switch workspace.Proof.CookieStoreRelation {
	case "different":
		if workspace.Proof.RefreshCheck == "not-required" ||
			workspace.Proof.RefreshCheck == "passed" {
			return authorizationIsolationSeparateStoreConditional
		}
	case "same":
		if workspace.Proof.RefreshCheck == "passed" {
			return authorizationIsolationTabLocalConditional
		}
	}
	return ""
}

func buildExtensionAuthorizationPlan(
	workspace ExtensionAuthorizationWorkspace,
	candidateID string,
	now int64,
) (*ExtensionAuthorizationPlan, error) {
	return buildExtensionAuthorizationPlanWithCanaries(workspace, candidateID, nil, now)
}

func buildExtensionAuthorizationPlanWithCanaries(
	workspace ExtensionAuthorizationWorkspace,
	candidateID string,
	canaryPaths []string,
	now int64,
) (*ExtensionAuthorizationPlan, error) {
	return buildExtensionAuthorizationPlanWithOptions(
		workspace,
		candidateID,
		canaryPaths,
		nil,
		nil,
		now,
	)
}

func buildExtensionAuthorizationPlanWithOptions(
	workspace ExtensionAuthorizationWorkspace,
	candidateID string,
	canaryPaths []string,
	transforms *ExtensionAuthorizationTransformPair,
	operationTransform *ExtensionAuthorizationOperationTransformBinding,
	now int64,
) (*ExtensionAuthorizationPlan, error) {
	if workspace.Mode == "vertical" {
		return buildVerticalExtensionAuthorizationPlan(
			workspace,
			candidateID,
			canaryPaths,
			operationTransform,
			now,
		)
	}
	if operationTransform != nil {
		return nil, errors.New("horizontal authorization plan cannot use an operation transform")
	}
	normalizedCanaryPaths, err := normalizeAuthorizationCanaryPaths(canaryPaths)
	if err != nil {
		return nil, err
	}
	isolationMode := eligibleExtensionAuthorizationIsolationMode(workspace)
	if isolationMode == "" {
		return nil, errors.New("authorization workspace does not have an eligible current isolation proof")
	}
	if workspace.BaselinePair.State != "matched" ||
		workspace.Baselines.Left == nil ||
		workspace.Baselines.Right == nil {
		return nil, errors.New("authorization plan requires a matched A/B baseline pair")
	}
	var selected *ExtensionAuthorizationResourceCandidate
	for index := range workspace.BaselinePair.ResourceCandidates {
		candidate := &workspace.BaselinePair.ResourceCandidates[index]
		if candidate.ID == candidateID {
			selected = candidate
			break
		}
	}
	if selected == nil {
		return nil, errors.New("authorization plan selector is not a verified A/B resource candidate")
	}
	left := workspace.Baselines.Left
	right := workspace.Baselines.Right
	hasDynamicFields := authorizationBaselineRequiresDynamicRebuild(left) ||
		authorizationBaselineRequiresDynamicRebuild(right)
	if !hasDynamicFields && transforms != nil {
		return nil, errors.New("authorization transform profiles were supplied for a baseline without dynamic fields")
	}
	requiresDynamicRebuild := hasDynamicFields && transforms == nil
	if selected.Source == "logical" && transforms == nil {
		requiresDynamicRebuild = true
	}
	cases := []ExtensionAuthorizationPlanCase{
		authorizationPlanCase("a-own", "身份 A 访问自己的资源", "left", "left", "left", left),
		authorizationPlanCase("b-own", "身份 B 访问自己的资源", "right", "right", "right", right),
		authorizationPlanCase("a-to-b", "身份 A 访问身份 B 的资源", "left", "left", "right", left),
		authorizationPlanCase("b-to-a", "身份 B 访问身份 A 的资源", "right", "right", "left", right),
	}
	state := "ready"
	reasons := []string{
		"四项矩阵固定使用各身份自己的请求骨架与认证上下文，只交换已确认的资源字段",
	}
	if len(normalizedCanaryPaths) > 0 {
		reasons = append(
			reasons,
			fmt.Sprintf("将优先验证 %d 个用户指定的响应归属路径，并继续保留自动 JSON 差分", len(normalizedCanaryPaths)),
		)
	}
	switch isolationMode {
	case authorizationIsolationTabLocalConditional:
		reasons = append(
			reasons,
			"同 Cookie Store 的两个 Tab 仅因 sessionStorage 与实际请求认证字段均不同而获得条件隔离",
		)
	case authorizationIsolationSeparateStoreConditional:
		reasons = append(
			reasons,
			"A/B 使用不同 Cookie Store，且正常请求证明实际认证材料不同；页面登录启发式为 unknown 不影响隔离资格",
		)
	}
	if requiresDynamicRebuild {
		state = "blocked"
		reasons = append(reasons, "请求包含动态字段或逻辑明文资源，执行前必须绑定 A/B 各自可重算的 Transform Profile")
	} else if selected.RequiresLogicalBinding {
		state = "blocked"
		reasons = append(reasons, "线上 Body 不是可确定性替换的结构化资源字段，必须先建立逻辑明文绑定")
	} else {
		if hasDynamicFields {
			reasons = append(reasons, "A/B 动态字段分别绑定各自页面的 Transform Profile，交叉请求不会复用另一身份的页面函数")
		}
		for _, testCase := range cases {
			if testCase.SideEffect {
				state = "review-required"
				reasons = append(reasons, "矩阵包含可能产生副作用的非只读请求，执行前需要明确 Review")
				break
			}
		}
	}
	return &ExtensionAuthorizationPlan{
		Version:     1,
		ID:          "authorization-plan-" + uuid.NewString(),
		WorkspaceID: workspace.ID,
		Mode:        "horizontal",
		ProofID:     workspace.Proof.ID,
		CandidateID: selected.ID,
		CanaryPaths: normalizedCanaryPaths,
		State:       state,
		Selector: ExtensionAuthorizationPlanSelector{
			Source:   selected.Source,
			Location: selected.Location,
			Path:     selected.Path,
		},
		Cases:                  cases,
		RequestBudget:          len(cases),
		RequiresDynamicRebuild: requiresDynamicRebuild,
		Transforms:             transforms,
		Reasons:                reasons,
		CreatedAt:              now,
		ExpiresAt:              workspace.ExpiresAt,
	}, nil
}

func buildVerticalExtensionAuthorizationPlan(
	workspace ExtensionAuthorizationWorkspace,
	candidateID string,
	canaryPaths []string,
	operationTransform *ExtensionAuthorizationOperationTransformBinding,
	now int64,
) (*ExtensionAuthorizationPlan, error) {
	normalizedCanaryPaths, err := normalizeAuthorizationCanaryPaths(canaryPaths)
	if err != nil {
		return nil, err
	}
	isolationMode := eligibleExtensionAuthorizationIsolationMode(workspace)
	if isolationMode == "" {
		return nil, errors.New("authorization workspace does not have an eligible current isolation proof")
	}
	if workspace.BaselinePair.State != "matched" ||
		workspace.Baselines.Left == nil ||
		workspace.Baselines.Right == nil {
		return nil, errors.New(
			"vertical authorization plan requires low-control and privileged baselines",
		)
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
		return nil, errors.New(
			"vertical authorization plan requires a verified operation template candidate",
		)
	}
	if !selected.RequiresDynamicRebuild && operationTransform != nil {
		return nil, errors.New(
			"vertical authorization operation transform was supplied without dynamic fields",
		)
	}
	if operationTransform != nil {
		if operationTransform.Version != 1 ||
			operationTransform.AuthBaselineID != workspace.Baselines.Left.ID ||
			operationTransform.TemplateBaselineID != workspace.Baselines.Right.ID ||
			strings.TrimSpace(operationTransform.ProfileID) == "" ||
			len([]rune(operationTransform.ProfileName)) == 0 ||
			len([]rune(operationTransform.ProfileName)) > 120 ||
			operationTransform.ProfileUpdatedAt <= 0 ||
			operationTransform.IsolationContextID != workspace.Left.IsolationContextID ||
			operationTransform.CookieStoreID != workspace.Left.CookieStoreID ||
			operationTransform.Target != workspace.Left.Target ||
			operationTransform.Origin != workspace.Left.Origin ||
			operationTransform.ActionFingerprint != selected.ActionFingerprint ||
			operationTransform.ExpiresAt != workspace.ExpiresAt ||
			operationTransform.ExpiresAt <= now ||
			operationTransform.CreatedAt <= 0 ||
			operationTransform.CreatedAt > now ||
			!sameAuthorizationDynamicPaths(
				operationTransform.DynamicPaths,
				selected.DynamicPaths,
			) ||
			operationTransform.BindingFingerprint !=
				authorizationOperationTransformFingerprint(*operationTransform) {
			return nil, errors.New(
				"vertical authorization operation transform binding is invalid",
			)
		}
	}
	left := workspace.Baselines.Left
	right := workspace.Baselines.Right
	verification := workspace.Baselines.Verification
	if verification != nil {
		if err := validateVerticalAuthorizationVerificationBaseline(verification); err != nil {
			return nil, err
		}
	}
	cases := []ExtensionAuthorizationPlanCase{
		authorizationPlanCase(
			"low-control",
			"低权限身份执行自己的正常控制请求",
			"left",
			"left",
			"",
			left,
		),
		authorizationPlanCase(
			"privileged-baseline",
			"高权限身份执行特权操作",
			"right",
			"right",
			"",
			right,
		),
	}
	if verification != nil {
		cases = append(
			cases,
			authorizationPlanCase(
				"post-state-before",
				"高权限对照完成后的状态快照",
				"verification",
				"right",
				"",
				verification,
			),
		)
	}
	cases = append(
		cases,
		authorizationPlanCase(
			"low-privileged-probe",
			"低权限身份执行高权限操作模板",
			"right",
			"left",
			"",
			right,
		),
	)
	if verification != nil {
		cases = append(
			cases,
			authorizationPlanCase(
				"post-state-after",
				"低权限探测完成后的状态快照",
				"verification",
				"right",
				"",
				verification,
			),
		)
	}
	state := "ready"
	reasons := []string{
		"高权限操作模板保持 method、route、业务 Body 与非认证 Header 不变",
		"低权限探测只移植身份 A 的同路径认证与 CSRF 原始值，不复用身份 B 的认证材料",
		"首版垂直结论不会仅凭成功状态提升为 confirmed；写操作需要后置状态证据",
	}
	switch isolationMode {
	case authorizationIsolationTabLocalConditional:
		reasons = append(
			reasons,
			"同 Cookie Store 的两个 Tab 仅因 sessionStorage 与实际请求认证字段均不同而获得条件隔离",
		)
	case authorizationIsolationSeparateStoreConditional:
		reasons = append(
			reasons,
			"A/B 使用不同 Cookie Store，且正常请求证明实际认证材料不同；页面登录启发式为 unknown 不影响隔离资格",
		)
	}
	if !selected.Eligible {
		state = "blocked"
		reasons = append(reasons, selected.Reasons...)
	}
	if selected.RequiresDynamicRebuild && operationTransform == nil {
		state = "blocked"
		reasons = append(
			reasons,
			"高权限模板包含签名、Nonce 或时间字段；需要证明低权限页面能够按该模板重算后才能执行",
		)
	} else if operationTransform != nil {
		reasons = append(
			reasons,
			"低权限页面已绑定同路由 Transform Profile；认证移植后将只重算已声明的动态字段",
		)
	}
	if state == "ready" {
		for _, testCase := range cases {
			if testCase.SideEffect {
				state = "review-required"
				reasons = append(
					reasons,
					"垂直矩阵包含可能产生副作用的请求，执行前需要明确 Review",
				)
				break
			}
		}
	}
	if len(normalizedCanaryPaths) > 0 {
		reasons = append(
			reasons,
			fmt.Sprintf("将优先比较 %d 个用户指定的响应语义路径", len(normalizedCanaryPaths)),
		)
	}
	if verification != nil {
		reasons = append(
			reasons,
			"已绑定高权限只读状态请求；高权限操作完成后先取前快照，低权限探测成功后再取后快照",
		)
	}
	verificationBaselineID := ""
	if verification != nil {
		verificationBaselineID = verification.ID
	}
	return &ExtensionAuthorizationPlan{
		Version:     1,
		ID:          "authorization-plan-" + uuid.NewString(),
		WorkspaceID: workspace.ID,
		Mode:        "vertical",
		ProofID:     workspace.Proof.ID,
		CandidateID: selected.ID,
		CanaryPaths: normalizedCanaryPaths,
		State:       state,
		Selector: ExtensionAuthorizationPlanSelector{
			Source:   "operation",
			Location: "request",
			Path:     "right",
		},
		Operation: &ExtensionAuthorizationPlanOperation{
			TemplateBaselineSide:   selected.TemplateSide,
			AuthContextSide:        selected.AuthContextSide,
			LowControlSide:         selected.LowControlSide,
			AuthenticationPaths:    append([]string(nil), selected.AuthenticationPaths...),
			DynamicPaths:           append([]string(nil), selected.DynamicPaths...),
			VerificationBaselineID: verificationBaselineID,
			Transform:              operationTransform,
		},
		Cases:                  cases,
		RequestBudget:          len(cases),
		RequiresDynamicRebuild: selected.RequiresDynamicRebuild && operationTransform == nil,
		Reasons:                reasons,
		CreatedAt:              now,
		ExpiresAt:              workspace.ExpiresAt,
	}, nil
}

func validateAuthorizationTransformBinding(
	binding ExtensionAuthorizationTransformBinding,
	baseline *ExtensionAuthorizationBaseline,
	slot ExtensionAuthorizationIdentitySlot,
	profileID string,
	now int64,
) error {
	if baseline == nil ||
		binding.Version != 1 ||
		binding.BaselineID != baseline.ID ||
		binding.ProfileID != profileID ||
		binding.IsolationContextID != baseline.IsolationContextID ||
		binding.IsolationContextID != slot.IsolationContextID ||
		binding.CookieStoreID != baseline.CookieStoreID ||
		binding.CookieStoreID != slot.CookieStoreID ||
		binding.Origin != baseline.Origin ||
		binding.Target != baseline.Target ||
		binding.Target != slot.Target ||
		binding.CreatedAt <= 0 ||
		binding.ExpiresAt != baseline.ExpiresAt ||
		binding.ExpiresAt <= now {
		return errors.New("authorization transform binding identity is invalid")
	}
	if len([]rune(binding.ProfileName)) == 0 || len([]rune(binding.ProfileName)) > 120 {
		return errors.New("authorization transform binding profile name is invalid")
	}
	if !validAuthorizationFingerprint(binding.BindingFingerprint, "sha256:") {
		return errors.New("authorization transform binding fingerprint is invalid")
	}
	if len(binding.DynamicPaths) == 0 || len(binding.DynamicPaths) > 32 {
		return errors.New("authorization transform binding dynamic paths are invalid")
	}
	seen := make(map[string]struct{}, len(binding.DynamicPaths))
	for _, path := range binding.DynamicPaths {
		lower := strings.ToLower(path)
		if len(path) > 512 ||
			(path != "body" &&
				!strings.HasPrefix(path, "body.") &&
				!strings.HasPrefix(lower, "header.") &&
				!strings.HasPrefix(path, "query.")) ||
			lower == "header.authorization" ||
			lower == "header.cookie" ||
			lower == "header.host" ||
			lower == "header.proxy-authorization" {
			return errors.New("authorization transform binding contains an unsupported dynamic path")
		}
		if _, ok := seen[path]; ok {
			return errors.New("authorization transform binding contains duplicate dynamic paths")
		}
		seen[path] = struct{}{}
	}
	return nil
}

func sameAuthorizationDynamicPaths(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (m *ExtensionBridgeManager) inspectExtensionAuthorizationTransforms(
	ctx context.Context,
	workspace ExtensionAuthorizationWorkspace,
	profiles ExtensionAuthorizationTransformProfileInput,
) (*ExtensionAuthorizationTransformPair, error) {
	profiles.Left = strings.TrimSpace(profiles.Left)
	profiles.Right = strings.TrimSpace(profiles.Right)
	if profiles.Left == "" || profiles.Right == "" {
		return nil, errors.New("authorization transform profiles require both left and right identity bindings")
	}
	if workspace.Baselines.Left == nil || workspace.Baselines.Right == nil {
		return nil, errors.New("authorization transform profiles require a matched A/B baseline pair")
	}
	snapshot := m.Snapshot()
	for _, slot := range []ExtensionAuthorizationIdentitySlot{workspace.Left, workspace.Right} {
		if _, _, err := authorizationDevice(
			snapshot,
			slot.DeviceID,
			"browser.authorization.baseline.transform.inspect",
			"browser.authorization.baseline.transform.compile",
		); err != nil {
			return nil, err
		}
	}
	leftRaw, rightRaw, err := m.callAuthorizationPair(
		ctx,
		workspace.Left.DeviceID,
		"browser.authorization.baseline.transform.inspect",
		map[string]interface{}{
			"id":        workspace.Baselines.Left.ID,
			"profileId": profiles.Left,
		},
		workspace.Right.DeviceID,
		"browser.authorization.baseline.transform.inspect",
		map[string]interface{}{
			"id":        workspace.Baselines.Right.ID,
			"profileId": profiles.Right,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("inspect authorization transform profiles: %w", err)
	}
	var pair ExtensionAuthorizationTransformPair
	if err := decodeAuthorizationResult(leftRaw, &pair.Left); err != nil {
		return nil, err
	}
	if err := decodeAuthorizationResult(rightRaw, &pair.Right); err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if err := validateAuthorizationTransformBinding(
		pair.Left,
		workspace.Baselines.Left,
		workspace.Left,
		profiles.Left,
		now,
	); err != nil {
		return nil, err
	}
	if err := validateAuthorizationTransformBinding(
		pair.Right,
		workspace.Baselines.Right,
		workspace.Right,
		profiles.Right,
		now,
	); err != nil {
		return nil, err
	}
	if !sameAuthorizationDynamicPaths(pair.Left.DynamicPaths, pair.Right.DynamicPaths) {
		return nil, errors.New("A/B authorization transform profiles do not cover the same dynamic field structure")
	}
	return &pair, nil
}

func (m *ExtensionBridgeManager) CreateExtensionAuthorizationPlan(
	ctx context.Context,
	input ExtensionAuthorizationPlanInput,
) (ExtensionAuthorizationWorkspace, error) {
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.CandidateID = strings.TrimSpace(input.CandidateID)
	input.OperationTransformProfileID = strings.TrimSpace(
		input.OperationTransformProfileID,
	)
	if input.WorkspaceID == "" || input.CandidateID == "" {
		return ExtensionAuthorizationWorkspace{}, errors.New("authorization plan workspaceId and candidateId are required")
	}
	workspace, err := m.GetExtensionAuthorizationWorkspace(ctx, input.WorkspaceID, true)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	requestedCanaries := input.CanaryPaths
	if requestedCanaries == nil &&
		workspace.Plan != nil &&
		workspace.Plan.CandidateID == input.CandidateID {
		requestedCanaries = workspace.Plan.CanaryPaths
	}
	canaryPaths, err := normalizeAuthorizationCanaryPaths(requestedCanaries)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	requestedProfiles := input.TransformProfiles
	if workspace.Mode == "vertical" && requestedProfiles != nil {
		return ExtensionAuthorizationWorkspace{}, errors.New(
			"vertical authorization plans use one low-privilege operation transform",
		)
	}
	if workspace.Mode != "vertical" &&
		requestedProfiles == nil &&
		workspace.Plan != nil &&
		workspace.Plan.CandidateID == input.CandidateID &&
		workspace.Plan.Transforms != nil {
		requestedProfiles = &ExtensionAuthorizationTransformProfileInput{
			Left:  workspace.Plan.Transforms.Left.ProfileID,
			Right: workspace.Plan.Transforms.Right.ProfileID,
		}
	}
	if workspace.Mode != "vertical" &&
		requestedProfiles == nil &&
		workspace.Baselines.Left != nil &&
		workspace.Baselines.Right != nil &&
		workspace.Baselines.Left.LogicalRequest != nil &&
		workspace.Baselines.Right.LogicalRequest != nil {
		requestedProfiles = authorizationLogicalProfilesForCandidate(
			workspace,
			input.CandidateID,
		)
	}
	var transforms *ExtensionAuthorizationTransformPair
	if requestedProfiles != nil {
		transforms, err = m.inspectExtensionAuthorizationTransforms(
			ctx,
			workspace,
			*requestedProfiles,
		)
		if err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
	}
	operationProfileID := input.OperationTransformProfileID
	if workspace.Mode == "vertical" &&
		operationProfileID == "" &&
		workspace.Plan != nil &&
		workspace.Plan.CandidateID == input.CandidateID &&
		workspace.Plan.Operation != nil &&
		workspace.Plan.Operation.Transform != nil {
		operationProfileID = workspace.Plan.Operation.Transform.ProfileID
	}
	var operationTransform *ExtensionAuthorizationOperationTransformBinding
	if workspace.Mode == "vertical" && operationProfileID != "" {
		operationTransform, err = m.inspectVerticalAuthorizationOperationTransform(
			ctx,
			workspace,
			input.CandidateID,
			operationProfileID,
		)
		if err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
	}
	workspace.Execution = nil
	workspace.Plan, err = buildExtensionAuthorizationPlanWithOptions(
		workspace,
		input.CandidateID,
		canaryPaths,
		transforms,
		operationTransform,
		time.Now().UnixMilli(),
	)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if err := m.updateExtensionAuthorizationWorkspace(workspace); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	return workspace, nil
}
