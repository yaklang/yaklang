package codeaudit

import "testing"

// TestRunFrameworkAudit_MultilangEntryPoints verifies per-language entry
// point detection.
func TestRunFrameworkAudit_MultilangEntryPoints(t *testing.T) {
	cases := []struct {
		sample    string
		language  string
		framework string
	}{
		{"django", "python", "django"},
		{"gogin", "go", "gin"},
		{"wordpress", "php", "wordpress"},
		{"express", "node", "express"},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			dir := multilangSampleDir(t, tc.sample)
			report := RunFrameworkAudit(dir, tc.framework, WithLanguage(tc.language))

			if report.Framework != tc.framework {
				t.Errorf("expected framework %q, got %q", tc.framework, report.Framework)
			}
			entryPoints, ok := report.Artifacts["entry_points"].([]string)
			if !ok {
				t.Fatalf("expected entry_points to be []string, got %T", report.Artifacts["entry_points"])
			}
			if len(entryPoints) == 0 {
				t.Errorf("expected at least 1 entry point for %s, got 0", tc.language)
			}
		})
	}
}
