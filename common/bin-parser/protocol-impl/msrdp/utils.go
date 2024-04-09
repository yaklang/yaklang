package msrdp

import (
	"encoding/binary"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	utils2 "github.com/yaklang/yaklang/common/bin-parser/utils"
	"golang.org/x/crypto/md4"
	"io"
	"unicode/utf16"
)

func convertUTF16ToLittleEndianBytes(u []uint16) []byte {
	b := make([]byte, 2*len(u))
	for index, value := range u {
		binary.LittleEndian.PutUint16(b[index*2:], value)
	}
	return b
}
func UnicodeEncode(p string) []byte {
	return convertUTF16ToLittleEndianBytes(utf16.Encode([]rune(p)))
}
func MD4(data []byte) []byte {
	h := md4.New()
	h.Write(data)
	return h.Sum(nil)
}

func readFieldFromBlock(block []byte, offset uint32, length uint16) []byte {
	return block[offset : offset+uint32(length)]
}
func GenRdpSubProtocol(data map[string]any, key string) ([]byte, error) {
	node, err := parser.GenerateBinary(data, "application-layer.msrdp", key)
	if err != nil {
		return nil, err
	}
	d := utils2.NodeToBytes(node)
	return d, nil
}
func ParseRdpSubProtocol(reader io.Reader, key string) (any, error) {
	node, err := parser.ParseBinary(reader, "application-layer.msrdp", key)
	if err != nil {
		return nil, err
	}
	d := utils2.NodeToData(node)
	return d, nil
}
func readeNtlmFieldFromBlock(raw map[string]any, block []byte, name string) []byte {
	infoFields := raw[name].(map[string]any)
	offset := infoFields["BufferOffset"].(uint32) - 56 // header length is 65
	length := infoFields["Length"].(uint16)
	return readFieldFromBlock(block, offset, length)
}

type NTLMFieldBuilder struct {
	payload []byte
}

func NewNTLMFieldBuilder() *NTLMFieldBuilder {
	return &NTLMFieldBuilder{}
}
func (n *NTLMFieldBuilder) WriteField(data []byte) map[string]any {
	l := uint16(len(data))
	offset := uint32(len(n.payload))
	n.payload = append(n.payload, data...)
	return map[string]any{
		"Length":       l,
		"MaxLength":    l,
		"BufferOffset": offset + 88,
	}
}

func (n *NTLMFieldBuilder) GetPayload() []byte {
	return n.payload
}
func toWindowsString(s string) (res []byte) {
	if s == "" {
		return nil
	}
	for _, ch := range utf16.Encode([]rune(s)) {
		bs := make([]byte, 2)
		binary.LittleEndian.PutUint16(bs, ch)
		res = append(res, bs...)
	}
	return
}
