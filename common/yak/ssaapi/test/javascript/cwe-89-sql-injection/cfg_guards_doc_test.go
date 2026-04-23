package cwe89sqlinjection

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCfgGuardsKindNoneDoc(t *testing.T) {
	ssatest.Check(t, `
println("x")
`, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
println(* #-> as $arg)
$arg<cfgGuards()> as $guards
`)
		require.NoError(t, err)
		require.Contains(t, res.String(), "cfgGuard(kind="+ssaapi.GuardKindNone)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

func TestTypeofGuardEmbeddedRuleShape_WithCfgGuardsRule(t *testing.T) {
	ssatest.Check(t, `
const express = require("express");
const mongoose = require("mongoose");
const app = express();
app.use(express.json());

const Todo = mongoose.model("Todo", new mongoose.Schema({ text: String }));

app.delete("/api/delete", async (req, res) => {
    let id = req.body.id;
    if (typeof id !== "string") {
        res.status(400).json({ status: "error" });
        return;
    }
    await Todo.deleteOne({ _id: id });
    res.json({ status: "ok" });
});
`, func(prog *ssaapi.Program) error {
		result, err := prog.SyntaxFlowWithError(`
<include('js-param')> as $params;

*.deleteOne(* as $nosqlArg)
$nosqlArg<cfgGuards()> as $guards

		`)
		require.NoError(t, err)
		out := result.String()
		require.Contains(t, out, "cfgGuard(kind="+ssaapi.GuardKindEarlyReturn, "typeof+return 守卫应对 deleteOne 产生 earlyReturn cfgGuard")
		require.NotContains(t, out, "cfgGuard(kind="+ssaapi.GuardKindNone+", synthetic", "不应仅余 synthetic kind=none（漏检 early return）")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JS))
}
