package browser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func authorizationResponseExact(
	left *ExtensionAuthorizationRequestExecution,
	right *ExtensionAuthorizationRequestExecution,
) bool {
	return left != nil &&
		right != nil &&
		!left.Response.Truncated &&
		!right.Response.Truncated &&
		left.Response.AnalysisState != "encoded-unavailable" &&
		right.Response.AnalysisState != "encoded-unavailable" &&
		left.Response.ValueFingerprint == right.Response.ValueFingerprint
}

func authorizationResponseShape(
	left *ExtensionAuthorizationRequestExecution,
	right *ExtensionAuthorizationRequestExecution,
) bool {
	return left != nil &&
		right != nil &&
		left.Response.ShapeFingerprint != "" &&
		left.Response.ShapeFingerprint == right.Response.ShapeFingerprint
}

func flattenAuthorizationResponseJSON(
	body []byte,
) map[string][]byte {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root interface{}
	if err := decoder.Decode(&root); err != nil {
		return nil
	}
	output := make(map[string][]byte)
	visited := 0
	var walk func(interface{}, string, int)
	walk = func(value interface{}, path string, depth int) {
		visited++
		if visited > 1000 || depth > 8 || len(output) >= 300 {
			return
		}
		switch typed := value.(type) {
		case map[string]interface{}:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			if len(keys) > 100 {
				keys = keys[:100]
			}
			for _, key := range keys {
				walk(typed[key], path+"."+key, depth+1)
			}
		case []interface{}:
			limit := len(typed)
			if limit > 20 {
				limit = 20
			}
			for index, child := range typed[:limit] {
				walk(child, fmt.Sprintf("%s[%d]", path, index), depth+1)
			}
		case nil:
			return
		default:
			encoded, err := json.Marshal(typed)
			if err == nil && len(encoded) <= 8*1024 {
				output[path] = encoded
			}
		}
	}
	walk(root, "body", 0)
	return output
}

func authorizationResponseCanaryPath(path string) bool {
	normalized := strings.ToLower(path)
	for _, blocked := range []string{
		"auth", "token", "cookie", "csrf", "xsrf", "signature", "hmac",
		"password", "passwd", "secret", "session", "credential", "privatekey",
		"nonce", "timestamp", "requestid", "request-id", "traceid", "trace-id",
		"correlationid", "correlation-id", "idempotency", "message", "error",
		"status", "success",
	} {
		if strings.Contains(normalized, blocked) {
			return false
		}
	}
	compactPath := strings.NewReplacer("-", "", "_", "", "[", ".", "]", ".").Replace(normalized)
	for _, semantic := range []string{
		"userid", "accountid", "tenantid", "organizationid", "workspaceid",
		"projectid", "customerid", "orderid", "resourceid", "ownerid",
		"username", "email", "nickname", "displayname", "tenant",
		"organization", "owner", "role", "profile", "remark", "note",
		"order", "resource", "project", "workspace",
	} {
		if strings.Contains(compactPath, semantic) {
			return true
		}
	}
	last := normalized
	if dot := strings.LastIndex(last, "."); dot >= 0 {
		last = last[dot+1:]
	}
	if bracket := strings.Index(last, "["); bracket >= 0 {
		last = last[:bracket]
	}
	compact := strings.NewReplacer("-", "", "_", "").Replace(last)
	switch compact {
	case "id", "uid", "uuid", "userid", "accountid", "tenantid", "orgid",
		"organizationid", "workspaceid", "projectid", "teamid", "customerid",
		"orderid", "resourceid", "objectid", "recordid", "documentid",
		"fileid", "invoiceid", "owner", "ownerid", "username", "email", "slug",
		"name", "nickname", "displayname", "tenant", "organization", "role",
		"profile", "remark", "note", "order", "resource", "project", "workspace":
		return true
	case "number":
		return strings.Contains(normalized, ".order.") ||
			strings.Contains(normalized, ".invoice.") ||
			strings.Contains(normalized, ".account.")
	default:
		return false
	}
}

func authorizationResponseLeaves(
	result *ExtensionAuthorizationRequestExecution,
) map[string][]byte {
	if result == nil ||
		result.Outcome != "success" ||
		result.Response.Truncated ||
		result.Response.AnalysisState == "encoded-unavailable" ||
		result.Response.AnalysisRepresentation == "binary" {
		return nil
	}
	body := result.responseBody
	if len(body) == 0 && len(result.responsePacket) > 0 {
		_, body = lowhttp.SplitHTTPPacketFast(result.responsePacket)
	}
	if len(body) == 0 {
		return nil
	}
	return flattenAuthorizationResponseSemanticLeaves(
		body,
		result.Response.ContentType,
	)
}

