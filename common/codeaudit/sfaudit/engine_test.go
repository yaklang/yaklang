package sfaudit

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEngineSmokeAwsKey(t *testing.T) {
	files := map[string]string{
		"src/main/java/Const.java": "class Const { String key = \"AKIAIOSFODNN7EXAMPLE\"; }",
		"README.md":                "no keys here",
	}
	e := NewEngine("smoke", files)
	hits, err := e.Run(context.Background(), "secret.aws_access_key")
	require.NoError(t, err)
	require.Len(t, hits, 1)
	h := hits[0]
	require.Equal(t, "secret.aws_access_key", h.RuleID)
	require.Equal(t, "src/main/java/Const.java", h.File)
	require.Equal(t, 1, h.Line)
	require.Equal(t, "critical", h.Severity)
	require.Contains(t, h.Snippet, "AKIAIOSFODNN7EXAMPLE")
}

func TestEngineSmokeNoHits(t *testing.T) {
	e := NewEngine("smoke2", map[string]string{"a.txt": strings.Repeat("clean\n", 10)})
	hits, err := e.Run(context.Background(), "secret.aws_access_key")
	require.NoError(t, err)
	require.Empty(t, hits)
}

// ruleCases mirrors the behavior of the previous Go regex rules: each case
// runs one rule over a file set and asserts the hit count plus a snippet
// substring. Negative files are included in the same set to assert that
// placeholders and safe shapes do not fire.
var ruleCases = []struct {
	name     string
	rule     string
	files    map[string]string
	wantHits int
	contains string
}{
	{
		name: "password assignment positive/negative",
		rule: "secret.password_assignment",
		files: map[string]string{
			"a.java":         "String password = \"SuperSecret123\";",
			"safe.java":      "password = \"changeme\";\napi_key = \"password\";",
			"cfg.properties": "db.password = \"rootpw\"",
		},
		wantHits: 2,
		contains: "SuperSecret123",
	},
	{
		name:     "jdbc inline credential",
		rule:     "secret.jdbc_inline_credential",
		files:    map[string]string{"application.properties": "spring.datasource.url=jdbc:mysql://root:secret123@localhost:3306/app"},
		wantHits: 1,
		contains: "jdbc:mysql://root:secret123@localhost:3306/app",
	},
	{
		name:     "jdbc safe url",
		rule:     "secret.jdbc_inline_credential",
		files:    map[string]string{"application.properties": "spring.datasource.url=jdbc:mysql://localhost:3306/app"},
		wantHits: 0,
		contains: "",
	},
	{
		name:     "aws access key",
		rule:     "secret.aws_access_key",
		files:    map[string]string{"Const.java": "String a = \"AKIAIOSFODNN7EXAMPLE\";"},
		wantHits: 1,
		contains: "AKIAIOSFODNN7EXAMPLE",
	},
	{
		name:     "private key block",
		rule:     "secret.private_key_block",
		files:    map[string]string{"server.pem": "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA7\n-----END RSA PRIVATE KEY-----"},
		wantHits: 1,
		contains: "BEGIN RSA PRIVATE KEY",
	},
	{
		name:     "jwt literal",
		rule:     "secret.jwt_hardcoded",
		files:    map[string]string{"Auth.java": "String token = \"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N0XgL0n3I9PlFUP0THsR8U\";"},
		wantHits: 1,
		contains: "eyJhbGciOiJIUzI1NiJ9",
	},
	{
		name: "static final secret positive/negative",
		rule: "secret.static_final_secret",
		files: map[string]string{
			"Creds.java": "static final MY_PASSWORD = \"SuperSecret123\";",
			"Safe.java":  "static final MY_PASSWORD = \"changeme\";",
		},
		wantHits: 1,
		contains: "SuperSecret123",
	},
	{
		name: "config password property positive/negative",
		rule: "config.password.property",
		files: map[string]string{
			"application.properties": "spring.datasource.password=SuperSecret123\n# comment",
			"safe.properties":        "password = changeme\napi.key = ${API_KEY}",
		},
		wantHits: 1,
		contains: "SuperSecret123",
	},
	{
		name: "actuator exposed yaml",
		rule: "spring.actuator.exposed",
		files: map[string]string{
			"application.yml": "management:\n  endpoints:\n    web:\n      exposure:\n        include: \"*\"\n",
			"safe.yml":        "management:\n  endpoints:\n    web:\n      exposure:\n        include: health,info\n",
		},
		wantHits: 1,
		contains: "include: \"*\"",
	},
	{
		name:     "actuator exposed properties",
		rule:     "spring.actuator.exposed",
		files:    map[string]string{"application.properties": "management.endpoints.web.exposure.include=*"},
		wantHits: 1,
		contains: "include=*",
	},
	{
		name: "stacktrace always",
		rule: "spring.error.stacktrace",
		files: map[string]string{
			"application.yml":        "server:\n  error:\n    include-stacktrace: always\n",
			"application.properties": "server.error.include-stacktrace=never",
		},
		wantHits: 1,
		contains: "include-stacktrace: always",
	},
	{
		name: "datasource password yaml positive/negative",
		rule: "spring.datasource.password.plain",
		files: map[string]string{
			"application.yml":  "spring:\n  datasource:\n    username: root\n    password: SuperSecret123\n",
			"envsafe.yml":      "spring:\n  datasource:\n    password: ${DB_PASSWORD}\n",
			"placeholders.yml": "spring:\n  datasource:\n    password: changeme\n",
		},
		wantHits: 1,
		contains: "password: SuperSecret123",
	},
	{
		name:     "datasource password properties",
		rule:     "spring.datasource.password.plain",
		files:    map[string]string{"application.properties": "spring.datasource.password=ruoyi@2024"},
		wantHits: 1,
		contains: "ruoyi@2024",
	},
	{
		name:     "cors wildcard",
		rule:     "spring.cors.wildcard",
		files:    map[string]string{"application.yml": "cors:\n  allowed-origins: \"*\"\n"},
		wantHits: 1,
		contains: "allowed-origins",
	},
	{
		name:     "devtools secret",
		rule:     "spring.devtools.secret",
		files:    map[string]string{"application.properties": "spring.devtools.remote.secret=my-secret"},
		wantHits: 1,
		contains: "my-secret",
	},
	{
		name:     "shiro anon url",
		rule:     "shiro.anon.url",
		files:    map[string]string{"shiro.ini": "[urls]\n/admin/** = anon\n/login = authc\n"},
		wantHits: 1,
		contains: "/admin/** = anon",
	},
	{
		name:     "shiro cipher key",
		rule:     "shiro.rememberme.cipherKey",
		files:    map[string]string{"shiro.ini": "securityManager.rememberMeManager.cipherKey = kPH+bIxk5D2deZiIxcaaaA==\n"},
		wantHits: 1,
		contains: "kPH+bIxk5D2deZiIxcaaaA==",
	},
	{
		name:     "struts2 devmode xml",
		rule:     "struts2.devmode",
		files:    map[string]string{"struts.xml": "<struts>\n  <constant name=\"struts.devMode\" value=\"true\" />\n</struts>\n"},
		wantHits: 1,
		contains: "struts.devMode",
	},
	{
		name:     "struts2 devmode properties",
		rule:     "struts2.devmode",
		files:    map[string]string{"struts.properties": "struts.devMode = true\n"},
		wantHits: 1,
		contains: "struts.devMode = true",
	},
	{
		name:     "struts2 dmi",
		rule:     "struts2.dmi",
		files:    map[string]string{"struts.xml": "<constant name=\"struts.enable.DynamicMethodInvocation\" value=\"true\" />\n"},
		wantHits: 1,
		contains: "DynamicMethodInvocation",
	},
	{
		name:     "servlet security-constraint present",
		rule:     "servlet.missing_security_headers",
		files:    map[string]string{"web.xml": "<web-app>\n  <security-constraint>...</security-constraint>\n</web-app>\n"},
		wantHits: 1,
		contains: "security-constraint",
	},
	{
		name:     "csrf disabled",
		rule:     "spring_security.csrf_disabled",
		files:    map[string]string{"spring-security.xml": "<http><csrf disabled=\"true\"/></http>\n"},
		wantHits: 1,
		contains: "csrf",
	},
	{
		name:     "permit all",
		rule:     "spring_security.permit_all",
		files:    map[string]string{"SecurityConfig.java": "http.authorizeRequests().anyRequest().permitAll();\n"},
		wantHits: 1,
		contains: "permitAll()",
	},
	{
		name:     "mybatis dollar placeholder",
		rule:     "mybatis.dollar_placeholder",
		files:    map[string]string{"UserMapper.xml": "<select id=\"getUser\">SELECT * FROM user WHERE name = '${name}'</select>\n"},
		wantHits: 1,
		contains: "${name}",
	},
	{
		name:     "jpa show sql",
		rule:     "jpa.show_sql",
		files:    map[string]string{"application.yml": "spring:\n  jpa:\n    show-sql: true\n"},
		wantHits: 1,
		contains: "show-sql: true",
	},
	{
		name:     "jpa password inline",
		rule:     "jpa.password.inline",
		files:    map[string]string{"persistence.xml": "<property name=\"javax.persistence.jdbc.password\" value=\"secret123\" />\n"},
		wantHits: 1,
		contains: "javax.persistence.jdbc.password",
	},
	{
		name:     "dubbo token inline",
		rule:     "dubbo.token.inline",
		files:    map[string]string{"application.properties": "dubbo.protocol.token=abc123"},
		wantHits: 1,
		contains: "dubbo.protocol.token=abc123",
	},
	{
		name: "spring cloud secret inline positive/negative",
		rule: "spring_cloud.secret.inline",
		files: map[string]string{
			"bootstrap.yml":      "spring:\n  cloud:\n    config:\n      password: sc-password\n",
			"bootstrap-safe.yml": "spring:\n  cloud:\n    config:\n      password: ${CONFIG_PASS}\n",
		},
		wantHits: 1,
		contains: "sc-password",
	},
	{
		name:     "jfinal password inline",
		rule:     "jfinal.password.inline",
		files:    map[string]string{"config.txt": "jdbc.password = jf123456"},
		wantHits: 1,
		contains: "jf123456",
	},
	{
		name:     "vertx admin enabled",
		rule:     "vertx.admin.enabled",
		files:    map[string]string{"application.conf": "vertx.admin.enabled = true"},
		wantHits: 1,
		contains: "vertx.admin.enabled = true",
	},
	{
		name: "play crypto secret positive/negative",
		rule: "play.crypto.secret",
		files: map[string]string{
			"application.conf":      "play.crypto.secret = \"abcdefgh12345678\"\n",
			"application-safe.conf": "play.crypto.secret = \"changeme\"\n",
		},
		wantHits: 1,
		contains: "abcdefgh12345678",
	},
	{
		name:     "ruoyi password plain yaml",
		rule:     "ruoyi.password.plain",
		files:    map[string]string{"application.yml": "spring:\n  datasource:\n    druid:\n      username: ruoyi\n      password: ruoyi@2024\n"},
		wantHits: 1,
		contains: "ruoyi@2024",
	},
}

