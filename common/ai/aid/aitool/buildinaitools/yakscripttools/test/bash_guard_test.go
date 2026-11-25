package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"gotest.tools/v3/assert"
)

// TestBashToolCommandGuardBlocks uses a harmless chmod target: if the guard
// ever regresses, the command only fails on a nonexistent path instead of
// touching the host.
func TestBashToolCommandGuardBlocks(t *testing.T) {
	result := invokeBashResult(t, aitool.InvokeParams{
		"command":             "chmod u+s /tmp/yak-guard-canary-does-not-exist",
		"shell":               "bash",
		"timeout":             5,
		"accepted-exit-codes": "0,1",
	})

	assert.Equal(t, utils.InterfaceToString(result["termination_reason"]), "blocked_by_bash_guard")
	assert.Equal(t, utils.InterfaceToBoolean(result["process_started"]), false)
	assert.Equal(t, utils.InterfaceToBoolean(result["executed"]), false)
	assert.Equal(t, utils.InterfaceToBoolean(result["security_denied"]), true)
	assert.Equal(t, utils.InterfaceToString(result["verdict"]), "deny")
}

func TestBashToolCommandGuardAllowsSafe(t *testing.T) {
	result := invokeBashResult(t, aitool.InvokeParams{
		"command": "printf 'guard-ok\\n'",
		"shell":   "bash",
		"timeout": 5,
	})

	assert.Equal(t, utils.InterfaceToBoolean(result["process_started"]), true)
	assert.Equal(t, utils.InterfaceToInt(result["exit_code"]), 0)
	assert.Equal(t, utils.InterfaceToBoolean(result["exit_code_accepted"]), true)
}
