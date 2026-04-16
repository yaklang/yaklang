//go:build hids

package rule

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
	builtinrules "github.com/yaklang/yaklang/common/hids/rule/builtin"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

type Engine struct {
	sandbox *yak.Sandbox
	builtin []builtinrules.Rule
	rules   []compiledRule
}

type compiledRule struct {
	rule model.TemporaryRule
}

func NewEngine(spec model.DesiredSpec) (*Engine, error) {
	engine := &Engine{
		sandbox: NewSandbox(),
	}
	builtinRules, err := builtinrules.Compile(spec.BuiltinRuleSets)
	if err != nil {
		return nil, err
	}
	engine.builtin = builtinRules

	for i, rule := range spec.TemporaryRules {
		if !rule.Enabled || rule.IsBlank() {
			continue
		}
		if err := engine.validateRule(i, rule); err != nil {
			return nil, err
		}
		engine.rules = append(engine.rules, compiledRule{
			rule: rule,
		})
	}

	return engine, nil
}

func (e *Engine) Evaluate(event model.Event) []model.Alert {
	if e == nil {
		return nil
	}

	alerts := builtinrules.Evaluate(e.builtin, event)
	if e.sandbox == nil || len(e.rules) == 0 {
		return alerts
	}

	context := buildEvalContext(event)
	for _, compiled := range e.rules {
		rule := compiled.rule
		if !rule.Enabled || rule.MatchEventType != event.Type {
			continue
		}

		matched, err := e.sandbox.ExecuteAsBoolean(rule.Condition, context)
		if err != nil {
			log.Warnf("hids temporary rule evaluation failed: rule_id=%s err=%v", rule.RuleID, err)
			continue
		}
		if !matched {
			continue
		}

		actionResult, actionErr := e.evaluateAction(rule, context)
		alerts = append(alerts, buildAlert(rule, event, context, actionResult, actionErr))
	}
	return alerts
}

func (e *Engine) validateRule(index int, rule model.TemporaryRule) error {
	if e == nil || e.sandbox == nil {
		return fmt.Errorf("rule sandbox is not initialized")
	}
	_, err := e.sandbox.ExecuteAsBoolean(rule.Condition, buildValidationContext(rule.MatchEventType))
	if err != nil {
		return &model.ValidationError{
			Field:  fmt.Sprintf("temporary_rules[%d].condition", index),
			Reason: fmt.Sprintf("invalid yak rule condition: %v", err),
		}
	}
	if err := validateActionExpression(e.sandbox, rule.Action, buildValidationContext(rule.MatchEventType)); err != nil {
		return &model.ValidationError{
			Field:  fmt.Sprintf("temporary_rules[%d].action", index),
			Reason: fmt.Sprintf("invalid yak rule action: %v", err),
		}
	}
	return nil
}

func (e *Engine) evaluateAction(
	rule model.TemporaryRule,
	context map[string]any,
) (temporaryRuleActionResult, error) {
	if e == nil || e.sandbox == nil || strings.TrimSpace(rule.Action) == "" {
		return temporaryRuleActionResult{}, nil
	}

	raw, err := e.sandbox.ExecuteAsExpression(rule.Action, context)
	if err != nil {
		return temporaryRuleActionResult{}, err
	}
	return parseTemporaryRuleActionResult(raw)
}

func buildAlert(
	rule model.TemporaryRule,
	event model.Event,
	context map[string]any,
	actionResult temporaryRuleActionResult,
	actionErr error,
) model.Alert {
	detail := map[string]any{
		"rule_id":          rule.RuleID,
		"source":           "temporary",
		"match_event_type": rule.MatchEventType,
		"condition":        rule.Condition,
		"event":            cloneValue(context["event"]),
	}
	if strings.TrimSpace(rule.Description) != "" {
		detail["rule_description"] = rule.Description
	}
	if len(rule.Metadata) > 0 {
		detail["rule_metadata"] = cloneValue(rule.Metadata)
	}
	if strings.TrimSpace(rule.Action) != "" {
		detail["action_script"] = rule.Action
	}
	if actionErr != nil {
		detail["action_error"] = actionErr.Error()
	}
	if actionResultMap := temporaryRuleActionResultMap(actionResult); len(actionResultMap) > 0 {
		detail["action_result"] = actionResultMap
	}
	if len(actionResult.Detail) > 0 {
		for key, value := range actionResult.Detail {
			detail[key] = cloneValue(value)
		}
	}
	if len(actionResult.EvidenceRequests) > 0 {
		detail["evidence_requests"] = cloneValue(actionResult.EvidenceRequests)
	}

	observedAt := event.Timestamp.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	title := strings.TrimSpace(rule.Title)
	if title == "" {
		title = fmt.Sprintf("hids rule matched: %s", rule.RuleID)
	}
	if actionResult.Title != "" {
		title = actionResult.Title
	}

	severity := strings.TrimSpace(rule.Severity)
	if actionResult.Severity != "" {
		severity = actionResult.Severity
	}

	tags := mergeTags(rule.Tags, event.Tags)
	tags = mergeTags(tags, actionResult.Tags)

	return model.Alert{
		RuleID:     rule.RuleID,
		Severity:   severity,
		Title:      title,
		Tags:       tags,
		Detail:     detail,
		ObservedAt: observedAt,
	}
}