func authorizationResponseEvidenceFormat(
	result *ExtensionAuthorizationRequestExecution,
) string {
	if result == nil {
		return "structured"
	}
	contentType := strings.ToLower(result.Response.ContentType)
	switch {
	case strings.Contains(contentType, "json"):
		return "json"
	case strings.Contains(contentType, "html"):
		return "dom"
	case strings.Contains(contentType, "x-www-form-urlencoded"):
		return "form"
	default:
		return "structured"
	}
}

func deriveAuthorizationCanaryEvidence(
	direction string,
	otherOwn *ExtensionAuthorizationRequestExecution,
	targetOwn *ExtensionAuthorizationRequestExecution,
	cross *ExtensionAuthorizationRequestExecution,
	comparisonKey string,
	userPaths []string,
) []ExtensionAuthorizationCanaryEvidence {
	otherValues := authorizationResponseLeaves(otherOwn)
	targetValues := authorizationResponseLeaves(targetOwn)
	crossValues := authorizationResponseLeaves(cross)
	if len(otherValues) == 0 || len(targetValues) == 0 || len(crossValues) == 0 {
		return nil
	}
	paths := make([]string, 0, len(userPaths)+len(targetValues))
	userPathSet := make(map[string]struct{}, len(userPaths))
	seen := make(map[string]struct{}, len(userPaths)+len(targetValues))
	for _, path := range userPaths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		userPathSet[path] = struct{}{}
		paths = append(paths, path)
	}
	automaticPaths := make([]string, 0, len(targetValues))
	for path := range targetValues {
		if _, ok := seen[path]; ok || !authorizationResponseCanaryPath(path) {
			continue
		}
		automaticPaths = append(automaticPaths, path)
	}
	sort.Strings(automaticPaths)
	paths = append(paths, automaticPaths...)
	evidence := make([]ExtensionAuthorizationCanaryEvidence, 0, 8)
	for _, path := range paths {
		if len(evidence) >= 8 {
			break
		}
		targetValue := targetValues[path]
		otherValue, otherOK := otherValues[path]
		crossValue, crossOK := crossValues[path]
		if !otherOK || !crossOK ||
			bytes.Equal(targetValue, otherValue) ||
			!bytes.Equal(targetValue, crossValue) ||
			authorizationVolatileDiff(path, targetValue) ||
			authorizationVolatileDiff(path, otherValue) ||
			authorizationVolatileDiff(path, crossValue) {
			continue
		}
		fingerprint, err := authorizationComparisonFingerprint(comparisonKey, targetValue)
		if err != nil {
			continue
		}
		format := authorizationResponseEvidenceFormat(targetOwn)
		source := "response-" + format + "-differential"
		if _, ok := userPathSet[path]; ok {
			source = "response-" + format + "-user-canary"
		}
		evidence = append(evidence, ExtensionAuthorizationCanaryEvidence{
			Direction:        direction,
			Path:             path,
			ValueFingerprint: fingerprint,
			Source:           source,
		})
	}
	return evidence
}

func deriveVerticalAuthorizationCanaryEvidence(
	lowControl *ExtensionAuthorizationRequestExecution,
	privilegedControl *ExtensionAuthorizationRequestExecution,
	probe *ExtensionAuthorizationRequestExecution,
	comparisonKey string,
	userPaths []string,
) []ExtensionAuthorizationCanaryEvidence {
	lowValues := authorizationResponseLeaves(lowControl)
	privilegedValues := authorizationResponseLeaves(privilegedControl)
	probeValues := authorizationResponseLeaves(probe)
	if len(privilegedValues) == 0 || len(probeValues) == 0 {
		return nil
	}
	paths := make([]string, 0, len(userPaths)+len(privilegedValues))
	userPathSet := make(map[string]struct{}, len(userPaths))
	seen := make(map[string]struct{}, len(userPaths)+len(privilegedValues))
	for _, path := range userPaths {
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		userPathSet[path] = struct{}{}
		paths = append(paths, path)
	}
	automaticPaths := make([]string, 0, len(privilegedValues))
	for path := range privilegedValues {
		if _, exists := seen[path]; exists || !authorizationResponseCanaryPath(path) {
			continue
		}
		automaticPaths = append(automaticPaths, path)
	}
	sort.Strings(automaticPaths)
	paths = append(paths, automaticPaths...)
	evidence := make([]ExtensionAuthorizationCanaryEvidence, 0, 8)
	for _, path := range paths {
		if len(evidence) >= 8 {
			break
		}
		privilegedValue, privilegedExists := privilegedValues[path]
		probeValue, probeExists := probeValues[path]
		lowValue, lowExists := lowValues[path]
		if !privilegedExists ||
			!probeExists ||
			!bytes.Equal(privilegedValue, probeValue) ||
			(lowExists && bytes.Equal(lowValue, privilegedValue)) ||
			authorizationVolatileDiff(path, privilegedValue) ||
			authorizationVolatileDiff(path, probeValue) {
			continue
		}
		fingerprint, err := authorizationComparisonFingerprint(
			comparisonKey,
			privilegedValue,
		)
		if err != nil {
			continue
		}
		format := authorizationResponseEvidenceFormat(privilegedControl)
		source := "vertical-response-" + format + "-differential"
		if _, exists := userPathSet[path]; exists {
			source = "vertical-response-" + format + "-user-canary"
		}
		evidence = append(evidence, ExtensionAuthorizationCanaryEvidence{
			Direction:        "low-to-privileged",
			Path:             path,
			ValueFingerprint: fingerprint,
			Source:           source,
		})
	}
	return evidence
}

