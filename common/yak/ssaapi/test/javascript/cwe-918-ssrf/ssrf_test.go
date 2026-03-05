package cwe918ssrf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func loadRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-918-server-side-request-forgery/js-request-forgery.sf")
	if !ok {
		t.Skip("js-request-forgery.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content)
	return content
}

func runOnFile(t *testing.T, rule, filename, code string) int {
	t.Helper()
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, code)
	total := 0
	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		require.Greater(t, len(programs), 0)
		result, err := programs[0].SyntaxFlowWithError(rule)
		require.NoError(t, err)
		for _, v := range result.GetAlertVariables() {
			total += len(result.GetValues(v))
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JS))
	return total
}

// TestSSRF_Positive 验证用户输入拼入 HTTP 请求 URL 触发告警。
func TestSSRF_Positive(t *testing.T) {
	rule := loadRule(t)
	code := `
import http from 'http';

const server = http.createServer(function(req, res) {
    const target = new URL(req.url, "http://example.com").searchParams.get("target");

    // BAD: target is controlled by the attacker
    http.get('https://' + target + ".example.com/data/", res => {
        // process request response ...
    });
});
`
	total := runOnFile(t, rule, "positive.js", code)
	assert.Greater(t, total, 0, "URL 拼接用户输入应触发 SSRF 告警（漏报）")
}

// TestSSRF_NegativeAllowlist 验证服务端 allowlist 映射不触发告警。
func TestSSRF_NegativeAllowlist(t *testing.T) {
	rule := loadRule(t)
	code := `
import http from 'http';

const server = http.createServer(function(req, res) {
    const target = new URL(req.url, "http://example.com").searchParams.get("target");

    let subdomain;
    if (target === 'EU') {
        subdomain = "europe"
    } else {
        subdomain = "world"
    }

    // GOOD: subdomain is controlled by the server
    http.get('https://' + subdomain + ".example.com/data/", res => {
        // process request response ...
    });
});
`
	total := runOnFile(t, rule, "negative.js", code)
	assert.Equal(t, 0, total, "allowlist 映射（phi of constants）不应触发 SSRF 告警（误报）")
}

// TestSSRF_NegativeEqGuard 验证等值常量守卫不触发告警。
func TestSSRF_NegativeEqGuard(t *testing.T) {
	rule := loadRule(t)
	code := `
const http = require('http');

const TRUSTED_API = "https://trusted-api.example.com/data";

const server = http.createServer(function(req, res) {
    const url = req.query.url;
    // GOOD: strict equality check ensures url can only be the trusted constant
    if (url === TRUSTED_API) {
        http.get(url, res2 => { res2.pipe(res); });
    } else {
        res.writeHead(400);
        res.end("Invalid URL");
    }
});
`
	total := runOnFile(t, rule, "negative_eq_guard.js", code)
	assert.Equal(t, 0, total, "等值常量守卫后不应触发 SSRF 告警（误报）")
}

// TestSSRF_AxiosPositive 验证 axios 请求携带用户输入触发告警。
func TestSSRF_AxiosPositive(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require('express');
const axios = require('axios');
const app = express();

app.get('/proxy', async (req, res) => {
    const url = req.query.url;
    // BAD: url is fully attacker-controlled
    const response = await axios.get(url);
    res.json(response.data);
});
`
	total := runOnFile(t, rule, "axios.js", code)
	assert.Greater(t, total, 0, "axios 直接使用用户输入 URL 应触发告警")
}

// TestSSRF_FetchPathPositive 验证 fetch 路径包含用户输入触发告警。
func TestSSRF_FetchPathPositive(t *testing.T) {
	rule := loadRule(t)
	code := `
const express = require('express');
const app = express();

app.get('/api/proxy', async (req, res) => {
    const path = req.query.path;
    // BAD: path is attacker-controlled
    const response = await fetch('https://internal-api.example.com/' + path);
    const data = await response.json();
    res.json(data);
});
`
	total := runOnFile(t, rule, "fetch_path.js", code)
	assert.Greater(t, total, 0, "fetch 路径包含用户输入应触发告警")
}

// TestSSRF_PartialAllowlistPositive 验证部分 allowlist（else 含用户输入）触发告警。
func TestSSRF_PartialAllowlistPositive(t *testing.T) {
	rule := loadRule(t)
	code := `
const http = require('http');

const server = http.createServer(function(req, res) {
    const target = req.query.target;

    let host;
    if (target === 'safe') {
        host = "safe.example.com";
    } else {
        // BAD: else branch is still attacker-controlled
        host = target;
    }

    http.get('https://' + host + '/api', res2 => {});
});
`
	total := runOnFile(t, rule, "partial_allowlist.js", code)
	assert.Greater(t, total, 0, "部分 allowlist（else 含用户输入）应触发告警")
}

// TestSSRF_HardcodedNoAlert 验证硬编码 URL 不触发告警。
func TestSSRF_HardcodedNoAlert(t *testing.T) {
	rule := loadRule(t)
	code := `
const axios = require('axios');

async function fetchPublicData() {
    // GOOD: URL is hardcoded, not user-controlled
    const response = await axios.get('https://api.example.com/public/data');
    return response.data;
}
`
	total := runOnFile(t, rule, "hardcoded.js", code)
	assert.Equal(t, 0, total, "硬编码 URL 不应触发告警（误报）")
}
