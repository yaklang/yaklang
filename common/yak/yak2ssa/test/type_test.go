package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestYaklangBasic_Foreach(t *testing.T) {
	t.Run("for each with chan", func(t *testing.T) {
		CheckType(t, `
		ch = make(chan int)

		for i in ch { 
			_ = i 
		}
		`,
			"i", ssa.NumberTypeKind)
	})

	t.Run("for each with list", func(t *testing.T) {
		CheckType(t, `
		ch = make([]int, 3)

		for i in ch { 
			_ = i 
		}
		`,
			"i", ssa.NumberTypeKind)
	})
}

func TestYaklangType_Loop(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		CheckType(t, `
		num = make([]int, 3)
		for i=0; i < 3; i++ {
			n = num[i]
		}
		`,
			"n", ssa.NumberTypeKind)
	})
}
