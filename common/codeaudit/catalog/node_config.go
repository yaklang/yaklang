package catalog

// NodeConfigRules are declarative metadata for the Node.js-side .sf rules.
var NodeConfigRules = []ConfigCheckRule{
	{
		Framework:      "node",
		ID:             "node.tls.reject_unauthorized",
		Severity:       "high",
		Title:          "Node.js TLS verification disabled",
		Recommendation: "remove rejectUnauthorized: false and NODE_TLS_REJECT_UNAUTHORIZED=0; pin a CA bundle instead",
		FilePatterns:   []string{"*.js", "*.ts", "*.mjs", "*.cjs"},
	},
	{
		Framework:      "node",
		ID:             "node.password.plain",
		Severity:       "high",
		Title:          "Hardcoded password or secret",
		Recommendation: "move secrets to environment variables or a secret manager",
		FilePatterns:   []string{"*.js", "*.ts", "*.mjs", "*.cjs", "*.json"},
		MaskValue:      true,
	},
	{
		Framework:      "node",
		ID:             "node.cors.wildcard",
		Severity:       "medium",
		Title:          "CORS configured to allow any origin",
		Recommendation: "restrict CORS origins to concrete production hostnames",
		FilePatterns:   []string{"*.js", "*.ts", "*.mjs", "*.cjs"},
	},
	{
		Framework:      "node",
		ID:             "node.eval.expr",
		Severity:       "medium",
		Title:          "eval() used in JavaScript/TypeScript code",
		Recommendation: "remove eval(); refactor to explicit logic",
		FilePatterns:   []string{"*.js", "*.ts", "*.mjs", "*.cjs"},
	},
	{
		Framework:      "node",
		ID:             "node.child_process.exec",
		Severity:       "medium",
		Title:          "child_process command execution",
		Recommendation: "avoid exec/execSync with user-controlled input; prefer execFile with argv lists",
		FilePatterns:   []string{"*.js", "*.ts", "*.mjs", "*.cjs"},
	},
}
