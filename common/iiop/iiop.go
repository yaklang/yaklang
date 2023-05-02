package iiop

import (
	"bytes"
	"net"
	"time"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"
	"yaklang/common/yserx"
)

const (
	LocateRequest byte = 0x03
	LocateReply   byte = 0x04
	Request       byte = 0x00
	Reply         byte = 0x01
)
const (
	ObjectForward   = 2
	LocationForward = 3
	NoException     = 0
	UserException   = 1
	SystemException = 2
)

var (
	REQUESTID2           = []byte("\x00\x00\x00\x02")
	REQUESTID3           = []byte("\x00\x00\x00\x03")
	REQUESTFLAG          = []byte("\x03")
	RESERVED             = []byte("\x00\x00\x00")
	TARGET               = []byte("\x00\x00\x00\x00")
	KEYLENGTH            = []byte("\x00\x00\x00\x78")
	OP_bind_any          = []byte("bind_any\x00")
	OP_rebind_any        = []byte("rebind_any\x00")
	OP_resolve_any       = []byte("resolve_any\x00")
	OP_getServerLocation = []byte("getServerLocation\x00")
	context_bind_any     = []byte("\x00\x00\x00\x00\x00\x00\x06\x00\x00\x00\x05\x00\x00\x00\x18\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x0a\x31\x32\x37\x2e\x30\x2e\x30\x2e\x31\x00\xd6\x1a\x00\x00\x00\x01\x00\x00\x00\x0c\x00\x00\x00\x00\x00\x01\x00\x20\x05\x01\x00\x01\x00\x00\x00\x06\x00\x00\x00\xf0\x00\x00\x00\x00\x00\x00\x00\x28\x49\x44\x4c\x3a\x6f\x6d\x67\x2e\x6f\x72\x67\x2f\x53\x65\x6e\x64\x69\x6e\x67\x43\x6f\x6e\x74\x65\x78\x74\x2f\x43\x6f\x64\x65\x42\x61\x73\x65\x3a\x31\x2e\x30\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\xb4\x00\x01\x02\x00\x00\x00\x00\x0a\x31\x32\x37\x2e\x30\x2e\x30\x2e\x31\x00\xd6\x1a\x00\x00\x00\x64\x00\x42\x45\x41\x08\x01\x03\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x28\x49\x44\x4c\x3a\x6f\x6d\x67\x2e\x6f\x72\x67\x2f\x53\x65\x6e\x64\x69\x6e\x67\x43\x6f\x6e\x74\x65\x78\x74\x2f\x43\x6f\x64\x65\x42\x61\x73\x65\x3a\x31\x2e\x30\x00\x00\x00\x00\x03\x31\x32\x00\x00\x00\x00\x00\x01\x42\x45\x41\x2a\x00\x00\x00\x10\x00\x00\x00\x00\x00\x00\x00\x00\x8c\x8f\xcc\xd1\x88\x86\xd2\xd6\x00\x00\x00\x01\x00\x00\x00\x01\x00\x00\x00\x2c\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x03\x00\x01\x00\x20\x00\x01\x00\x01\x05\x01\x00\x01\x00\x01\x01\x00\x00\x00\x00\x03\x00\x01\x01\x00\x00\x01\x01\x09\x05\x01\x00\x01\x00\x00\x00\x0f\x00\x00\x00\x20\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x42\x45\x41\x03\x00\x00\x00\x14\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x6f\x1d\xf5\x84\x00\x00\x00\x00\x42\x45\x41\x00\x00\x00\x00\x04\x00\x0a\x03\x06\x00\x00\x00\x00")
	context_reslove_any2 = []byte("\x00\x00\x00\x06\x00\x00\x00\x05\x00\x00\x00\x18\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x0a\x31\x32\x37\x2e\x30\x2e\x30\x2e\x31\x00\xe7\x53\x00\x00\x00\x01\x00\x00\x00\x0c\x00\x00\x00\x00\x00\x01\x00\x20\x05\x01\x00\x01\x00\x00\x00\x06\x00\x00\x00\xf0\x00\x00\x00\x00\x00\x00\x00\x28\x49\x44\x4c\x3a\x6f\x6d\x67\x2e\x6f\x72\x67\x2f\x53\x65\x6e\x64\x69\x6e\x67\x43\x6f\x6e\x74\x65\x78\x74\x2f\x43\x6f\x64\x65\x42\x61\x73\x65\x3a\x31\x2e\x30\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\xb4\x00\x01\x02\x00\x00\x00\x00\x0a\x31\x32\x37\x2e\x30\x2e\x30\x2e\x31\x00\xe7\x53\x00\x00\x00\x64\x00\x42\x45\x41\x08\x01\x03\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x28\x49\x44\x4c\x3a\x6f\x6d\x67\x2e\x6f\x72\x67\x2f\x53\x65\x6e\x64\x69\x6e\x67\x43\x6f\x6e\x74\x65\x78\x74\x2f\x43\x6f\x64\x65\x42\x61\x73\x65\x3a\x31\x2e\x30\x00\x00\x00\x00\x03\x31\x32\x00\x00\x00\x00\x00\x01\x42\x45\x41\x2a\x00\x00\x00\x10\x00\x00\x00\x00\x00\x00\x00\x00\x4d\x04\x1e\x5c\x71\xfd\xf0\x60\x00\x00\x00\x01\x00\x00\x00\x01\x00\x00\x00\x2c\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x03\x00\x01\x00\x20\x00\x01\x00\x01\x05\x01\x00\x01\x00\x01\x01\x00\x00\x00\x00\x03\x00\x01\x01\x00\x00\x01\x01\x09\x05\x01\x00\x01\x00\x00\x00\x0f\x00\x00\x00\x20\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x42\x45\x41\x03\x00\x00\x00\x14\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\xff\xff\xe6\xde\xe4\xdf\x00\x00\x00\x00\x42\x45\x41\x00\x00\x00\x00\x04\x00\x0a\x03\x06\x00\x00\x00\x00")
	context_reslove_any3 = []byte("\x00\x00\x00\x04\x00\x00\x00\x05\x00\x00\x00\x18\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x0a\x31\x32\x37\x2e\x30\x2e\x30\x2e\x31\x00\xf7\x41\x00\x00\x00\x01\x00\x00\x00\x0c\x00\x00\x00\x00\x00\x01\x00\x20\x05\x01\x00\x01\x42\x45\x41\x03\x00\x00\x00\x14\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\xff\xff\xe6\xde\xe4\xdf\x00\x00\x00\x00\x42\x45\x41\x00\x00\x00\x00\x04\x00\x0a\x03\x06\x00\x00\x00\x00")
	context_rebind_any   = []byte("\x00\x00\x00\x06\x00\x00\x00\x05\x00\x00\x00\x1e\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x10\x31\x39\x32\x2e\x31\x36\x38\x2e\x31\x30\x31\x2e\x31\x31\x36\x00\xdb\xf7\x00\x00\x00\x00\x00\x01\x00\x00\x00\x0c\x00\x00\x00\x00\x00\x01\x00\x20\x05\x01\x00\x01\x00\x00\x00\x06\x00\x00\x00\xf8\x00\x00\x00\x00\x00\x00\x00\x28\x49\x44\x4c\x3a\x6f\x6d\x67\x2e\x6f\x72\x67\x2f\x53\x65\x6e\x64\x69\x6e\x67\x43\x6f\x6e\x74\x65\x78\x74\x2f\x43\x6f\x64\x65\x42\x61\x73\x65\x3a\x31\x2e\x30\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\xbc\x00\x01\x02\x00\x00\x00\x00\x10\x31\x39\x32\x2e\x31\x36\x38\x2e\x31\x30\x31\x2e\x31\x31\x36\x00\xdb\xf7\x00\x00\x00\x00\x00\x64\x00\x42\x45\x41\x08\x01\x03\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x28\x49\x44\x4c\x3a\x6f\x6d\x67\x2e\x6f\x72\x67\x2f\x53\x65\x6e\x64\x69\x6e\x67\x43\x6f\x6e\x74\x65\x78\x74\x2f\x43\x6f\x64\x65\x42\x61\x73\x65\x3a\x31\x2e\x30\x00\x00\x00\x00\x03\x31\x32\x00\x00\x00\x00\x00\x01\x42\x45\x41\x2a\x00\x00\x00\x10\x00\x00\x00\x00\x00\x00\x00\x00\x2d\x98\xc0\x5d\x6a\xbb\x50\x50\x00\x00\x00\x01\x00\x00\x00\x01\x00\x00\x00\x2c\x00\x00\x00\x00\x00\x01\x00\x20\x00\x00\x00\x03\x00\x01\x00\x20\x00\x01\x00\x01\x05\x01\x00\x01\x00\x01\x01\x00\x00\x00\x00\x03\x00\x01\x01\x00\x00\x01\x01\x09\x05\x01\x00\x01\x00\x00\x00\x0f\x00\x00\x00\x20\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x42\x45\x41\x03\x00\x00\x00\x14\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x6f\x1d\xf5\x84\x00\x00\x00\x00\x42\x45\x41\x00\x00\x00\x00\x04\x00\x0a\x03\x06\x00\x00\x00\x00")
	dataLen_bind_any     = 1452 - 912
	dataLen_reslove_any2 = 565 - 25
	dataLen_reslove_any3 = 277 - 25
	dataLen_rebind_any   = 1468 - 912
)

