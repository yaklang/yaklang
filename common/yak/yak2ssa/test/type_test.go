package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestYaklangBasic_Foreach(t *testing.T) {
	t.Run("for each with chan", func(t *testing.T) {
		test := assert.New(t)
		prog, err := ssaapi.Parse(`
		ch = make(chan int)

		for i in ch { 
			_ = i 
		}
		`)
		test.Nil(err)

		prog.Show()

		vs := prog.Ref("i")
		test.Equal(1, len(vs))

		v := vs[0]
		test.NotNil(v)

		kind := v.GetTypeKind()
		log.Info("type kind", kind)
		test.Equal(kind, ssa.Number)
	})
}
