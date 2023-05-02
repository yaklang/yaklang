package yakvm

import (
	"fmt"
	"sort"
	"testing"
	"yaklang/common/go-funk"
)

func TestOpcodeToName(t *testing.T) {
	keys := funk.Keys(OpcodeVerboseName).([]OpcodeFlag)
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	println("|Opcode Flag| 助记符(Opcode Verbose) | Unary | Op1 | Op2 | 补充描述 |")
	println("|--------|:--------|------|------|------|------|")
	for _, k := range keys {
		fmt.Printf("| %v | %v | - | - | - | - |\n", k, OpcodeVerboseName[k])
	}
}
