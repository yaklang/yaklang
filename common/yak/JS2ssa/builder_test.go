package js2ssa

import (
	"testing"
)

func TestMain(t *testing.T) {
	prog := parseSSA(`
	a = 0
	for(;;a++){a= 1}
	for(a=0;;){}
	for(;a>0;){}
	for(a=0;a>0;){}
	for(a=0;a>0;a++){}
	for(;;){}
	`)
	prog.Show()
}