func deriveVerticalAuthorizationPostStateEvidence(
	before *ExtensionAuthorizationRequestExecution,
	after *ExtensionAuthorizationRequestExecution,
	comparisonKey string,
	userPaths []string,
) []ExtensionAuthorizationCanaryEvidence {
	beforeValues := authorizationResponseLeaves(before)
	afterValues := authorizationResponseLeaves(after)
	if len(beforeValues) == 0 || len(afterValues) == 0 {
		return nil
	}
	paths := make([]string, 0, len(userPaths)+len(afterValues))
	userPathSet := make(map[string]struct{}, len(userPaths))
	seen := make(map[string]struct{}, len(userPaths)+len(afterValues))
	for _, path := range userPaths {
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		userPathSet[path] = struct{}{}
		paths = append(paths, path)
	}
	automaticPaths := make([]string, 0, len(afterValues))
	for path := range afterValues {
		if _, exists := seen[path]; exists || !authorizationResponseCanaryPath(path) {
			continue
		}
		automaticPaths = append(automaticPaths, path)
	}
	sort.Strings(automaticPaths)
	paths = append(paths, automaticPaths...)
	evidence := make([]ExtensionAuthorizationCanaryEvidence, 0, 8)
	for _, path := range paths {
		if len(evidence) >= 8 {
			break
		}
		beforeValue, beforeExists := beforeValues[path]
		afterValue, afterExists := afterValues[path]
		if !beforeExists ||
			!afterExists ||
			bytes.Equal(beforeValue, afterValue) ||
			authorizationVolatileDiff(path, beforeValue) ||
			authorizationVolatileDiff(path, afterValue) {
			continue
		}
		fingerprint, err := authorizationComparisonFingerprint(
			comparisonKey,
			afterValue,
		)
		if err != nil {
			continue
		}
		format := authorizationResponseEvidenceFormat(after)
		source := "vertical-post-state-" + format + "-differential"
		if _, exists := userPathSet[path]; exists {
			source = "vertical-post-state-" + format + "-user-canary"
		}
		evidence = append(evidence, ExtensionAuthorizationCanaryEvidence{
			Direction:        "low-to-privileged",
			Path:             path,
			ValueFingerprint: fingerprint,
			Source:           source,
		})
	}
	return evidence
}

