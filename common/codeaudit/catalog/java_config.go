package catalog

// FindingProxy is a proxy type to avoid circular imports between catalog and codeaudit.
// It is converted to codeaudit.Finding by the engine.
type FindingProxy struct {
	ID             string
	Severity       string
	Title          string
	Recommendation string
	Evidence       []EvidenceProxy
}

// EvidenceProxy is a proxy type for Evidence.
type EvidenceProxy struct {
	File    string
	Line    int
	Snippet string
}

// ConfigCheckFunc is the signature for a configuration check rule.
type ConfigCheckFunc func(content string, kv map[string]string, filePath string) *FindingProxy

// ConfigCheckRule describes a configuration audit rule.
type ConfigCheckRule struct {
	Framework      string
	ID             string
	Severity       string
	Title          string
	Recommendation string
	FilePatterns   []string // glob patterns, e.g. ["application*.yml", "application*.properties"]
	Check          ConfigCheckFunc
}

// JavaConfigRules contains all Java framework configuration audit rules.
var JavaConfigRules = []ConfigCheckRule{
	// === Spring Boot ===
	{
		Framework:      "spring_boot",
		ID:             "spring.actuator.exposed",
		Severity:       "high",
		Title:          "Actuator endpoints overly exposed",
		Recommendation: "restrict management endpoints and protect with authentication",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			// Check properties-style: management.endpoints.web.exposure.include=*
			if v, ok := kv["management.endpoints.web.exposure.include"]; ok && (v == "*" || containsStar(v)) {
				return &FindingProxy{
					ID:             "spring.actuator.exposed",
					Severity:       "high",
					Title:          "Actuator endpoints overly exposed",
					Recommendation: "restrict management endpoints and protect with authentication",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "management.endpoints.web.exposure.include=" + v}},
				}
			}
			// Check YAML nested format
			yamlPattern := mustCompile(`(?is)management:\s*\n(\s+)endpoints:\s*\n\1\s+web:\s*\n\1\s+\1exposure:\s*\n\1\s+\1\s+include:\s*["']?\*`)
			if yamlPattern.MatchString(content) {
				return &FindingProxy{
					ID:             "spring.actuator.exposed",
					Severity:       "high",
					Title:          "Actuator endpoints overly exposed",
					Recommendation: "restrict management endpoints and protect with authentication",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "management.endpoints.web.exposure.include: *"}},
				}
			}
			return nil
		},
	},
	{
		Framework:      "spring_boot",
		ID:             "spring.error.stacktrace",
		Severity:       "medium",
		Title:          "Stack trace disclosure enabled",
		Recommendation: "set server.error.include-stacktrace to never or on_trace_param",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["server.error.include-stacktrace"]; ok && v == "always" {
				return &FindingProxy{
					ID:             "spring.error.stacktrace",
					Severity:       "medium",
					Title:          "Stack trace disclosure enabled",
					Recommendation: "set server.error.include-stacktrace to never or on_trace_param",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "server.error.include-stacktrace=" + v}},
				}
			}
			// YAML: include-stacktrace: always
			if matchSimple(content, `include-stacktrace:\s*always`) {
				return &FindingProxy{
					ID:             "spring.error.stacktrace",
					Severity:       "medium",
					Title:          "Stack trace disclosure enabled",
					Recommendation: "set server.error.include-stacktrace to never or on_trace_param",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "include-stacktrace: always"}},
				}
			}
			return nil
		},
	},
	{
		Framework:      "spring_boot",
		ID:             "spring.datasource.password.plain",
		Severity:       "high",
		Title:          "Plain-text database password in configuration",
		Recommendation: "move database credentials to environment variables or a secret manager",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["spring.datasource.password"]; ok && v != "" && !isPlaceholder(v) {
				return &FindingProxy{
					ID:             "spring.datasource.password.plain",
					Severity:       "high",
					Title:          "Plain-text database password in configuration",
					Recommendation: "move database credentials to environment variables or a secret manager",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "spring.datasource.password=" + maskSecret(v)}},
				}
			}
			// YAML: password: <value>
			yamlPattern := mustCompile(`(?i)spring:\s*\n(\s+)datasource:\s*\n[\s\S]*?\1\s+password:\s*(\S+)`)
			m := yamlPattern.FindStringSubmatch(content)
			if m != nil && !isPlaceholder(m[2]) {
				return &FindingProxy{
					ID:             "spring.datasource.password.plain",
					Severity:       "high",
					Title:          "Plain-text database password in configuration",
					Recommendation: "move database credentials to environment variables or a secret manager",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "spring.datasource.password=" + maskSecret(m[2])}},
				}
			}
			return nil
		},
	},
	{
		Framework:      "spring_boot",
		ID:             "spring.cors.wildcard",
		Severity:       "medium",
		Title:          "CORS allows all origins",
		Recommendation: "restrict allowed origins to specific trusted domains",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["spring.web.cors.allowed-origins"]; ok && (v == "*" || containsStar(v)) {
				return &FindingProxy{
					ID:             "spring.cors.wildcard",
					Severity:       "medium",
					Title:          "CORS allows all origins",
					Recommendation: "restrict allowed origins to specific trusted domains",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "allowed-origins=" + v}},
				}
			}
			if matchSimple(content, `allowed-origins:\s*["']?\*`) {
				return &FindingProxy{
					ID:             "spring.cors.wildcard",
					Severity:       "medium",
					Title:          "CORS allows all origins",
					Recommendation: "restrict allowed origins to specific trusted domains",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "allowed-origins: *"}},
				}
			}
			return nil
		},
	},
	{
		Framework:      "spring_boot",
		ID:             "spring.devtools.secret",
		Severity:       "high",
		Title:          "Spring DevTools remote secret exposed",
		Recommendation: "remove remote DevTools or use a strong, rotated secret",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["spring.devtools.remote.secret"]; ok && v != "" {
				return &FindingProxy{
					ID:             "spring.devtools.secret",
					Severity:       "high",
					Title:          "Spring DevTools remote secret exposed",
					Recommendation: "remove remote DevTools or use a strong, rotated secret",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "spring.devtools.remote.secret=" + maskSecret(v)}},
				}
			}
			return nil
		},
	},

	// === Shiro ===
	{
		Framework:      "shiro",
		ID:             "shiro.anon.url",
		Severity:       "high",
		Title:          "Unauthenticated URL pattern in Shiro configuration",
		Recommendation: "remove overly broad anon patterns or restrict to specific public paths",
		FilePatterns:   []string{"shiro.ini", "*.ini"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			lines := splitLines(content)
			for _, line := range lines {
				trimmed := trimSpace(line)
				if hasPrefix(trimmed, "/") && containsStr(trimmed, "anon") {
					return &FindingProxy{
						ID:             "shiro.anon.url",
						Severity:       "high",
						Title:          "Unauthenticated URL pattern in Shiro configuration",
						Recommendation: "remove overly broad anon patterns or restrict to specific public paths",
						Evidence:       []EvidenceProxy{{File: fp, Snippet: trimmed}},
					}
				}
			}
			return nil
		},
	},
	{
		Framework:      "shiro",
		ID:             "shiro.rememberme.cipherKey",
		Severity:       "critical",
		Title:          "Hardcoded Shiro rememberMe cipher key",
		Recommendation: "generate a unique random key; rotate any compromised keys",
		FilePatterns:   []string{"shiro.ini", "*.ini", "*.yml", "*.yaml", "*.properties"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["securityManager.rememberMeManager.cipherKey"]; ok && v != "" {
				return &FindingProxy{
					ID:             "shiro.rememberme.cipherKey",
					Severity:       "critical",
					Title:          "Hardcoded Shiro rememberMe cipher key",
					Recommendation: "generate a unique random key; rotate any compromised keys",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "cipherKey=" + maskSecret(v)}},
				}
			}
			if matchSimple(content, `rememberMeManager\.cipherKey\s*=\s*\S+`) {
				line := findLineWith(content, "cipherKey")
				return &FindingProxy{
					ID:             "shiro.rememberme.cipherKey",
					Severity:       "critical",
					Title:          "Hardcoded Shiro rememberMe cipher key",
					Recommendation: "generate a unique random key; rotate any compromised keys",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: line}},
				}
			}
			return nil
		},
	},

	// === Struts2 ===
	{
		Framework:      "struts2",
		ID:             "struts2.devmode",
		Severity:       "high",
		Title:          "Struts2 devMode enabled in production",
		Recommendation: "set struts.devMode to false in production",
		FilePatterns:   []string{"struts.xml", "struts.properties"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["struts.devMode"]; ok && v == "true" {
				return &FindingProxy{
					ID:             "struts2.devmode",
					Severity:       "high",
					Title:          "Struts2 devMode enabled in production",
					Recommendation: "set struts.devMode to false in production",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "struts.devMode=true"}},
				}
			}
			// XML constant format
			if matchSimple(content, `struts\.devMode.*value\s*=\s*"true"`) {
				line := findLineWith(content, "devMode")
				return &FindingProxy{
					ID:             "struts2.devmode",
					Severity:       "high",
					Title:          "Struts2 devMode enabled in production",
					Recommendation: "set struts.devMode to false in production",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: line}},
				}
			}
			return nil
		},
	},
	{
		Framework:      "struts2",
		ID:             "struts2.dmi",
		Severity:       "high",
		Title:          "Struts2 dynamic method invocation enabled",
		Recommendation: "disable DMI to prevent method invocation attacks",
		FilePatterns:   []string{"struts.xml", "struts.properties"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["struts.enable.DynamicMethodInvocation"]; ok && v == "true" {
				return &FindingProxy{
					ID:             "struts2.dmi",
					Severity:       "high",
					Title:          "Struts2 dynamic method invocation enabled",
					Recommendation: "disable DMI to prevent method invocation attacks",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "struts.enable.DynamicMethodInvocation=true"}},
				}
			}
			if matchSimple(content, `DynamicMethodInvocation.*value\s*=\s*"true"`) {
				line := findLineWith(content, "DynamicMethodInvocation")
				return &FindingProxy{
					ID:             "struts2.dmi",
					Severity:       "high",
					Title:          "Struts2 dynamic method invocation enabled",
					Recommendation: "disable DMI to prevent method invocation attacks",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: line}},
				}
			}
			return nil
		},
	},

	// === Servlet ===
	{
		Framework:      "servlet",
		ID:             "servlet.missing_security_headers",
		Severity:       "low",
		Title:          "Missing security headers in web.xml",
		Recommendation: "add security headers such as X-Content-Type-Options, X-Frame-Options",
		FilePatterns:   []string{"web.xml"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			// Check if web.xml lacks security constraints
			if !matchSimple(content, `security-constraint`) {
				return &FindingProxy{
					ID:             "servlet.missing_security_headers",
					Severity:       "low",
					Title:          "Missing security headers in web.xml",
					Recommendation: "add security headers such as X-Content-Type-Options, X-Frame-Options",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "no security-constraint defined"}},
				}
			}
			return nil
		},
	},

	// === Spring Security ===
	{
		Framework:      "spring_security",
		ID:             "spring_security.csrf_disabled",
		Severity:       "medium",
		Title:          "CSRF protection disabled",
		Recommendation: "enable CSRF protection unless the API is stateless and token-based",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties", "*.xml"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["spring.security.csrf.enabled"]; ok && v == "false" {
				return &FindingProxy{
					ID:             "spring_security.csrf_disabled",
					Severity:       "medium",
					Title:          "CSRF protection disabled",
					Recommendation: "enable CSRF protection unless the API is stateless and token-based",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "csrf.enabled=false"}},
				}
			}
			if matchSimple(content, `csrf.*disabled`) {
				return &FindingProxy{
					ID:             "spring_security.csrf_disabled",
					Severity:       "medium",
					Title:          "CSRF protection disabled",
					Recommendation: "enable CSRF protection unless the API is stateless and token-based",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "csrf disabled"}},
				}
			}
			return nil
		},
	},
	{
		Framework:      "spring_security",
		ID:             "spring_security.permit_all",
		Severity:       "high",
		Title:          "Spring Security permits all requests",
		Recommendation: "restrict permitAll to specific public endpoints only",
		FilePatterns:   []string{"*.java", "*.yml", "*.yaml", "*.xml"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if matchSimple(content, `permitAll\(\)`) {
				line := findLineWith(content, "permitAll")
				return &FindingProxy{
					ID:             "spring_security.permit_all",
					Severity:       "high",
					Title:          "Spring Security permits all requests",
					Recommendation: "restrict permitAll to specific public endpoints only",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: line}},
				}
			}
			return nil
		},
	},

	// === MyBatis ===
	{
		Framework:      "mybatis",
		ID:             "mybatis.dollar_placeholder",
		Severity:       "high",
		Title:          "MyBatis uses ${} placeholder (SQL injection risk)",
		Recommendation: "use #{} instead of ${} to prevent SQL injection",
		FilePatterns:   []string{"*Mapper.xml", "*.xml"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if matchSimple(content, `\$\{`) {
				line := findLineWith(content, "${")
				return &FindingProxy{
					ID:             "mybatis.dollar_placeholder",
					Severity:       "high",
					Title:          "MyBatis uses ${} placeholder (SQL injection risk)",
					Recommendation: "use #{} instead of ${} to prevent SQL injection",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: line}},
				}
			}
			return nil
		},
	},

	// === JPA/Hibernate ===
	{
		Framework:      "jpa",
		ID:             "jpa.show_sql",
		Severity:       "low",
		Title:          "JPA/Hibernate SQL logging enabled",
		Recommendation: "disable show-sql in production to prevent information leakage",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties", "persistence.xml"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["spring.jpa.show-sql"]; ok && v == "true" {
				return &FindingProxy{
					ID:             "jpa.show_sql",
					Severity:       "low",
					Title:          "JPA/Hibernate SQL logging enabled",
					Recommendation: "disable show-sql in production to prevent information leakage",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "show-sql=true"}},
				}
			}
			if matchSimple(content, `show-sql:\s*true`) || matchSimple(content, `show_sql.*true`) {
				return &FindingProxy{
					ID:             "jpa.show_sql",
					Severity:       "low",
					Title:          "JPA/Hibernate SQL logging enabled",
					Recommendation: "disable show-sql in production to prevent information leakage",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "show-sql: true"}},
				}
			}
			return nil
		},
	},
	{
		Framework:      "jpa",
		ID:             "jpa.password.inline",
		Severity:       "high",
		Title:          "JPA/Hibernate password in configuration",
		Recommendation: "move database passwords to environment variables",
		FilePatterns:   []string{"persistence.xml", "*.xml"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["javax.persistence.jdbc.password"]; ok && v != "" && !isPlaceholder(v) {
				return &FindingProxy{
					ID:             "jpa.password.inline",
					Severity:       "high",
					Title:          "JPA/Hibernate password in configuration",
					Recommendation: "move database passwords to environment variables",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "password=" + maskSecret(v)}},
				}
			}
			return nil
		},
	},

	// === Dubbo ===
	{
		Framework:      "dubbo",
		ID:             "dubbo.token.inline",
		Severity:       "medium",
		Title:          "Dubbo token configured inline",
		Recommendation: "move token to external configuration or secret manager",
		FilePatterns:   []string{"dubbo.xml", "application*.yml", "application*.yaml", "application*.properties"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["dubbo.protocol.token"]; ok && v != "" {
				return &FindingProxy{
					ID:             "dubbo.token.inline",
					Severity:       "medium",
					Title:          "Dubbo token configured inline",
					Recommendation: "move token to external configuration or secret manager",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "dubbo.protocol.token=" + maskSecret(v)}},
				}
			}
			return nil
		},
	},

	// === Spring Cloud ===
	{
		Framework:      "spring_cloud",
		ID:             "spring_cloud.secret.inline",
		Severity:       "high",
		Title:          "Spring Cloud sensitive secret in config",
		Recommendation: "use Vault or encrypted config server instead of inline secrets",
		FilePatterns:   []string{"bootstrap*.yml", "bootstrap*.yaml", "bootstrap*.properties", "application*.yml"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			for k, v := range kv {
				if containsStr(k, "secret") || containsStr(k, "password") || containsStr(k, "key") {
					if !isPlaceholder(v) && v != "" {
						return &FindingProxy{
							ID:             "spring_cloud.secret.inline",
							Severity:       "high",
							Title:          "Spring Cloud sensitive secret in config",
							Recommendation: "use Vault or encrypted config server instead of inline secrets",
							Evidence:       []EvidenceProxy{{File: fp, Snippet: k + "=" + maskSecret(v)}},
						}
					}
				}
			}
			return nil
		},
	},

	// === JFinal ===
	{
		Framework:      "jfinal",
		ID:             "jfinal.password.inline",
		Severity:       "high",
		Title:          "JFinal database password in config",
		Recommendation: "use environment variables for database credentials",
		FilePatterns:   []string{"*.txt", "*.properties", "*.yml", "*.yaml"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["jdbc.password"]; ok && v != "" && !isPlaceholder(v) {
				return &FindingProxy{
					ID:             "jfinal.password.inline",
					Severity:       "high",
					Title:          "JFinal database password in config",
					Recommendation: "use environment variables for database credentials",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "jdbc.password=" + maskSecret(v)}},
				}
			}
			return nil
		},
	},

	// === Vert.x ===
	{
		Framework:      "vertx",
		ID:             "vertx.admin.enabled",
		Severity:       "medium",
		Title:          "Vert.x admin endpoint enabled",
		Recommendation: "disable admin endpoints in production or restrict access",
		FilePatterns:   []string{"application*.conf", "application*.yml", "application*.yaml"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["vertx.admin.enabled"]; ok && v == "true" {
				return &FindingProxy{
					ID:             "vertx.admin.enabled",
					Severity:       "medium",
					Title:          "Vert.x admin endpoint enabled",
					Recommendation: "disable admin endpoints in production or restrict access",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "vertx.admin.enabled=true"}},
				}
			}
			return nil
		},
	},

	// === Play ===
	{
		Framework:      "play",
		ID:             "play.crypto.secret",
		Severity:       "critical",
		Title:          "Play Framework crypto secret in config",
		Recommendation: "use environment variable APPLICATION_SECRET; rotate exposed keys",
		FilePatterns:   []string{"application.conf", "application*.conf"},
		Check: func(content string, kv map[string]string, fp string) *FindingProxy {
			if v, ok := kv["play.crypto.secret"]; ok && v != "" && !isPlaceholder(v) {
				return &FindingProxy{
					ID:             "play.crypto.secret",
					Severity:       "critical",
					Title:          "Play Framework crypto secret in config",
					Recommendation: "use environment variable APPLICATION_SECRET; rotate exposed keys",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "play.crypto.secret=" + maskSecret(v)}},
				}
			}
			if v, ok := kv["play.http.secret.key"]; ok && v != "" && !isPlaceholder(v) {
				return &FindingProxy{
					ID:             "play.crypto.secret",
					Severity:       "critical",
					Title:          "Play Framework crypto secret in config",
					Recommendation: "use environment variable APPLICATION_SECRET; rotate exposed keys",
					Evidence:       []EvidenceProxy{{File: fp, Snippet: "play.http.secret.key=" + maskSecret(v)}},
				}
			}
			return nil
		},
	},
}

