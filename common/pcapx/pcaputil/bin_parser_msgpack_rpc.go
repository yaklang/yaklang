package pcaputil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"unicode/utf8"
)

const (
	msgpackRPCMaxDepth  = 8
	msgpackRPCMaxNodes  = 1024
	msgpackRPCMaxArray  = 256
	msgpackRPCMaxString = 16 << 10
)

var errMessagePackNeedMore = errors.New("MessagePack value is incomplete")

type msgpackKind uint8

const (
	msgpackInteger msgpackKind = iota + 1
	msgpackString
	msgpackArray
	msgpackNil
	msgpackBoolean
)

type msgpackValue struct {
	kind msgpackKind
	int  int64
	uint uint64
	neg  bool
	str  string
	arr  []msgpackValue
	bool bool
}

type msgpackDecoder struct {
	wire  []byte
	at    int
	nodes int
}

type msgpackRPCMessage struct {
	typ    uint8
	id     uint32
	method string
	params []msgpackValue
	error  msgpackValue
	result msgpackValue
}

type binMsgpackRPC struct {
	clientDir  int
	pending    map[uint32]string
	maxPending int
}

func (f *binFlow) frameMessagePackRPC(wire []byte) (int, *binSpec, error) {
	_, n, err := parseMessagePackRPC(wire)
	if err == errMessagePackNeedMore {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	if n > f.a.config.MaxMessageBytes {
		return n, nil, protocolError(ErrResourceExceeded, "MessagePack-RPC message exceeds parser limit")
	}
	return n, &binSpec{}, nil
}

func probeMessagePackRPC(wire []byte) bool {
	message, n, err := parseMessagePackRPC(wire)
	if err != nil || n <= 0 || message.typ != 0 || message.method != "lab.status" || len(message.params) != 0 {
		return false
	}
	return true
}

func messagePackRPCRequestNeedsMore(wire []byte) bool {
	if len(wire) == 0 || wire[0] != 0x94 {
		return false
	}
	_, _, err := parseMessagePackRPC(wire)
	return errors.Is(err, errMessagePackNeedMore)
}

func (s *binMsgpackRPC) consume(dir int, wire []byte, maxPending int) (map[string]any, error) {
	message, n, err := parseMessagePackRPC(wire)
	if err != nil {
		return nil, err
	}
	if n != len(wire) {
		return nil, protocolError(ErrMalformedMessage, "MessagePack-RPC object does not match the framed message")
	}
	if s.pending == nil {
		s.pending = make(map[uint32]string)
	}
	if s.maxPending <= 0 {
		s.maxPending = maxPending
	}
	switch message.typ {
	case 0:
		if dir != s.clientDir {
			return nil, sessionContext("MessagePack-RPC request was observed from the response direction")
		}
		if _, exists := s.pending[message.id]; exists {
			return nil, sessionContext("MessagePack-RPC request id is already outstanding")
		}
		if len(s.pending) >= s.maxPending {
			return nil, protocolError(ErrResourceExceeded, "MessagePack-RPC pending request budget exceeded")
		}
		s.pending[message.id] = message.method
		fields := map[string]any{
			"Message Type Code": uint8(0),
			"Message Type":      "request",
			"Message ID":        message.id,
			"Method":            message.method,
			"Parameter Count":   len(message.params),
		}
		if message.method == "lab.set" {
			if len(message.params) != 3 || message.params[0].kind != msgpackString || message.params[1].kind != msgpackInteger || message.params[2].kind != msgpackInteger {
				return nil, protocolError(ErrMalformedMessage, "MessagePack-RPC lab.set parameters do not match the supported profile")
			}
			index, indexOK := msgpackInt64(message.params[1])
			value, valueOK := msgpackInt64(message.params[2])
			if !indexOK || !valueOK {
				return nil, protocolError(ErrUnsupportedFeature, "MessagePack-RPC lab.set integer is outside the signed 64-bit profile")
			}
			fields["Set Point"] = message.params[0].str
			fields["Set Index"] = index
			fields["Set Value"] = value
		}
		return fields, nil
	case 1:
		if dir == s.clientDir {
			return nil, sessionContext("MessagePack-RPC response was observed from the request direction")
		}
		method, exists := s.pending[message.id]
		if !exists {
			return nil, sessionContext("MessagePack-RPC response id has no matching request")
		}
		delete(s.pending, message.id)
		fields := map[string]any{
			"Message Type Code": uint8(1),
			"Message Type":      "response",
			"Message ID":        message.id,
			"In Reply To":       method,
			"Error":             msgpackValueForFields(message.error),
		}
		if value, ok := msgpackValueForFieldsOK(message.result); ok {
			fields["Result"] = value
		} else {
			fields["Result Type"] = "array"
		}
		return fields, nil
	case 2:
		if dir != s.clientDir {
			return nil, sessionContext("MessagePack-RPC notification was observed from the response direction")
		}
		fields := map[string]any{
			"Message Type Code": uint8(2),
			"Message Type":      "notification",
			"Method":            message.method,
			"Parameter Count":   len(message.params),
		}
		if message.method == "lab.event" {
			if len(message.params) != 2 || message.params[0].kind != msgpackString || message.params[1].kind != msgpackString {
				return nil, protocolError(ErrMalformedMessage, "MessagePack-RPC lab.event parameters do not match the supported profile")
			}
			fields["Event Name"], fields["Job ID"] = message.params[0].str, message.params[1].str
		}
		return fields, nil
	default:
		return nil, protocolError(ErrMalformedMessage, "MessagePack-RPC message type %d is invalid", message.typ)
	}
}

func parseMessagePackRPC(wire []byte) (msgpackRPCMessage, int, error) {
	value, n, err := parseMessagePackValue(wire)
	if err != nil {
		return msgpackRPCMessage{}, 0, err
	}
	if value.kind != msgpackArray || len(value.arr) == 0 || value.arr[0].kind != msgpackInteger || value.arr[0].neg {
		return msgpackRPCMessage{}, 0, protocolError(ErrMalformedMessage, "MessagePack-RPC message must be an array beginning with an unsigned type")
	}
	typ, ok := msgpackUint(value.arr[0], 2)
	if !ok {
		return msgpackRPCMessage{}, 0, protocolError(ErrMalformedMessage, "MessagePack-RPC message type is outside 0..2")
	}
	message := msgpackRPCMessage{typ: uint8(typ)}
	switch message.typ {
	case 0:
		if len(value.arr) != 4 || value.arr[2].kind != msgpackString || value.arr[3].kind != msgpackArray {
			return msgpackRPCMessage{}, 0, protocolError(ErrMalformedMessage, "MessagePack-RPC request must be [0, msgid, method, params]")
		}
		id, validID := msgpackUint(value.arr[1], math.MaxUint32)
		if !validID || !validMsgpackRPCMethod(value.arr[2].str) {
			return msgpackRPCMessage{}, 0, protocolError(ErrMalformedMessage, "MessagePack-RPC request id or method is invalid")
		}
		message.id, message.method, message.params = uint32(id), value.arr[2].str, value.arr[3].arr
	case 1:
		if len(value.arr) != 4 {
			return msgpackRPCMessage{}, 0, protocolError(ErrMalformedMessage, "MessagePack-RPC response must be [1, msgid, error, result]")
		}
		id, validID := msgpackUint(value.arr[1], math.MaxUint32)
		if !validID || !msgpackRPCScalar(value.arr[2]) || !msgpackRPCScalarOrArray(value.arr[3]) {
			return msgpackRPCMessage{}, 0, protocolError(ErrUnsupportedFeature, "MessagePack-RPC response id or value is outside the supported profile")
		}
		message.id, message.error, message.result = uint32(id), value.arr[2], value.arr[3]
	case 2:
		if len(value.arr) != 3 || value.arr[1].kind != msgpackString || value.arr[2].kind != msgpackArray || !validMsgpackRPCMethod(value.arr[1].str) {
			return msgpackRPCMessage{}, 0, protocolError(ErrMalformedMessage, "MessagePack-RPC notification must be [2, method, params]")
		}
		message.method, message.params = value.arr[1].str, value.arr[2].arr
	}
	return message, n, nil
}

func validMsgpackRPCMethod(method string) bool {
	if len(method) == 0 || len(method) > 256 || bytes.IndexByte([]byte(method), 0) >= 0 {
		return false
	}
	for i := range method {
		c := method[i]
		if c < 0x21 || c > 0x7e {
			return false
		}
	}
	return true
}

func msgpackRPCScalar(value msgpackValue) bool {
	return value.kind == msgpackNil || value.kind == msgpackString || value.kind == msgpackInteger || value.kind == msgpackBoolean
}

func msgpackRPCScalarOrArray(value msgpackValue) bool {
	if msgpackRPCScalar(value) {
		return true
	}
	if value.kind != msgpackArray {
		return false
	}
	for _, item := range value.arr {
		if !msgpackRPCScalar(item) {
			return false
		}
	}
	return true
}

func msgpackValueForFields(value msgpackValue) any {
	field, _ := msgpackValueForFieldsOK(value)
	return field
}

func msgpackValueForFieldsOK(value msgpackValue) (any, bool) {
	switch value.kind {
	case msgpackNil:
		return nil, true
	case msgpackString:
		return value.str, true
	case msgpackBoolean:
		return value.bool, true
	case msgpackInteger:
		if value.neg {
			return value.int, true
		}
		return value.uint, true
	default:
		return nil, false
	}
}

func msgpackUint(value msgpackValue, max uint64) (uint64, bool) {
	if value.kind != msgpackInteger || value.neg || value.uint > max {
		return 0, false
	}
	return value.uint, true
}

func msgpackInt64(value msgpackValue) (int64, bool) {
	if value.kind != msgpackInteger {
		return 0, false
	}
	if value.neg {
		return value.int, true
	}
	if value.uint > math.MaxInt64 {
		return 0, false
	}
	return int64(value.uint), true
}

func parseMessagePackValue(wire []byte) (msgpackValue, int, error) {
	decoder := msgpackDecoder{wire: wire}
	value, err := decoder.readValue(0)
	if err != nil {
		return msgpackValue{}, 0, err
	}
	return value, decoder.at, nil
}

func (d *msgpackDecoder) readValue(depth int) (msgpackValue, error) {
	if depth > msgpackRPCMaxDepth || d.nodes >= msgpackRPCMaxNodes {
		return msgpackValue{}, protocolError(ErrResourceExceeded, "MessagePack value exceeds parser depth or node budget")
	}
	if d.at >= len(d.wire) {
		return msgpackValue{}, errMessagePackNeedMore
	}
	d.nodes++
	code := d.wire[d.at]
	d.at++
	if code <= 0x7f {
		return msgpackValue{kind: msgpackInteger, uint: uint64(code)}, nil
	}
	if code >= 0xe0 {
		return msgpackValue{kind: msgpackInteger, int: int64(int8(code)), neg: true}, nil
	}
	if code >= 0x90 && code <= 0x9f {
		return d.readArray(int(code&0x0f), depth)
	}
	if code >= 0xa0 && code <= 0xbf {
		return d.readString(int(code & 0x1f))
	}
	switch code {
	case 0xc0:
		return msgpackValue{kind: msgpackNil}, nil
	case 0xc2:
		return msgpackValue{kind: msgpackBoolean, bool: false}, nil
	case 0xc3:
		return msgpackValue{kind: msgpackBoolean, bool: true}, nil
	case 0xcc:
		b, err := d.take(1)
		if err != nil {
			return msgpackValue{}, err
		}
		return msgpackValue{kind: msgpackInteger, uint: uint64(b[0])}, nil
	case 0xcd:
		b, err := d.take(2)
		if err != nil {
			return msgpackValue{}, err
		}
		return msgpackValue{kind: msgpackInteger, uint: uint64(binary.BigEndian.Uint16(b))}, nil
	case 0xce:
		b, err := d.take(4)
		if err != nil {
			return msgpackValue{}, err
		}
		return msgpackValue{kind: msgpackInteger, uint: uint64(binary.BigEndian.Uint32(b))}, nil
	case 0xcf:
		b, err := d.take(8)
		if err != nil {
			return msgpackValue{}, err
		}
		value := binary.BigEndian.Uint64(b)
		if value > math.MaxInt64 {
			return msgpackValue{}, protocolError(ErrUnsupportedFeature, "MessagePack unsigned integer exceeds the supported signed range")
		}
		return msgpackValue{kind: msgpackInteger, uint: value}, nil
	case 0xd0:
		b, err := d.take(1)
		if err != nil {
			return msgpackValue{}, err
		}
		return msgpackSigned(int64(int8(b[0]))), nil
	case 0xd1:
		b, err := d.take(2)
		if err != nil {
			return msgpackValue{}, err
		}
		return msgpackSigned(int64(int16(binary.BigEndian.Uint16(b)))), nil
	case 0xd2:
		b, err := d.take(4)
		if err != nil {
			return msgpackValue{}, err
		}
		return msgpackSigned(int64(int32(binary.BigEndian.Uint32(b)))), nil
	case 0xd3:
		b, err := d.take(8)
		if err != nil {
			return msgpackValue{}, err
		}
		return msgpackSigned(int64(binary.BigEndian.Uint64(b))), nil
	case 0xd9:
		b, err := d.take(1)
		if err != nil {
			return msgpackValue{}, err
		}
		return d.readString(int(b[0]))
	case 0xda:
		b, err := d.take(2)
		if err != nil {
			return msgpackValue{}, err
		}
		return d.readString(int(binary.BigEndian.Uint16(b)))
	case 0xdb:
		b, err := d.take(4)
		if err != nil {
			return msgpackValue{}, err
		}
		length := binary.BigEndian.Uint32(b)
		if uint64(length) > msgpackRPCMaxString {
			return msgpackValue{}, protocolError(ErrResourceExceeded, "MessagePack string exceeds parser limit")
		}
		return d.readString(int(length))
	case 0xdc:
		b, err := d.take(2)
		if err != nil {
			return msgpackValue{}, err
		}
		return d.readArray(int(binary.BigEndian.Uint16(b)), depth)
	case 0xdd:
		b, err := d.take(4)
		if err != nil {
			return msgpackValue{}, err
		}
		length := binary.BigEndian.Uint32(b)
		if uint64(length) > msgpackRPCMaxArray {
			return msgpackValue{}, protocolError(ErrResourceExceeded, "MessagePack array exceeds parser limit")
		}
		return d.readArray(int(length), depth)
	default:
		return msgpackValue{}, protocolError(ErrUnsupportedFeature, "MessagePack format 0x%02x is outside the supported scalar/array profile", code)
	}
}

func (d *msgpackDecoder) readArray(length, depth int) (msgpackValue, error) {
	if length < 0 || length > msgpackRPCMaxArray || length > msgpackRPCMaxNodes-d.nodes {
		return msgpackValue{}, protocolError(ErrResourceExceeded, "MessagePack array exceeds parser element budget")
	}
	values := make([]msgpackValue, length)
	for i := range values {
		value, err := d.readValue(depth + 1)
		if err != nil {
			return msgpackValue{}, err
		}
		values[i] = value
	}
	return msgpackValue{kind: msgpackArray, arr: values}, nil
}

func (d *msgpackDecoder) readString(length int) (msgpackValue, error) {
	if length < 0 || length > msgpackRPCMaxString {
		return msgpackValue{}, protocolError(ErrResourceExceeded, "MessagePack string exceeds parser limit")
	}
	b, err := d.take(length)
	if err != nil {
		return msgpackValue{}, err
	}
	if !utf8.Valid(b) {
		return msgpackValue{}, protocolError(ErrMalformedMessage, "MessagePack string is not valid UTF-8")
	}
	return msgpackValue{kind: msgpackString, str: string(b)}, nil
}

func (d *msgpackDecoder) take(length int) ([]byte, error) {
	if length < 0 || length > len(d.wire)-d.at {
		return nil, errMessagePackNeedMore
	}
	b := d.wire[d.at : d.at+length]
	d.at += length
	return b, nil
}

func msgpackSigned(value int64) msgpackValue {
	if value < 0 {
		return msgpackValue{kind: msgpackInteger, int: value, neg: true}
	}
	return msgpackValue{kind: msgpackInteger, int: value, uint: uint64(value)}
}
