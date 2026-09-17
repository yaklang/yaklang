package bin_parser

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestCorpusAutomaticDispatchPreparedEquivalence(t *testing.T) {
	works := corpusDispatchWorks(t, false)
	for _, tcp := range []bool{false, true} {
		for _, size := range []int{128, 1200} {
			wire := dispatchUnknownFrame(tcp, size)
			// Capture/header boundaries, payload prefixes and the full message.
			for n := 0; n <= len(wire); n++ {
				if n > 64 && n != len(wire) && n%127 != 0 {
					continue
				}
				works = append(works, currentCorpusWork{id: fmt.Sprintf("unknown/%t/%d/truncated/%d", tcp, size, n), wire: append([]byte(nil), wire[:n]...)})
			}
			for i := 0; i < len(wire); i += 31 {
				mutated := append([]byte(nil), wire...)
				mutated[i] ^= 0xff
				works = append(works, currentCorpusWork{id: fmt.Sprintf("unknown/%t/%d/mutated/%d", tcp, size, i), wire: mutated})
			}
		}
	}
	assertDispatchEquivalence(t, works)
	t.Logf("compared %d Ethernet packets, including all 511 representatives and unknown/truncated/mutated inputs", len(works))
}

func assertDispatchEquivalence(t *testing.T, works []currentCorpusWork) {
	t.Helper()
	for _, work := range works {
		a := newProtocolCorpusBoundedReader(work.wire)
		old, oldErr := parser.ParseBinaryWithConfig(a, "ethernet", map[string]any{"preparedOperatorLegacy": true}, "Ethernet")
		b := newProtocolCorpusBoundedReader(work.wire)
		current, err := parser.ParseBinary(b, "ethernet", "Ethernet")
		if fmt.Sprint(oldErr) != fmt.Sprint(err) || a.Len() != b.Len() {
			t.Fatalf("%s: remaining %d/%d; legacy error %v; prepared error %v", work.id, a.Len(), b.Len(), oldErr, err)
		}
		if err != nil {
			continue
		}
		if !reflect.DeepEqual(NodeToMap(old), NodeToMap(current)) {
			t.Fatalf("%s: exported fields or concrete Go value types differ", work.id)
		}
		var metadata func(*base.Node) []any
		metadata = func(n *base.Node) []any {
			values := []any{n.Name, n.Cfg.GetItem("additionInfo"), len(n.Children)}
			for _, c := range n.Children {
				values = append(values, metadata(c))
			}
			return values
		}
		if !reflect.DeepEqual(metadata(old), metadata(current)) {
			t.Fatalf("%s: nested metadata/tree order differs", work.id)
		}
	}
}

func TestCorpusAutomaticDispatchNetworkPlansEquivalence(t *testing.T) {
	var works []currentCorpusWork
	for protocol := 0; protocol < 256; protocol++ {
		wire := dispatchUnknownFrame(true, 128)
		wire[23] = byte(protocol)
		works = append(works, currentCorpusWork{id: fmt.Sprintf("ip-protocol/%d", protocol), wire: wire})
	}
	for _, total := range []int{0, 1, 19, 20, 21, 39, 40, 41, 167, 168, 169, 65535} {
		for _, protocol := range []byte{6, 17, 33, 2, 9, 36, 53, 136, 254} {
			wire := dispatchUnknownFrame(true, 128)
			wire[23] = protocol
			binary.BigEndian.PutUint16(wire[16:18], uint16(total))
			works = append(works, currentCorpusWork{id: fmt.Sprintf("ip-length/%d/%d", protocol, total), wire: wire})
		}
	}
	for header := 0; header < 16; header++ {
		for _, options := range [][]byte{{0}, {1, 1, 0}, {2, 4, 5, 180}, {3, 3, 7, 0}, {4, 2, 0}, {8, 10, 0, 0, 0, 1, 0, 0, 0, 2}, {30, 1}, {30, 4, 1, 2}, {255, 0}} {
			wire := dispatchUnknownFrame(true, 16)
			wire[46] = byte(header << 4)
			copy(wire[54:], options)
			for _, end := range []int{54, 55, 56, 57, 58, 64, len(wire)} {
				works = append(works, currentCorpusWork{id: fmt.Sprintf("tcp-options/%d/%x/truncated/%d", header, options, end), wire: append([]byte(nil), wire[:end]...)})
			}
		}
	}
	assertDispatchEquivalence(t, works)
	t.Logf("compared %d protocol-number, enclosing-length and option-boundary cases", len(works))
}

func TestCorpusAutomaticDispatchPreparedConcurrentIsolation(t *testing.T) {
	works := corpusDispatchWorks(t, false)
	want := make([]any, len(works))
	wantError := make([]string, len(works))
	wantRemaining := make([]int, len(works))
	for i, work := range works {
		r := newProtocolCorpusBoundedReader(work.wire)
		n, err := parser.ParseBinaryWithConfig(r, "ethernet", map[string]any{"preparedOperatorLegacy": true}, "Ethernet")
		wantError[i], wantRemaining[i] = fmt.Sprint(err), r.Len()
		if err == nil {
			want[i] = NodeToMap(n)
		}
	}
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for repeat := 0; repeat < 2; repeat++ {
				for i := worker; i < len(works); i += 4 {
					result, _, remaining, err := dispatchParse(works[i])
					if fmt.Sprint(err) != wantError[i] || remaining != wantRemaining[i] || err == nil && !reflect.DeepEqual(result["fields"], want[i]) {
						errors <- fmt.Errorf("worker %d, repeat %d, %s: shared program altered result/error/boundary", worker, repeat, works[i].id)
						return
					}
				}
			}
		}(worker)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}
