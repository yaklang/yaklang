package codeaudit

import (
	"os"
	"path/filepath"
	"testing"
)

// multilangSamples holds the per-language sample projects used by the
// multilang tests. Fixtures are generated into t.TempDir() at test time, so
// nothing static (in particular no go.mod / main.go) lives in the repo tree.
var multilangSamples = map[string]map[string]string{
	"django": {
		"requirements.txt": "django==4.2.1\nrequests==2.31.0\n",
		"manage.py": "#!/usr/bin/env python\n" +
			"import os\n" +
			"import sys\n" +
			"def main():\n" +
			"    os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'myapp.settings')\n" +
			"    from django.core.management import execute_from_command_line\n" +
			"    execute_from_command_line(sys.argv)\n" +
			"if __name__ == '__main__':\n" +
			"    main()\n",
		"myapp/settings.py": "SECRET_KEY = 'django-insecure-v8#1o^k2pz&5wq!abc123'\n" +
			"DEBUG = True\n" +
			"ALLOWED_HOSTS = ['*']\n" +
			"CORS_ALLOW_ALL_ORIGINS = True\n" +
			"INSTALLED_APPS = [\n" +
			"    'django.contrib.admin',\n" +
			"    'django.contrib.auth',\n" +
			"]\n",
	},
	"gogin": {
		"go.mod": "module example.com/ginapp\n\ngo 1.21\n\nrequire github.com/gin-gonic/gin v1.9.1\n",
		"main.go": "package main\n" +
			"import (\n" +
			"    \"crypto/tls\"\n" +
			"    \"net/http\"\n" +
			"    \"github.com/gin-gonic/gin\"\n" +
			")\n" +
			"func main() {\n" +
			"    tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}\n" +
			"    client := &http.Client{Transport: tr}\n" +
			"    _ = client\n" +
			"    r := gin.Default()\n" +
			"    r.GET(\"/ping\", func(c *gin.Context) {\n" +
			"        c.JSON(http.StatusOK, gin.H{\"message\": \"pong\"})\n" +
			"    })\n" +
			"    r.Run()\n" +
			"}\n",
	},
	"wordpress": {
		"wp-config.php": "<?php\n" +
			"define( 'DB_NAME', 'wpdb' );\n" +
			"define( 'DB_USER', 'wpuser' );\n" +
			"define( 'DB_PASSWORD', 'W0rdpress!2024' );\n" +
			"define( 'DB_HOST', 'localhost' );\n" +
			"if ( ! defined( 'ABSPATH' ) ) {\n" +
			"    define( 'ABSPATH', __DIR__ . '/' );\n" +
			"}\n",
		"wp-settings.php": "<?php\n",
		"index.php": "<?php\ndefine( 'WP_USE_THEMES', true );\nrequire( dirname( __FILE__ ) . '/wp-blog-header.php' );\n",
		"composer.json": "{\n    \"name\": \"org/wordpress-site\",\n    \"require\": {\"php\": \">=7.4\"}\n}\n",
	},
	"express": {
		"package.json": "{\n" +
			"  \"name\": \"express-app\",\n" +
			"  \"version\": \"1.0.0\",\n" +
			"  \"dependencies\": {\n" +
			"    \"express\": \"^4.18.2\",\n" +
			"    \"node-serialize\": \"0.0.4\"\n" +
			"  }\n" +
			"}\n",
		"app.js": "const express = require('express');\n" +
			"const { exec } = require('child_process');\n" +
			"const app = express();\n" +
			"const password = 'SuperSecret123';\n" +
			"app.use(cors({ origin: '*' }));\n" +
			"app.get('/run', (req, res) => {\n" +
			"  exec(req.query.cmd, (err, out) => res.send(out));\n" +
			"});\n" +
			"app.get('/eval', (req, res) => {\n" +
			"  res.send(eval(req.query.expr));\n" +
			"});\n" +
			"process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';\n" +
			"app.listen(3000);\n",
	},
}

// multilangSampleDir materializes a named sample project into a fresh temp
// directory and returns its root path.
func multilangSampleDir(t *testing.T, name string) string {
	t.Helper()
	files, ok := multilangSamples[name]
	if !ok {
		t.Fatalf("unknown multilang sample %q", name)
	}
	root := t.TempDir()
	for rel, content := range files {
		fp := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(fp), err)
		}
		if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", fp, err)
		}
	}
	return root
}
