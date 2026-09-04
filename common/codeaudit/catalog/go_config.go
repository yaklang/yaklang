package catalog

// GoConfigRules are declarative metadata for the Go-side .sf rules. Go
// configuration is mostly code-level, so the "config audit" for Go runs
// source-pattern rules over .go files.
var GoConfigRules = []ConfigCheckRule{
	{
		Framework:      "go",
		ID:             "go.tls.insecure_skip_verify",
		Severity:       "high",
		Title:          "TLS certificate verification disabled",
		Recommendation: "remove InsecureSkipVerify: true; trust a custom CA bundle instead",
		FilePatterns:   []string{"*.go"},
	},
	{
		Framework:      "go",
		ID:             "go.crypto.weak_hash",
		Severity:       "medium",
		Title:          "Weak hash algorithm (MD5/SHA-1)",
		Recommendation: "use SHA-256 or stronger; keep MD5/SHA-1 only for non-security checksums",
		FilePatterns:   []string{"*.go"},
	},
	{
		Framework:      "go",
		ID:             "go.hardcoded.password",
		Severity:       "high",
		Title:          "Hardcoded password or secret",
		Recommendation: "move secrets to environment variables or a secret manager",
		FilePatterns:   []string{"*.go"},
		MaskValue:      true,
	},
	{
		Framework:      "go",
		ID:             "go.http.client_no_timeout",
		Severity:       "medium",
		Title:          "http.Client constructed without a Timeout",
		Recommendation: "set an explicit Timeout on http.Client to avoid hung requests",
		FilePatterns:   []string{"*.go"},
	},
	{
		Framework:      "go",
		ID:             "go.jwt.none_alg",
		Severity:       "medium",
		Title:          "JWT signed with the none algorithm",
		Recommendation: "never accept jwt.SigningMethodNone; pin an HMAC/RSA signing method",
		FilePatterns:   []string{"*.go"},
	},
}