func TestRuleCases(t *testing.T) {
	for _, tc := range ruleCases {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEngine("rulecase", tc.files)
			hits, err := e.Run(context.Background(), tc.rule)
			require.NoError(t, err, "rule %s", tc.rule)
			require.Len(t, hits, tc.wantHits, "rule %s hits: %+v", tc.rule, hits)
			if tc.contains != "" {
				found := false
				for _, h := range hits {
					if strings.Contains(h.Snippet, tc.contains) {
						found = true
						require.NotEmpty(t, h.File)
						require.Greater(t, h.Line, 0)
					}
				}
				require.True(t, found, "expected a hit snippet containing %q", tc.contains)
			}
		})
	}
}

// TestRuleCaseCoverage welds the test table to the registry: every embedded
// rule must be exercised by at least one case, and every case must reference
// a registered rule, so the two cannot drift apart.
func TestRuleCaseCoverage(t *testing.T) {
	covered := map[string]bool{}
	for _, tc := range ruleCases {
		require.True(t, HasRule(tc.rule), "case %q references unregistered rule %q", tc.name, tc.rule)
		covered[tc.rule] = true
	}
	for _, id := range RuleIDs() {
		require.True(t, covered[id], "rule %q has no test case", id)
	}
}

// TestAllRulesCompile ensures every registered rule parses and executes
// without error, even against an empty file set.
func TestAllRulesCompile(t *testing.T) {
	for _, id := range RuleIDs() {
		e := NewEngine("compile-check", map[string]string{"placeholder.txt": "x"})
		_, err := e.Run(context.Background(), id)
		require.NoError(t, err, "rule %s", id)
	}
}
