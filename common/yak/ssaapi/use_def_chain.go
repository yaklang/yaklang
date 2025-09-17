package ssaapi

import (
	"fmt"
	"strconv"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func showUseDefChain(v *Value) {
	showUserDefChainEx(0, v)
}

func showAllUseDefChain(v *Value) {
	showUserDefChainEx(1, v)
}

func showUserDefChainEx(flag int, v *Value) {
	ret := "UseDef: |Type\t|index\t|Opcode\t|Value\n"

	show := func(prefix string, index int, v *Value) string {
		indexStr := ""
		if index >= 0 {
			indexStr = strconv.FormatInt(int64(index), 10)
		}
		ret := fmt.Sprintf("%8s", "")
		switch flag {
		case 0:
			ret += fmt.Sprintf("%-7s\t%s\t%s\t%s\n", prefix, indexStr, ssa.SSAOpcode2Name[v.innerValue.GetOpcode()], v)
		case 1:
			ret += fmt.Sprintf("%s\t%s\n\t\t%s\n\t\t%s\n\t\t%s\n", prefix, indexStr, v, v.innerValue, v.innerValue.GetRange())
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

	if v.IsMember() {
		ret += "Members:\n"
		ret += show("Key", -1, v.GetKey())
		ret += show("Object", -1, v.GetObject())
	}

	if v.IsObject() {
		ret += "Object:\n"
		for _, value := range v.GetAllMember() {
			ret += show("Key", -1, value.GetKey())
			ret += show("Member", -1, value)
		}
	}
	log.Infof(ret)
}
