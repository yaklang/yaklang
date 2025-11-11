package lowhttp

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/pkg/errors"
)

const (
	FINALBIT               = 1 << 7
	RSV1BIT                = 1 << 6
	RSV2BIT                = 1 << 5
	RSV3BIT                = 1 << 4
	MASKBIT                = 1 << 7
	UNRSV1BIT              = 0b10111111
	RESET_MESSAGE_TYPE_BIT = 0b11110000
	FRAME_TYPE_BIT         = 0b00001111
	TWO_BYTE_BIT           = 0b01111110
	EIGHT_BYTE_BIT         = 0b01111111

	TWO_BYTE_SIZE  = 65535
	SEVEN_BIT_SIZE = 125 // 根据websocket协议，126和127分别代表用后续两个字节/八个字节表示长度，所以这里只能用125

	DEFAULT_TEXT_MESSAGE_FISRT_BYTE = 0b10000001

	DEFAULT_CLOSE_MESSAGE_FIRST_BYTE = 0b10001000

	TextMessage     = 1
	BinaryMessage   = 2
	CloseMessage    = 8
	PingMessage     = 9
	PongMessage     = 10
	ContinueMessage = 0
)

// Close codes defined in RFC 6455, section 11.7.
const (
	CloseNormalClosure           = 1000
	CloseGoingAway               = 1001
	CloseProtocolError           = 1002
	CloseUnsupportedData         = 1003
	CloseNoStatusReceived        = 1005
	CloseAbnormalClosure         = 1006
	CloseInvalidFramePayloadData = 1007
	ClosePolicyViolation         = 1008
	CloseMessageTooBig           = 1009
	CloseMandatoryExtension      = 1010
	CloseInternalServerErr       = 1011
	CloseServiceRestart          = 1012
	CloseTryAgainLater           = 1013
	CloseTLSHandshake            = 1015
)

func GetClosePayloadFromCloseCode(closeCode int) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(closeCode))
	return buf
}

var keyGUID = []byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11")

type Frame struct {
	raw           []byte
	maskingKey    []byte
	rawPayload    []byte  // payload without mask
	payload       []byte  // masked payload
	data          []byte  // decoded text
	closeCode     *uint16 // Close codes defined in RFC 6455, section 11.7.
	payloadLength uint64
	messageType   int
	firstByte     byte
	secondByte    byte
	mask          bool
}

func (f *Frame) SetOpcode(opcode int) {
	firstByte := f.firstByte
	firstByte &= RESET_MESSAGE_TYPE_BIT
	firstByte |= uint8(opcode)
	f.firstByte = firstByte
	f.messageType = opcode
}

func (f *Frame) Bytes() ([]byte, []byte) {
	data := utils.BytesClone(f.payload)
	firstByte, secondByte := f.firstByte, f.secondByte

	dataLength := len(data)
	if f.mask {
		secondByte = secondByte | MASKBIT
	}

	rawBuf := bytes.NewBuffer(nil)
	rawBuf.WriteByte(firstByte)
	rawBuf.WriteByte(secondByte)

	if dataLength > TWO_BYTE_SIZE {
		l := make([]byte, 8)
		binary.BigEndian.PutUint64(l, uint64(dataLength))
		rawBuf.Write(l)
	} else if dataLength > SEVEN_BIT_SIZE {
		l := make([]byte, 2)
		binary.BigEndian.PutUint16(l, uint16(dataLength))
		rawBuf.Write(l)
	}

	// masking key
	if f.mask {
		rawBuf.Write(f.maskingKey)
		maskBytes(f.maskingKey, data, int(dataLength))
	}

	rawBuf.Write(data)
	return rawBuf.Bytes(), f.data
}

func (f *Frame) FIN() bool {
	return f.firstByte&FINALBIT != 0
}

func (f *Frame) RSV1() bool {
	return f.firstByte&RSV1BIT != 0
}

func (f *Frame) SetRSV1() {
	f.firstByte |= RSV1BIT
}

func (f *Frame) UnsetRSV1() {
	f.firstByte &= UNRSV1BIT
}

func (f *Frame) RSV2() bool {
	return f.firstByte&RSV2BIT != 0
}

func (f *Frame) RSV3() bool {
	return f.firstByte&RSV3BIT != 0
}

func (f *Frame) HasRsv() bool {
	return f.RSV1() || f.RSV2() || f.RSV3()
}

func (f *Frame) Type() int {
	return f.messageType
}

func (f *Frame) IsReservedType() bool {
	return f.messageType >= 3 && f.messageType <= 7 || f.messageType >= 11
}

