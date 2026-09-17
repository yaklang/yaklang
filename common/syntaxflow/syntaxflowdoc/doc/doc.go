package doc

import (
	"bytes"
	_ "embed"
	"encoding/gob"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/syntaxflowdoc"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

//go:embed doc.gob.zst
var embedDocument []byte

var (
	defaultHelper *syntaxflowdoc.DocumentHelper
	once          sync.Once
)

func emptyHelper() *syntaxflowdoc.DocumentHelper {
	return syntaxflowdoc.NewEmptyDocumentHelper()
}

// nativeCallsFromSSAAPI copies live NativeCall docs registered in ssaapi
// (sf_native_call.go registerNativeCall / NativeCallDocuments). No hardcoded list.
func nativeCallsFromSSAAPI() []syntaxflowdoc.NativeCallInfo {
	out := make([]syntaxflowdoc.NativeCallInfo, 0, len(ssaapi.NativeCallDocuments))
	for name, d := range ssaapi.NativeCallDocuments {
		name = strings.TrimSpace(name)
		if name == "" || d == nil {
			continue
		}
		out = append(out, syntaxflowdoc.NativeCallInfo{
			Name:        name,
			Description: d.Description,
		})
	}
	return out
}

// overlayLiveNativeCalls replaces embed NativeCall rows with the live ssaapi registry
// so docs always track sf_native_call.go without regenerating the gob snapshot.
func overlayLiveNativeCalls(h *syntaxflowdoc.DocumentHelper) {
	if h == nil {
		return
	}
	items := nativeCallsFromSSAAPI()
	if len(items) == 0 {
		log.Warnf("syntaxflowdoc: live NativeCallDocuments empty; keeping embed native_call rows")
		return
	}
	h.NativeCalls = make(map[string]*syntaxflowdoc.DocEntry)
	syntaxflowdoc.CollectNativeCalls(h, items)
}

// GetDefaultDocumentHelper loads the embedded SyntaxFlowDoc snapshot, then overlays
// live NativeCall docs from ssaapi.NativeCallDocuments.
func GetDefaultDocumentHelper() *syntaxflowdoc.DocumentHelper {
	once.Do(func() {
		if len(embedDocument) == 0 {
			log.Warnf("syntaxflowdoc embed is empty")
			defaultHelper = emptyHelper()
			overlayLiveNativeCalls(defaultHelper)
			return
		}
		raw, err := utils.ZstdDeCompress(embedDocument)
		if err != nil {
			log.Warnf("syntaxflowdoc decompress embed failed: %v", err)
			defaultHelper = emptyHelper()
			overlayLiveNativeCalls(defaultHelper)
			return
		}
		var helper syntaxflowdoc.DocumentHelper
		if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&helper); err != nil {
			log.Warnf("syntaxflowdoc decode embed failed: %v", err)
			defaultHelper = emptyHelper()
			overlayLiveNativeCalls(defaultHelper)
			return
		}
		if helper.NativeCalls == nil {
			helper.NativeCalls = make(map[string]*syntaxflowdoc.DocEntry)
		}
		if helper.Operators == nil {
			helper.Operators = make(map[string]*syntaxflowdoc.DocEntry)
		}
		if helper.Opcodes == nil {
			helper.Opcodes = make(map[string]*syntaxflowdoc.DocEntry)
		}
		if helper.DescKeys == nil {
			helper.DescKeys = make(map[string]*syntaxflowdoc.DocEntry)
		}
		if helper.BuiltinLibs == nil {
			helper.BuiltinLibs = make(map[string]*syntaxflowdoc.DocEntry)
		}
		if helper.Syntax == nil {
			helper.Syntax = make(map[string]*syntaxflowdoc.DocEntry)
		}
		defaultHelper = &helper
		overlayLiveNativeCalls(defaultHelper)
	})
	return defaultHelper
}

// IsDocumentAvailable reports whether the embed loaded a non-empty index.
func IsDocumentAvailable() bool {
	h := GetDefaultDocumentHelper()
	return h != nil && h.Total() > 0
}

// SearchDocument searches the embedded SyntaxFlowDoc.
func SearchDocument(query string, limit int, categoryFilter string) []*syntaxflowdoc.SearchHit {
	return syntaxflowdoc.SearchDocument(GetDefaultDocumentHelper(), query, limit, categoryFilter)
}
