package codeaudit

import "testing"

// TestConfigAudit_Multilang verifies language-dispatched config rule findings.
func TestConfigAudit_Multilang(t *testing.T) {
	cases := []struct {
		name     string
		sample   string
		language string
		framework string
		wantIDs  []string
	}{
		{
			name: "python django", sample: "django", language: "python", framework: "django",
			wantIDs: []string{
				"py.django.debug",
				"py.django.secret_key",
				"py.django.allowed_hosts_wildcard",
				"py.django.cors_allow_all",
			},
		},
		{
			name: "go", sample: "gogin", language: "go", framework: "go",
			wantIDs: []string{
				"go.tls.insecure_skip_verify",
				"go.http.client_no_timeout",
			},
		},
		{
			name: "node", sample: "express", language: "node", framework: "node",
			wantIDs: []string{
				"node.tls.reject_unauthorized",
				"node.password.plain",
				"node.cors.wildcard",
				"node.eval.expr",
				"node.child_process.exec",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := multilangSampleDir(t, tc.sample)
			report := AuditConfig(dir, tc.framework, WithLanguage(tc.language))

			if report.Status != "ok" && report.Status != "partial" {
				t.Errorf("expected status ok or partial, got %q", report.Status)
			}
			for _, want := range tc.wantIDs {
				found := false
				for _, f := range report.Findings {
					if f.ID == want {
						found = true
					}
				}
				if !found {
					t.Errorf("expected finding %s, got: %+v", want, findingIDs(report))
				}
			}
		})
	}
}

// TestCmsAudit_WordPress verifies the WordPress CMS config rule.
func TestCmsAudit_WordPress(t *testing.T) {
	dir := multilangSampleDir(t, "wordpress")
	report := AuditCmsProduct(dir, WithLanguage("php"))

	found := false
	for _, f := range report.Findings {
		if f.ID == "php.wordpress.db_password" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected php.wordpress.db_password finding, got: %+v", findingIDs(report))
	}
}

// findingIDs returns the finding IDs of a report for error messages.
func findingIDs(report *Report) []string {
	var ids []string
	for _, f := range report.Findings {
		ids = append(ids, f.ID)
	}
	return ids
}
