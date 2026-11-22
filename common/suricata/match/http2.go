package match

import (
	"bytes"
	"encoding/binary"

	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

var http2ClientPreface = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")

const (
	http2FrameData         uint8 = 0
	http2FrameHeaders      uint8 = 1
	http2FramePriority     uint8 = 2
	http2FrameRSTStream    uint8 = 3
	http2FrameSettings     uint8 = 4
	http2FramePushPromise  uint8 = 5
	http2FramePing         uint8 = 6
	http2FrameGoAway       uint8 = 7
	http2FrameWindowUpdate uint8 = 8
	http2FrameContinuation uint8 = 9
)

// HTTP2Frame represents one decoded RFC 9113 HTTP/2 frame header and
// WindowUpdate increment when FrameType == WINDOW_UPDATE.
type HTTP2Frame struct {
	Type            uint8
	Flags           uint8
	StreamID        uint32
	Length          uint32
	WindowIncrement uint32
	HasWindow       bool
	StartOffset     int
	EndOffset       int
}

// LooksLikeHTTP2 performs a cheap structural check for HTTP/2 client preface
// or valid RFC 9113 binary frames so non-HTTP/2 streams fail fast.
func LooksLikeHTTP2(payload []byte) bool {
	if len(payload) < 9 {
		return false
	}
	if bytes.HasPrefix(payload, http2ClientPreface) {
		return true
	}
	// Without the preface (e.g. server->client or mid-connection frames), verify
	// that the leading bytes form at least one structurally valid HTTP/2 frame.
	length := uint32(payload[0])<<16 | uint32(payload[1])<<8 | uint32(payload[2])
	frameType := payload[3]
	streamID := binary.BigEndian.Uint32(payload[5:9]) & 0x7fffffff
	if frameType > http2FrameContinuation {
		return false
	}
	if !validHTTP2FrameHeader(frameType, length, streamID) {
		return false
	}
	if uint64(9)+uint64(length) > uint64(len(payload)) {
		return false
	}
	return true
}

func validHTTP2FrameHeader(frameType uint8, length uint32, streamID uint32) bool {
	switch frameType {
	case http2FramePriority:
		return length == 5 && streamID != 0
	case http2FrameRSTStream:
		return length == 4 && streamID != 0
	case http2FrameSettings:
		return length%6 == 0 && streamID == 0 && length <= 600
	case http2FramePing:
		return length == 8 && streamID == 0
	case http2FrameGoAway:
		return length >= 8 && streamID == 0
	case http2FrameWindowUpdate:
		return length == 4
	case http2FrameData, http2FrameHeaders, http2FramePushPromise, http2FrameContinuation:
		return streamID != 0 && length <= 16777215
	default:
		return false
	}
}

// ParseHTTP2Frames walks complete RFC 9113 binary frames in payload.
func ParseHTTP2Frames(payload []byte) []HTTP2Frame {
	if len(payload) < 9 {
		return nil
	}
	off := 0
	hasPreface := false
	if bytes.HasPrefix(payload, http2ClientPreface) {
		hasPreface = true
		off = len(http2ClientPreface)
	}
	var frames []HTTP2Frame
	const maxFrames = 4096
	for off+9 <= len(payload) && len(frames) < maxFrames {
		length := uint32(payload[off])<<16 | uint32(payload[off+1])<<8 | uint32(payload[off+2])
		frameType := payload[off+3]
		flags := payload[off+4]
		streamID := binary.BigEndian.Uint32(payload[off+5:off+9]) & 0x7fffffff

		if !hasPreface && frameType > http2FrameContinuation {
			return nil
		}
		if !validHTTP2FrameHeader(frameType, length, streamID) {
			if len(frames) == 0 && !hasPreface {
				return nil
			}
			break
		}
		next := off + 9 + int(length)
		if next < off+9 || next > len(payload) {
			break
		}
		f := HTTP2Frame{
			Type:        frameType,
			Flags:       flags,
			StreamID:    streamID,
			Length:      length,
			StartOffset: off,
			EndOffset:   next,
		}
		if frameType == http2FrameWindowUpdate && length == 4 {
			f.WindowIncrement = binary.BigEndian.Uint32(payload[off+9:off+13]) & 0x7fffffff
			f.HasWindow = true
		}
		frames = append(frames, f)
		off = next
	}
	return frames
}

func http2Matcher(c *matchContext) error {
	if c.Rule == nil || c.Rule.ContentRuleConfig == nil || c.Rule.ContentRuleConfig.HTTP2Config == nil {
		return nil
	}
	cfg := c.Rule.ContentRuleConfig.HTTP2Config
	payload := c.GetBuffer(modifier.Default)
	frames := ParseHTTP2Frames(payload)
	if !c.Must(len(frames) > 0) {
		return nil
	}
	matched := false
	maxEnd := 0
	for _, f := range frames {
		if cfg.FrameTypeSet {
			typeMatch := f.Type == cfg.FrameType
			if cfg.FrameTypeNegated {
				typeMatch = !typeMatch
			}
			if !typeMatch {
				continue
			}
		}
		if cfg.Window != nil {
			if !f.HasWindow || !cfg.Window.Match(int(f.WindowIncrement)) {
				continue
			}
		}
		matched = true
		if f.EndOffset > maxEnd {
			maxEnd = f.EndOffset
		}
	}
	if c.Must(matched) && maxEnd > c.protocolEvidenceEnd {
		c.protocolEvidenceEnd = maxEnd
	}
	return nil
}
