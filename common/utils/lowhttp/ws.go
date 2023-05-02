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
	"io/ioutil"
	"strconv"
	"strings"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/pkg/errors"
)

const (
	FINALBIT               = 1 << 7
	RSV1BIT                = 1 << 6
	MASKBIT                = 1 << 7
	RESET_MESSAGE_TYPE_BIT = 0b11110000
	FRAME_TYPE_BIT         = 0b00001111
	TWO_BYTE_BIT           = 0b01111110
	EIGHT_BYTE_BIT         = 0b01111111

	TWO_BYTE_SIZE  = 65535
	SEVEN_BIT_SIZE = 125 // 根据websocket协议，126和127分别代表用后续两个字节/八个字节表示长度，所以这里只能用125

	DEFAULT_TEXT_MESSAGE_FISRT_BYTE = 0b10000001

	TextMessage     = 1
	BinaryMessage   = 2
	CloseMessage    = 8
	PingMessage     = 9
	PongMessage     = 10
	ContinueMessage = 0
)

var (
	keyGUID = []byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11")
)

type Frame struct {
	raw []byte

	firstByte      byte
	secondByte     byte
	mask           bool
	maskingKey     []byte
	payloadLength  uint64
	payloadData    []byte
	rawPayloadData []byte // 未经过inflate得到的payloadData
	isDeflate      bool

	messageType int
}

