//go:build hids && linux

package auditd

import (
	"errors"
	"fmt"
	"strings"
	"syscall"

	libaudit "github.com/elastic/go-libaudit/v2"
	auditrule "github.com/elastic/go-libaudit/v2/rule"
	auditflags "github.com/elastic/go-libaudit/v2/rule/flags"
)

const hidsAuditRuleKeyPrefix = "yak-hids"

var hidsAuditCommandRuleCommands = []string{
	"-a always,exit -S execve -S execveat -F auid!=4294967295 -k yak-hids-command",
	"-a always,exit -S execve -S execveat -F auid=4294967295 -F euid=0 -k yak-hids-command-root",
}

type managedAuditWatchPath struct {
	path string
	key  string
}

var hidsAuditWatchPaths = []managedAuditWatchPath{
	{path: "/etc/passwd", key: "identity"},
	{path: "/etc/shadow", key: "identity"},
	{path: "/etc/group", key: "identity"},
	{path: "/etc/gshadow", key: "identity"},
	{path: "/etc/sudoers", key: "privilege"},
	{path: "/etc/sudoers.d", key: "privilege"},
	{path: "/etc/ssh/sshd_config", key: "login"},
	{path: "/etc/pam.d", key: "login"},
	{path: "/etc/security", key: "login"},
	{path: "/etc/crontab", key: "persistence"},
	{path: "/etc/cron.d", key: "persistence"},
	{path: "/var/spool/cron", key: "persistence"},
	{path: "/etc/ld.so.preload", key: "persistence"},
	{path: "/etc/environment", key: "persistence"},
	{path: "/etc/profile", key: "persistence"},
	{path: "/etc/profile.d", key: "persistence"},
	{path: "/etc/bash.bashrc", key: "persistence"},
	{path: "/etc/rc.local", key: "persistence"},
	{path: "/etc/systemd/system", key: "persistence"},
	{path: "/lib/systemd/system", key: "persistence"},
	{path: "/usr/lib/systemd/system", key: "persistence"},
}

type managedAuditRule struct {
	command string
	wire    []byte
}

type auditRuleInstallResult struct {
	total    int
	added    int
	existing int
	skipped  int
	rules    [][]byte
}

func installHIDSAuditRules(client *libaudit.AuditClient) (auditRuleInstallResult, error) {
	result := auditRuleInstallResult{}
	if client == nil {
		return result, fmt.Errorf("audit rule control client is nil")
	}

	rules, err := buildManagedAuditRules()
	if err != nil {
		return result, err
	}
	result.total = len(rules)

	for _, managed := range rules {
		if err := client.AddRule(managed.wire); err != nil {
			if isAuditRuleExistsError(err) {
				result.existing++
				continue
			}
			if isAuditRuleNotFoundError(err) {
				result.skipped++
				continue
			}
			if len(result.rules) > 0 {
				_ = deleteManagedAuditRules(client, result.rules)
			}
			return result, wrapAuditRuleInstallError(managed.command, err)
		}
		result.added++
		result.rules = append(result.rules, cloneRuleWire(managed.wire))
	}
	return result, nil
}

func deleteManagedAuditRules(client *libaudit.AuditClient, rules [][]byte) error {
	if client == nil || len(rules) == 0 {
		return nil
	}

	var errs []error
	for _, rawRule := range rules {
		if len(rawRule) == 0 {
			continue
		}
		if err := client.DeleteRule(rawRule); err != nil && !isAuditRuleNotFoundError(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func buildManagedAuditRules() ([]managedAuditRule, error) {
	rules := make([]managedAuditRule, 0, len(hidsAuditCommandRuleCommands)+len(hidsAuditWatchPaths))
	for _, command := range hidsAuditCommandRuleCommands {
		parsed, err := auditflags.Parse(command)
		if err != nil {
			return nil, fmt.Errorf("parse managed audit rule %q: %w", command, err)
		}
		wire, err := auditrule.Build(parsed)
		if err != nil {
			return nil, fmt.Errorf("build managed audit rule %q: %w", command, err)
		}
		rules = append(rules, managedAuditRule{
			command: command,
			wire:    cloneRuleWire(wire),
		})
	}
	for _, watch := range hidsAuditWatchPaths {
		command := fmt.Sprintf("-w %s -p rwa -k %s-%s", watch.path, hidsAuditRuleKeyPrefix, watch.key)
		parsed, err := auditflags.Parse(command)
		if err != nil {
			return nil, fmt.Errorf("parse managed audit rule %q: %w", command, err)
		}
		wire, err := auditrule.Build(parsed)
		if err != nil {
			return nil, fmt.Errorf("build managed audit rule %q: %w", command, err)
		}
		rules = append(rules, managedAuditRule{
			command: command,
			wire:    cloneRuleWire(wire),
		})
	}
	return rules, nil
}

func wrapAuditRuleInstallError(command string, err error) error {
	if err == nil {
		return nil
	}
	if isAuditPermissionError(err) {
		return fmt.Errorf(
			"install HIDS audit rule %q: %w; HIDS audit collector needs root or CAP_AUDIT_CONTROL to provision command and sensitive-file audit rules; if you are already root, this kernel/environment may still block audit rule management (common in containers, WSL, or restricted desktop kernels)",
			command,
			err,
		)
	}
	return fmt.Errorf("install HIDS audit rule %q: %w", command, err)
}

func isAuditRuleExistsError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "rule exists")
}

func isAuditRuleNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	lowercase := strings.ToLower(err.Error())
	return errors.Is(err, syscall.ENOENT) ||
		strings.Contains(lowercase, "no such file") ||
		strings.Contains(lowercase, "no such rule") ||
		strings.Contains(lowercase, "does not exist")
}

func isAuditPermissionError(err error) bool {
	if err == nil {
		return false
	}
	lowercase := strings.ToLower(err.Error())
	return errors.Is(err, syscall.EPERM) ||
		errors.Is(err, syscall.EACCES) ||
		strings.Contains(lowercase, "operation not permitted") ||
		strings.Contains(lowercase, "permission denied")
}

func cloneRuleWire(rule []byte) []byte {
	if len(rule) == 0 {
		return nil
	}
	cloned := make([]byte, len(rule))
	copy(cloned, rule)
	return cloned
}