type MessageHeader struct {
	Magic       []byte
	Version     []byte
	MessageType byte
	MessageSize int
}

func (m *MessageHeader) bytes() []byte {
	var buf bytes.Buffer
	buf.Write(m.Magic)
	buf.Write(m.Version)
	buf.WriteByte(m.MessageType)
	buf.Write(yserx.IntTo4Bytes(m.MessageSize))
	return buf.Bytes()
}
func NewMessageHeader() *MessageHeader {
	return &MessageHeader{
		Magic:       []byte("GIOP"),
		Version:     []byte("\x01\x02\x00"),
		MessageType: 0x03,
		MessageSize: 0,
	}
}

type MessageRequest struct {
	Header             *MessageHeader
	RequestId          int
	TargetAddr         int
	ResponseFalg       byte
	Reserved           []byte
	TargetAddress      int
	KeyAddress         []byte
	Operation          []byte
	ServiceContextList []*ServiceContext
	StubData           []byte
}

func (m *MessageRequest) orbBytes() []byte {
	var buf bytes.Buffer
	switch m.Header.MessageType {
	case LocateRequest:
		buf.Write(yserx.IntTo4Bytes(m.RequestId))
		buf.Write(yserx.IntTo4Bytes(m.TargetAddr))
		buf.Write(yserx.IntTo4Bytes(len(m.KeyAddress)))
		buf.Write(m.KeyAddress)
		return buf.Bytes()
	case Request:
		buf.Write(yserx.IntTo4Bytes(m.RequestId))
		buf.WriteByte(m.ResponseFalg)
		buf.Write(m.Reserved)
		buf.Write(yserx.IntTo4Bytes(m.TargetAddr))
		buf.Write(yserx.IntTo4Bytes(len(m.KeyAddress)))
		buf.Write(m.KeyAddress)
		buf.Write(yserx.IntTo4Bytes(len(m.Operation) + 1))
		opB := append(m.Operation, 0x00)
		lopb := len(opB)
		if (lopb/4)*4 < lopb {
			excpectL := ((lopb / 4) + 1) * 4
			for i := 0; i < excpectL-lopb; i++ {
				opB = append(opB, 0x00)
			}
		}

		buf.Write(opB)
		buf.Write(yserx.IntTo4Bytes(len(m.ServiceContextList)))
		for i := 0; i < len(m.ServiceContextList); i++ {
			context := m.ServiceContextList[i]
			buf.Write(context.Vscid)
			buf.WriteByte(context.Unknow)
			buf.WriteByte(context.Scid)
			buf.Write(yserx.IntTo4Bytes(len(context.Data) + 1))
			buf.WriteByte(context.Endianness)
			buf.Write(context.Data)
		}
		buf.Write([]byte("\x00\x00\x00\x00"))
		buf.Write(m.StubData)
		return buf.Bytes()
	}
	return nil
}
func (m *MessageRequest) Bytes() []byte {
	orbBytes := m.orbBytes()
	m.Header.MessageSize = len(orbBytes)
	return append(m.Header.bytes(), orbBytes...)
}
func NewMessageRequest() *MessageRequest {
	return &MessageRequest{
		TargetAddr: 0,
		Header:     NewMessageHeader(),
	}
}