func buildEvalContext(event model.Event) map[string]any {
	process := map[string]any{
		"pid":         0,
		"parent_pid":  0,
		"name":        "",
		"username":    "",
		"image":       "",
		"command":     "",
		"parent_name": "",
		"artifact":    buildArtifactContext(nil),
	}
	parent := map[string]any{
		"pid":  0,
		"name": "",
	}
	file := map[string]any{
		"path":      "",
		"operation": "",
		"is_dir":    false,
		"mode":      "",
		"uid":       "",
		"gid":       "",
		"owner":     "",
		"group":     "",
		"artifact":  buildArtifactContext(nil),
	}
	network := map[string]any{
		"protocol":         "",
		"source_address":   "",
		"dest_address":     "",
		"source_port":      0,
		"dest_port":        0,
		"connection_state": "",
	}
	audit := map[string]any{
		"sequence":            0,
		"record_types":        []string{},
		"family":              "",
		"category":            "",
		"record_type":         "",
		"result":              "",
		"session_id":          "",
		"action":              "",
		"object_type":         "",
		"object_primary":      "",
		"object_secondary":    "",
		"how":                 "",
		"username":            "",
		"uid":                 "",
		"login_user":          "",
		"auid":                "",
		"terminal":            "",
		"remote_ip":           "",
		"remote_port":         "",
		"remote_host":         "",
		"process_cwd":         "",
		"file_mode":           "",
		"file_uid":            "",
		"file_gid":            "",
		"file_owner":          "",
		"file_group":          "",
		"previous_file_mode":  "",
		"previous_file_uid":   "",
		"previous_file_gid":   "",
		"previous_file_owner": "",
		"previous_file_group": "",
	}
	if event.Process != nil {
		process["pid"] = event.Process.PID
		process["parent_pid"] = event.Process.ParentPID
		process["name"] = event.Process.Name
		process["username"] = event.Process.Username
		process["image"] = event.Process.Image
		process["command"] = event.Process.Command
		process["parent_name"] = event.Process.ParentName
		process["artifact"] = buildArtifactContext(event.Process.Artifact)
		parent["pid"] = event.Process.ParentPID
		parent["name"] = event.Process.ParentName
	}
	if event.File != nil {
		file["path"] = event.File.Path
		file["operation"] = event.File.Operation
		file["is_dir"] = event.File.IsDir
		file["mode"] = event.File.Mode
		file["uid"] = event.File.UID
		file["gid"] = event.File.GID
		file["owner"] = event.File.Owner
		file["group"] = event.File.Group
		file["artifact"] = buildArtifactContext(event.File.Artifact)
	}
	if event.Network != nil {
		network["protocol"] = event.Network.Protocol
		network["source_address"] = event.Network.SourceAddress
		network["dest_address"] = event.Network.DestAddress
		network["source_port"] = event.Network.SourcePort
		network["dest_port"] = event.Network.DestPort
		network["connection_state"] = event.Network.ConnectionState
	}
	if event.Audit != nil {
		audit["sequence"] = event.Audit.Sequence
		audit["record_types"] = cloneStringSlice(event.Audit.RecordTypes)
		audit["family"] = event.Audit.Family
		audit["category"] = event.Audit.Category
		audit["record_type"] = event.Audit.RecordType
		audit["result"] = event.Audit.Result
		audit["session_id"] = event.Audit.SessionID
		audit["action"] = event.Audit.Action
		audit["object_type"] = event.Audit.ObjectType
		audit["object_primary"] = event.Audit.ObjectPrimary
		audit["object_secondary"] = event.Audit.ObjectSecondary
		audit["how"] = event.Audit.How
		audit["username"] = event.Audit.Username
		audit["uid"] = event.Audit.UID
		audit["login_user"] = event.Audit.LoginUser
		audit["auid"] = event.Audit.AUID
		audit["terminal"] = event.Audit.Terminal
		audit["remote_ip"] = event.Audit.RemoteIP
		audit["remote_port"] = event.Audit.RemotePort
		audit["remote_host"] = event.Audit.RemoteHost
		audit["process_cwd"] = event.Audit.ProcessCWD
		audit["file_mode"] = event.Audit.FileMode
		audit["file_uid"] = event.Audit.FileUID
		audit["file_gid"] = event.Audit.FileGID
		audit["file_owner"] = event.Audit.FileOwner
		audit["file_group"] = event.Audit.FileGroup
		audit["previous_file_mode"] = event.Audit.PreviousFileMode
		audit["previous_file_uid"] = event.Audit.PreviousFileUID
		audit["previous_file_gid"] = event.Audit.PreviousFileGID
		audit["previous_file_owner"] = event.Audit.PreviousFileOwner
		audit["previous_file_group"] = event.Audit.PreviousFileGroup
	}

	timestamp := ""
	timestampUnix := int64(0)
	if !event.Timestamp.IsZero() {
		ts := event.Timestamp.UTC()
		timestamp = ts.Format(time.RFC3339Nano)
		timestampUnix = ts.Unix()
	}

	labels := cloneStringMap(event.Labels)
	tags := cloneStringSlice(event.Tags)
	data := defaultEventData(event.Type)
	for key, value := range cloneMapStringAny(event.Data) {
		data[key] = value
	}
	eventValue := map[string]any{
		"type":           event.Type,
		"source":         event.Source,
		"timestamp":      timestamp,
		"timestamp_unix": timestampUnix,
		"tags":           cloneStringSlice(tags),
		"labels":         cloneStringMap(labels),
		"process":        cloneMapStringAny(process),
		"parent":         cloneMapStringAny(parent),
		"file":           cloneMapStringAny(file),
		"network":        cloneMapStringAny(network),
		"audit":          cloneMapStringAny(audit),
		"artifact":       buildPrimaryArtifactContext(event),
		"data":           cloneMapStringAny(data),
	}

	return map[string]any{
		"event":    eventValue,
		"process":  process,
		"parent":   parent,
		"file":     file,
		"network":  network,
		"audit":    audit,
		"artifact": buildPrimaryArtifactContext(event),
		"labels":   labels,
		"tags":     tags,
		"data":     data,
	}
}