func (f *Frame) IsValidCloseCode() bool {
	if f.closeCode == nil {
		return true
	}
	return isValidCloseCode(int(*f.closeCode))
}

func (f *Frame) GetCloseCode() int {
	if f.closeCode == nil {
		return 0
	}
	return int(*f.closeCode)
}

func (f *Frame) GetRaw() []byte {
	return f.raw
}

func (f *Frame) GetMask() bool {
	return f.mask
}

func (f *Frame) GetData() []byte {
	return f.data
}

func (f *Frame) GetPayload() []byte {
	return f.payload
}

func (f *Frame) GetFirstByte() byte {
	return f.firstByte
}

func (f *Frame) GetMaskingKey() []byte {
	return f.maskingKey
}

func (f *Frame) SetMaskingKey(r []byte) {
	f.maskingKey = r[:]
}

func (f *Frame) SetData(d []byte) {
	f.data = d
}

func (f *Frame) IsControl() bool {
	return IsControlMessage(f.messageType)
}

func (f *Frame) Show() {
	raw := utils.BytesClone(f.data)
	rawString := strings.Clone(string(raw))
	if len(raw) > 30 {
		if len(raw) > 60 {
			rawString = rawString[:30] + "..." + rawString[len(raw)-30:]
		} else {
			rawString = rawString[:15] + "..." + rawString[len(raw)-15:]
		}
	}
	rawString = fmt.Sprintf("%d %s", len(raw), strconv.Quote(rawString))
	switch f.Type() {
	case TextMessage:
		log.Infof("text:    %v (%v)", rawString, raw)
	case BinaryMessage:
		log.Infof("binary:  %v (%v)", rawString, raw)
	case CloseMessage:
		log.Infof("close:   %v (%v)", rawString, raw)
	case PingMessage:
		log.Infof("ping:    %v (%v)", rawString, raw)
	case PongMessage:
		log.Infof("pong:    %v (%v)", rawString, raw)
	case ContinueMessage:
		log.Infof("continue:%v (%v)", rawString, raw)
	default:
		log.Infof("unk-%02x:%v (%v)", f.Type(), rawString, raw)
	}
}

func IsControlMessage(opcode int) bool {
	return opcode == CloseMessage || opcode == PingMessage || opcode == PongMessage
}

type FrameReader struct {
	r              *bufio.Reader     // raw reader
	limitReader    *io.LimitedReader // io.LimitReader
	flateTail      *bytes.Reader     // flate tail \x00\x00\xff\xff
	flateReader    io.Reader         // flate reader
	fragmentBuffer *bytes.Buffer     // fragment buffer
	c              *WebsocketClient
	dict           *slidingWindow
	frame          *Frame
	isDeflate      bool
}

func NewFrameReader(r io.Reader, isDeflate bool) *FrameReader {
	return NewFrameReaderFromBufio(bufio.NewReader(r), isDeflate)
}

func NewFrameReaderFromBufio(r *bufio.Reader, isDeflate bool) *FrameReader {
	fr := &FrameReader{
		r:           r,
		limitReader: &io.LimitedReader{R: r, N: 4096},
		isDeflate:   isDeflate,
	}
	if isDeflate {
		fr.flateTail = bytes.NewReader(compressionReadTail)
	}

	return fr
}

func (fr *FrameReader) SetWebsocketClient(c *WebsocketClient) {
	fr.c = c
}