func evaluateExtensionAuthorizationExecution(
	execution *ExtensionAuthorizationExecution,
	comparisonKey string,
	userCanaryPaths ...string,
) {
	execution.State = "completed"
	execution.Verdict = "inconclusive"
	execution.Confidence = "none"
	if len(execution.Cases) < 2 ||
		execution.Cases[0].State != "completed" ||
		execution.Cases[1].State != "completed" {
		execution.State = "partial"
		execution.Reasons = append(execution.Reasons, "至少一个正常对照请求执行失败，未继续发送交叉请求")
		return
	}
	leftOwn := execution.Cases[0].Result
	rightOwn := execution.Cases[1].Result
	if leftOwn == nil || rightOwn == nil ||
		leftOwn.Outcome != "success" ||
		rightOwn.Outcome != "success" {
		execution.Verdict = "invalid-controls"
		execution.Confidence = "high"
		execution.Reasons = append(execution.Reasons, "至少一个正常对照请求未成功，交叉结果不具备解释基础")
		return
	}
	if len(execution.Cases) < 4 ||
		execution.Cases[2].State != "completed" ||
		execution.Cases[3].State != "completed" {
		execution.State = "partial"
		execution.Reasons = append(execution.Reasons, "至少一个交叉请求执行失败")
		return
	}
	leftToRight := execution.Cases[2].Result
	rightToLeft := execution.Cases[3].Result
	if leftToRight == nil || rightToLeft == nil {
		execution.State = "partial"
		execution.Reasons = append(execution.Reasons, "交叉请求没有返回结构化结果")
		return
	}
	if leftToRight.Outcome == "denied" && rightToLeft.Outcome == "denied" {
		execution.Verdict = "protected"
		execution.Confidence = "high"
		execution.Reasons = append(execution.Reasons, "两项正常对照成功，两个交叉身份请求均被拒绝")
		return
	}
	execution.Evidence = append(
		execution.Evidence,
		deriveAuthorizationCanaryEvidence(
			"a-to-b",
			leftOwn,
			rightOwn,
			leftToRight,
			comparisonKey,
			userCanaryPaths,
		)...,
	)
	execution.Evidence = append(
		execution.Evidence,
		deriveAuthorizationCanaryEvidence(
			"b-to-a",
			rightOwn,
			leftOwn,
			rightToLeft,
			comparisonKey,
			userCanaryPaths,
		)...,
	)
	if len(execution.Evidence) > 0 {
		execution.Verdict = "confirmed"
		execution.Confidence = "high"
		execution.Reasons = append(
			execution.Reasons,
			fmt.Sprintf("发现 %d 项响应字段 canary：交叉响应复现目标身份的差异资源值", len(execution.Evidence)),
		)
		return
	}
	controlsDistinct := !authorizationResponseExact(leftOwn, rightOwn)
	leftMatchedTarget := authorizationResponseExact(leftToRight, rightOwn) &&
		!authorizationResponseExact(leftToRight, leftOwn)
	rightMatchedTarget := authorizationResponseExact(rightToLeft, leftOwn) &&
		!authorizationResponseExact(rightToLeft, rightOwn)
	if controlsDistinct && (leftMatchedTarget || rightMatchedTarget) {
		execution.Verdict = "likely"
		execution.Confidence = "high"
		execution.Reasons = append(
			execution.Reasons,
			"至少一个交叉请求成功，且完整响应与资源所属身份的正常响应精确一致",
		)
		return
	}
	if leftToRight.Outcome == "success" || rightToLeft.Outcome == "success" {
		execution.Verdict = "inconclusive"
		execution.Confidence = "low"
		execution.Reasons = append(execution.Reasons, "至少一个交叉身份请求返回成功，但 2xx 状态本身不能证明越权")
		if authorizationResponseShape(leftToRight, rightOwn) ||
			authorizationResponseShape(rightToLeft, leftOwn) {
			execution.Reasons = append(execution.Reasons, "交叉响应与目标资源正常响应具有相同 JSON 结构；结构相同仍不足以证明读取了目标对象")
		}
		return
	}
	execution.Confidence = "low"
	execution.Reasons = append(execution.Reasons, "交叉响应既未明确拒绝，也不足以证明读取了另一身份的资源")
}

func authorizationExecutionCaseByID(
	execution *ExtensionAuthorizationExecution,
	id string,
) *ExtensionAuthorizationCaseExecution {
	if execution == nil {
		return nil
	}
	for index := range execution.Cases {
		if execution.Cases[index].ID == id {
			return &execution.Cases[index]
		}
	}
	return nil
}

