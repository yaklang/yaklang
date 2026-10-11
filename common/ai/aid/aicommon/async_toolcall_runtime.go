package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// AsyncToolCallRuntime prepares a call for its explicit owner without executing
// it. It is optional so AIInvokeRuntime implementations need not add methods.
// The returned cancel function releases preparation on failed admission or owner
// release, including when the manager cancels a pending call before Execute.
type AsyncToolCallRuntime interface {
	PrepareAsyncToolCall(
		task AIStatefulTask, name string, params aitool.InvokeParams, opts ...ToolCallerOption,
	) (asyncexecmanager.Resource, func(), error)
}
