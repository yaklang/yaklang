package behinder

import (
	"fmt"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func Test_getParamedClass(t *testing.T) {
	//var t ypb.ShellType
	key, ok := ypb.ShellType_value["Godzilla"]
	if !ok {
		panic("x")
	}
	fmt.Printf("%v\n", ypb.ShellType(key))
}
