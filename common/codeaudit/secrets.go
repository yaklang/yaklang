package codeaudit

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SecretRule describes a secret scanning rule.
type SecretRule struct {
	ID             string
	Severity       string
	Pattern        *regexp.Regexp
	Note           string
	FileExtensions []string // empty = scan all files; non-empty = only scan specified extensions
}

// DefaultSecretRules are language-agnostic secret scanning rules.
var DefaultSecretRules = []SecretRule{
	{
		ID:             "secret.password_assignment",
		Severity:       "high",
		Pattern:        regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|api[_-]?key|access[_-]?key)\b[^=\n]{0,40}=\s*["'][^"'\s]{3,}["']`),
		Note:           "literal secret-like assignment in source",
	},
	{
		ID:             "secret.jdbc_inline_credential",
		Severity:       "high",
		Pattern:        regexp.MustCompile(`(?i)jdbc:[a-z0-9]+://[^"'\s]*:[^/"'\s]+@`),
		Note:           "JDBC URL contains inline username/password",
	},
	{
		ID:             "secret.aws_access_key",
		Severity:       "critical",
		Pattern:        regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		Note:           "possible AWS access key id",
	},
	{
		ID:             "secret.private_key_block",
		Severity:       "critical",
		Pattern:        regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY`),
		Note:           "PEM private key embedded in source",
	},
	{
		ID:             "secret.jwt_hardcoded",
		Severity:       "high",
		Pattern:        regexp.MustCompile(`["']eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+["']`),
		Note:           "hardcoded JWT token literal",
	},
	{
		ID:             "secret.static_final_secret",
		Severity:       "medium",
		Pattern:        regexp.MustCompile(`(?i)static\s+final\s+\w*(PASSWORD|SECRET|TOKEN|API_KEY)\w*\s*=\s*["'][^"']+["']`),
		Note:           "static final secret constant",
	},
	{
		ID:             "config.password.property",
		Severity:       "high",
		Pattern:        regexp.MustCompile(`(?i)(spring\.datasource\.(password|username)|password|secret|api\.key)\s*[:=]\s*\S+`),
		Note:           "sensitive value in configuration file",
		FileExtensions: []string{".properties", ".yml", ".yaml", ".xml", ".ini"},
	},
}

// placeholderPatterns are patterns that should NOT be reported as secrets.
var placeholderPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^changeme$`),
	regexp.MustCompile(`(?i)^password$`),
	regexp.MustCompile(`(?i)^123456$`),
	regexp.MustCompile(`(?i)^admin$`),
	regexp.MustCompile(`(?i)^xxx+$`),
	regexp.MustCompile(`(?i)^your[_-]?password`),
	regexp.MustCompile(`(?i)^example$`),
	regexp.MustCompile(`(?i)^test$`),
	regexp.MustCompile(`(?i)^null$`),
	regexp.MustCompile(`(?i)^none$`),
	regexp.MustCompile(`(?i)^placeholder$`),
	regexp.MustCompile(`(?i)^changeme`),
	regexp.MustCompile(`^\$\{`),
	regexp.MustCompile(`(?i)^env\.`),
}

// ScanSecrets scans the target directory for hardcoded secrets.
func ScanSecrets(target string, opts ...ProbeOption) *Report {
	o := applyOptions(opts...)
	start := time.Now().UnixMilli()
	root := resolveScanRoot(target, o)
	idx := BuildFSIndex(root, o)

	report := NewReport("codeaudit/secrets", root, o.Language)

	for _, rule := range DefaultSecretRules {
		for _, fp := range idx.AllFiles {
			if !shouldScanFile(fp, rule, o) {
				continue
			}
			content, ok := ReadFileLimited(fp, 2000000)
			if !ok {
				continue
			}

			matches := rule.Pattern.FindAllStringSubmatchIndex(content, -1)
			for _, m := range matches {
				matched := content[m[0]:m[1]]
				if isPlaceholderSecret(matched) {
					continue
				}
				line := strings.Count(content[:m[0]], "\n") + 1
				snippet := extractLineSnippet(content, m[0])
				report.AddFinding(rule.ID, rule.Severity, rule.Note,
					"move secrets to environment variables or a secret manager; rotate exposed credentials",
					[]Evidence{{File: fp, Line: line, Snippet: snippet}})
			}
		}
	}

	report.Artifacts = map[string]any{"scan_root": root, "audit_options": o}
	return report.Finish(start, idx.FilesScanned, o)
}

// shouldScanFile determines whether a file should be scanned for a given rule.
func shouldScanFile(fp string, rule SecretRule, opts *ProbeOptions) bool {
	if len(rule.FileExtensions) > 0 {
		ext := strings.ToLower(filepath.Ext(fp))
		found := false
		for _, allowed := range rule.FileExtensions {
			if ext == strings.ToLower(allowed) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Skip binary-like files
	ext := strings.ToLower(filepath.Ext(fp))
	skipExts := map[string]bool{
		".jar": true, ".class": true, ".war": true, ".ear": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".pdf": true, ".zip": true, ".tar": true, ".gz": true,
		".so": true, ".dll": true, ".exe": true, ".bin": true,
		".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
		".svg": true, ".ico": true, ".bmp": true, ".mp4": true,
		".mp3": true, ".wav": true,
	}
	if skipExts[ext] {
		return false
	}

	return true
}

// isPlaceholderSecret checks if a matched string is a placeholder, not a real secret.
func isPlaceholderSecret(matched string) bool {
	// Extract the value part (after = or :)
	var value string
	if idx := strings.Index(matched, "="); idx >= 0 {
		value = strings.TrimSpace(matched[idx+1:])
	} else if idx := strings.Index(matched, ":"); idx >= 0 {
		value = strings.TrimSpace(matched[idx+1:])
	} else {
		value = matched
	}

	// Strip quotes
	value = strings.Trim(value, `"' `)

	// If empty, treat as placeholder
	if value == "" {
		return true
	}

	// Check against placeholder patterns
	for _, p := range placeholderPatterns {
		if p.MatchString(value) {
			return true
		}
	}

	return false
}

// extractLineSnippet extracts the line containing the match position.
func extractLineSnippet(content string, pos int) string {
	// Find start of line
	start := pos
	for start > 0 && content[start-1] != '\n' {
		start--
	}
	// Find end of line
	end := pos
	for end < len(content) && content[end] != '\n' {
		end++
	}
	snippet := strings.TrimSpace(content[start:end])
	if len(snippet) > 240 {
		snippet = snippet[:240]
	}
	return snippet
}