func (fr *FrameReader) ReadFrame() (frame *Frame, err error) {
	frame = &Frame{}
	defer func() {
		if recoveredError := recover(); recoveredError != nil {
			err = utils.Errorf("read frame panic: %s", recoveredError)
			return
		}
	}()
	var (
		p, tempBytes []byte
		remaining    uint8
		dataLength   uint64
		frameType    int
		rawBuf       bytes.Buffer
	)

	p = make([]byte, 2)
	p[0], err = fr.r.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "websocket: read first byte error")
	}
	p[1], err = fr.r.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "websocket: read second byte error")
	}
	rawBuf.Write(p)

	// header bytes
	frame.firstByte = p[0]
	frame.secondByte = p[1]
	frame.mask = (frame.secondByte & MASKBIT) == 0b10000000

	// data length
	remaining = p[1] & EIGHT_BYTE_BIT
	frameType = int(p[0] & FRAME_TYPE_BIT)
	frame.messageType = frameType

	switch frameType {
	case BinaryMessage, TextMessage,
		PingMessage, PongMessage,
		CloseMessage, ContinueMessage:
		break
	default:
		log.Errorf("unknown 0x%02x (FrameType)", frameType)
	}

	switch remaining {
	case TWO_BYTE_BIT:
		tempBytes = make([]byte, 2)
		_, err = io.ReadFull(fr.r, tempBytes)
		if err != nil {
			return frame, errors.Wrap(err, "websocket: read payload-length 2 bytes error")
		}
		rawBuf.Write(tempBytes)
		dataLength = uint64(binary.BigEndian.Uint16(tempBytes))
	case EIGHT_BYTE_BIT:
		tempBytes = make([]byte, 8)
		_, err = io.ReadFull(fr.r, tempBytes)
		if err != nil {
			return frame, errors.Wrap(err, "websocket: read payload-length 8 bytes error")
		}
		rawBuf.Write(tempBytes)
		dataLength = binary.BigEndian.Uint64(tempBytes)
	default:
		dataLength = uint64(remaining)
	}
	frame.payloadLength = dataLength

	// masking-key
	if frame.mask {
		tempBytes = make([]byte, 4)
		_, err = io.ReadFull(fr.r, tempBytes)
		if err != nil {
			return frame, errors.Wrap(err, "websocket: read masking-key 4 bytes failed")
		}
		rawBuf.Write(tempBytes)
		frame.maskingKey = tempBytes
	}
	fr.reset(frame)

	// data
	data, err := fr.readFramePayload(dataLength)
	if err != nil {
		return frame, errors.Wrap(err, "websocket: read frame payload failed")
	}
	rawBuf.Write(frame.rawPayload)
	frame.payload = make([]byte, len(data))
	copy(frame.payload, data)

	// close frame
	if frameType == CloseMessage {
		if dataLength >= 2 {
			closeCode := binary.BigEndian.Uint16(data[:2])
			frame.closeCode = &closeCode
			data = data[2:]
		} else if dataLength == 1 {
			closeCode := uint16(0)
			frame.closeCode = &closeCode
		}
	}

	frame.data = data
	frame.raw = rawBuf.Bytes()
	return
}

func (fr *FrameReader) readFramePayload(dataLength uint64) (data []byte, err error) {
	frame := fr.frame
	frameType := frame.messageType
	data = make([]byte, dataLength)

	// fast failed for invalid utf8
	if !fr.isDeflate && !frame.RSV1() && frameType == TextMessage && fr.c != nil && fr.c.strictMode {
		if dataLength == 0 {
			return make([]byte, 0), nil
		}
		offset := uint64(0)
		for {
			// read all buffered
			bufferLen := uint64(fr.r.Buffered())
			if bufferLen > 0 {
				if offset+bufferLen > dataLength {
					bufferLen = dataLength - offset
				}
				n, err := fr.r.Read(data[offset : offset+bufferLen])
				if err != nil {
					return nil, errors.Wrap(err, "read payload data failed")
				}
				offset += uint64(n)
				if offset >= dataLength {
					break
				}
				if valid, _ := IsValidUTF8WithRemind(data[:offset]); !valid {
					fr.c.WriteCloseEx(CloseInvalidFramePayloadData, "")
					return nil, errors.New("payload invalid utf8")
				}
			}
			// peek to wait for next read
			_, err = fr.r.Peek(1)
			if err != nil {
				return nil, errors.Wrap(err, "read payload data failed")
			}
		}
	} else {
		data, err = fr.readPayloadN(dataLength)
	}
	return data, err
}

type FrameWriter struct {
	w              *bufio.Writer
	fw             *msgWriter
	c              *WebsocketClient
	opcode         int
	flateThreshold int
	isDeflate      bool
}

func NewFrameWriter(w io.Writer, isDeflate bool) *FrameWriter {
	return NewFrameWriterFromBufio(bufio.NewWriter(w), isDeflate)
}

func NewFrameWriterFromBufio(w *bufio.Writer, isDeflate bool) *FrameWriter {
	return &FrameWriter{
		w:              w,
		isDeflate:      isDeflate,
		flateThreshold: 128,
	}
}

func (fw *FrameWriter) SetWebsocketClient(c *WebsocketClient) {
	fw.c = c
	if !c.Extensions.flateContextTakeover() {
		fw.flateThreshold = 512
	}
}

func (fw *FrameWriter) Flush() error {
	return fw.w.Flush()
}

func (fw *FrameWriter) shouldDeflate(opcode int, data []byte) bool {
	isDeflate := fw.isDeflate

	// control message or continue message or small message should not deflate
	if isDeflate && (opcode == ContinueMessage || IsControlMessage(opcode) || len(data) < fw.flateThreshold) {
		isDeflate = false
	}

	return isDeflate
}

func (fw *FrameWriter) WriteText(data []byte, mask bool) (err error) {
	return fw.WriteEx(data, TextMessage, mask)
}

