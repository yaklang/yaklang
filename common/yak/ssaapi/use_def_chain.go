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
			ret += fmt.Sprintf("%-7s\t%s\t%s\t%s\n", prefix, indexStr, ssa.SSAOpcode2Name[v.getValue().GetOpcode()], v)
		case 1:
			ret += fmt.Sprintf("%s\t%s\n\t\t%s\n\t\t%s\n\t\t%s\n", prefix, indexStr, v, v.getValue(), v.getValue().GetRange())
		}
		return ret
	}

	for i, operand := range v.GetOperands() {
		ret += show("Operand", i, operand)
	}

	ret += show("Self", -1, v)
	for i, user := range v.GetUsers() {
		ret += show("User", i, user)
	}

	if v.IsMember() {
		ret += "Members:\n"
		for _, pair := range v.GetObjectKeyPairs() {
			if len(pair) != 2 {
				continue
			}
			ret += show("Key", -1, pair[1])
			ret += show("Object", -1, pair[0])
		}
	}

	if v.IsObject() {
		ret += "Object:\n"
		for _, pair := range v.GetMembers() {
			if len(pair) != 2 {
				continue
			}
			ret += show("Key", -1, pair[0])
			ret += show("Member", -1, pair[1])
		}
	}
	log.Infof(ret)
}
