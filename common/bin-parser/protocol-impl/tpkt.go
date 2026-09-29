package protocol_impl

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/utils"
	utils2 "github.com/yaklang/yaklang/common/utils"
)

type TpktPacket struct {
	Version  uint8
	Reserved uint8
	TPDU     []byte
}

func NewTpktPacket(data []byte) *TpktPacket {
	return &TpktPacket{
		Version:  3,
		Reserved: 0,
		TPDU:     data,
	}
}

func (t *TpktPacket) WriteTo(writer io.Writer) (int, error) {
	res, err := t.Marshal()
	if err != nil {
		return 0, err
	}
	n, err := writer.Write(res)
	if err == nil && n != len(res) {
		err = io.ErrShortWrite
	}
	return n, err
}

func (t *TpktPacket) Marshal() ([]byte, error) {
	// RFC 1006 section 6: version 3 and a total length of 7..65535.
	// Check before converting PacketLength to the rule's uint16 field.
	if t.Version != 3 {
		return nil, fmt.Errorf("tpkt: unsupported version %d", t.Version)
	}
	if len(t.TPDU) < 3 || len(t.TPDU) > 65531 {
		return nil, fmt.Errorf("tpkt: invalid TPDU length %d", len(t.TPDU))
	}
	data := map[string]any{
		"Version":      t.Version,
		"Reserved":     t.Reserved,
		"PacketLength": len(t.TPDU) + 4,
		"TPDU":         t.TPDU,
	}
	node, err := parser.GenerateBinary(data, "application-layer.msrdp", "TPKT")
	if err != nil {
		return nil, err
	}
	return utils.NodeToBytes(node), nil
}

func ParseTpkt(r io.Reader) (*TpktPacket, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	if header[0] != 3 {
		return nil, fmt.Errorf("tpkt: unsupported version %d", header[0])
	}
	length := binary.BigEndian.Uint16(header[2:])
	if length < 7 {
		return nil, fmt.Errorf("tpkt: invalid packet length %d", length)
	}
	// The reader can contain coalesced packets. Never consume the next
	// envelope while parsing this one, including on malformed input.
	framed := io.MultiReader(bytes.NewReader(header[:]), io.LimitReader(r, int64(length)-4))
	node, err := parser.ParseBinary(framed, "application-layer.msrdp", "TPKT")
	if err != nil {
		return nil, err
	}
	ires := utils.NodeToData(node)
	if v, ok := ires.(map[string]any); ok {
		version := v["Version"].(byte)
		reserved := v["Reserved"].(byte)
		payload := utils2.InterfaceToBytes(utils2.MapGetRaw(v, "TPDU"))
		return &TpktPacket{
			Version:  version,
			Reserved: reserved,
			TPDU:     payload,
		}, nil
	} else {
		return nil, errors.New("node result data format is invalid")
	}
}
