//go:build hids && linux

package auditd

import (
	"encoding/json"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/go-libaudit/v2/aucoalesce"
	"github.com/elastic/go-libaudit/v2/auparse"

	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/hids/policy"
)

func buildAuditObservation(
	msgs []*auparse.AuditMessage,
	journalAvailable bool,
) (model.Event, auditObservationOutcome) {
	outcome := auditObservationOutcome{}
	recordTypes := auditRecordTypes(msgs)
	event := model.Event{
		Type:      model.EventTypeAudit,
		Source:    "auditd",
		Timestamp: firstAuditTimestamp(msgs),
		Tags:      []string{"audit", "auditd"},
		Labels: map[string]string{
			"journal_available": strconv.FormatBool(journalAvailable),
		},
		Audit: &model.Audit{
			Sequence:    firstAuditSequence(msgs),
			RecordTypes: recordTypes,
		},
		Data: map[string]any{
			"messages": buildAuditMessagePayload(msgs),
		},
	}

	normalized, err := aucoalesce.CoalesceMessages(msgs)
	if err != nil {
		loss := buildAuditRuntimeLossObservation("normalize-error", err, journalAvailable)
		loss.Data["record_types"] = append([]string(nil), recordTypes...)
		loss.Data["sequence"] = firstAuditSequence(msgs)
		return loss, auditObservationOutcome{
			event:          loss,
			keep:           true,
			family:         "loss",
			filterReason:   "runtime.normalize-error",
			normalizeError: true,
		}
	}
	if normalized == nil {
		return model.Event{}, auditObservationOutcome{
			keep:         false,
			filterReason: "normalize.empty",
		}
	}

	family := classifyAuditFamily(normalized, recordTypes)
	outcome.family = family
	audit := buildAuditEnvelope(normalized, family, recordTypes)
	event.Audit = &audit
	event.Process = buildAuditProcess(normalized)
	event.File = buildAuditFile(normalized, audit.Action)
	if event.Audit != nil && event.File != nil && strings.TrimSpace(event.Audit.FileMode) == "" {
		event.Audit.FileMode = strings.TrimSpace(event.File.Mode)
	}
	event.Timestamp = normalized.Timestamp.UTC()
	event.Tags = mergeAuditTags(
		event.Tags,
		[]string{family, normalized.Category.String(), normalized.Type.String()},
	)
	if audit.Result != "" && audit.Result != "unknown" {
		event.Tags = mergeAuditTags(event.Tags, []string{audit.Result})
	}
	event.Labels["audit_family"] = family
	event.Labels["audit_category"] = audit.Category
	event.Labels["audit_record_type"] = audit.RecordType
	event.Labels["audit_action"] = audit.Action
	event.Labels["audit_result"] = audit.Result
	event.Labels["audit_normalized"] = "true"
	if audit.SessionID != "" {
		event.Labels["audit_session_id"] = audit.SessionID
	}
	if audit.Username != "" {
		event.Labels["audit_user"] = audit.Username
	}
	if normalizedMap := toAuditMap(normalized); len(normalizedMap) > 0 {
		event.Data["normalized"] = normalizedMap
	}
	keep, reason := shouldKeepAuditObservation(event)
	if !keep {
		outcome.filterReason = reason
		return model.Event{}, outcome
	}
	outcome.keep = true
	outcome.event = event
	return event, outcome
}

type auditObservationOutcome struct {
	event          model.Event
	keep           bool
	family         string
	filterReason   string
	normalizeError bool
}

func buildAuditLossObservation(count int, journalAvailable bool) model.Event {
	return model.Event{
		Type:      model.EventTypeAuditLoss,
		Source:    "auditd",
		Timestamp: time.Now().UTC(),
		Tags:      []string{"audit", "auditd", "loss"},
		Labels: map[string]string{
			"journal_available": strconv.FormatBool(journalAvailable),
			"audit_family":      "loss",
			"audit_category":    "audit-daemon",
			"audit_action":      "events-lost",
			"audit_result":      "unknown",
		},
		Audit: &model.Audit{
			Family:     "loss",
			Category:   "audit-daemon",
			Result:     "unknown",
			Action:     "events-lost",
			RecordType: "EVENTS_LOST",
		},
		Data: map[string]any{
			"lost_count": count,
		},
	}
}

