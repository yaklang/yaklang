package stream_parser

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"testing"
)

// Frozen pre-optimization bit-reader oracle.
func legacyNodeResultWindow(node *base.Node, isByte bool) (any, error) {
	endian := node.Cfg.GetString(CfgEndian)
	if endian != "little" {
		endian = "big"
	}
	var resPoint [2]uint64
	composite := !node.Cfg.Has(CfgNodeResult)
	if composite {
		var start, end uint64
		first := true
		walkNode(node, func(n *base.Node) bool {
			if NodeHasResult(n) {
				if first {
					first = false
					p := GetNodeResultPos(n)
					start = p[0]
					end = p[1]
				} else {
					p := GetNodeResultPos(n)
					end = p[1]
				}
			}
			return true
		})
		// Composite nodes have a derived span, not an integral-byte slice.
		// Use the same checked bit extraction as leaves, including pending
		// writer bits. Composite values historically return raw bytes here.
		resPoint = [2]uint64{start, end}
		isByte = true
	} else {
		resPoint = node.Cfg.GetItem(CfgNodeResult).([2]uint64)
	}
	buffer := node.Ctx.GetItem("buffer").(*bytes.Buffer)
	byts := buffer.Bytes()
	writer := node.Ctx.GetItem("writer").(*base.BitWriter)
	availableBytes := uint64(len(byts))
	if writer.PreIsBit {
		availableBytes++
	}
	// Seek directly to the containing octet. Reading and discarding the whole
	// prefix allocates in proportion to the field offset for every Result call.
	// Keep the pending writer octet (including its padding) readable as before.
	if resPoint[1] < resPoint[0] || resPoint[1] > availableBytes*8 {
		return nil, fmt.Errorf("read bits error: invalid result span [%d, %d) for %d bytes", resPoint[0], resPoint[1], availableBytes)
	}
	byteStart, byteEnd := resPoint[0]/8, (resPoint[1]+7)/8
	if composite && resPoint[0]%8 == 0 && resPoint[1]%8 == 0 && byteEnd <= uint64(len(byts)) {
		// Preserve the existing zero-copy behavior of aligned composites.
		return byts[byteStart:byteEnd], nil
	}
	if byteStart > uint64(len(byts)) {
		// The only valid position beyond the buffer is the zero-width span
		// immediately after a pending writer octet.
		byts = nil
	} else {
		needsPending := byteEnd > uint64(len(byts))
		byts = byts[byteStart:min(byteEnd, uint64(len(byts)))]
		if needsPending {
			// Append only to the field window, never copy the preceding input.
			byts = append(byts, writer.PreByte<<(8-writer.PreByteLen))
		}
	}
	reader := base.NewBitReader(bytes.NewBuffer(byts))
	if _, err := reader.ReadBits(resPoint[0] % 8); err != nil {
		return nil, fmt.Errorf("read bits error: %w", err)
	}
	buf, err := reader.ReadBits(resPoint[1] - resPoint[0])
	if err != nil {
		return nil, fmt.Errorf("read bits error: %w", err)
	}
	if isByte {
		return buf, nil
	} else {
		if node.Cfg.GetString(CfgType) == "string" {
			return string(buf), nil
		}
		_ = endian
		typeName := node.Cfg.GetString(CfgType)
		bitLength := resPoint[1] - resPoint[0]
		// ReadBits returns full octets followed by a right-aligned final
		// partial octet. A big-endian integer needs the entire value aligned
		// right instead: 14 bits 00000100 100011 mean 0x0123, not 0x0423.
		// Raw bit results and little-endian octet interpretation are unchanged.
		if endian == "big" && bitLength%8 != 0 && len(buf) > 1 {
			switch typeName {
			case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
				padding := 8 - bitLength%8
				aligned := make([]byte, len(buf))
				for index := 0; index < len(buf)-1; index++ {
					aligned[index] |= buf[index] >> padding
					aligned[index+1] = buf[index] << (8 - padding)
				}
				aligned[len(buf)-1] |= buf[len(buf)-1]
				buf = aligned
			}
		}
		return ConvertToVar(buf, uint64(len(buf)), endian, typeName), nil
	}
}
func TestNodeResultWindowMatchesBitReader(t *testing.T) {
	for _, pending := range []uint8{0, 1, 7} {
		root := giopBridgeInlineRoot(t, "endian: big\nPackage:\n  Value: raw\n")
		p := &DefParser{}
		require.NoError(t, p.OnRoot(root))
		buffer := root.Ctx.GetItem("buffer").(*bytes.Buffer)
		buffer.Write([]byte{0x80, 0x23, 0xff, 0x12, 0x34, 0x56, 0x78, 0x90, 0xab, 0xcd, 0xef, 0x01})
		writer := root.Ctx.GetItem("writer").(*base.BitWriter)
		if pending > 0 {
			writer.PreIsBit = true
			writer.PreByte = 0x55
			writer.PreByteLen = pending
		}
		n := root.Children[0].Children[0]
		for _, endian := range []string{"big", "little"} {
			n.Cfg.SetItem(CfgEndian, endian)
			for _, typ := range []string{"raw", "string", "bytes", "bool", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64"} {
				n.Cfg.SetItem(CfgType, typ)
				for start := uint64(0); start <= 24; start++ {
					for length := uint64(0); length <= 80; length++ {
						n.Cfg.SetItem(CfgNodeResult, [2]uint64{start, start + length})
						for _, asBytes := range []bool{false, true} {
							want, oldErr := legacyNodeResultWindow(n, asBytes)
							got, err := getNodeResult(n, asBytes)
							require.Equal(t, fmt.Sprint(oldErr), fmt.Sprint(err))
							require.Equal(t, want, got, "%s/%s start=%d length=%d bytes=%t", endian, typ, start, length, asBytes)
						}
					}
				}
			}
		}
		n.Cfg.SetItem(CfgType, "raw")
		n.Cfg.SetItem(CfgNodeResult, [2]uint64{0, 8})
		first, err := getNodeResult(n, false)
		require.NoError(t, err)
		first.([]byte)[0] = 0
		second, err := getNodeResult(n, false)
		require.NoError(t, err)
		require.Equal(t, []byte{0x80}, second)
	}
}
