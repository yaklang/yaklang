package protocol_impl

import (
	"bytes"
	"errors"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/utils"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"io"
)

type TpktPacket struct {
	Version  uint8
	Reserved uint8
	TPDU     []byte
}

func (t *TpktPacket) WriteTo(writer io.Writer) (int, error) {
	res, err := t.Marshal()
	if err != nil {
		return 0, err
	}
	return writer.Write(res)
}

func (t *TpktPacket) Marshal() ([]byte, error) {
	data := map[string]any{
		"Version":      t.Version,
		"Reserved":     t.Reserved,
		"PacketLength": len(t.TPDU) + 4,
		"TPDU":         t.TPDU,
	}
	node, err := parser.GenerateBinary(data, "application-layer.tpkt", "TPKT")
	if err != nil {
		return nil, err
	}
	return utils.NodeToBytes(node), nil
}

func ParseTpkt(data []byte) (*TpktPacket, error) {
	node, err := parser.ParseBinary(bytes.NewReader(data), "application-layer.tpkt")
	if err != nil {
		return nil, err
	}
	node = utils.GetSubNode(node, "TPKT")
	if node == nil {
		return nil, errors.New("not found TPKT node")
	}
	ires := utils.NodeToData(node)
	if v, ok := ires.(map[string]any); ok {
		version := uint8(utils.GetUint64FromMap(v, "Version"))
		reserved := uint8(utils.GetUint64FromMap(v, "Reserved"))
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
