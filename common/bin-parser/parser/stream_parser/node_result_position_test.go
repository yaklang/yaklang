package stream_parser

import (
	"bytes"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func positionedResultNode(data []byte, pending uint8) *base.Node {
	buffer := bytes.NewBuffer(append([]byte{}, data...))
	writer := base.NewBitWriter(buffer)
	if pending != 0 {
		if err := writer.WriteBits([]byte{0x55}, uint64(pending)); err != nil {
			panic(err)
		}
	}
	ctx := &base.NodeContext{BaseKV: base.NewEmptyConfig().BaseKV}
	ctx.SetItem("buffer", buffer)
	ctx.SetItem("writer", writer)
	return &base.Node{Cfg: base.NewEmptyConfig(), Ctx: ctx}
}

// This intentionally retains the old full-prefix read as an independent
// reference for all valid spans, including padding in the pending octet.
func legacyResultBits(node *base.Node) ([]byte, error) {
	_ = node.Cfg.GetString(CfgEndian)
	span := node.Cfg.GetItem(CfgNodeResult).([2]uint64)
	data := node.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes()
	writer := node.Ctx.GetItem("writer").(*base.BitWriter)
	if writer.PreIsBit {
		data = append(data, writer.PreByte<<(8-writer.PreByteLen))
	}
	reader := base.NewBitReader(bytes.NewBuffer(data))
	if _, err := reader.ReadBits(span[0]); err != nil {
		return nil, err
	}
	return reader.ReadBits(span[1] - span[0])
}

func TestNodeResultPositionAllBitSpans(t *testing.T) {
	for pending := uint8(0); pending < 8; pending++ {
		node := positionedResultNode([]byte{0xa5, 0x13, 0xec, 0x81, 0x72, 0x0f, 0xb9}, pending)
		limit := uint64(56)
		if pending != 0 {
			limit += 8
		}
		for start := uint64(0); start <= limit; start++ {
			for end := start; end <= limit; end++ {
				node.Cfg.SetItem(CfgNodeResult, [2]uint64{start, end})
				want, err := legacyResultBits(node)
				if err != nil {
					t.Fatal(err)
				}
				got, err := getNodeResult(node, true)
				if err != nil || !reflect.DeepEqual(want, got) {
					t.Fatalf("pending=%d span=[%d,%d): got %x/%v, want %x", pending, start, end, got, err, want)
				}
				if len(want) > 0 {
					got.([]byte)[0] ^= 0xff
					again, err := getNodeResult(node, true)
					if err != nil || !reflect.DeepEqual(want, again) {
						t.Fatalf("returned bytes alias source: pending=%d span=[%d,%d)", pending, start, end)
					}
				}
				for _, endian := range []string{"big", "little"} {
					node.Cfg.SetItem(CfgEndian, endian)
					node.Cfg.SetItem(CfgType, "string")
					text, err := getNodeResult(node, false)
					if err != nil || text != string(want) {
						t.Fatalf("string changed: %v %v", text, err)
					}
					if end == start {
						continue
					}
					node.Cfg.SetItem(CfgType, "uint64")
					number, err := getNodeResult(node, false)
					var expected uint64
					if endian == "little" {
						for i, octet := range want {
							expected |= uint64(octet) << (8 * i)
						}
					} else {
						for i, octet := range want {
							bits := uint64(8)
							if i == len(want)-1 && (end-start)%8 != 0 {
								bits = (end - start) % 8
							}
							expected = expected<<bits | uint64(octet)
						}
					}
					if err != nil || number != expected {
						t.Fatalf("pending=%d span=[%d,%d) %s: got %v/%v, want %d", pending, start, end, endian, number, err, expected)
					}
				}
			}
		}
	}
}

func TestNodeResultPositionInvalidSpans(t *testing.T) {
	for _, pending := range []uint8{0, 1, 7} {
		node := positionedResultNode([]byte{0xa5}, pending)
		limit := uint64(8)
		if pending != 0 {
			limit = 16
		}
		for _, span := range [][2]uint64{{1, 0}, {0, limit + 1}, {limit + 1, limit + 1}, {math.MaxUint64, math.MaxUint64}, {0, math.MaxUint64}} {
			node.Cfg.SetItem(CfgNodeResult, span)
			if _, err := getNodeResult(node, true); err == nil {
				t.Fatalf("pending=%d invalid span %v accepted", pending, span)
			}
		}
	}
	for _, pending := range []uint8{0, 1, 7} {
		node := positionedResultNode(nil, pending)
		node.Cfg.SetItem(CfgNodeResult, [2]uint64{})
		if got, err := getNodeResult(node, true); err != nil || len(got.([]byte)) != 0 {
			t.Fatalf("empty result: %v %v", got, err)
		}
	}
}

func BenchmarkNodeResultPosition(b *testing.B) {
	for _, size := range []int{64, 4096, 65536, 1048576} {
		for _, pending := range []uint8{0, 7} {
			for _, legacy := range []bool{true, false} {
				b.Run(fmt.Sprintf("bytes-%d/pending-%d/legacy-%t", size, pending, legacy), func(b *testing.B) {
					node := positionedResultNode(bytes.Repeat([]byte{0xa5}, size), pending)
					end := uint64(size)*8 + uint64(pending)
					node.Cfg.SetItem(CfgNodeResult, [2]uint64{end - 16, end})
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						var err error
						if legacy {
							_, err = legacyResultBits(node)
						} else {
							_, err = getNodeResult(node, true)
						}
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}
