// Command textsectlayout checks the current GOROOT's moduledata.textsectmap
// offset. It exits non-zero when the layout cannot be derived or the Go
// version is not in the verified table. Build scripts use that to refuse a
// runtime whose text map would be patched at the wrong offset.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/textsectlayout"
)

func main() {
	goroot := flag.String("goroot", runtime.GOROOT(), "GOROOT whose runtime.moduledata layout is checked")
	flag.Parse()
	res, err := textsectlayout.Check(*goroot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "textsectlayout: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%s textsectmap=%#x\n", res.Version, res.Offset)
}