func (f *Frame) Bytes() ([]byte, []byte) {
	var err error

	data := utils.BytesClone(f.payloadData)

	dataLength := f.payloadLength
	firstByte, secondByte := f.firstByte, f.secondByte
	if f.isDeflate && !f.IsControl() {
		data, err = deflate(data)
		if err != nil {
			log.Errorf("frame deflate error: %v", err)
			return nil, nil
		}
		dataLength = uint64(len(data))

		// set rsv1
		firstByte |= RSV1BIT

		// reset secondByte payload length
		secondByte &= MASKBIT
		if dataLength > TWO_BYTE_SIZE {
			secondByte |= EIGHT_BYTE_BIT
		} else if dataLength > SEVEN_BIT_SIZE {
			secondByte |= TWO_BYTE_BIT
		} else {
			secondByte |= byte(dataLength)
		}
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

	// 存储明文信息
	var clearData = make([]byte, len(data))
	copy(clearData, data)

	// masking key
	if f.mask {
		rawBuf.Write(f.maskingKey)
		maskBytes(f.maskingKey, data, int(dataLength))
	}

	if len(data) > int(dataLength) {
		rawBuf.Write(data[:dataLength])
		return rawBuf.Bytes(), clearData[:dataLength]
	} else {
		rawBuf.Write(data)
	}

	return rawBuf.Bytes(), clearData
}

func (f *Frame) Type() int {
	return f.messageType
}

func (f *Frame) RawPayloadData() []byte {
	return f.rawPayloadData
}

func (f *Frame) GetMaskingKey() []byte {
	return f.maskingKey
}

func (f *Frame) SetMaskingKey(r []byte) {
	f.maskingKey = r[:]
}

func (f *Frame) IsControl() bool {
	return f.messageType != TextMessage && f.messageType != BinaryMessage
}

type FrameReader struct {
	r         *bufio.Reader
	isDeflate bool
}

func NewFrameReader(r io.Reader, isDeflate bool) *FrameReader {
	return &FrameReader{
		r:         bufio.NewReader(r),
		isDeflate: isDeflate,
	}
}

func NewFrameReaderFromBufio(r *bufio.Reader, isDeflate bool) *FrameReader {
	return &FrameReader{
		r:         r,
		isDeflate: isDeflate,
	}
}

func (f *Frame) Show() {

	raw := utils.BytesClone(f.rawPayloadData)
	rawString := strings.Clone(string(raw))
	if len(raw) > 30 {
		rawString = rawString[:30] + "..."
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

func (fr *FrameReader) ReadFrame() (frame *Frame, err error) {
	frame = &Frame{
		isDeflate: fr.isDeflate,
	}
	defer func() {
		/*
			这儿不用也没关系，但是保护性编程，还是留着
		*/
		if recoveredError := recover(); recoveredError != nil {
			log.Errorf("read frame failed: %s", recoveredError)
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
		return nil, errors.Wrap(err, "read frameType byte failed")
	}
	p[1], err = fr.r.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "read remaining flag failed")
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
		return frame, utils.Errorf("unknown 0x%02x (FrameType)", frameType)
	}

	switch remaining {
	case TWO_BYTE_BIT:
		tempBytes = make([]byte, 2)
		_, err = io.ReadFull(fr.r, tempBytes)
		if err != nil {
			return frame, errors.Wrap(err, "read payload-length 2 bytes failed")
		}
		rawBuf.Write(tempBytes)
		dataLength = uint64(binary.BigEndian.Uint16(tempBytes))
	case EIGHT_BYTE_BIT:
		tempBytes = make([]byte, 8)
		_, err = io.ReadFull(fr.r, tempBytes)
		if err != nil {
			return frame, errors.Wrap(err, "read payload-length 8 bytes failed")
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
			return frame, errors.Wrap(err, "read masking-key 4 bytes failed")
		}
		rawBuf.Write(tempBytes)
		frame.maskingKey = tempBytes
	}

	// data
	// todo: uint64 -> int64 maybe overflow
	data, err := ioutil.ReadAll(io.LimitReader(fr.r, int64(dataLength)))

	frame.payloadData = data
	frame.rawPayloadData = make([]byte, len(data))
	copy(frame.rawPayloadData, data)
	rawBuf.Write(data)

	if err != nil {
		return frame, errors.Wrap(err, "ws frameReader.Reader io.LimitReader failed")
	}

	// websocket扩展：permessage-deflate，只有frameType为TextMessage和BinaryMessage时才需要解压缩
	if fr.isDeflate && !frame.IsControl() {
		newData, err := inflate(data)
		if err != nil {
			log.Warn("permessage-deflate is set, but permessage-deflate failed!")
			log.Warnf("ws frameReader.Reader inflate failed: %v", err)
		} else {
			data = newData
		}
	}

	frame.payloadData = data
	if frame.mask {
		maskBytes(frame.maskingKey, frame.payloadData, len(frame.payloadData))
	}

	frame.raw = rawBuf.Bytes()
	return
}

type FrameWriter struct {
	w         *bufio.Writer
	isDeflate bool
}

func NewFrameWriter(w io.Writer, isDeflate bool) *FrameWriter {
	return &FrameWriter{
		w:         bufio.NewWriter(w),
		isDeflate: isDeflate,
	}
}

func NewFrameWriterFromBufio(w *bufio.Writer, isDeflate bool) *FrameWriter {
	return &FrameWriter{
		w:         w,
		isDeflate: isDeflate,
	}
}

func (fw *FrameWriter) Flush() error {
	return fw.w.Flush()
}

func (fw *FrameWriter) WriteText(data []byte, mask bool, headerBytes ...byte) (err error) {
	return fw.write(data, TextMessage, mask, headerBytes...)
}

func (fw *FrameWriter) WriteBinary(data []byte, mask bool, headerBytes ...byte) (err error) {
	return fw.write(data, BinaryMessage, mask, headerBytes...)
}

func (fw *FrameWriter) WritePong(data []byte, mask bool) (err error) {
	return fw.writeControl(data, PongMessage, mask)
}

func (fw *FrameWriter) WriteFrame(frame *Frame, messageTypes ...int) (err error) {
	frame.isDeflate = fw.isDeflate
	// change opcode
	if len(messageTypes) > 0 {
		messageType := messageTypes[0]
		if messageType != 0 {
			firstByte := frame.firstByte
			firstByte &= RESET_MESSAGE_TYPE_BIT
			firstByte |= uint8(messageType)
			frame.firstByte = firstByte
			frame.messageType = messageType
		}
	}

	raw, _ := frame.Bytes()

	_, err = fw.w.Write(raw)
	return err
}

func (fw *FrameWriter) WriteRaw(raw []byte) (err error) {
	_, err = fw.w.Write(raw)
	return err
}

// 客户端发送数据时需要设置mask为true
func (fw *FrameWriter) write(data []byte, messageType int, mask bool, headerBytes ...byte) error {
	headerBytesLength := len(headerBytes)
	if headerBytesLength == 0 {
		headerBytes = []byte{DEFAULT_TEXT_MESSAGE_FISRT_BYTE, 0}
	} else if headerBytesLength == 1 {
		headerBytes = append(headerBytes, 0)
	}

	frame, err := DataToWebsocketFrame(data, headerBytes[0], mask)
	if err != nil {
		return err
	}

	return fw.WriteFrame(frame, messageType)
}

func (fw *FrameWriter) writeControl(data []byte, messageType int, mask bool) error {
	frame, err := DataToWebsocketControlFrame(messageType, data, mask)
	if err != nil {
		return err
	}
	return fw.WriteFrame(frame, messageType)
}

func WebsocketFrameToData(frame *Frame) (data []byte) {

	return frame.payloadData
}

func DataToWebsocketControlFrame(messageType int, data []byte, mask bool) (frame *Frame, err error) {
	dataLength := len(data)
	frame = new(Frame)
	frame.firstByte = byte(messageType) | FINALBIT
	frame.secondByte = byte(dataLength) | MASKBIT
	frame.payloadLength = uint64(dataLength)
	frame.messageType = messageType
	frame.payloadData = data

	if mask {
		maskKey, err := generateMaskKey()
		if err != nil {
			return nil, err
		}
		frame.mask = mask
		frame.maskingKey = maskKey
	}

	return
}

func DataToWebsocketFrame(data []byte, firstByte byte, mask bool) (frame *Frame, err error) {
	frame = new(Frame)
	frame.firstByte = firstByte
	frame.mask = mask

	secondByte := byte(0)

	// count payload length
	dataLength := len(data)
	if dataLength > TWO_BYTE_SIZE {
		secondByte |= EIGHT_BYTE_BIT
	} else if dataLength > SEVEN_BIT_SIZE {
		secondByte |= TWO_BYTE_BIT
	} else {
		secondByte |= byte(dataLength)
	}
	// mask bit
	if mask {
		// mask -> 1
		secondByte |= MASKBIT // 10000000
	}
	frame.secondByte = secondByte

	// payload length
	frame.payloadLength = uint64(dataLength)

	// masking key
	if mask {
		maskKey, err := generateMaskKey()
		if err != nil {
			return nil, err
		}
		frame.maskingKey = maskKey
	}
	frame.payloadData = data
	return frame, nil
}

func ComputeWebsocketAcceptKey(websocketKey string) string {
	h := sha1.New()
	h.Write([]byte(websocketKey))
	h.Write(keyGUID)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func generateMaskKey() ([]byte, error) {
	var (
		key = make([]byte, 4)
	)
	_, err := rand.Read(key)

	return key, err
}

func maskBytes(key []byte, b []byte, length int) {
	if length > len(b) {
		length = len(b)
	}

	for i := 0; i < length; i++ {
		b[i] ^= key[i&3]
	}
}