type ServiceContext struct {
	Vscid      []byte
	Unknow     byte
	Scid       byte
	Endianness byte
	Data       []byte
}
type MessageIOR struct {
	Length      int
	IOR_type    []byte
	ProfileId   int
	Endianness  byte
	Version     []byte
	ProfileHost []byte
	ProfilePort int
	ObjectKey   []byte
	Others      []byte
}
type MessageResponse struct {
	Header             *MessageHeader
	RequestId          int
	ReplyStatus        int
	IOR                *MessageIOR
	ServiceContextList []*ServiceContext
	ExceptionId        []byte
	StubData           []byte
}

func GenRequest(header []byte, REQUESTID []byte, keyAddress []byte, data []byte, op []byte, contextList []byte) []byte {
	opLen := yserx.IntTo4Bytes(len(op))
	//contextListS := "00000000000006000000050000001800000000000000010000000a3132372e302e302e3100d61a000000010000000c00000000000100200501000100000006000000f0000000000000002849444c3a6f6d672e6f72672f53656e64696e67436f6e746578742f436f6465426173653a312e30000000000100000000000000b4000102000000000a3132372e302e302e3100d61a0000006400424541080103000000000100000000000000000000002849444c3a6f6d672e6f72672f53656e64696e67436f6e746578742f436f6465426173653a312e30000000000331320000000000014245412a0000001000000000000000008c8fccd18886d2d600000001000000010000002c00000000000100200000000300010020000100010501000100010100000000030001010000010109050100010000000f00000020000000000000000000000000000000010000000000000000010000000000000042454103000000140000000000000000000000006f1df584000000004245410000000004000a030600000000"
	//contextList, err := codec.DecodeHex(contextListS)
	//if err != nil {
	//	log.Errorf("%v", err)
	//	return nil
	//}
	var buf bytes.Buffer

	buf.Write(header)
	buf.Write(REQUESTID)
	buf.Write(REQUESTFLAG)
	buf.Write(RESERVED)
	buf.Write(TARGET)
	buf.Write(KEYLENGTH)
	buf.Write(keyAddress)
	buf.Write(opLen)
	buf.Write(append(op))
	//buf.Write(op)
	buf.Write(contextList)
	buf.Write(data)
	return buf.Bytes()
}
func GenHeader(l int) []byte {
	reqHeaderS := "47494f5001020000"
	reqHeader, _ := codec.DecodeHex(reqHeaderS)
	return append(reqHeader, yserx.IntTo4Bytes(l)...)
}

