package catalog

// ConfigCheckRule describes a configuration audit rule. The matching itself
// is delegated to the embedded SyntaxFlow source rule registered under ID in
// codeaudit/sfaudit; this struct carries the presentation metadata plus the
// glob patterns used to collect candidate files.
type ConfigCheckRule struct {
	Framework        string
	ID               string
	Severity         string
	Title            string
	Recommendation   string
	FilePatterns     []string // glob patterns, e.g. ["application*.yml", "application*.properties"]
	MaskValue        bool     // mask the value part of the evidence snippet
	ReportWhenAbsent bool     // report a finding when the rule produces no hits on collected files
}

// JavaConfigRules contains all Java framework configuration audit rules.
var JavaConfigRules = []ConfigCheckRule{
	// === Spring Boot ===
	{
		Framework:      "spring_boot",
		ID:             "java.spring.actuator.exposed",
		Severity:       "high",
		Title:          "Actuator endpoints overly exposed",
		Recommendation: "restrict management endpoints and protect with authentication",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
	},
	{
		Framework:      "spring_boot",
		ID:             "java.spring.error.stacktrace",
		Severity:       "medium",
		Title:          "Stack trace disclosure enabled",
		Recommendation: "set server.error.include-stacktrace to never or on_trace_param",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
	},
	{
		Framework:      "spring_boot",
		ID:             "java.spring.datasource.password.plain",
		Severity:       "high",
		Title:          "Plain-text database password in configuration",
		Recommendation: "move database credentials to environment variables or a secret manager",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
		MaskValue:      true,
	},
	{
		Framework:      "spring_boot",
		ID:             "java.spring.cors.wildcard",
		Severity:       "medium",
		Title:          "CORS allows all origins",
		Recommendation: "restrict allowed origins to specific trusted domains",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
	},
	{
		Framework:      "spring_boot",
		ID:             "java.spring.devtools.secret",
		Severity:       "high",
		Title:          "Spring DevTools remote secret exposed",
		Recommendation: "remove remote DevTools or use a strong, rotated secret",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
		MaskValue:      true,
	},

	// === Shiro ===
	{
		Framework:      "shiro",
		ID:             "java.shiro.anon.url",
		Severity:       "high",
		Title:          "Unauthenticated URL pattern in Shiro configuration",
		Recommendation: "remove overly broad anon patterns or restrict to specific public paths",
		FilePatterns:   []string{"shiro.ini", "*.ini"},
	},
	{
		Framework:      "shiro",
		ID:             "java.shiro.remember_me.cipher_key",
		Severity:       "critical",
		Title:          "Hardcoded Shiro rememberMe cipher key",
		Recommendation: "generate a unique random key; rotate any compromised keys",
		FilePatterns:   []string{"shiro.ini", "*.ini", "*.yml", "*.yaml", "*.properties"},
	},

	// === Struts2 ===
	{
		Framework:      "struts2",
		ID:             "java.struts2.dev_mode",
		Severity:       "high",
		Title:          "Struts2 devMode enabled in production",
		Recommendation: "set struts.devMode to false in production",
		FilePatterns:   []string{"struts.xml", "struts.properties"},
	},
	{
		Framework:      "struts2",
		ID:             "java.struts2.dynamic_method_invocation",
		Severity:       "high",
		Title:          "Struts2 dynamic method invocation enabled",
		Recommendation: "disable DMI to prevent method invocation attacks",
		FilePatterns:   []string{"struts.xml", "struts.properties"},
	},

	// === Servlet ===
	{
		Framework:        "servlet",
		ID:               "java.servlet.missing_security_headers",
		Severity:         "low",
		Title:            "Missing security headers in web.xml",
		Recommendation:   "add security headers such as X-Content-Type-Options, X-Frame-Options",
		FilePatterns:     []string{"web.xml"},
		ReportWhenAbsent: true,
	},

	// === Spring Security ===
	{
		Framework:      "spring_security",
		ID:             "java.spring_security.csrf_disabled",
		Severity:       "medium",
		Title:          "CSRF protection disabled",
		Recommendation: "enable CSRF protection unless the API is stateless and token-based",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties", "*.xml"},
	},
	{
		Framework:      "spring_security",
		ID:             "java.spring_security.permit_all",
		Severity:       "high",
		Title:          "Spring Security permits all requests",
		Recommendation: "restrict permitAll to specific public endpoints only",
		FilePatterns:   []string{"*.java", "*.yml", "*.yaml", "*.xml"},
	},

	// === MyBatis ===
	{
		Framework:      "mybatis",
		ID:             "java.mybatis.dollar_placeholder",
		Severity:       "high",
		Title:          "MyBatis uses ${} placeholder (SQL injection risk)",
		Recommendation: "use #{} instead of ${} to prevent SQL injection",
		FilePatterns:   []string{"*Mapper.xml", "*.xml"},
	},

	// === JPA/Hibernate ===
	{
		Framework:      "jpa",
		ID:             "java.jpa.show_sql",
		Severity:       "low",
		Title:          "JPA/Hibernate SQL logging enabled",
		Recommendation: "disable show-sql in production to prevent information leakage",
		FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties", "persistence.xml"},
	},
	{
		Framework:      "jpa",
		ID:             "java.jpa.password.inline",
		Severity:       "high",
		Title:          "JPA/Hibernate password in configuration",
		Recommendation: "move database passwords to environment variables",
		FilePatterns:   []string{"persistence.xml", "*.xml"},
		MaskValue:      true,
	},

	// === Dubbo ===
	{
		Framework:      "dubbo",
		ID:             "java.dubbo.token.inline",
		Severity:       "medium",
		Title:          "Dubbo token configured inline",
		Recommendation: "move token to external configuration or secret manager",
		FilePatterns:   []string{"dubbo.xml", "application*.yml", "application*.yaml", "application*.properties"},
		MaskValue:      true,
	},

	// === Spring Cloud ===
	{
		Framework:      "spring_cloud",
		ID:             "java.spring_cloud.secret.inline",
		Severity:       "high",
		Title:          "Spring Cloud sensitive secret in config",
		Recommendation: "use Vault or encrypted config server instead of inline secrets",
		FilePatterns:   []string{"bootstrap*.yml", "bootstrap*.yaml", "bootstrap*.properties", "application*.yml"},
		MaskValue:      true,
	},

	// === JFinal ===
	{
		Framework:      "jfinal",
		ID:             "java.jfinal.password.inline",
		Severity:       "high",
		Title:          "JFinal database password in config",
		Recommendation: "use environment variables for database credentials",
		FilePatterns:   []string{"*.txt", "*.properties", "*.yml", "*.yaml"},
		MaskValue:      true,
	},

	// === Vert.x ===
	{
		Framework:      "vertx",
		ID:             "java.vertx.admin.enabled",
		Severity:       "medium",
		Title:          "Vert.x admin endpoint enabled",
		Recommendation: "disable admin endpoints in production or restrict access",
		FilePatterns:   []string{"application*.conf", "application*.yml", "application*.yaml"},
	},

	// === Play ===
	{
		Framework:      "play",
		ID:             "java.play.crypto.secret",
		Severity:       "critical",
		Title:          "Play Framework crypto secret in config",
		Recommendation: "use environment variable APPLICATION_SECRET; rotate exposed keys",
		FilePatterns:   []string{"application.conf", "application*.conf"},
		MaskValue:      true,
	},
}

