package ssaapi

import (
	"fmt"
	"strconv"
)

func showUseDefChain(v *Value) {
	showUserDefChainEx(0, v)
}

func showAllUseDefChain(v *Value) {
	showUserDefChainEx(1, v)
}

func showUserDefChainEx(flag int, v *Value) {
	ret := "use-def: |Type\t|index\t|Opcode\t|Value\n"

	show := func(prefix string, index int, v *Value) string {
		indexStr := ""
		if index >= 0 {
			indexStr = strconv.FormatInt(int64(index), 10)
		}
		ret := fmt.Sprintf("%8s", "")
		switch flag {
		case 0:
			ret += fmt.Sprintf("%-7s\t%s\t%s\t%s\n", prefix, indexStr, v.node.GetOpcode(), v)
		case 1:
			ret += fmt.Sprintf("%s\t%s\n\t\t%s\n\t\t%s\n\t\t%s\n", prefix, indexStr, v, v.node, v.node.GetRange())
		default:
		}
		return ret
	}

	for i, v := range v.GetOperands() {
		ret += show("Operand", i, v)
	}

	ret += show("Self", -1, v)
	for i, u := range v.GetUsers() {
		ret += show("User", i, u)
	}
	fmt.Println(ret)
}
