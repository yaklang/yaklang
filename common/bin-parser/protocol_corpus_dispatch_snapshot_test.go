package bin_parser

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// Cross-binary fingerprints include concrete Go types (not JSON-normalized
// numbers), all exported fields, nested metadata, child order and unread bytes.
// The legacy snapshot is generated once before changing the implementation.
// Hashing/serialization is correctness work, never part of throughput timing.
func TestCorpusAutomaticDispatchSnapshot(t *testing.T) {
	path := os.Getenv("BIN_PARSER_DISPATCH_SNAPSHOT")
	if path == "" {
		t.Skip("opt-in cross-binary correctness snapshot")
	}
	works := corpusDispatchWorks(t, os.Getenv("BIN_PARSER_DISPATCH_ALL") == "1")
	type record struct {
		ID            string
		Remaining     int
		Error, Digest string
	}
	rows := make([]record, 0, len(works))
	printer := &spew.ConfigState{SortKeys: true, DisablePointerAddresses: true, DisableCapacities: true}
	for i, work := range works {
		r := newProtocolCorpusBoundedReader(work.wire)
		node, err := parser.ParseBinaryWithConfig(r, "ethernet", map[string]any{"preparedOperatorLegacy": os.Getenv("BIN_PARSER_DISPATCH_LEGACY") == "1"}, "Ethernet")
		row := record{ID: work.id, Remaining: r.Len()}
		if err != nil {
			row.Error = err.Error()
		} else {
			h := sha256.New()
			printer.Fprintf(h, "%#v", NodeToMap(node))
			var walk func(*base.Node)
			walk = func(n *base.Node) {
				printer.Fprintf(h, "%#v", []any{n.Name, n.Cfg.GetItem("additionInfo"), len(n.Children)})
				for _, c := range n.Children {
					walk(c)
				}
			}
			walk(node)
			row.Digest = fmt.Sprintf("%x", h.Sum(nil))
		}
		rows = append(rows, row)
		if (i+1)%5000 == 0 {
			t.Logf("fingerprinted %d/%d packets", i+1, len(works))
		}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("fingerprinted %d packets", len(rows))
}