func buildPrimaryArtifactContext(event model.Event) map[string]any {
	if event.File != nil && event.File.Artifact != nil {
		return buildArtifactContext(event.File.Artifact)
	}
	if event.Process != nil && event.Process.Artifact != nil {
		return buildArtifactContext(event.Process.Artifact)
	}
	return buildArtifactContext(nil)
}

func buildArtifactContext(artifact *model.Artifact) map[string]any {
	hashes := map[string]any{
		"sha256": "",
		"md5":    "",
	}
	elfValue := map[string]any{
		"class":         "",
		"machine":       "",
		"byte_order":    "",
		"entry_address": "",
		"section_count": 0,
		"segment_count": 0,
		"sections":      []string{},
		"segments":      []string{},
	}
	value := map[string]any{
		"path":        "",
		"exists":      false,
		"size_bytes":  int64(0),
		"file_type":   "",
		"type_source": "",
		"magic":       "",
		"mime_type":   "",
		"extension":   "",
		"hashes":      hashes,
		"elf":         elfValue,
	}
	if artifact == nil {
		return value
	}

	value["path"] = artifact.Path
	value["exists"] = artifact.Exists
	value["size_bytes"] = artifact.SizeBytes
	value["file_type"] = artifact.FileType
	value["type_source"] = artifact.TypeSource
	value["magic"] = artifact.Magic
	value["mime_type"] = artifact.MimeType
	value["extension"] = artifact.Extension
	if artifact.Hashes != nil {
		hashes["sha256"] = artifact.Hashes.SHA256
		hashes["md5"] = artifact.Hashes.MD5
	}
	if artifact.ELF != nil {
		elfValue["class"] = artifact.ELF.Class
		elfValue["machine"] = artifact.ELF.Machine
		elfValue["byte_order"] = artifact.ELF.ByteOrder
		elfValue["entry_address"] = artifact.ELF.EntryAddress
		elfValue["section_count"] = artifact.ELF.SectionCount
		elfValue["segment_count"] = artifact.ELF.SegmentCount
		elfValue["sections"] = cloneStringSlice(artifact.ELF.Sections)
		elfValue["segments"] = cloneStringSlice(artifact.ELF.Segments)
	}
	return value
}

