package catalog

// PHPConfigRules are declarative metadata for the PHP-side .sf rules.
var PHPConfigRules = []ConfigCheckRule{
	// === Laravel ===
	{
		Framework:      "laravel",
		ID:             "php.laravel.app_key",
		Severity:       "high",
		Title:          "Laravel APP_KEY committed to the repository",
		Recommendation: "keep APP_KEY out of version control; rotate exposed keys",
		FilePatterns:   []string{".env", "*.env", ".env.*"},
		MaskValue:      true,
	},
	{
		Framework:      "laravel",
		ID:             "php.dotenv.debug",
		Severity:       "medium",
		Title:          "Application debug enabled in .env",
		Recommendation: "set APP_DEBUG=false in production",
		FilePatterns:   []string{".env", "*.env", ".env.*"},
	},
	{
		Framework:      "laravel",
		ID:             "php.db.password_config",
		Severity:       "high",
		Title:          "Database password in plain text config",
		Recommendation: "move database password to environment variables",
		FilePatterns:   []string{"database.php", "*.php"},
		MaskValue:      true,
	},
	// === language-generic PHP ===
	{
		Framework:      "php",
		ID:             "php.eval.expr",
		Severity:       "medium",
		Title:          "eval() used in PHP code",
		Recommendation: "remove eval(); refactor to explicit logic",
		FilePatterns:   []string{"*.php"},
	},
	{
		Framework:      "php",
		ID:             "php.command.system",
		Severity:       "medium",
		Title:          "OS command execution function used",
		Recommendation: "avoid system/exec/shell_exec/passthru with user-controlled input; use escapeshellarg",
		FilePatterns:   []string{"*.php"},
	},
	{
		Framework:      "php",
		ID:             "php.unserialize",
		Severity:       "medium",
		Title:          "unserialize() on potentially untrusted data",
		Recommendation: "use JSON instead of PHP serialization for untrusted input",
		FilePatterns:   []string{"*.php"},
	},
	{
		Framework:      "php",
		ID:             "php.crypto.md5_var",
		Severity:       "medium",
		Title:          "MD5 used over variables",
		Recommendation: "use password_hash()/password_verify() for credentials and SHA-256+ elsewhere",
		FilePatterns:   []string{"*.php"},
	},
}