func buildAuditRuntimeLossObservation(
	action string,
	err error,
	journalAvailable bool,
) model.Event {
	if strings.TrimSpace(action) == "" {
		action = "runtime-error"
	}

	detail := map[string]any{}
	if err != nil {
		detail["error"] = err.Error()
	}

	return model.Event{
		Type:      model.EventTypeAuditLoss,
		Source:    "auditd",
		Timestamp: time.Now().UTC(),
		Tags:      []string{"audit", "auditd", "loss", "runtime"},
		Labels: map[string]string{
			"journal_available": strconv.FormatBool(journalAvailable),
			"audit_family":      "loss",
			"audit_category":    "audit-runtime",
			"audit_action":      action,
			"audit_result":      "unknown",
		},
		Audit: &model.Audit{
			Family:     "loss",
			Category:   "audit-runtime",
			Result:     "unknown",
			Action:     action,
			RecordType: "RUNTIME_ERROR",
		},
		Data: detail,
	}
}

func buildAuditEnvelope(
	event *aucoalesce.Event,
	family string,
	recordTypes []string,
) model.Audit {
	if event == nil {
		return model.Audit{Family: family, RecordTypes: append([]string(nil), recordTypes...)}
	}
	recordTypeSet := buildAuditRecordTypeSet(recordTypes)

	uid := strings.TrimSpace(event.User.IDs["uid"])
	auid := strings.TrimSpace(event.User.IDs["auid"])
	username := strings.TrimSpace(firstNonEmpty(
		event.User.Names["uid"],
		lookupAuditUsername(uid),
		event.Data["acct"],
		event.Summary.Actor.Secondary,
	))
	loginUser := strings.TrimSpace(firstNonEmpty(
		event.User.Names["auid"],
		lookupAuditUsername(auid),
		event.Summary.Actor.Primary,
	))

	return model.Audit{
		Sequence:        event.Sequence,
		RecordTypes:     append([]string(nil), recordTypes...),
		Family:          family,
		Category:        event.Category.String(),
		RecordType:      event.Type.String(),
		Result:          normalizeAuditResultFromEvent(event),
		SessionID:       strings.TrimSpace(event.Session),
		Action:          normalizeAuditAction(event, family, recordTypeSet),
		ObjectType:      strings.TrimSpace(event.Summary.Object.Type),
		ObjectPrimary:   strings.TrimSpace(event.Summary.Object.Primary),
		ObjectSecondary: strings.TrimSpace(event.Summary.Object.Secondary),
		How:             strings.TrimSpace(firstNonEmpty(event.Summary.How, event.Process.Exe, event.Process.Title)),
		Username:        username,
		UID:             uid,
		LoginUser:       loginUser,
		AUID:            auid,
		Terminal:        strings.TrimSpace(firstNonEmpty(event.Data["terminal"], event.Data["tty"])),
		RemoteIP:        strings.TrimSpace(firstNonEmpty(valueAddressField(event.Source, "ip"), event.Data["addr"])),
		RemotePort:      strings.TrimSpace(firstNonEmpty(valueAddressField(event.Source, "port"), event.Data["port"])),
		RemoteHost:      strings.TrimSpace(firstNonEmpty(valueAddressField(event.Source, "hostname"), event.Data["hostname"])),
		ProcessCWD:      strings.TrimSpace(event.Process.CWD),
		FileMode:        strings.TrimSpace(auditFileField(event, func(file *aucoalesce.File) string { return file.Mode })),
		FileUID:         strings.TrimSpace(auditFileField(event, func(file *aucoalesce.File) string { return file.UID })),
		FileGID:         strings.TrimSpace(auditFileField(event, func(file *aucoalesce.File) string { return file.GID })),
		FileOwner:       strings.TrimSpace(auditFileField(event, func(file *aucoalesce.File) string { return file.Owner })),
		FileGroup:       strings.TrimSpace(auditFileField(event, func(file *aucoalesce.File) string { return file.Group })),
	}
}