func evaluateVerticalAuthorizationExecution(
	execution *ExtensionAuthorizationExecution,
	comparisonKey string,
	userCanaryPaths ...string,
) {
	execution.State = "completed"
	execution.Verdict = "inconclusive"
	execution.Confidence = "none"
	lowControl := authorizationExecutionCaseByID(execution, "low-control")
	privilegedControl := authorizationExecutionCaseByID(execution, "privileged-baseline")
	probe := authorizationExecutionCaseByID(execution, "low-privileged-probe")
	before := authorizationExecutionCaseByID(execution, "post-state-before")
	after := authorizationExecutionCaseByID(execution, "post-state-after")
	hasPostState := before != nil && after != nil
	if lowControl == nil ||
		privilegedControl == nil ||
		probe == nil ||
		(len(execution.Cases) != 3 && len(execution.Cases) != 5) ||
		(len(execution.Cases) == 5 && !hasPostState) ||
		(len(execution.Cases) == 3 && (before != nil || after != nil)) {
		execution.State = "partial"
		execution.Reasons = append(
			execution.Reasons,
			"纵向授权矩阵结构不完整，未生成结论",
		)
		return
	}
	if lowControl.State != "completed" ||
		privilegedControl.State != "completed" ||
		lowControl.Result == nil ||
		privilegedControl.Result == nil {
		execution.State = "partial"
		execution.Reasons = append(
			execution.Reasons,
			"低权限正常请求或高权限特权操作对照执行失败，未发送低权限探测",
		)
		return
	}
	if lowControl.Result.Outcome != "success" ||
		privilegedControl.Result.Outcome != "success" {
		execution.Verdict = "invalid-controls"
		execution.Confidence = "high"
		execution.Reasons = append(
			execution.Reasons,
			"低权限正常请求与高权限特权操作必须同时成功，当前结果不具备纵向解释基础",
		)
		return
	}
	if hasPostState {
		if before.State != "completed" || before.Result == nil {
			execution.State = "partial"
			execution.Reasons = append(
				execution.Reasons,
				"高权限操作后的状态前快照执行失败，未发送低权限探测",
			)
			return
		}
		if before.Result.Outcome != "success" {
			execution.Verdict = "invalid-controls"
			execution.Confidence = "high"
			execution.Reasons = append(
				execution.Reasons,
				"后置状态验证请求的前快照未成功，不能解释探测后的状态变化",
			)
			return
		}
	}
	if probe.State != "completed" || probe.Result == nil {
		execution.State = "partial"
		execution.Reasons = append(
			execution.Reasons,
			"低权限特权操作探测没有返回结构化结果",
		)
		return
	}
	if probe.Result.Outcome == "denied" {
		execution.Verdict = "protected"
		execution.Confidence = "high"
		execution.Reasons = append(
			execution.Reasons,
			"低权限正常请求与高权限特权操作对照均成功，而低权限特权操作被明确拒绝",
		)
		return
	}
	execution.Evidence = append(
		execution.Evidence,
		deriveVerticalAuthorizationCanaryEvidence(
			lowControl.Result,
			privilegedControl.Result,
			probe.Result,
			comparisonKey,
			userCanaryPaths,
		)...,
	)
	if probe.Result.Outcome == "success" {
		if hasPostState {
			if after.State != "completed" || after.Result == nil {
				execution.State = "partial"
				execution.Reasons = append(
					execution.Reasons,
					"低权限探测成功，但状态后快照执行失败，无法形成独立后置状态证据",
				)
				return
			}
			if after.Result.Outcome != "success" {
				execution.Reasons = append(
					execution.Reasons,
					"低权限探测成功，但状态后快照未成功，无法形成独立后置状态证据",
				)
			} else {
				postStateEvidence := deriveVerticalAuthorizationPostStateEvidence(
					before.Result,
					after.Result,
					comparisonKey,
					userCanaryPaths,
				)
				execution.Evidence = append(execution.Evidence, postStateEvidence...)
				if len(postStateEvidence) > 0 {
					execution.Verdict = "confirmed"
					execution.Confidence = "high"
					execution.Reasons = append(
						execution.Reasons,
						fmt.Sprintf(
							"低权限探测成功，并在独立只读快照中确认 %d 项业务状态发生变化",
							len(postStateEvidence),
						),
					)
					return
				}
				execution.Reasons = append(
					execution.Reasons,
					"低权限探测成功，但独立只读快照未发现所选业务字段变化",
				)
			}
		}
		targetExact := authorizationResponseExact(
			probe.Result,
			privilegedControl.Result,
		)
		controlsDistinct := !authorizationResponseExact(
			lowControl.Result,
			privilegedControl.Result,
		)
		if len(execution.Evidence) > 0 || (targetExact && controlsDistinct) {
			execution.Verdict = "likely"
			execution.Confidence = "high"
			if len(execution.Evidence) > 0 {
				execution.Reasons = append(
					execution.Reasons,
					fmt.Sprintf(
						"低权限探测复现了 %d 项高权限响应业务字段，但独立后置状态证据仍不足，因此不提升为 confirmed",
						len(execution.Evidence),
					),
				)
			} else {
				execution.Reasons = append(
					execution.Reasons,
					"低权限探测与高权限特权操作的完整响应精确一致，且与低权限正常控制不同；尚缺独立后置状态证据",
				)
			}
			return
		}
		execution.Verdict = "inconclusive"
		execution.Confidence = "low"
		execution.Reasons = append(
			execution.Reasons,
			"低权限特权操作返回成功，但 2xx 状态本身不能证明纵向越权",
		)
		if authorizationResponseShape(probe.Result, privilegedControl.Result) {
			execution.Reasons = append(
				execution.Reasons,
				"低权限探测与高权限对照具有相同响应结构；结构相同仍不足以证明操作生效",
			)
		}
		return
	}
	execution.Confidence = "low"
	execution.Reasons = append(
		execution.Reasons,
		"低权限特权操作既未明确拒绝，也没有足够证据证明操作生效",
	)
}
