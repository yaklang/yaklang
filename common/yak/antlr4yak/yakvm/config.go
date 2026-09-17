package yakvm

type YVMMode string

const (
	NASL YVMMode = "NASL"
	LUA  YVMMode = "LUA"
	YAK  YVMMode = "YAK"
)

type VirtualMachineConfig struct {
	functionParamNumberCheck bool
	stopRecover              bool
	closureSupport           bool
	vmMode                   YVMMode
	suppressPanicDebugStack  bool
	synchronousExecution     bool
}

// SetSynchronousExecution is for privately owned VMs whose caller guarantees
// that all execution and callbacks stay on the calling goroutine. Configure it
// before execution; never enable it for a general purpose or shared engine.
// The ordinary VM continues to keep a separate frame stack per goroutine.
func (c *VirtualMachineConfig) SetSynchronousExecution(b bool) {
	c.synchronousExecution = b
}

// SetSuppressPanicDebugStack disables only the optional Go runtime stack dump
// on panic construction. The returned VMPanic, Yak source review and recovery
// behavior are unchanged. Configure this VM before starting execution.
func (c *VirtualMachineConfig) SetSuppressPanicDebugStack(b bool) {
	c.suppressPanicDebugStack = b
}

func NewVMConfig() *VirtualMachineConfig {
	return &VirtualMachineConfig{
		functionParamNumberCheck: true,
		stopRecover:              false,
		closureSupport:           true,
		vmMode:                   YAK,
	}
}
func (c *VirtualMachineConfig) SetYVMMode(mode YVMMode) {
	c.vmMode = mode
}
func (c *VirtualMachineConfig) SetClosureSupport(b bool) {
	c.closureSupport = b
}
func (c *VirtualMachineConfig) GetClosureSupport() bool {
	return c.closureSupport
}
func (c *VirtualMachineConfig) SetStopRecover(b bool) {
	c.stopRecover = b
}
func (c *VirtualMachineConfig) GetStopRecover() bool {
	return c.stopRecover
}

func (c *VirtualMachineConfig) SetFunctionNumberCheck(b bool) {
	c.functionParamNumberCheck = b
}
func (c *VirtualMachineConfig) GetFunctionNumberCheck() bool {
	return c.functionParamNumberCheck
}