func GetKeyFromBytes(locateReply []byte) ([]byte, error) {
	n := bytes.Index(locateReply, []byte("\x00\x00\x00\x78\x00BEA"))
	if n != -1 {
		n += 4
		if len(locateReply) >= n+120 {
			return locateReply[n : n+120], nil
		}
	}

	n = bytes.Index(locateReply, []byte("\x00\x00\x00\x88\x00BEA"))
	if n != -1 {
		n += 4
		if len(locateReply) >= n+136 {
			return locateReply[n : n+136], nil
		}
	}

	return nil, utils.Errorf("GetKey error")

}

//	func Bytes2Int(data []byte) int {
//		return int(binary.BigEndian.Uint32(data))
//	}
func ParseHeader(data []byte) (*MessageHeader, error) {
	if len(data) < 12 {
		return nil, utils.Error(" header format error")
	}
	du := NewBytesUtils(data)
	return &MessageHeader{
		Magic:       du.ReadBytesUnsafe(4),
		Version:     du.ReadBytesUnsafe(3),
		MessageType: du.ReadByteUnsafe(),
		MessageSize: du.Read4BytesToIntUnsafe(),
	}, nil
}
func parseIOR(bu *BytesUtils) (*MessageIOR, error) {
	ior := &MessageIOR{}
	//读取IOR
	l, err := bu.Read4BytesToInt()
	ior_type, err := bu.ReadBytes(l)
	if err != nil {
		return nil, err
	}
	ior.IOR_type = ior_type
	_, err = bu.ReadBytes(5)
	id, err := bu.Read4BytesToInt()
	if err != nil {
		return nil, err
	}
	ior.ProfileId = id
	sequenceLen, err := bu.Read4BytesToInt()
	if err != nil {
		return nil, err
	}
	iorBu, err := bu.NewChildBytesUtils(sequenceLen)
	if err != nil {
		return nil, err
	}
	iorBu.Next(4)
	l, err = iorBu.Read4BytesToInt()
	host, err := iorBu.ReadBytes(l)
	port, err := iorBu.Read2BytesToInt()
	if err != nil {
		return nil, err
	}
	ior.ProfileHost = host
	ior.ProfilePort = port
	iorBu.Next(2)
	l, err = iorBu.Read4BytesToInt()
	key, err := iorBu.ReadBytes(l)
	others, err := iorBu.ReadOthers()
	if err != nil {
		return nil, err
	}
	ior.ObjectKey = key
	ior.Others = others
	return ior, nil
}
func parseContextList(bu *BytesUtils) ([]*ServiceContext, error) {
	var ServiceContextList []*ServiceContext
	contextLen, err := bu.Read4BytesToInt()
	if err != nil {
		return nil, err
	}
	for i := 0; i < contextLen; i++ {
		newBu, err := bu.NewChildBytesUtils(8)
		if err != nil {
			return nil, err
		}

		vscid := newBu.ReadBytesUnsafe(2)
		unknow := newBu.ReadByteUnsafe()
		scid := newBu.ReadByteUnsafe()
		l := newBu.Read4BytesToIntUnsafe()
		endianness, err := bu.ReadByte()
		data, err := bu.ReadBytes(l - 1)
		if err != nil {
			return nil, err
		}
		context := &ServiceContext{
			Scid:       scid,
			Unknow:     unknow,
			Vscid:      vscid,
			Endianness: endianness,
			Data:       data,
		}
		ServiceContextList = append(ServiceContextList, context)
	}
	return ServiceContextList, nil
}
func ParseMessageResponse(data []byte) (*MessageResponse, error) {
	bu := NewBytesUtils(data)
	//读取header
	headerByte, err := bu.ReadBytes(12)
	if err != nil {
		return nil, utils.Error("header format error")
	}
	header, err := ParseHeader(headerByte)
	if err != nil {
		return nil, err
	}
	msg := &MessageResponse{
		Header: header,
	}

	//读取一些flag
	flagsBu, err := bu.NewChildBytesUtils(8)
	if err != nil {
		return nil, utils.Error("ior format error")
	}
	id := flagsBu.Read4BytesToIntUnsafe()
	status := flagsBu.Read4BytesToIntUnsafe()
	msg.RequestId = id
	msg.ReplyStatus = status
	switch header.MessageType {
	case LocateReply:
		switch status {
		case ObjectForward:
			ior, err := parseIOR(bu)
			if err != nil {
				return nil, err
			}
			msg.IOR = ior
		}
	case Reply:
		switch status {
		case SystemException:
			contextList, err := parseContextList(bu)
			if err != nil {
				return nil, err
			}
			msg.ServiceContextList = contextList
			l, err := bu.Read4BytesToInt()
			exceptionId, err := bu.ReadBytes(l)
			other, err := bu.ReadOthers()
			if err != nil {
				return nil, err
			}
			msg.ExceptionId = exceptionId
			msg.StubData = other
		case LocationForward:
			contextList, err := parseContextList(bu)
			if err != nil {
				return nil, err
			}
			msg.ServiceContextList = contextList
			bu.Next(7)
			ior, err := parseIOR(bu)
			if err != nil {
				return nil, err
			}
			msg.IOR = ior
		case NoException:
			contextList, err := parseContextList(bu)
			if err != nil {
				return nil, err
			}
			msg.ServiceContextList = contextList
			other, err := bu.ReadOthers()
			if err != nil {
				return nil, err
			}
			msg.StubData = other
		case UserException:
			contextList, err := parseContextList(bu)
			if err != nil {
				return nil, err
			}
			msg.ServiceContextList = contextList
			l, err := bu.Read4BytesToInt()
			exceptionId, err := bu.ReadBytes(l)
			other, err := bu.ReadOthers()
			if err != nil {
				return nil, err
			}
			msg.ExceptionId = exceptionId
			msg.StubData = other
		}

	}
	return msg, nil

}
func ParseMessageRequest(data []byte) (*MessageRequest, error) {
	bu := NewBytesUtils(data)
	//读取header
	headerByte, err := bu.ReadBytes(12)
	if err != nil {
		return nil, utils.Error("header format error")
	}
	header, err := ParseHeader(headerByte)
	if err != nil {
		return nil, err
	}
	msg := &MessageRequest{Header: header}

	switch header.MessageType {
	case LocateRequest:
		flagsBu, err := bu.NewChildBytesUtils(12)
		if err != nil {
			return nil, utils.Error("ior format error")
		}
		id := flagsBu.Read4BytesToIntUnsafe()
		targetAddr := flagsBu.Read4BytesToIntUnsafe()
		l := flagsBu.Read4BytesToIntUnsafe()
		key, err := bu.ReadBytes(l)
		if err != nil {
			return nil, utils.Error("ior format error")
		}
		msg.RequestId = id
		msg.KeyAddress = key
		msg.TargetAddr = targetAddr
		return msg, nil
	case Request:
		//读取一些flag
		flagsBu, err := bu.NewChildBytesUtils(16)
		if err != nil {
			return nil, utils.Error("ior format error")
		}
		id := flagsBu.Read4BytesToIntUnsafe()
		flag := flagsBu.ReadByteUnsafe()
		reserved := flagsBu.ReadBytesUnsafe(3)
		targetAddr := flagsBu.Read4BytesToIntUnsafe()
		keyLen := flagsBu.Read4BytesToIntUnsafe()
		msg.RequestId = id
		msg.ResponseFalg = flag
		msg.Reserved = reserved
		msg.TargetAddress = targetAddr
		key, err := bu.ReadBytes(keyLen)
		if err != nil {
			return nil, err
		}
		msg.KeyAddress = key
		lop, err := bu.Read4BytesToInt()
		var sz int
		if lop%4 == 0 {
			sz = lop
		} else {
			sz = ((lop / 4) + 1) * 4
			//sz = int(math.Ceil(float64(lop/4)) * 4)
		}
		opBu, err := bu.NewChildBytesUtils(sz)
		if err != nil {
			return nil, err
		}
		op := opBu.ReadBytesUnsafe(lop - 1)
		msg.Operation = op
		//读取contextList
		//bu.Next(1)
		//abu, _ := bu.NewChildBytesUtils(10)
		//_ = abu
		contextList, err := parseContextList(bu)
		if err != nil {
			return nil, err
		}
		msg.ServiceContextList = contextList

		msg.StubData, err = bu.ReadOthers()
		if err != nil {
			return nil, err
		}
		return msg, nil
	}
	return nil, utils.Error("Not support MessageType")
}
func GetKeyAddress(conn net.Conn) ([]byte, error) {
	msgReq := GetLocateRequestMsgTmp()
	msgReq.Bytes()
	//header, err := codec.DecodeHex("47494f50010200030000001700000002000000000000000b4e616d6553657276696365")
	//if err != nil {
	//	log.Infof("Decode header error: %v", err)
	//	return nil, utils.Errorf("header format error")
	//}
	//
	header := msgReq.Bytes()
	conn.Write(header)
	locateReply := utils.StableReaderEx(conn, 5*time.Second, 10240)
	return GetKeyFromBytes(locateReply)
	//msg, err := ParseMessageResponse(locateReply)
	//if err != nil {
	//	return nil, err
	//}
	//return msg.IOR.ObjectKey, nil
}

func GenBind_any(keyAddress []byte, data []byte) []byte {
	req := GetBindMsgTmp()
	req.KeyAddress = keyAddress
	req.StubData = data
	return req.Bytes()
}

func GenResolve_any2(keyAddress []byte, data []byte) []byte {
	req := GetReslove_anyMsgTmp()
	req.KeyAddress = keyAddress
	req.StubData = data
	return req.Bytes()
	//return GenRequest(GenHeader(dataLen_reslove_any2+len(data)), REQUESTID2, keyAddress, data, OP_resolve_any, context_reslove_any2)
}
func GenResolve_any3(keyAddress []byte, data []byte) []byte {
	return GenRequest(GenHeader(dataLen_reslove_any3+len(data)), REQUESTID3, keyAddress, data, OP_resolve_any, context_reslove_any3)
}

func GenRebind_any(keyAddress []byte, data []byte) []byte {
	req := GetRebindMsgTmp()
	req.StubData = data
	req.KeyAddress = keyAddress
	return req.Bytes()
}
