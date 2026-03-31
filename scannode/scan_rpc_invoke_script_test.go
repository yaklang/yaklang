package scannode

import "testing"

func TestBuildScriptBaseParams(t *testing.T) {
	t.Parallel()

	params := buildScriptBaseParams("http://127.0.0.1:8080/webhook", "runtime-1")
	if len(params) != 4 {
		t.Fatalf("unexpected params length: %d", len(params))
	}
	if params[0] != "--yakit-webhook" || params[1] != "http://127.0.0.1:8080/webhook" {
		t.Fatalf("unexpected webhook params: %#v", params)
	}
	if params[2] != "--runtime_id" || params[3] != "runtime-1" {
		t.Fatalf("unexpected runtime params: %#v", params)
	}
}

func TestBuildScriptBaseParamsWithoutRuntimeID(t *testing.T) {
	t.Parallel()

	params := buildScriptBaseParams("http://127.0.0.1:8080/webhook", "")
	if len(params) != 2 {
		t.Fatalf("unexpected params length: %d", len(params))
	}
	if params[0] != "--yakit-webhook" || params[1] != "http://127.0.0.1:8080/webhook" {
		t.Fatalf("unexpected webhook params: %#v", params)
	}
}
