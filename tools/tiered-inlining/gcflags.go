// Command tiered-inlining prints package-scoped Go `-gcflags="<pkg>=-l"`
// arguments for tiered inlining release builds.
//
// Design principle:
//   - Keep Go's default inlining on ALL hot paths (std, runtime, YakVM, SSA
//     core, SyntaxFlow VM, yaklib, antlr/v4 runtime, lowhttp, utils, codec).
//   - Disable inlining (`-l`) ONLY on cold/generated/heavy-peripheral packages
//     (ANTLR generated parsers, language *2ssa frontends, gRPC/AI/WSM/RPA/
//     heavy 3rd-party libs) to cut __text, __gopclntab (inltree/pctab), and
//     __rodata with ~0% runtime overhead.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Profiles maps tiered-inlining profile names to package patterns that receive `-l`.
var Profiles = map[string][]string{
	// tierA: Ultra-conservative — only generated ANTLR parsers + 7 heavy cold 3rd-party libs.
	"tierA": {
		"github.com/yaklang/yaklang/common/yak/csharp/parser",
		"github.com/yaklang/yaklang/common/yak/rust/parser",
		"github.com/yaklang/yaklang/common/yak/php/parser",
		"github.com/yaklang/yaklang/common/yak/java/parser",
		"github.com/yaklang/yaklang/common/yak/cpp/parser",
		"github.com/yaklang/yaklang/common/yak/kotlin/parser",
		"github.com/yaklang/yaklang/common/yak/scala/parser",
		"github.com/yaklang/yaklang/common/yak/python/parser",
		"github.com/yaklang/yaklang/common/yak/antlr4go/parser",
		"github.com/yaklang/yaklang/common/yak/antlr4c/parser",
		"github.com/yaklang/yaklang/common/yak/antlr4JS/parser",
		"github.com/yaklang/yaklang/common/yak/antlr4yak/parser",
		"github.com/yaklang/yaklang/common/yak/antlr4nasl/parser",
		"github.com/yaklang/yaklang/common/yak/antlr4Lua/parser",
		"github.com/yaklang/yaklang/common/syntaxflow/sf",
		"github.com/yaklang/yaklang/common/suricata/parser",
		"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast",
		"github.com/xuri/excelize/v2/...",
		"github.com/go-rod/rod/...",
		"github.com/russross/blackfriday/v2/...",
		"github.com/yaklang/javajive/...",
		"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/...",
		"google.golang.org/grpc/...",
		"github.com/gopacket/gopacket/...",
	},

	// tierB: Recommended — tierA + cold language frontends (*2ssa) and peripheral
	// feature packages (gRPC, AI orchestration, WSM, vulinbox, RPA, crawler, etc.).
	// Hot paths (std, yakvm, ssa, sfvm, yaklib, antlr/v4, lowhttp, utils) stay at default inlining.
	"tierB": {
		// tierA
		"github.com/yaklang/yaklang/common/yak/csharp/...",
		"github.com/yaklang/yaklang/common/yak/rust/...",
		"github.com/yaklang/yaklang/common/yak/php/...",
		"github.com/yaklang/yaklang/common/yak/java/...",
		"github.com/yaklang/yaklang/common/yak/cpp/...",
		"github.com/yaklang/yaklang/common/yak/kotlin/...",
		"github.com/yaklang/yaklang/common/yak/scala/...",
		"github.com/yaklang/yaklang/common/yak/python/...",
		"github.com/yaklang/yaklang/common/yak/antlr4go/parser",
		"github.com/yaklang/yaklang/common/yak/antlr4c/parser",
		"github.com/yaklang/yaklang/common/yak/antlr4JS/parser",
		"github.com/yaklang/yaklang/common/yak/antlr4yak/parser",
		"github.com/yaklang/yaklang/common/yak/antlr4nasl/parser",
		"github.com/yaklang/yaklang/common/yak/antlr4Lua/parser",
		"github.com/yaklang/yaklang/common/syntaxflow/sf",
		"github.com/yaklang/yaklang/common/suricata/parser",
		"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast",
		"github.com/xuri/excelize/v2/...",
		"github.com/go-rod/rod/...",
		"github.com/russross/blackfriday/v2/...",
		"github.com/yaklang/javajive/...",
		"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/...",
		"google.golang.org/grpc/...",
		"github.com/gopacket/gopacket/...",
		// peripheral / cold business packages
		"github.com/yaklang/yaklang/common/yakgrpc/...",
		"github.com/yaklang/yaklang/common/ai/...",
		"github.com/yaklang/yaklang/common/wsm/...",
		"github.com/yaklang/yaklang/common/vulinbox/...",
		"github.com/yaklang/yaklang/common/rpa/...",
		"github.com/yaklang/yaklang/common/crawlerx/...",
		"github.com/yaklang/yaklang/common/bin-parser/...",
		"github.com/yaklang/yaklang/common/sarif/...",
		"github.com/yaklang/yaklang/common/openapi/...",
	},
}

// GCFlags returns the `-gcflags=<pattern>=-l` arguments for the requested profile.
func GCFlags(profile string) ([]string, error) {
	p := strings.TrimSpace(profile)
	if p == "" || p == "off" || p == "default" {
		return nil, nil
	}
	if p == "all" {
		return []string{"-gcflags=all=-l"}, nil
	}
	pkgs, ok := Profiles[p]
	if !ok {
		return nil, fmt.Errorf("unknown tiered-inlining profile %q (supported: off, tierA, tierB, all)", profile)
	}
	out := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		out = append(out, fmt.Sprintf("-gcflags=%s=-l", pkg))
	}
	return out, nil
}

func main() {
	profile := flag.String("profile", "tierB", "profile name: off, tierA, tierB, all")
	flag.Parse()

	flags, err := GCFlags(*profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tiered-inlining: %v\n", err)
		os.Exit(1)
	}
	for _, f := range flags {
		fmt.Println(f)
	}
}
