package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func TestDCERPCSecondaryAddressAndPaddingKeepOriginalBytes(t *testing.T) {
	wire := make([]byte, 60)
	wire[0], wire[2], wire[3], wire[4] = 5, 12, 3, 0x10
	binary.LittleEndian.PutUint16(wire[8:10], uint16(len(wire)))
	binary.LittleEndian.PutUint16(wire[24:26], 5)
	copy(wire[26:31], []byte{'1', '0', '3', '6', 0})
	wire[31], wire[32] = 0xb0, 1
	root, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.dcerpc", "DCERPC")
	require.NoError(t, err)
	protocolCorpusRequireValue(t, root, "Sec Addr", wire[26:31])
	protocolCorpusRequireValue(t, root, "Pad", []byte{0xb0})
	for _, name := range []string{"Sec Addr", "Pad"} {
		value, err := protocolCorpusFindNode(root, name).Result()
		require.NoError(t, err)
		require.IsType(t, []byte{}, value.Value, "wire octets must not be serialized as potentially invalid UTF-8")
	}
}

func TestDCERPCResultCodesValidateEveryOrdinal(t *testing.T) {
	for _, at := range []int{32, 56} {
		wire := make([]byte, 80)
		wire[0], wire[2], wire[3], wire[4] = 5, 15, 3, 0x10
		binary.LittleEndian.PutUint16(wire[8:10], uint16(len(wire)))
		wire[28] = 2
		binary.LittleEndian.PutUint16(wire[at:at+2], 256)
		_, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.dcerpc", "DCERPC")
		require.ErrorContains(t, err, "invalid bind result")
	}
}