func (fw *FrameWriter) WriteBinary(data []byte, mask bool) (err error) {
	return fw.WriteEx(data, BinaryMessage, mask)
}

func (fw *FrameWriter) WritePong(data []byte, mask bool) (err error) {
	return fw.WriteEx(data, PongMessage, mask)
}

func (fw *FrameWriter) WriteFrame(f *Frame, opcodes ...int) (err error) {
	// change opcode
	if len(opcodes) > 0 {
		f.SetOpcode(opcodes[0])
	}

	data := f.payload
	isDeflate := fw.shouldDeflate(f.messageType, data)
	// reset
	if isDeflate {
		f.SetRSV1()
	}
	fw.reset(f.messageType, f.RSV1() && isDeflate)

	if isDeflate {
		_, err = fw.writeDeflateFrame(data)
	} else {
		_, err = fw.WriteDirect(f.FIN(), f.RSV1(), f.messageType, f.mask, data)
	}

	return err
}

func (fw *FrameWriter) WriteDirect(fin bool, flate bool, opcode int, mask bool, data []byte) (n int, err error) {
	var firstByte, secondByte byte
	dataLength := len(data)

	// calc firstByte
	firstByte = byte(opcode)
	if fin {
		firstByte = firstByte | FINALBIT
	}
	if flate {
		firstByte = firstByte | RSV1BIT
	}

	// calc secondByte
	if dataLength > TWO_BYTE_SIZE {
		secondByte = EIGHT_BYTE_BIT
	} else if dataLength > SEVEN_BIT_SIZE {
		secondByte = TWO_BYTE_BIT
	} else {
		secondByte = byte(dataLength)
	}
	if mask {
		secondByte = secondByte | MASKBIT
	}

	w := fw.w
	if err = w.WriteByte(firstByte); err != nil {
		return
	}
	n += 1
	if err = w.WriteByte(secondByte); err != nil {
		return
	}
	n += 1

	// extra payload length
	var l []byte
	if dataLength > TWO_BYTE_SIZE {
		l = make([]byte, 8)
		binary.BigEndian.PutUint64(l, uint64(dataLength))

	} else if dataLength > SEVEN_BIT_SIZE {
		l = make([]byte, 2)
		binary.BigEndian.PutUint16(l, uint16(dataLength))
	}
	if len(l) > 0 {
		nn, err := w.Write(l)
		if err != nil {
			return n + nn, err
		}
		n += nn
	}

	// masking key
	if mask {
		maskingKey, _ := generateMaskKey()

		nn, err := w.Write(maskingKey)
		if err != nil {
			return n + nn, err
		}
		n += nn
		maskBytes(maskingKey, data, int(dataLength))
	}

	// data
	nn, err := w.Write(data)
	if err != nil {
		return n + nn, err
	}
	n += nn
	return n, w.Flush()
}

func (fw *FrameWriter) WriteRaw(raw []byte) (err error) {
	_, err = fw.w.Write(raw)
	return err
}

// 客户端发送数据时需要设置mask为true
func (fw *FrameWriter) WriteEx(data []byte, opcode int, mask bool) error {
	if IsControlMessage(opcode) && len(data) > 125 {
		return utils.Error("websocket: control message length must be less than 126 bytes")
	}

	isDeflate := fw.shouldDeflate(opcode, data)
	fw.reset(opcode, isDeflate)

	var err error
	if isDeflate {
		_, err = fw.writeDeflateFrame(data)
	} else {
		_, err = fw.WriteDirect(true, isDeflate, opcode, mask, data)
	}
	return err
}

func ComputeWebsocketAcceptKey(websocketKey string) string {
	h := sha1.New()
	h.Write([]byte(websocketKey))
	h.Write(keyGUID)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func generateMaskKey() ([]byte, error) {
	key := make([]byte, 4)
	_, err := rand.Read(key)

	return key, err
}

func maskBytes(key []byte, b []byte, length int) {
	if key == nil {
		key, _ = generateMaskKey()
	}
	if length > len(b) {
		length = len(b)
	}

	for i := 0; i < length; i++ {
		b[i] ^= key[i&3]
	}
}

func isValidCloseCode(closeCode int) bool {
	// rfc 6455, section 7.4.2

	return closeCode == CloseNormalClosure || closeCode == CloseGoingAway || closeCode == CloseProtocolError || closeCode == CloseUnsupportedData || closeCode == CloseInvalidFramePayloadData || closeCode == ClosePolicyViolation || closeCode == CloseMessageTooBig || closeCode == CloseMandatoryExtension || closeCode == CloseInternalServerErr || closeCode == CloseTLSHandshake || (closeCode >= 3000 && closeCode <= 4999)
}