func buildAuditProcess(event *aucoalesce.Event) *model.Process {
	if event == nil || event.Process.IsEmpty() {
		return nil
	}

	command := strings.TrimSpace(strings.Join(event.Process.Args, " "))
	if command == "" {
		command = strings.TrimSpace(firstNonEmpty(event.Process.Title, event.Process.Exe, event.Process.Name))
	}

	return &model.Process{
		PID:       parseAuditInt(event.Process.PID),
		ParentPID: parseAuditInt(event.Process.PPID),
		Name:      strings.TrimSpace(event.Process.Name),
		Username: strings.TrimSpace(firstNonEmpty(
			event.User.Names["uid"],
			lookupAuditUsername(event.User.IDs["uid"]),
			event.Data["acct"],
		)),
		Image:   strings.TrimSpace(event.Process.Exe),
		Command: command,
	}
}

func buildAuditFile(event *aucoalesce.Event, action string) *model.File {
	if event == nil {
		return nil
	}

	filePath := strings.TrimSpace(firstNonEmpty(
		auditFileField(event, func(file *aucoalesce.File) string { return file.Path }),
		event.Summary.Object.Primary,
		event.Summary.Object.Secondary,
		event.Data["name"],
	))
	if filePath == "" && event.File == nil {
		return nil
	}

	objectType := strings.TrimSpace(event.Summary.Object.Type)
	if action == "" {
		action = normalizeSyscallAction(strings.TrimSpace(event.Data["syscall"]))
	}

	return &model.File{
		Path:      filePath,
		Operation: action,
		IsDir:     objectType == "directory",
		Mode:      strings.TrimSpace(auditFileField(event, func(file *aucoalesce.File) string { return file.Mode })),
		UID:       strings.TrimSpace(auditFileField(event, func(file *aucoalesce.File) string { return file.UID })),
		GID:       strings.TrimSpace(auditFileField(event, func(file *aucoalesce.File) string { return file.GID })),
		Owner:     strings.TrimSpace(auditFileField(event, func(file *aucoalesce.File) string { return file.Owner })),
		Group:     strings.TrimSpace(auditFileField(event, func(file *aucoalesce.File) string { return file.Group })),
	}
}

func buildAuditMessagePayload(msgs []*auparse.AuditMessage) []map[string]any {
	if len(msgs) == 0 {
		return nil
	}

	payload := make([]map[string]any, 0, len(msgs))
	for _, msg := range msgs {
		detail := map[string]any{
			"record_type":    msg.RecordType.String(),
			"record_type_id": uint32(msg.RecordType),
			"raw":            msg.RawData,
		}
		if data, err := msg.Data(); err == nil && len(data) > 0 {
			detail["data"] = data
		}
		payload = append(payload, detail)
	}
	return payload
}

func auditRecordTypes(msgs []*auparse.AuditMessage) []string {
	if len(msgs) == 0 {
		return nil
	}
	recordTypes := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		recordTypes = append(recordTypes, msg.RecordType.String())
	}
	return recordTypes
}

func firstAuditTimestamp(msgs []*auparse.AuditMessage) time.Time {
	if len(msgs) == 0 || msgs[0] == nil {
		return time.Now().UTC()
	}
	return msgs[0].Timestamp.UTC()
}

func firstAuditSequence(msgs []*auparse.AuditMessage) uint32 {
	if len(msgs) == 0 || msgs[0] == nil {
		return 0
	}
	return msgs[0].Sequence
}

