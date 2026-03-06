package cwe502unsafedeserialization

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

// loadDeserRule 从内置 embed FS 读取 js-unsafe-deserialization.sf 规则内容。
func loadDeserRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-502-unsafe-deserialization/js-unsafe-deserialization.sf")
	if !ok {
		t.Skip("ecmascript/cwe-502-unsafe-deserialization/js-unsafe-deserialization.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "js-unsafe-deserialization.sf 内容为空")
	return content
}

// runOnFile 用单文件 VirtualFS 执行规则，返回 (totalAlerts, highAlerts)。
func runOnFile(t *testing.T, ruleContent, filename, code string) (total, high int) {
	t.Helper()
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, code)

	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		require.Greater(t, len(programs), 0, "SSA 编译应至少产生一个程序")
		result, err := programs[0].SyntaxFlowWithError(ruleContent)
		require.NoError(t, err, "规则执行不应报错")
		for _, varName := range result.GetAlertVariables() {
			vals := result.GetValues(varName)
			total += len(vals)
			if info, ok := result.GetAlertInfo(varName); ok {
				if info.Severity == "high" || info.Severity == "h" {
					high += len(vals)
				}
			}
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JS))
	return
}

// ============================================================
// js-yaml: load / loadAll — 危险用法
// ============================================================

// TestDeser_Positive_JsYamlLoad 验证 jsyaml.load(req.params.data) 直接注入触发 high 告警。
func TestDeser_Positive_JsYamlLoad(t *testing.T) {
	rule := loadDeserRule(t)
	total, high := runOnFile(t, rule, "jsyaml_load_positive.js", `
const app = require("express")(),
      jsyaml = require("js-yaml");

app.get("load", function(req, res) {
    let data = jsyaml.load(req.params.data);
    res.json({ result: data });
});
`)
	assert.Greater(t, total, 0, "jsyaml.load(req.params.data) 应触发告警（漏报）")
	assert.Greater(t, high, 0, "直接传入用户输入应触发 high 告警")
}

// TestDeser_Positive_JsYamlLoadAll 验证 jsyaml.loadAll(req.body.content) 触发 high 告警。
func TestDeser_Positive_JsYamlLoadAll(t *testing.T) {
	rule := loadDeserRule(t)
	total, high := runOnFile(t, rule, "jsyaml_loadall_positive.js", `
const express = require("express");
const jsyaml = require("js-yaml");
const app = express();
app.use(express.json());

app.post("/loadall", (req, res) => {
    const docs = jsyaml.loadAll(req.body.content);
    res.json({ docs });
});
`)
	assert.Greater(t, total, 0, "jsyaml.loadAll(req.body.content) 应触发告警（漏报）")
	assert.Greater(t, high, 0, "直接传入用户输入应触发 high 告警")
}

// ============================================================
// js-yaml: safeLoad — 安全用法（不应触发告警）
// ============================================================

// TestDeser_Negative_JsYamlSafeLoad 验证 jsyaml.safeLoad() 不触发任何告警。
// safeLoad() 是 js-yaml v3.x 的安全 API，不支持 JS 函数类型。
func TestDeser_Negative_JsYamlSafeLoad(t *testing.T) {
	rule := loadDeserRule(t)
	total, _ := runOnFile(t, rule, "jsyaml_safeload_negative.js", `
const app = require("express")(),
      jsyaml = require("js-yaml");

app.get("load", function(req, res) {
    // GOOD: safeLoad() only supports basic YAML types, no JS functions
    let data = jsyaml.safeLoad(req.params.data);
    res.json({ result: data });
});
`)
	assert.Equal(t, 0, total, "jsyaml.safeLoad() 不应触发任何告警（误报）")
}

// TestDeser_Negative_JsYamlStaticContent 验证 jsyaml.load() 处理静态字符串不触发告警。
func TestDeser_Negative_JsYamlStaticContent(t *testing.T) {
	rule := loadDeserRule(t)
	total, _ := runOnFile(t, rule, "jsyaml_static_negative.js", `
const jsyaml = require("js-yaml");

// GOOD: loading a hard-coded config string — no user input
const config = jsyaml.load("server:\n  host: localhost\n  port: 8080\n");
console.log(config);
`)
	assert.Equal(t, 0, total, "jsyaml.load() 处理静态字符串不应触发告警（误报）")
}

// ============================================================
// node-serialize: unserialize — 极度危险
// ============================================================

// TestDeser_Positive_NodeSerialize 验证 serialize.unserialize(req.body.data) 触发 high 告警。
// node-serialize 的 unserialize() 支持 IIFE 模式，是最危险的反序列化函数之一。
func TestDeser_Positive_NodeSerialize(t *testing.T) {
	rule := loadDeserRule(t)
	total, high := runOnFile(t, rule, "node_serialize_positive.js", `
const express = require("express");
const serialize = require("node-serialize");
const app = express();
app.use(express.json());

app.post("/restore", (req, res) => {
    // BAD: IIFE payload {"rce":"_$$ND_FUNC$$_function(){require('child_process').exec('id')}()"}
    const obj = serialize.unserialize(req.body.data);
    res.json(obj);
});
`)
	assert.Greater(t, total, 0, "serialize.unserialize(req.body.data) 应触发告警（漏报）")
	assert.Greater(t, high, 0, "直接传入用户输入应触发 high 告警")
}

// ============================================================
// funcster: deepDeserialize — 危险
// ============================================================

// TestDeser_Positive_Funcster 验证 funcster.deepDeserialize(req.body.payload) 触发 high 告警。
func TestDeser_Positive_Funcster(t *testing.T) {
	rule := loadDeserRule(t)
	total, high := runOnFile(t, rule, "funcster_positive.js", `
const express = require("express");
const funcster = require("funcster");
const app = express();
app.use(express.json());

app.post("/deserialize", (req, res) => {
    // BAD: deepDeserialize uses new Function() to restore serialized functions
    const result = funcster.deepDeserialize(req.body.payload);
    res.json({ result });
});
`)
	assert.Greater(t, total, 0, "funcster.deepDeserialize(req.body.payload) 应触发告警（漏报）")
	assert.Greater(t, high, 0, "直接传入用户输入应触发 high 告警")
}

// ============================================================
// 间接注入（MID）：通过中间调用流入
// ============================================================

// TestDeser_Mid_JsYamlLoadViaIntermediate 验证 jsyaml.load() 通过中间函数调用接收用户输入触发告警。
// processInput(req.query.data) 是中间调用，阻断了 HIGH 分类，但数据仍流向 load()，应触发 mid。
func TestDeser_Mid_JsYamlLoadViaIntermediate(t *testing.T) {
	rule := loadDeserRule(t)
	total, _ := runOnFile(t, rule, "jsyaml_mid.js", `
const express = require("express");
const jsyaml = require("js-yaml");
const app = express();

function processInput(raw) {
    return raw; // passes through
}

app.post("/load", (req, res) => {
    // Intermediate function call — prevents HIGH but data still flows to load()
    const yamlStr = processInput(req.query.data);
    const parsed = jsyaml.load(yamlStr);
    res.json({ parsed });
});
`)
	assert.Greater(t, total, 0, "经过中间函数调用的 jsyaml.load() 仍应触发告警（漏报）")
}
