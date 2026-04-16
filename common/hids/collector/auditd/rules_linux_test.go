//go:build hids && linux

package auditd

import (
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestBuildManagedAuditRulesCompilesCommandAndSensitiveFileRules(t *testing.T) {
	t.Parallel()

	rules, err := buildManagedAuditRules()
	if err != nil {
		t.Fatalf("buildManagedAuditRules returned error: %v", err)
	}
	if len(rules) != len(hidsAuditCommandRuleCommands)+len(hidsAuditWatchPaths) {
		t.Fatalf("unexpected rule count: %d", len(rules))
	}

	var hasCommandRule bool
	var hasRootCommandRule bool
	var hasShadowRule bool
	var hasPAMRule bool
	var hasLoaderRule bool
	var hasProfileDirRule bool
	var hasSystemdRule bool
	for _, managed := range rules {
		if managed.command == "" {
			t.Fatal("expected command text on managed audit rule")
		}
		if len(managed.wire) == 0 {
			t.Fatalf("expected non-empty wire format for %q", managed.command)
		}
		if strings.Contains(managed.command, "execve") &&
			strings.Contains(managed.command, "auid!=4294967295") &&
			strings.Contains(managed.command, "yak-hids-command") {
			hasCommandRule = true
		}
		if strings.Contains(managed.command, "execve") &&
			strings.Contains(managed.command, "auid=4294967295") &&
			strings.Contains(managed.command, "euid=0") &&
			strings.Contains(managed.command, "yak-hids-command-root") {
			hasRootCommandRule = true
		}
		if strings.Contains(managed.command, "/etc/shadow") &&
			strings.Contains(managed.command, "yak-hids-identity") {
			hasShadowRule = true
		}
		if strings.Contains(managed.command, "/etc/pam.d") &&
			strings.Contains(managed.command, "yak-hids-login") {
			hasPAMRule = true
		}
		if strings.Contains(managed.command, "/etc/ld.so.preload") &&
			strings.Contains(managed.command, "yak-hids-persistence") {
			hasLoaderRule = true
		}
		if strings.Contains(managed.command, "/etc/profile.d") &&
			strings.Contains(managed.command, "yak-hids-persistence") {
			hasProfileDirRule = true
		}
		if strings.Contains(managed.command, "/etc/systemd/system") &&
			strings.Contains(managed.command, "yak-hids-persistence") {
			hasSystemdRule = true
		}
	}
	if !hasCommandRule {
		t.Fatal("expected managed command exec audit rule")
	}
	if !hasRootCommandRule {
		t.Fatal("expected managed root exec audit rule for unset auid")
	}
	if !hasShadowRule {
		t.Fatal("expected managed /etc/shadow audit rule")
	}
	if !hasPAMRule {
		t.Fatal("expected managed /etc/pam.d audit rule")
	}
	if !hasLoaderRule {
		t.Fatal("expected managed /etc/ld.so.preload audit rule")
	}
	if !hasProfileDirRule {
		t.Fatal("expected managed /etc/profile.d audit rule")
	}
	if !hasSystemdRule {
		t.Fatal("expected managed systemd unit audit rule")
	}
}

func TestWrapAuditRuleInstallErrorAddsCapabilityGuidance(t *testing.T) {
	t.Parallel()

	err := wrapAuditRuleInstallError(
		"-a always,exit -S execve -k yak-hids-command",
		syscall.EPERM,
	)
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !errors.Is(err, syscall.EPERM) {
		t.Fatal("expected wrapped error to preserve errno")
	}
	if !strings.Contains(err.Error(), "CAP_AUDIT_CONTROL") {
		t.Fatalf("expected CAP_AUDIT_CONTROL guidance, got %q", err.Error())
	}
}

func TestAuditRuleErrorClassifiers(t *testing.T) {
	t.Parallel()

	if !isAuditRuleExistsError(errors.New("rule exists")) {
		t.Fatal("expected rule exists classifier to match")
	}
	if !isAuditRuleNotFoundError(syscall.ENOENT) {
		t.Fatal("expected not found classifier to match ENOENT")
	}
	if !isAuditRuleNotFoundError(errors.New("error adding audit rule: no such file or directory")) {
		t.Fatal("expected not found classifier to match path-missing add errors")
	}
	if !isAuditPermissionError(syscall.EACCES) {
		t.Fatal("expected permission classifier to match EACCES")
	}
}