func classifyAuditFamily(event *aucoalesce.Event, recordTypes []string) string {
	if event == nil {
		return "unknown"
	}

	recordTypeSet := buildAuditRecordTypeSet(recordTypes)

	switch {
	case hasAuditRecordType(recordTypeSet, "EXECVE", "USER_CMD"):
		return "command"
	case event.Category == aucoalesce.EventTypeUserLogin ||
		hasAuditRecordType(recordTypeSet, "USER_LOGIN", "USER_AUTH", "USER_ACCT", "CRED_ACQ"):
		return "login"
	case event.Category == aucoalesce.EventTypeUserAccount ||
		event.Category == aucoalesce.EventTypeGroupChange ||
		hasAuditRecordType(
			recordTypeSet,
			"USER_ROLE_CHANGE",
			"ADD_USER",
			"DEL_USER",
			"ADD_GROUP",
			"DEL_GROUP",
			"GRP_MGMT",
			"GRP_CHAUTHTOK",
			"USER_CHAUTHTOK",
			"CHUSER_ID",
			"CHGRP_ID",
		):
		return "privilege"
	case hasAuditRecordType(recordTypeSet, "USER_START", "USER_END"):
		return "session"
	case event.File != nil || event.Summary.Object.Type == "file" || event.Summary.Object.Type == "directory":
		return "file"
	default:
		return "unknown"
	}
}

func buildAuditRecordTypeSet(recordTypes []string) map[string]struct{} {
	recordTypeSet := make(map[string]struct{}, len(recordTypes))
	for _, recordType := range recordTypes {
		recordTypeSet[strings.ToUpper(strings.TrimSpace(recordType))] = struct{}{}
	}
	return recordTypeSet
}

func hasAuditRecordType(recordTypes map[string]struct{}, values ...string) bool {
	for _, value := range values {
		if _, ok := recordTypes[strings.ToUpper(strings.TrimSpace(value))]; ok {
			return true
		}
	}
	return false
}

func toAuditMap(event *aucoalesce.Event) map[string]any {
	if event == nil {
		return nil
	}

	raw, err := json.Marshal(event)
	if err != nil {
		return nil
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded
}

func parseAuditInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func normalizeAuditResult(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "yes", "true", "success", "succeeded", "ok", "1":
		return "success"
	case "no", "false", "fail", "failed", "error", "-1":
		return "fail"
	case "":
		return "unknown"
	default:
		return normalized
	}
}

func normalizeAuditResultFromEvent(event *aucoalesce.Event) string {
	if event == nil {
		return "unknown"
	}
	for _, candidate := range []string{
		event.Result,
		event.Data["res"],
		event.Data["result"],
		event.Data["success"],
		auditResultFromExitCode(event.Data["exit"]),
	} {
		if normalized := normalizeAuditResult(candidate); normalized != "unknown" {
			return normalized
		}
	}
	return normalizeAuditResult(event.Result)
}

func normalizeAuditAction(
	event *aucoalesce.Event,
	family string,
	recordTypeSet map[string]struct{},
) string {
	if event == nil {
		return ""
	}

	syscallAction := normalizeSyscallAction(strings.TrimSpace(event.Data["syscall"]))
	rawAction := normalizeFreeformAuditAction(firstNonEmpty(
		event.Summary.Action,
		event.Data["op"],
		event.Data["syscall"],
	))

	switch family {
	case "command":
		if syscallAction != "" {
			return syscallAction
		}
		if hasAuditRecordType(recordTypeSet, "EXECVE", "USER_CMD") {
			return "exec"
		}
	case "login":
		switch {
		case hasAuditRecordType(recordTypeSet, "USER_AUTH"):
			return "authenticate"
		case hasAuditRecordType(recordTypeSet, "USER_ACCT"):
			return "account"
		case hasAuditRecordType(recordTypeSet, "USER_LOGIN", "USER_START", "CRED_ACQ"):
			return "login"
		case hasAuditRecordType(recordTypeSet, "USER_END"):
			return "logout"
		}
	case "session":
		switch {
		case hasAuditRecordType(recordTypeSet, "USER_START"):
			return "session-start"
		case hasAuditRecordType(recordTypeSet, "USER_END"):
			return "session-end"
		}
	case "privilege":
		switch {
		case hasAuditRecordType(recordTypeSet, "ADD_USER"):
			return "add-user"
		case hasAuditRecordType(recordTypeSet, "DEL_USER"):
			return "delete-user"
		case hasAuditRecordType(recordTypeSet, "ADD_GROUP"):
			return "add-group"
		case hasAuditRecordType(recordTypeSet, "DEL_GROUP"):
			return "delete-group"
		case hasAuditRecordType(recordTypeSet, "CHUSER_ID"):
			return "change-user-id"
		case hasAuditRecordType(recordTypeSet, "CHGRP_ID"):
			return "change-group-id"
		case hasAuditRecordType(recordTypeSet, "GRP_MGMT"):
			return "group-manage"
		case hasAuditRecordType(recordTypeSet, "GRP_CHAUTHTOK"):
			return "group-auth-change"
		case hasAuditRecordType(recordTypeSet, "USER_CHAUTHTOK"):
			return "password-change"
		case hasAuditRecordType(recordTypeSet, "USER_ROLE_CHANGE"):
			return "role-change"
		}
	case "file":
		if syscallAction != "" {
			return syscallAction
		}
	}

	if rawAction != "" {
		return rawAction
	}
	return syscallAction
}

