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
