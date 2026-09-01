package yakvm

import "fmt"

func getRuntimeInfo(frame *Frame, infoType string, args ...interface{}) (interface{}, error) {
	if frame == nil {
		return nil, fmt.Errorf("current frame is empty")
	}

	switch infoType {
	case "line":
		code := frame.CurrentCode()
		if code == nil {
			return nil, fmt.Errorf("current code is empty")
		}
		return code.StartLineNumber, nil
	case "runtimeId":
		result, ok := frame.GlobalVariables.Load("runtimeId")
		if !ok {
			return "", nil
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unknown info type: %s", infoType)
	}
}

func newRuntimeLib(vm *VirtualMachine) map[string]interface{} {
	return map[string]interface{}{
		// Resolve the frame at call time. Top-level executions share the VM's
		// runtime globals, so binding this closure to whichever frame happened to
		// be created last makes concurrent hooks report each other's line number.
		"GetInfo": func(infoType string, args ...interface{}) (interface{}, error) {
			return getRuntimeInfo(vm.CurrentFM(), infoType, args...)
		},
	}
}

// ImportRuntimeLib is kept for callers that explicitly refresh a frame's
// runtime helpers. Preserve the historical frame-bound behavior for this
// explicit API, including callers that execute a Frame directly without first
// registering it in the VM's goroutine-local stack. New VMs use the dynamic
// VM-bound helper installed during initialization for ordinary execution.
func ImportRuntimeLib(frame *Frame) {
	if frame == nil || frame.vm == nil {
		return
	}
	// Frames normally point at the VM-wide runtime globals. Keep this explicit
	// compatibility binding private to the supplied frame; otherwise one direct
	// Frame.Exec caller would replace the dynamic helper for every concurrent
	// execution on the VM.
	frame.GlobalVariables = frame.GlobalVariables.Clone()
	frame.GlobalVariables.Store("runtime", map[string]interface{}{
		"GetInfo": func(infoType string, args ...interface{}) (interface{}, error) {
			return getRuntimeInfo(frame, infoType, args...)
		},
	})
}
