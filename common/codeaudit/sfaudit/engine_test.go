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
		name: "secret.config_sensitive_value positive/negative",
		rule: "secret.config_sensitive_value",
		files: map[string]string{
			"application.properties": "spring.datasource.password=SuperSecret123\n# comment",
			"safe.properties":        "password = changeme\napi.key = ${API_KEY}",
		},
		wantHits: 1,
		contains: "SuperSecret123",
	},
	{
		name: "actuator exposed yaml",
		rule: "java.spring.actuator.exposed",
		files: map[string]string{
			"application.yml": "management:\n  endpoints:\n    web:\n      exposure:\n        include: \"*\"\n",
			"safe.yml":        "management:\n  endpoints:\n    web:\n      exposure:\n        include: health,info\n",
		},
		wantHits: 1,
		contains: "include: \"*\"",
	},
	{
		name:     "actuator exposed properties",
		rule:     "java.spring.actuator.exposed",
		files:    map[string]string{"application.properties": "management.endpoints.web.exposure.include=*"},
		wantHits: 1,
		contains: "include=*",
	},
	{
		name: "stacktrace always",
		rule: "java.spring.error.stacktrace",
		files: map[string]string{
			"application.yml":        "server:\n  error:\n    include-stacktrace: always\n",
			"application.properties": "server.error.include-stacktrace=never",
		},
		wantHits: 1,
		contains: "include-stacktrace: always",
	},
	{
		name: "datasource password yaml positive/negative",
		rule: "java.spring.datasource.password.plain",
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
		rule:     "java.spring.datasource.password.plain",
		files:    map[string]string{"application.properties": "spring.datasource.password=ruoyi@2024"},
		wantHits: 1,
		contains: "ruoyi@2024",
	},
	{
		name:     "cors wildcard",
		rule:     "java.spring.cors.wildcard",
		files:    map[string]string{"application.yml": "cors:\n  allowed-origins: \"*\"\n"},
		wantHits: 1,
		contains: "allowed-origins",
	},
	{
		name:     "devtools secret",
		rule:     "java.spring.devtools.secret",
		files:    map[string]string{"application.properties": "spring.devtools.remote.secret=my-secret"},
		wantHits: 1,
		contains: "my-secret",
	},
	{
		name:     "java.shiro.anon.url",
		rule:     "java.shiro.anon.url",
		files:    map[string]string{"shiro.ini": "[urls]\n/admin/** = anon\n/login = authc\n"},
		wantHits: 1,
		contains: "/admin/** = anon",
	},
	{
		name:     "shiro cipher key",
		rule:     "java.shiro.remember_me.cipher_key",
		files:    map[string]string{"shiro.ini": "securityManager.rememberMeManager.cipherKey = kPH+bIxk5D2deZiIxcaaaA==\n"},
		wantHits: 1,
		contains: "kPH+bIxk5D2deZiIxcaaaA==",
	},
	{
		name:     "struts2 devmode xml",
		rule:     "java.struts2.dev_mode",
		files:    map[string]string{"struts.xml": "<struts>\n  <constant name=\"struts.devMode\" value=\"true\" />\n</struts>\n"},
		wantHits: 1,
		contains: "struts.devMode",
	},
	{
		name:     "struts2 devmode properties",
		rule:     "java.struts2.dev_mode",
		files:    map[string]string{"struts.properties": "struts.devMode = true\n"},
		wantHits: 1,
		contains: "struts.devMode = true",
	},
	{
		name:     "struts2 dmi",
		rule:     "java.struts2.dynamic_method_invocation",
		files:    map[string]string{"struts.xml": "<constant name=\"struts.enable.DynamicMethodInvocation\" value=\"true\" />\n"},
		wantHits: 1,
		contains: "DynamicMethodInvocation",
	},
	{
		name:     "servlet security-constraint present",
		rule:     "java.servlet.missing_security_headers",
		files:    map[string]string{"web.xml": "<web-app>\n  <security-constraint>...</security-constraint>\n</web-app>\n"},
		wantHits: 1,
		contains: "security-constraint",
	},
	{
		name:     "csrf disabled",
		rule:     "java.spring_security.csrf_disabled",
		files:    map[string]string{"spring-security.xml": "<http><csrf disabled=\"true\"/></http>\n"},
		wantHits: 1,
		contains: "csrf",
	},
	{
		name:     "permit all",
		rule:     "java.spring_security.permit_all",
		files:    map[string]string{"SecurityConfig.java": "http.authorizeRequests().anyRequest().permitAll();\n"},
		wantHits: 1,
		contains: "permitAll()",
	},
	{
		name:     "mybatis dollar placeholder",
		rule:     "java.mybatis.dollar_placeholder",
		files:    map[string]string{"UserMapper.xml": "<select id=\"getUser\">SELECT * FROM user WHERE name = '${name}'</select>\n"},
		wantHits: 1,
		contains: "${name}",
	},
	{
		name:     "jpa show sql",
		rule:     "java.jpa.show_sql",
		files:    map[string]string{"application.yml": "spring:\n  jpa:\n    show-sql: true\n"},
		wantHits: 1,
		contains: "show-sql: true",
	},
	{
		name:     "java.jpa.password.inline",
		rule:     "java.jpa.password.inline",
		files:    map[string]string{"persistence.xml": "<property name=\"javax.persistence.jdbc.password\" value=\"secret123\" />\n"},
		wantHits: 1,
		contains: "javax.persistence.jdbc.password",
	},
	{
		name:     "java.dubbo.token.inline",
		rule:     "java.dubbo.token.inline",
		files:    map[string]string{"application.properties": "dubbo.protocol.token=abc123"},
		wantHits: 1,
		contains: "dubbo.protocol.token=abc123",
	},
	{
		name: "spring cloud secret inline positive/negative",
		rule: "java.spring_cloud.secret.inline",
		files: map[string]string{
			"bootstrap.yml":      "spring:\n  cloud:\n    config:\n      password: sc-password\n",
			"bootstrap-safe.yml": "spring:\n  cloud:\n    config:\n      password: ${CONFIG_PASS}\n",
		},
		wantHits: 1,
		contains: "sc-password",
	},
	{
		name:     "java.jfinal.password.inline",
		rule:     "java.jfinal.password.inline",
		files:    map[string]string{"config.txt": "jdbc.password = jf123456"},
		wantHits: 1,
		contains: "jf123456",
	},
	{
		name:     "java.vertx.admin.enabled",
		rule:     "java.vertx.admin.enabled",
		files:    map[string]string{"application.conf": "vertx.admin.enabled = true"},
		wantHits: 1,
		contains: "vertx.admin.enabled = true",
	},
	{
		name: "java.play.crypto.secret positive/negative",
		rule: "java.play.crypto.secret",
		files: map[string]string{
			"application.conf":      "play.crypto.secret = \"abcdefgh12345678\"\n",
			"application-safe.conf": "play.crypto.secret = \"changeme\"\n",
		},
		wantHits: 1,
		contains: "abcdefgh12345678",
	},
	{
		name:     "java.ruoyi.password.plain yaml",
		rule:     "java.ruoyi.password.plain",
		files:    map[string]string{"application.yml": "spring:\n  datasource:\n    druid:\n      username: ruoyi\n      password: ruoyi@2024\n"},
		wantHits: 1,
		contains: "ruoyi@2024",
	},
	// === generic multi-language secrets ===
	{
		name: "db url credential positive/negative",
		rule: "secret.db_url_credential",
		files: map[string]string{
			"config.py": "DATABASE_URL = \"postgresql://app:dbpass123@db.internal:5432/prod\"",
			"docs.md":   "see mysql://user:password@localhost:3306/demo for the dev sandbox",
			"k8s.conf":  "REDIS_URL=redis://cache:S3cret@redis.internal:6379",
			"clean.py":  "url = \"mysql://localhost:3306/app\"",
		},
		wantHits: 2,
		contains: "postgresql://app:dbpass123@db.internal:5432/prod",
	},
	{
		name: "dotenv credential positive/negative",
		rule: "secret.dotenv_credential",
		files: map[string]string{
			".env":        "DB_PASSWORD=SuperSecret123\nGITHUB_TOKEN=ghp_abc123def456\nAPP_ENV=production\n",
			"env.example": "DB_PASSWORD=changeme\nAPI_KEY=${API_KEY}\n",
		},
		wantHits: 2,
		contains: "SuperSecret123",
	},
	// === python ===
	{
		name: "django debug positive/negative",
		rule: "py.django.debug",
		files: map[string]string{
			"settings.py": "DEBUG = True\nALLOWED_HOSTS = []\n",
			"prod.py":     "DEBUG = False\n",
		},
		wantHits: 1,
		contains: "DEBUG = True",
	},
	{
		name: "django secret key positive/negative",
		rule: "py.django.secret_key",
		files: map[string]string{
			"settings.py": "SECRET_KEY = 'django-insecure-v8#1o^k2pz&5wq!abc'\n",
			"safe.py":     "SECRET_KEY = os.environ['DJANGO_SECRET_KEY']\n",
			"ph.py":       "SECRET_KEY = 'changeme'\n",
		},
		wantHits: 1,
		contains: "django-insecure",
	},
	{
		name:     "django allowed hosts wildcard",
		rule:     "py.django.allowed_hosts_wildcard",
		files:    map[string]string{"settings.py": "ALLOWED_HOSTS = ['*']\n"},
		wantHits: 1,
		contains: "ALLOWED_HOSTS = ['*']",
	},
	{
		name:     "django cors allow all",
		rule:     "py.django.cors_allow_all",
		files:    map[string]string{"settings.py": "CORS_ALLOW_ALL_ORIGINS = True\n"},
		wantHits: 1,
		contains: "CORS_ALLOW_ALL_ORIGINS = True",
	},
	{
		name: "flask debug run positive/negative",
		rule: "py.flask.debug_run",
		files: map[string]string{
			"app.py":  "app.run(debug=True)\n",
			"prod.py": "app.run(port=8000)\n",
		},
		wantHits: 1,
		contains: "debug=True",
	},
	{
		name:     "flask host all interfaces",
		rule:     "py.flask.host_all_interfaces",
		files:    map[string]string{"app.py": "app.run(host='0.0.0.0', port=8000)\n"},
		wantHits: 1,
		contains: "0.0.0.0",
	},
	{
		name:     "pickle load",
		rule:     "py.pickle.load",
		files:    map[string]string{"loader.py": "data = pickle.loads(request.data)\n"},
		wantHits: 1,
		contains: "pickle.loads",
	},
	{
		name: "yaml unsafe load positive/negative",
		rule: "py.yaml.unsafe_load",
		files: map[string]string{
			"bad.py":  "cfg = yaml.load(stream)\ncfg2 = yaml.unsafe_load(f)\n",
			"safe.py": "cfg = yaml.load(stream, Loader=yaml.SafeLoader)\ncfg2 = yaml.safe_load(f)\n",
		},
		wantHits: 2,
		contains: "yaml.load(stream)",
	},
	{
		name:     "requests verify false",
		rule:     "py.requests.verify_false",
		files:    map[string]string{"client.py": "resp = requests.get(url, verify=False)\n"},
		wantHits: 1,
		contains: "verify=False",
	},
	{
		name:     "subprocess shell true",
		rule:     "py.subprocess.shell_true",
		files:    map[string]string{"runner.py": "subprocess.Popen(cmd, shell=True)\n"},
		wantHits: 1,
		contains: "shell=True",
	},
	// === go ===
	{
		name:     "go insecure skip verify",
		rule:     "go.tls.insecure_skip_verify",
		files:    map[string]string{"main.go": "tls.Config{InsecureSkipVerify: true}\n"},
		wantHits: 1,
		contains: "InsecureSkipVerify: true",
	},
	{
		name: "go weak hash positive/negative",
		rule: "go.crypto.weak_hash",
		files: map[string]string{
			"hash.go": "h := md5.New()\ns := sha1.Sum(data)\n",
			"ok.go":   "h := sha256.New()\n",
		},
		wantHits: 2,
		contains: "md5.New()",
	},
	{
		name: "go hardcoded password positive/negative",
		rule: "go.hardcoded.password",
		files: map[string]string{
			"db.go":   "password := \"SuperSecret123\"\n",
			"safe.go": "password := os.Getenv(\"DB_PASSWORD\")\nif password == \"literal\" {\n",
		},
		wantHits: 1,
		contains: "SuperSecret123",
	},
	{
		name: "go http client no timeout positive/negative",
		rule: "go.http.client_no_timeout",
		files: map[string]string{
			"bad.go":  "client := &http.Client{}\n",
			"good.go": "client := &http.Client{Timeout: 10 * time.Second}\n",
		},
		wantHits: 1,
		contains: "http.Client{}",
	},
	{
		name:     "go jwt none alg",
		rule:     "go.jwt.none_alg",
		files:    map[string]string{"auth.go": "token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)\n"},
		wantHits: 1,
		contains: "SigningMethodNone",
	},
	// === php ===
	{
		name: "laravel app key positive/negative",
		rule: "php.laravel.app_key",
		files: map[string]string{
			".env":        "APP_KEY=base64:6LZaO1fGh2NkQ9rT3vXwY8uJm4BcD5eF7gHiJkLmNoP=\n",
			".env.sample": "APP_KEY=base64:xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx=\nAPP_KEY=\n",
		},
		wantHits: 1,
		contains: "base64:6LZa",
	},
	{
		name: "dotenv debug positive/negative",
		rule: "php.dotenv.debug",
		files: map[string]string{
			".env":      "APP_DEBUG=true\n",
			".env.prod": "APP_DEBUG=false\n",
		},
		wantHits: 1,
		contains: "APP_DEBUG=true",
	},
	{
		name: "php db password config positive/negative",
		rule: "php.db.password_config",
		files: map[string]string{
			"database.php": "'password' => 'P@ssw0rd2024',\n",
			"safe.php":     "'password' => env('DB_PASSWORD'),\n",
		},
		wantHits: 1,
		contains: "P@ssw0rd2024",
	},
	{
		name:     "wordpress db password",
		rule:     "php.wordpress.db_password",
		files:    map[string]string{"wp-config.php": "define( 'DB_PASSWORD', 'W0rdpress!2024' );\n"},
		wantHits: 1,
		contains: "W0rdpress!2024",
	},
	{
		name:     "php eval expr",
		rule:     "php.eval.expr",
		files:    map[string]string{"plugin.php": "eval($code);\n"},
		wantHits: 1,
		contains: "eval($code)",
	},
	{
		name:     "php command system",
		rule:     "php.command.system",
		files:    map[string]string{"shell.php": "system($_GET['cmd']);\n"},
		wantHits: 1,
		contains: "system(",
	},
	{
		name:     "php unserialize",
		rule:     "php.unserialize",
		files:    map[string]string{"data.php": "$obj = unserialize($input);\n"},
		wantHits: 1,
		contains: "unserialize($input)",
	},
	{
		name: "php md5 var positive/negative",
		rule: "php.crypto.md5_var",
		files: map[string]string{
			"auth.php":  "$hash = md5($password);\n",
			"const.php": "$h = md5('bootstrap');\n",
		},
		wantHits: 1,
		contains: "md5($password)",
	},
	// === node ===
	{
		name:     "node tls reject unauthorized",
		rule:     "node.tls.reject_unauthorized",
		files:    map[string]string{"agent.js": "https.get(url, { rejectUnauthorized: false });\n"},
		wantHits: 1,
		contains: "rejectUnauthorized: false",
	},
	{
		name: "node password plain positive/negative",
		rule: "node.password.plain",
		files: map[string]string{
			"db.js":     "const password = 'SuperSecret123';\n",
			"conf.json": "{ \"db_password\": \"hunter2prod\" }\n",
			"safe.js":   "const password = process.env.DB_PASSWORD;\n",
		},
		wantHits: 2,
		contains: "SuperSecret123",
	},
	{
		name: "node cors wildcard",
		rule: "node.cors.wildcard",
		files: map[string]string{
			"server.js": "app.use(cors({ origin: '*' }))\n",
			"hdr.js":    "res.setHeader('Access-Control-Allow-Origin', '*')\n",
		},
		wantHits: 2,
		contains: "origin: '*'",
	},
	{
		name: "node eval positive/negative",
		rule: "node.eval.expr",
		files: map[string]string{
			"run.js": "eval(userInput);\n",
			"neg.js": "const out = obj.eval(expr);\n",
		},
		wantHits: 1,
		contains: "eval(userInput)",
	},
	{
		name: "node child process exec positive/negative",
		rule: "node.child_process.exec",
		files: map[string]string{
			"run.js": "const { exec } = require('child_process');\nexec('ls -la', cb);\n",
			"neg.js": "const m = regex.exec(str);\n",
		},
		wantHits: 1,
		contains: "exec('ls -la'",
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
