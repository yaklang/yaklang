package catalog

// PythonConfigRules are declarative metadata for the Python-side .sf rules.
// Matching runs in common/codeaudit/sfaudit; IDs must equal the rule_id in
// the corresponding .sf file.
var PythonConfigRules = []ConfigCheckRule{
	// === Django ===
	{
		Framework:      "django",
		ID:             "py.django.debug",
		Severity:       "high",
		Title:          "Django DEBUG enabled",
		Recommendation: "set DEBUG = False in production settings; debug mode leaks stack traces and settings",
		FilePatterns:   []string{"settings.py", "*settings.py", "settings_*.py"},
	},
	{
		Framework:      "django",
		ID:             "py.django.secret_key",
		Severity:       "high",
		Title:          "Django SECRET_KEY hardcoded",
		Recommendation: "load SECRET_KEY from an environment variable or secret manager; rotate exposed keys",
		FilePatterns:   []string{"settings.py", "*settings.py", "settings_*.py"},
		MaskValue:      true,
	},
	{
		Framework:      "django",
		ID:             "py.django.allowed_hosts_wildcard",
		Severity:       "medium",
		Title:          "Django ALLOWED_HOSTS accepts any host",
		Recommendation: "restrict ALLOWED_HOSTS to the concrete production hostnames",
		FilePatterns:   []string{"settings.py", "*settings.py", "settings_*.py"},
	},
	{
		Framework:      "django",
		ID:             "py.django.cors_allow_all",
		Severity:       "medium",
		Title:          "Django CORS allows all origins",
		Recommendation: "restrict CORS_ALLOWED_ORIGINS / set CORS_ALLOW_ALL_ORIGINS = False",
		FilePatterns:   []string{"settings.py", "*settings.py", "settings_*.py"},
	},
	// === Flask ===
	{
		Framework:      "flask",
		ID:             "py.flask.debug_run",
		Severity:       "medium",
		Title:          "Flask app started with debug enabled",
		Recommendation: "never run with debug=True in production; use a WSGI server",
		FilePatterns:   []string{"*.py"},
	},
	{
		Framework:      "flask",
		ID:             "py.flask.host_all_interfaces",
		Severity:       "low",
		Title:          "Flask app bound to all interfaces",
		Recommendation: "bind to a specific interface or guard exposure behind a reverse proxy",
		FilePatterns:   []string{"*.py"},
	},
	// === language-generic Python ===
	{
		Framework:      "python",
		ID:             "py.pickle.load",
		Severity:       "medium",
		Title:          "pickle deserialization of untrusted data",
		Recommendation: "avoid pickle.loads on user-controlled input; use JSON or restrict via hmac signing",
		FilePatterns:   []string{"*.py"},
	},
	{
		Framework:      "python",
		ID:             "py.yaml.unsafe_load",
		Severity:       "medium",
		Title:          "yaml.load without a safe Loader",
		Recommendation: "use yaml.safe_load or pass Loader=yaml.SafeLoader",
		FilePatterns:   []string{"*.py"},
	},
	{
		Framework:      "python",
		ID:             "py.requests.verify_false",
		Severity:       "medium",
		Title:          "requests TLS verification disabled",
		Recommendation: "remove verify=False; pin a custom CA bundle instead",
		FilePatterns:   []string{"*.py"},
	},
	{
		Framework:      "python",
		ID:             "py.subprocess.shell_true",
		Severity:       "medium",
		Title:          "subprocess launched with shell=True",
		Recommendation: "avoid shell=True with user-controlled arguments; pass argv lists",
		FilePatterns:   []string{"*.py"},
	},
}
