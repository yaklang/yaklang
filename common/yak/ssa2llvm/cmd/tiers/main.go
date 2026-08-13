// tiers exposes the runtime tier ladder (runtime/tiers) to shell scripts, so
// build_yaklib.sh and CI read the module sets from the same Go definition the
// compiler selects with.
//
// Usage:
//
//	tiers list                    # tier names, smallest first
//	tiers modules <tier>          # that tier's SSA2LLVM_EMBED_MODULES value
//	tiers name <mod,mod,...>      # tier with exactly this module set, else "custom"
//	tiers select <mod,mod,...>    # smallest tier covering these modules
package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/tiers"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "list":
		for _, t := range tiers.All {
			fmt.Println(t.Name)
		}
	case "modules":
		requireArg(3)
		t, ok := tiers.Lookup(os.Args[2])
		if !ok {
			fail("unknown tier %q; tiers are %s", os.Args[2], tiers.Names())
		}
		fmt.Println(t.ModuleList())
	case "name":
		requireArg(3)
		fmt.Println(tierNameOf(splitModules(os.Args[2])))
	case "select":
		requireArg(3)
		t, err := tiers.Select(splitModules(os.Args[2]))
		if err != nil {
			fail("%v", err)
		}
		fmt.Println(t.Name)
	default:
		usage()
	}
}

// tierNameOf names an archive built from an explicit module list. Only an exact
// set match is a tier: a build with extra or missing modules behaves like none
// of the ladder's entries, and callers must not treat it as one.
func tierNameOf(modules []string) string {
	for _, t := range tiers.All {
		if slices.Equal(t.Modules, modules) {
			return t.Name
		}
	}
	return "custom"
}

func splitModules(csv string) []string {
	out := make([]string, 0, 8)
	for _, m := range strings.Split(csv, ",") {
		if m = strings.TrimSpace(m); m != "" {
			out = append(out, m)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func requireArg(n int) {
	if len(os.Args) < n {
		usage()
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: tiers list | modules <tier> | name <mods> | select <mods>\n")
	os.Exit(2)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "tiers: "+format+"\n", args...)
	os.Exit(1)
}
