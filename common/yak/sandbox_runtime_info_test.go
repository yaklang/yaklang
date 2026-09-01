package yak

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

func TestSandboxNestedFunctionRuntimeInfoAndLogging(t *testing.T) {
	const runtimeID = "sandbox-nested-runtime"

	box := NewSandbox()
	box.engine.SetVars(map[string]any{"runtimeId": runtimeID})

	var loggedLine int
	var loggedRuntimeID string
	var loggingInfoErr error
	logger := yaklib.CreateYakLogger("sandbox-runtime-info-test")
	logger.SetOutput(io.Discard)
	logger.SetLevel("info")
	logger.Logger.SetVMRuntimeInfoGetter(func(infoType string) (any, error) {
		value, err := box.engine.RuntimeInfo(infoType)
		if err != nil {
			loggingInfoErr = err
			return nil, err
		}
		switch infoType {
		case "line":
			line, ok := value.(int)
			if !ok {
				loggingInfoErr = fmt.Errorf("line has type %T", value)
				return nil, loggingInfoErr
			}
			loggedLine = line
		case "runtimeId":
			loggedRuntimeID = fmt.Sprint(value)
		}
		return value, nil
	})
	box.engine.ImportLibs(map[string]any{
		"testlog": map[string]any{"info": logger.Infof},
	})

	ok, err := box.ExecuteAsBoolean(`
(() => {
    outer = () => {
        inner = () => {
            line = runtime.GetInfo("line")~
            id = runtime.GetInfo("runtimeId")~
            testlog.info("sandbox nested runtime info")
            return line > 0 && id == "sandbox-nested-runtime"
        }
        return inner()
    }
    return outer()
})()
`)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, loggingInfoErr)
	require.Positive(t, loggedLine)

	// The normal logger currently asks only for the source line. Exercise the
	// other engine-backed logging query while the nested sandbox frame is live.
	box.engine.ImportLibs(map[string]any{
		"captureRuntimeID": func() bool {
			value, infoErr := box.engine.RuntimeInfo("runtimeId")
			if infoErr != nil {
				loggingInfoErr = infoErr
				return false
			}
			loggedRuntimeID = fmt.Sprint(value)
			return true
		},
	})
	ok, err = box.ExecuteAsBoolean(`(() => (() => captureRuntimeID())())()`)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, loggingInfoErr)
	require.Equal(t, runtimeID, loggedRuntimeID)
}
