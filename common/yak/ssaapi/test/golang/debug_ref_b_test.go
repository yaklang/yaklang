package ssaapi

import (
	"testing"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestDebugRefBMain(t *testing.T) {
	code := `package main
type Queue struct { mu int }
func NewQueue() *Queue { return &Queue{mu: 1} }
func main(){ a := NewQueue(); b := a.mu }`
	ssatest.Check(t, code, func(p *ssaapi.Program) error {
		require.NotEmpty(t, p.Ref("b"))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))
}
