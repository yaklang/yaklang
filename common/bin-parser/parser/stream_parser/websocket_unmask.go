package stream_parser

import (
	"fmt"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// RFC 6455 §5.3: payload octet i XOR masking_key[i % 4]. Header fields stay
// on the wire; this only replaces the application-byte projection.
func unmaskWebSocketPayload(key, payload any, offset int) any {
	k := websocketMaskKey(key)
	if len(k) != 4 {
		panic("websocket: masking key must be 4 bytes")
	}
	if offset < 0 {
		panic("websocket: negative mask offset")
	}
	switch v := payload.(type) {
	case string:
		b := []byte(v)
		for i := range b {
			b[i] ^= k[(i+offset)%4]
		}
		return string(b)
	case []byte:
		out := make([]byte, len(v))
		for i, x := range v {
			out[i] = x ^ k[(i+offset)%4]
		}
		return out
	case uint16:
		return unmaskWebSocketUint(k, uint64(v), 2, offset)
	case uint32:
		return unmaskWebSocketUint(k, uint64(v), 2, offset)
	case uint64:
		return unmaskWebSocketUint(k, v, 2, offset)
	case int:
		return unmaskWebSocketUint(k, uint64(v), 2, offset)
	case int16:
		return unmaskWebSocketUint(k, uint64(uint16(v)), 2, offset)
	case int32:
		return unmaskWebSocketUint(k, uint64(uint32(v)), 2, offset)
	case int64:
		return unmaskWebSocketUint(k, uint64(v), 2, offset)
	case *base.NodeValue:
		return unmaskWebSocketPayload(key, v.Value, offset)
	default:
		panic(fmt.Sprintf("websocket: unsupported payload type %T", payload))
	}
}

// Validate only semantics determined by one complete frame. Continuation
// messages and direction-dependent masking require separate session context.
func validateWebSocketValue(kind string, value any) {
	if kind == "code" {
		v, ok := base.InterfaceToUint64(value)
		if !ok || v < 1000 || v >= 5000 || v == 1004 || v == 1005 || v == 1006 || v == 1015 {
			panic("websocket: invalid close status code")
		}
		return
	}
	var b []byte
	switch v := value.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		panic("websocket: invalid text value type")
	}
	if !utf8.Valid(b) {
		panic("websocket: invalid UTF-8")
	}
}

func websocketMaskKey(key any) []byte {
	switch v := key.(type) {
	case []byte:
		return v
	case string:
		return []byte(v)
	case *base.NodeValue:
		return websocketMaskKey(v.Value)
	default:
		return nil
	}
}

func unmaskWebSocketUint(k []byte, v uint64, width, offset int) uint64 {
	b := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		b[i] = byte(v)
		v >>= 8
	}
	var out uint64
	for i, x := range b {
		out = out<<8 | uint64(x^k[(i+offset)%4])
	}
	return out
}