func normalizeSyscallAction(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open", "openat", "openat2":
		return "open"
	case "access", "faccessat", "faccessat2":
		return "access"
	case "read", "pread64":
		return "read"
	case "write", "pwrite64":
		return "write"
	case "execve", "execveat":
		return "exec"
	case "chmod", "fchmod", "fchmodat":
		return "chmod"
	case "chown", "fchown", "fchownat", "lchown":
		return "chown"
	case "unlink", "unlinkat":
		return "remove"
	case "rename", "renameat", "renameat2":
		return "rename"
	case "mkdir", "mkdirat", "creat":
		return "create"
	case "rmdir":
		return "remove"
	case "truncate", "ftruncate":
		return "truncate"
	default:
		return normalizeFreeformAuditAction(value)
	}
}

func normalizeFreeformAuditAction(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch {
	case normalized == "":
		return ""
	case strings.Contains(normalized, "open"):
		return "open"
	case strings.Contains(normalized, "access"):
		return "access"
	case strings.Contains(normalized, "read"):
		return "read"
	case strings.Contains(normalized, "write"):
		return "write"
	case strings.Contains(normalized, "exec"):
		return "exec"
	case strings.Contains(normalized, "chmod"):
		return "chmod"
	case strings.Contains(normalized, "chown"):
		return "chown"
	case strings.Contains(normalized, "rename"):
		return "rename"
	case strings.Contains(normalized, "unlink"),
		strings.Contains(normalized, "delete"),
		strings.Contains(normalized, "remove"):
		return "remove"
	case strings.Contains(normalized, "create"),
		strings.Contains(normalized, "mkdir"):
		return "create"
	case strings.Contains(normalized, "truncate"):
		return "truncate"
	default:
		return normalized
	}
}

func shouldKeepAuditObservation(event model.Event) (bool, string) {
	if event.Type != model.EventTypeAudit || event.Audit == nil {
		return false, "audit.invalid"
	}

	switch strings.TrimSpace(event.Audit.Family) {
	case "login":
		if hasAuditIdentityContext(event.Audit) {
			return true, ""
		}
		return false, "login.missing-context"
	case "command":
		return shouldKeepCommandAudit(event)
	case "file":
		return shouldKeepFileAudit(event)
	case "privilege":
		return shouldKeepPrivilegeAudit(event)
	case "session":
		if hasAuditIdentityContext(event.Audit) {
			return true, ""
		}
		return false, "session.missing-context"
	default:
		return false, "family.unknown"
	}
}

func shouldKeepCommandAudit(event model.Event) (bool, string) {
	command := strings.TrimSpace(firstNonEmpty(
		valueOrEmpty(event.Process, func(process *model.Process) string { return process.Command }),
		valueOrEmpty(event.Process, func(process *model.Process) string { return process.Image }),
		valueOrEmpty(event.Audit, func(audit *model.Audit) string { return audit.How }),
	))
	if command == "" {
		return false, "command.missing-command"
	}
	if policy.IsSecurityControlTamperCommand(command) {
		return true, ""
	}
	if hasAuditCommandContext(event.Audit) {
		return true, ""
	}
	return false, "command.unattributed"
}

