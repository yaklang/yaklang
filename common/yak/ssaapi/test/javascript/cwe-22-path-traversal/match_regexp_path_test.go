package cwe22pathtraversal

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// runMiddlewareRule runs a SyntaxFlow rule snippet that uses matchRegexpPath
// and returns alert count.
func runMiddlewareRule(t *testing.T, code string) int {
	t.Helper()
	rule := `
// Step 1: regex without i flag in middleware
*.use(* as $middlewareArg,)
$middlewareArg?{opcode: const}?{have: /^\/.*\/[^i]*$/} as $caseSensitiveRegex

// Step 2: string-path endpoints
*.get(* as $endpointArg,)
*.post(* as $endpointArg,)
*.put(* as $endpointArg,)
*.delete(* as $endpointArg,)
*.patch(* as $endpointArg,)
*.all(* as $endpointArg,)
$endpointArg?{opcode: const}?{have: /^\//}?{!have: /^\/.*\/[gimsuy]*$/} as $stringEndpoint

// Step 3: use matchRegexpPath to verify the bypass relationship
$caseSensitiveRegex<matchRegexpPath(target="$stringEndpoint")> as $bypassRisk

alert $bypassRisk
`
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("test.js", code)
	count := 0
	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		result, err := programs[0].SyntaxFlowWithError(rule)
		if err != nil {
			fmt.Printf("  rule error: %v\n", err)
			return nil
		}
		for _, varName := range result.GetAlertVariables() {
			count += len(result.GetValues(varName))
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JS))
	return count
}

// TestMatchRegexpPath_VulnerableCase: regex /\/admin\/.*/ guards string-path '/admin/users/:id'
// Attacker sends /ADMIN/USERS/1 → bypass middleware, hit endpoint.
func TestMatchRegexpPath_VulnerableCase(t *testing.T) {
	count := runMiddlewareRule(t, `
const express = require('express');
const app = express();
app.use(/\/admin\/.*/, (req, res, next) => {
    if (!req.user.isAdmin) { res.status(401).send('Unauthorized'); } else { next(); }
});
app.get('/admin/users/:id', (req, res) => { res.send('user data'); });
`)
	assert.Greater(t, count, 0, "should detect bypass risk (false negative)")
}

// TestMatchRegexpPath_SafeWithIFlag: regex /\/admin\/.*/i already has i flag → no bypass.
func TestMatchRegexpPath_SafeWithIFlag(t *testing.T) {
	count := runMiddlewareRule(t, `
const express = require('express');
const app = express();
app.use(/\/admin\/.*/i, (req, res, next) => {
    if (!req.user.isAdmin) { res.status(401).send('Unauthorized'); } else { next(); }
});
app.get('/admin/users/:id', (req, res) => { res.send('user data'); });
`)
	assert.Equal(t, 0, count, "i-flag regex should not trigger (false positive)")
}

// TestMatchRegexpPath_UnrelatedPaths: regex /\/api\/.*/ does NOT guard '/admin/users/:id'.
// The uppercase of '/admin/users/1' is '/ADMIN/USERS/1' which doesn't match /\/api\/.*/i.
func TestMatchRegexpPath_UnrelatedPaths(t *testing.T) {
	count := runMiddlewareRule(t, `
const express = require('express');
const app = express();
app.use(/\/api\/.*/, (req, res, next) => { next(); });
app.get('/admin/users/:id', (req, res) => { res.send('user data'); });
`)
	assert.Equal(t, 0, count, "unrelated paths should not trigger (false positive)")
}

// TestMatchRegexpPath_NoStringEndpoint: regex without i but no string-path endpoints → no alert.
func TestMatchRegexpPath_NoStringEndpoint(t *testing.T) {
	count := runMiddlewareRule(t, `
const express = require('express');
const app = express();
app.use(/\/admin\/.*/, (req, res, next) => {
    if (!req.user.isAdmin) { res.status(401).send('Unauthorized'); } else { next(); }
});
app.get(/\/admin\/users\/\d+/i, (req, res) => { res.send('user data'); });
`)
	assert.Equal(t, 0, count, "regex-only endpoints should not trigger (false positive)")
}

// TestMatchRegexpPath_MultipleRoutes: regex guards multiple string paths, one is related.
func TestMatchRegexpPath_MultipleRoutes(t *testing.T) {
	count := runMiddlewareRule(t, `
const express = require('express');
const app = express();
app.use(/\/admin\/.*/, (req, res, next) => {
    if (!req.user.isAdmin) { res.status(401).send('Unauthorized'); } else { next(); }
});
app.get('/public/info', (req, res) => { res.send('public'); });
app.get('/admin/settings', (req, res) => { res.send('settings'); });
`)
	assert.Greater(t, count, 0, "should detect bypass via /admin/settings route")
}
