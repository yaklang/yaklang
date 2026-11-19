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

	check := utils.InterfaceToGeneralMap(result["check"])
	assert.Assert(t, len(check) > 0, "missing structured check result: %v", result)
	assert.Equal(t, utils.InterfaceToString(check["verdict"]), "block")
	assert.Assert(t, len(utils.InterfaceToSliceInterface(check["findings"])) > 0, "missing findings: %v", check)
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
