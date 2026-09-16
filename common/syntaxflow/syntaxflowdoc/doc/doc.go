package doc

import (
	"bytes"
	_ "embed"
	"encoding/gob"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/syntaxflowdoc"
	"github.com/yaklang/yaklang/common/utils"
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

// GetDefaultDocumentHelper loads the embedded SyntaxFlowDoc snapshot.
func GetDefaultDocumentHelper() *syntaxflowdoc.DocumentHelper {
	once.Do(func() {
		if len(embedDocument) == 0 {
			log.Warnf("syntaxflowdoc embed is empty")
			defaultHelper = emptyHelper()
			return
		}
		raw, err := utils.ZstdDeCompress(embedDocument)
		if err != nil {
			log.Warnf("syntaxflowdoc decompress embed failed: %v", err)
			defaultHelper = emptyHelper()
			return
		}
		var helper syntaxflowdoc.DocumentHelper
		if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&helper); err != nil {
			log.Warnf("syntaxflowdoc decode embed failed: %v", err)
			defaultHelper = emptyHelper()
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