func buildValidationContext(eventType string) map[string]any {
	return buildEvalContext(model.Event{
		Type:      eventType,
		Source:    "validation",
		Timestamp: time.Unix(0, 0).UTC(),
		Tags:      []string{},
		Labels:    map[string]string{},
		Data:      map[string]any{},
	})
}

func defaultEventData(eventType string) map[string]any {
	data := map[string]any{}
	switch eventType {
	case model.EventTypeNetworkAccept, model.EventTypeNetworkConnect, model.EventTypeNetworkState, model.EventTypeNetworkClose:
		data["direction"] = ""
		data["source_scope"] = ""
		data["dest_scope"] = ""
		data["source_service"] = ""
		data["dest_service"] = ""
		data["process_roles"] = []string{}
		data["parent_roles"] = []string{}
		data["previous_connection_state"] = ""
		data["connection_opened_at_unix"] = int64(0)
		data["state_changed_at_unix"] = int64(0)
		data["connection_age_seconds"] = int64(0)
		data["state_age_seconds"] = int64(0)
		data["previous_state_age_seconds"] = int64(0)
	}
	return data
}

func allowedHelpers() map[string]any {
	return map[string]any{
		"str": map[string]any{
			"Contains":    yaklib.StringsExport["Contains"],
			"HasPrefix":   yaklib.StringsExport["HasPrefix"],
			"HasSuffix":   yaklib.StringsExport["HasSuffix"],
			"TrimSpace":   yaklib.StringsExport["TrimSpace"],
			"ToLower":     yaklib.StringsExport["ToLower"],
			"ToUpper":     yaklib.StringsExport["ToUpper"],
			"EqualFold":   yaklib.StringsExport["EqualFold"],
			"RegexpMatch": yaklib.StringsExport["RegexpMatch"],
		},
		"list": map[string]any{
			"Contains": listContains,
		},
		"path": map[string]any{
			"Normalize": pathNormalize,
			"Glob":      pathGlob,
			"AnyGlob":   pathAnyGlob,
			"Under":     pathUnder,
			"AnyUnder":  pathAnyUnder,
		},
		"artifact": map[string]any{
			"Path":         artifactPath,
			"Exists":       artifactExists,
			"FileType":     artifactFileType,
			"FileTypeIs":   artifactFileTypeIs,
			"IsELF":        artifactIsELF,
			"SHA256":       artifactSHA256,
			"SHA256In":     artifactSHA256In,
			"MD5":          artifactMD5,
			"MD5In":        artifactMD5In,
			"Machine":      artifactMachine,
			"PathGlob":     artifactPathGlob,
			"PathAnyGlob":  artifactPathAnyGlob,
			"PathUnder":    artifactPathUnder,
			"PathAnyUnder": artifactPathAnyUnder,
		},
		"auditx": map[string]any{
			"FamilyIs":      auditFamilyIs,
			"ResultIs":      auditResultIs,
			"ActionIs":      auditActionIs,
			"HasRecordType": auditHasRecordType,
			"AnyRecordType": auditAnyRecordType,
			"HasRemotePeer": auditHasRemotePeer,
		},
	}
}

func listContains(values any, want any) bool {
	if values == nil {
		return false
	}
	value := reflect.ValueOf(values)
	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return false
	}

	for i := 0; i < value.Len(); i++ {
		if reflect.DeepEqual(value.Index(i).Interface(), want) {
			return true
		}
	}
	return false
}

func mergeTags(left []string, right []string) []string {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(left)+len(right))
	merged := make([]string, 0, len(left)+len(right))
	for _, tags := range [][]string{left, right} {
		for _, tag := range tags {
			if _, exists := seen[tag]; exists {
				continue
			}
			seen[tag] = struct{}{}
			merged = append(merged, tag)
		}
	}
	return merged
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneMapStringAny(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMapStringAny(typed)
	case map[string]string:
		return cloneStringMap(typed)
	case []string:
		return cloneStringSlice(typed)
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneValue(item))
		}
		return cloned
	default:
		return typed
	}
}
