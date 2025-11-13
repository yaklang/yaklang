package ssa_test

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestCallPhiReplace(t *testing.T) {
	ssatest.CheckNoError(t, `
a = []
for i in 10 {
    println(a[0],a[0])
}
`)
}
