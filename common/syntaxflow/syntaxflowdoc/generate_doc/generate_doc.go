package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/syntaxflowdoc"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	_ "github.com/yaklang/yaklang/common/yak/ssaapi" // register NativeCallDocuments via init
)

func nativeCallsFromSSAAPI() []syntaxflowdoc.NativeCallInfo {
	out := make([]syntaxflowdoc.NativeCallInfo, 0, len(ssaapi.NativeCallDocuments))
	for name, doc := range ssaapi.NativeCallDocuments {
		if name == "" || doc == nil {
			continue
		}
		out = append(out, syntaxflowdoc.NativeCallInfo{Name: name, Description: doc.Description})
	}
	return out
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: generate_doc <output-doc.gob.zst>")
		os.Exit(2)
	}
	outPath := os.Args[1]

	helper := syntaxflowdoc.BuildDocumentHelper(syntaxflowdoc.BuildOptions{
		NativeCalls: nativeCallsFromSSAAPI(),
	})
	counts := helper.Counts()
	log.Infof("syntaxflowdoc generated: total=%d native_call=%d operator=%d opcode=%d desc_key=%d builtin_lib=%d syntax=%d",
		helper.Total(),
		counts[syntaxflowdoc.CategoryNativeCall],
		counts[syntaxflowdoc.CategoryOperator],
		counts[syntaxflowdoc.CategoryOpcode],
		counts[syntaxflowdoc.CategoryDescKey],
		counts[syntaxflowdoc.CategoryBuiltinLib],
		counts[syntaxflowdoc.CategorySyntax],
	)
	if helper.Total() == 0 {
		panic("syntaxflowdoc helper is empty")
	}
	if len(helper.NativeCalls) == 0 {
		panic("syntaxflowdoc: no native calls collected; ssaapi init may have failed")
	}
	if len(helper.BuiltinLibs) == 0 {
		log.Warnf("syntaxflowdoc: no builtin libs collected from %s", syntaxflowdoc.DefaultBuiltinRulesDir())
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(helper); err != nil {
		panic(err)
	}
	compressed, err := utils.ZstdCompress(buf.Bytes())
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(outPath, compressed, 0o666); err != nil {
		panic(err)
	}
	log.Infof("wrote %s (%d bytes compressed)", outPath, len(compressed))
}