func shouldKeepFileAudit(event model.Event) (bool, string) {
	filePath := strings.TrimSpace(firstNonEmpty(
		valueOrEmpty(event.File, func(file *model.File) string { return file.Path }),
		valueOrEmpty(event.Audit, func(audit *model.Audit) string { return audit.ObjectPrimary }),
	))
	if !policy.IsSensitiveAuditPath(filePath) {
		return false, "file.non-sensitive-path"
	}

	action := strings.TrimSpace(firstNonEmpty(
		valueOrEmpty(event.Audit, func(audit *model.Audit) string { return audit.Action }),
		valueOrEmpty(event.File, func(file *model.File) string { return file.Operation }),
	))
	if action == "" {
		return false, "file.missing-action"
	}
	if policy.IsAuditReadAction(action) || policy.IsAuditMutationAction(action) {
		return true, ""
	}
	return false, "file.unsupported-action"
}

func shouldKeepPrivilegeAudit(event model.Event) (bool, string) {
	if event.Audit == nil {
		return false, "privilege.missing-payload"
	}
	if hasAuditIdentityContext(event.Audit) ||
		strings.TrimSpace(event.Audit.ObjectPrimary) != "" ||
		strings.TrimSpace(event.Audit.ObjectSecondary) != "" {
		return true, ""
	}
	return false, "privilege.missing-context"
}

func hasAuditIdentityContext(audit *model.Audit) bool {
	if audit == nil {
		return false
	}
	return hasAttributableAuditUser(audit) ||
		strings.TrimSpace(audit.RemoteIP) != "" ||
		strings.TrimSpace(audit.RemoteHost) != "" ||
		hasAuditSessionContext(audit)
}

func hasAuditCommandContext(audit *model.Audit) bool {
	if audit == nil {
		return false
	}
	return strings.TrimSpace(audit.LoginUser) != "" ||
		!isUnsetAuditUserID(audit.AUID) ||
		strings.TrimSpace(audit.RemoteIP) != "" ||
		strings.TrimSpace(audit.RemoteHost) != "" ||
		hasAuditSessionContext(audit)
}

func hasAttributableAuditUser(audit *model.Audit) bool {
	if audit == nil {
		return false
	}
	return strings.TrimSpace(audit.Username) != "" ||
		!isUnsetAuditUserID(audit.UID) ||
		strings.TrimSpace(audit.LoginUser) != "" ||
		!isUnsetAuditUserID(audit.AUID)
}

func auditResultFromExitCode(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return ""
	}
	if parsed == 0 {
		return "success"
	}
	return "fail"
}

func hasAuditSessionContext(audit *model.Audit) bool {
	if audit == nil {
		return false
	}
	return strings.TrimSpace(audit.SessionID) != "" ||
		isMeaningfulTerminal(audit.Terminal)
}

func isMeaningfulTerminal(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized != "" && normalized != "?" && normalized != "(none)"
}

func isUnsetAuditUserID(value string) bool {
	normalized := strings.TrimSpace(value)
	return normalized == "" || normalized == "unset" || normalized == "4294967295" || normalized == "-1"
}

func valueOrEmpty[T any](value *T, getter func(*T) string) string {
	if value == nil || getter == nil {
		return ""
	}
	return strings.TrimSpace(getter(value))
}

func lookupAuditUsername(uid string) string {
	trimmed := strings.TrimSpace(uid)
	if trimmed == "" || trimmed == "unset" || trimmed == "4294967295" || trimmed == "-1" {
		return ""
	}

	lookup, err := user.LookupId(trimmed)
	if err != nil {
		return ""
	}
	return lookup.Username
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func valueAddressField(address *aucoalesce.Address, field string) string {
	if address == nil {
		return ""
	}
	switch field {
	case "hostname":
		return address.Hostname
	case "ip":
		return address.IP
	case "port":
		return address.Port
	default:
		return ""
	}
}

func auditFileField(event *aucoalesce.Event, getter func(file *aucoalesce.File) string) string {
	if event == nil || event.File == nil || getter == nil {
		return ""
	}
	return getter(event.File)
}

func mergeAuditTags(existing []string, values []string) []string {
	if len(existing) == 0 && len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(existing)+len(values))
	merged := make([]string, 0, len(existing)+len(values))
	for _, items := range [][]string{existing, values} {
		for _, item := range items {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			merged = append(merged, trimmed)
		}
	}
	return merged
}
