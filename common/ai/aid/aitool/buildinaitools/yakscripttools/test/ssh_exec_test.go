package test

import (
	"encoding/json"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

func TestSSHExecTool_MetadataAndParams(t *testing.T) {
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/system/ssh_exec.yak")
	assert.NilError(t, err)

	aiTool := yakscripttools.LoadYakScriptToAiTools("ssh_exec", string(content))
	assert.Assert(t, aiTool != nil, "failed to parse ssh_exec.yak metadata")
	assert.Equal(t, aiTool.Name, "ssh_exec")
	assert.Assert(t, aiTool.Description != "", "description should not be empty")

	var schema map[string]any
	assert.NilError(t, json.Unmarshal([]byte(aiTool.Params), &schema))
	props, ok := schema["properties"].(map[string]any)
	assert.Assert(t, ok, "schema properties missing")

	for _, name := range []string{
		"host",
		"port",
		"username",
		"password",
		"private-key",
		"key-passphrase",
		"command",
		"shell",
		"sudo",
		"timeout",
		"max-output-bytes",
	} {
		if _, ok := props[name]; !ok {
			t.Fatalf("ssh_exec schema missing parameter %q; schema=%s", name, aiTool.Params)
		}
	}
}
