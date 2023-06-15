package antlr4nasl

import (
	"testing"
)

func TestCallYak(t *testing.T) {
	engine := New()
	engine.InitBuildInLib()
	if err := engine.SafeEval(`
res = call_yak_method("codec.DecodeBase64", "eWFr")[0];
assert(string(res) == "yak", "yak call failed");
`); err != nil {
		t.Fatal(err)
	}
}

func TestHttpFuncLib(t *testing.T) {
	engine := New()
	engine.InitBuildInLib()
	if err := engine.SafeEval(`
include("http_func.nasl");
http_get("http://www.baidu.com");
`); err != nil {
		t.Fatal(err)
	}
}
