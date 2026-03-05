package cwe89sqlinjection

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

// loadSQLInjectionRule 从内置 embed FS 按路径读取 js-sql-injection.sf 规则内容。
// 若规则文件不在当前构建的 embed FS 中（如 irify_exclude 构建模式），则跳过测试。
func loadSQLInjectionRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-89-sql-injection/js-sql-injection.sf")
	if !ok {
		t.Skip("ecmascript/cwe-89-sql-injection/js-sql-injection.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "js-sql-injection.sf 内容为空")
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
// ex1: PostgreSQL SQL 注入
// ============================================================

// TestSQLInject_Ex1Positive_SQLConcatenation 检测 pg 字符串拼接注入（应报 high 告警）。
func TestSQLInject_Ex1Positive_SQLConcatenation(t *testing.T) {
	rule := loadSQLInjectionRule(t)
	total, high := runOnFile(t, rule, "ex1-positive-1.js", `
const app = require("express")(),
      pg = require("pg"),
      pool = new pg.Pool(config);

app.get("search", function handler(req, res) {
  // BAD: the category might have SQL special characters in it
  var query1 =
    "SELECT ITEM,PRICE FROM PRODUCT WHERE ITEM_CATEGORY='" +
    req.params.category +
    "' ORDER BY PRICE";
  pool.query(query1, [], function(err, results) {
    // process results
  });
});
`)
	assert.Greater(t, total, 0, "字符串拼接 SQL 注入应触发告警（漏报）")
	assert.Greater(t, high, 0, "字符串拼接 SQL 注入应触发 high 告警")
}

// TestSQLInject_Ex1Negative1_ParameterizedQuery 验证参数化查询不产生告警（应零告警）。
func TestSQLInject_Ex1Negative1_ParameterizedQuery(t *testing.T) {
	rule := loadSQLInjectionRule(t)
	total, _ := runOnFile(t, rule, "ex1-negative-1.js", `
const app = require("express")(),
      pg = require("pg"),
      pool = new pg.Pool(config);

app.get("search", function handler(req, res) {
  // GOOD: use parameters
  var query2 =
    "SELECT ITEM,PRICE FROM PRODUCT WHERE ITEM_CATEGORY=$1 ORDER BY PRICE";
  pool.query(query2, [req.params.category], function(err, results) {
    // process results
  });
});
`)
	assert.Equal(t, 0, total, "参数化查询不应触发任何告警（误报）")
}

// TestSQLInject_Ex1Negative2_SqlStringEscape 验证 SqlString.escape() 过滤后不产生高风险告警。
// SqlString.escape() 是已知的安全转义函数，应被排除在 high 告警范围外。
func TestSQLInject_Ex1Negative2_SqlStringEscape(t *testing.T) {
	rule := loadSQLInjectionRule(t)
	total, high := runOnFile(t, rule, "ex1-negative-2.js", `
const app = require("express")(),
      pg = require("pg"),
      SqlString = require('sqlstring'),
      pool = new pg.Pool(config);

app.get("search", function handler(req, res) {
  // GOOD: the category is escaped using SqlString.escape
  var query1 =
    "SELECT ITEM,PRICE FROM PRODUCT WHERE ITEM_CATEGORY='" +
    SqlString.escape(req.params.category) +
    "' ORDER BY PRICE";
  pool.query(query1, [], function(err, results) {
    // process results
  });
});
`)
	assert.Equal(t, 0, high, "SqlString.escape() 转义后不应触发 high 告警（误报）")
	_ = total
}

// ============================================================
// ex2: MongoDB / Mongoose NoSQL 注入
// ============================================================

// TestSQLInject_Ex2Positive_NoSQLDirectInput 检测 Mongoose deleteOne 直接传入用户输入（应报 high 告警）。
func TestSQLInject_Ex2Positive_NoSQLDirectInput(t *testing.T) {
	rule := loadSQLInjectionRule(t)
	total, high := runOnFile(t, rule, "ex2-positive-1.js", `
const express = require("express");
const mongoose = require("mongoose");
const Todo = mongoose.model(
  "Todo",
  new mongoose.Schema({ text: { type: String } }, { timestamps: true })
);

const app = express();
app.use(express.json());
app.use(express.urlencoded({ extended: false }));

app.delete("/api/delete", async (req, res) => {
  let id = req.body.id;

  await Todo.deleteOne({ _id: id }); // BAD: id might be an object with special properties

  res.json({ status: "ok" });
});
`)
	assert.Greater(t, total, 0, "NoSQL 直接注入应触发告警（漏报）")
	assert.Greater(t, high, 0, "NoSQL 直接注入应触发 high 告警")
}

// TestSQLInject_Ex2Negative1_EqOperator 验证使用 $eq 操作符后不产生告警。
func TestSQLInject_Ex2Negative1_EqOperator(t *testing.T) {
	rule := loadSQLInjectionRule(t)
	total, _ := runOnFile(t, rule, "ex2-negative-1.js", `
const express = require("express");
const mongoose = require("mongoose");
const Todo = mongoose.model(
  "Todo",
  new mongoose.Schema({ text: { type: String } }, { timestamps: true })
);

const app = express();
app.use(express.json());
app.use(express.urlencoded({ extended: false }));

app.delete("/api/delete", async (req, res) => {
  let id = req.body.id;
  await Todo.deleteOne({ _id: { $eq: id } }); // GOOD: using $eq operator for the comparison

  res.json({ status: "ok" });
});
`)
	assert.Equal(t, 0, total, "$eq 操作符包装后不应触发任何告警（误报）")
}

// TestSQLInject_Ex2Negative2_TypeofCheck 验证 typeof 类型检查守卫后不产生高风险告警。
// typeof 运行时类型检查是一种常见的 NoSQL 注入防御手段，规则应降低或消除其告警级别。
func TestSQLInject_Ex2Negative2_TypeofCheck(t *testing.T) {
	rule := loadSQLInjectionRule(t)
	total, high := runOnFile(t, rule, "ex2-negative-2.js", `
const express = require("express");
const mongoose = require("mongoose");
const Todo = mongoose.model(
  "Todo",
  new mongoose.Schema({ text: { type: String } }, { timestamps: true })
);

const app = express();
app.use(express.json());
app.use(express.urlencoded({ extended: false }));

app.delete("/api/delete", async (req, res) => {
  let id = req.body.id;
  if (typeof id !== "string") {
    res.status(400).json({ status: "error" });
    return;
  }
  await Todo.deleteOne({ _id: id }); // GOOD: id is guaranteed to be a string

  res.json({ status: "ok" });
});
`)
	assert.Equal(t, 0, high, "typeof 类型检查守卫后不应触发 high 告警（误报）")
	_ = total
}
