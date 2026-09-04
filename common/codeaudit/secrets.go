package codeaudit

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/codeaudit/sfaudit"
)

// SecretRule describes a secret scanning rule. The matching itself lives in
// the embedded SyntaxFlow source rule registered under ID (see the sfaudit
// package); placeholder suppression is expressed inside the rule.
type SecretRule struct {
	ID             string
	Severity       string
	Note           string
	FileExtensions []string // empty = scan all files; non-empty = only scan specified extensions
}

// DefaultSecretRules are language-agnostic secret scanning rules.
var DefaultSecretRules = []SecretRule{
	{
		ID:       "secret.password_assignment",
		Severity: "high",
		Note:     "literal secret-like assignment in source",
	},
	{
		ID:       "secret.jdbc_inline_credential",
		Severity: "high",
		Note:     "JDBC URL contains inline username/password",
	},
	{
		ID:       "secret.db_url_credential",
		Severity: "high",
		Note:     "database/service URL contains inline username/password",
	},
	{
		ID:       "secret.dotenv_credential",
		Severity: "high",
		Note:     "credential committed in dotenv-style KEY=value file",
	},
	{
		ID:       "secret.aws_access_key",
		Severity: "critical",
		Note:     "possible AWS access key id",
	},
	{
		ID:       "secret.private_key_block",
		Severity: "critical",
		Note:     "PEM private key embedded in source",
	},
	{
		ID:       "secret.jwt_hardcoded",
		Severity: "high",
		Note:     "hardcoded JWT token literal",
	},
	{
		ID:       "secret.static_final_secret",
		Severity: "medium",
		Note:     "static final secret constant",
	},
	{
		ID:             "secret.config_sensitive_value",
		Severity:       "high",
		Note:           "sensitive value in configuration file",
		FileExtensions: []string{".properties", ".yml", ".yaml", ".xml", ".ini"},
	},
}

// ScanSecrets scans the target directory for hardcoded secrets using the
// SyntaxFlow source-mode rules embedded in sfaudit.
func ScanSecrets(target string, opts ...ProbeOption) *Report {
	o := applyOptions(opts...)
	start := time.Now().UnixMilli()
	root := resolveScanRoot(target, o)
	idx := BuildFSIndex(root, o)

	report := NewReport("codeaudit/secrets", root, o.Language)

	// Load candidate file contents once (binary-like files skipped, 2MB cap).
	files := map[string]string{}
	for _, fp := range idx.AllFiles {
		if isBinaryLikeExt(fp) {
			continue
		}
		if content, ok := ReadFileLimited(fp, 2000000); ok {
			files[fp] = content
		}
	}

	ctx := context.Background()
	for _, rule := range DefaultSecretRules {
		subset := files
		if len(rule.FileExtensions) > 0 {
			subset = filterByExtensions(files, rule.FileExtensions)
		}
		if len(subset) == 0 {
			continue
		}
		hits, err := sfaudit.NewEngine("codeaudit-secrets", subset).Run(ctx, rule.ID)
		if err != nil {
			report.Status = "partial"
			continue
		}
		for _, h := range hits {
			report.AddFinding(rule.ID, rule.Severity, rule.Note,
				"move secrets to environment variables or a secret manager; rotate exposed credentials",
				[]Evidence{{File: h.File, Line: h.Line, Snippet: h.Snippet}})
		}
	}

	report.Artifacts = map[string]any{"scan_root": root, "audit_options": o}
	return report.Finish(start, idx.FilesScanned, o)
}

// filterByExtensions returns the subset of files whose extension matches one
// of the allowed extensions (case-insensitive).
func filterByExtensions(files map[string]string, allowed []string) map[string]string {
	out := map[string]string{}
	for fp, content := range files {
		ext := strings.ToLower(filepath.Ext(fp))
		for _, a := range allowed {
			if ext == strings.ToLower(a) {
				out[fp] = content
				break
			}
		}
	}
	return out
}

// binaryLikeExtensions are extensions never worth scanning as text.
var binaryLikeExtensions = map[string]bool{
	".jar": true, ".class": true, ".war": true, ".ear": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".pdf": true, ".zip": true, ".tar": true, ".gz": true,
	".so": true, ".dll": true, ".exe": true, ".bin": true,
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	".svg": true, ".ico": true, ".bmp": true, ".mp4": true,
	".mp3": true, ".wav": true,
}

// isBinaryLikeExt reports whether the file extension looks binary.
func isBinaryLikeExt(fp string) bool {
	return binaryLikeExtensions[strings.ToLower(filepath.Ext(fp))]
}
