package codeaudit

import (
	"testing"
)

func TestConfigAudit_SpringBoot(t *testing.T) {
	dir := javaAuditTestDataDir(t, "spring_boot_sample")
	report := AuditConfig(dir, "spring_boot", WithLanguage("java"))

	if report.Status != "ok" && report.Status != "partial" {
		t.Errorf("expected status ok or partial, got %q", report.Status)
	}

	// Should find at least 2 findings: actuator exposed + plain password + stacktrace
	if len(report.Findings) < 2 {
		t.Errorf("expected at least 2 findings, got %d: %+v", len(report.Findings), report.Findings)
	}

	// Check for specific finding IDs
	foundActuator := false
	foundPassword := false
	for _, f := range report.Findings {
		if f.ID == "spring.actuator.exposed" {
			foundActuator = true
		}
		if f.ID == "spring.datasource.password.plain" {
			foundPassword = true
		}
	}
	if !foundActuator {
		t.Errorf("expected to find spring.actuator.exposed finding")
	}
	if !foundPassword {
		t.Errorf("expected to find spring.datasource.password.plain finding")
	}
}

func TestConfigAudit_Struts2(t *testing.T) {
	dir := javaAuditTestDataDir(t, "struts2_sample")
	report := AuditConfig(dir, "struts2", WithLanguage("java"))

	if len(report.Findings) < 1 {
		t.Errorf("expected at least 1 finding, got %d: %+v", len(report.Findings), report.Findings)
	}

	foundDevMode := false
	for _, f := range report.Findings {
		if f.ID == "struts2.devmode" {
			foundDevMode = true
		}
	}
	if !foundDevMode {
		t.Errorf("expected to find struts2.devmode finding")
	}
}

func TestConfigAudit_Shiro(t *testing.T) {
	dir := javaAuditTestDataDir(t, "shiro_sample")
	report := AuditConfig(dir, "shiro", WithLanguage("java"))

	if len(report.Findings) < 1 {
		t.Errorf("expected at least 1 finding, got %d: %+v", len(report.Findings), report.Findings)
	}

	foundAnonURL := false
	foundCipherKey := false
	for _, f := range report.Findings {
		if f.ID == "shiro.anon.url" {
			foundAnonURL = true
		}
		if f.ID == "shiro.rememberme.cipherKey" {
			foundCipherKey = true
		}
	}
	if !foundAnonURL {
		t.Errorf("expected to find shiro.anon.url finding")
	}
	if !foundCipherKey {
		t.Errorf("expected to find shiro.rememberme.cipherKey finding")
	}
}
