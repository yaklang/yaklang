package netstackvm

import "github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"

type NIC struct {
	stack            *stack.Stack
	initNICIPAddress string
}