// GetConfigRules returns config rules filtered by language and framework.
func GetConfigRules(language, framework string) []ConfigCheckRule {
	var out []ConfigCheckRule
	for _, r := range JavaConfigRules {
		if r.Framework == framework {
			out = append(out, r)
		}
	}
	return out
}

// GetCmsConfigRules returns config rules for CMS-specific checks.
// These are rules that apply regardless of framework when a CMS is detected.
func GetCmsConfigRules(cmsID string) []ConfigCheckRule {
	switch cmsID {
	case "ruoyi", "ruoyi-cloud":
		return []ConfigCheckRule{
			{
				Framework:      "ruoyi",
				ID:             "ruoyi.password.plain",
				Severity:       "high",
				Title:          "RuoYi database password in plain text",
				Recommendation: "move database password to environment variable or encrypted config",
				FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
				Check: func(content string, kv map[string]string, fp string) *FindingProxy {
					if v, ok := kv["spring.datasource.password"]; ok && v != "" && !isPlaceholder(v) {
						return &FindingProxy{
							ID:             "ruoyi.password.plain",
							Severity:       "high",
							Title:          "RuoYi database password in plain text",
							Recommendation: "move database password to environment variable or encrypted config",
							Evidence:       []EvidenceProxy{{File: fp, Snippet: "spring.datasource.password=" + maskSecret(v)}},
						}
					}
					return nil
				},
			},
		}
	default:
		return nil
	}
}
