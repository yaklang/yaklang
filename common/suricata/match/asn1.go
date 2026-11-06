package match

import (
	"errors"

	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

const asn1DefaultMaxFrames = 30

type asn1Object struct {
	class       byte
	constructed bool
	tag         uint64
	definite    bool
	length      uint64
	content     []byte
	children    []asn1Object
}

var errASN1Malformed = errors.New("malformed ASN.1 object")

// asn1Matcher evaluates the Suricata asn1 keyword against the packet payload.
// Relative offsets are measured from the end of each candidate match produced
// by the preceding content rule, matching Suricata's PMATCH buffer offset.
func asn1Matcher(c *matchContext) error {
	config := c.Rule.ContentRuleConfig.ASN1
	if config == nil {
		return nil
	}
	payload := c.GetBuffer(modifier.Default)
	var offsets []int
	switch {
	case config.AbsoluteOffset != nil:
		offsets = append(offsets, int(*config.AbsoluteOffset))
	case config.RelativeOffset != nil:
		if len(c.prevMatched) == 0 {
			offsets = append(offsets, *config.RelativeOffset)
		} else {
			for _, matched := range c.prevMatched {
				offsets = append(offsets, matched.Pos+matched.Len+*config.RelativeOffset)
			}
		}
	default:
		offsets = append(offsets, 0)
	}
	for _, offset := range offsets {
		// Suricata computes the effective offset as a u16; larger relative
		// offsets are invalid rather than being clamped.
		if offset < 0 || offset > 0xffff || offset >= len(payload) {
			continue
		}
		if asn1Matches(payload[offset:], config) {
			return nil
		}
	}
	c.Reject()
	return nil
}

func asn1Matches(payload []byte, config *rule.ASN1Rule) bool {
	objects := parseASN1Stream(payload)
	for _, object := range objects {
		if asn1ObjectMatches(object, config, asn1DefaultMaxFrames) {
			return true
		}
	}
	return false
}

func parseASN1Stream(payload []byte) []asn1Object {
	var objects []asn1Object
	rest := payload
	for len(rest) > 0 {
		if len(objects) >= asn1DefaultMaxFrames {
			// Suricata aborts decoding when the top-level object count reaches
			// max_frames; the keyword then has no parsed objects to check.
			return nil
		}
		object, consumed, err := parseASN1Object(rest, asn1DefaultMaxFrames)
		if err != nil || consumed <= 0 {
			break
		}
		objects = append(objects, object)
		rest = rest[consumed:]
	}
	return objects
}

func parseASN1Object(data []byte, depth int) (asn1Object, int, error) {
	if depth <= 0 || len(data) < 2 {
		return asn1Object{}, 0, errASN1Malformed
	}
	object := asn1Object{class: data[0] >> 6, constructed: data[0]&0x20 != 0, tag: uint64(data[0] & 0x1f)}
	offset := 1
	if object.tag == 0x1f {
		object.tag = 0
		for {
			if offset >= len(data) {
				return asn1Object{}, 0, errASN1Malformed
			}
			value := data[offset]
			offset++
			if object.tag > (^uint64(0) >> 7) {
				return asn1Object{}, 0, errASN1Malformed
			}
			object.tag = object.tag<<7 | uint64(value&0x7f)
			if value&0x80 == 0 {
				break
			}
		}
	}
	if offset >= len(data) {
		return asn1Object{}, 0, errASN1Malformed
	}
	lengthByte := data[offset]
	offset++
	switch {
	case lengthByte < 0x80:
		object.definite = true
		object.length = uint64(lengthByte)
	case lengthByte == 0x80:
		// Indefinite BER length. It is parsed for recursion but Suricata's
		// checks skip indefinite objects because they have no definite length.
		if !object.constructed {
			return asn1Object{}, 0, errASN1Malformed
		}
	case lengthByte == 0xff:
		return asn1Object{}, 0, errASN1Malformed
	default:
		count := int(lengthByte & 0x7f)
		if count == 0 || count > 8 || offset+count > len(data) {
			return asn1Object{}, 0, errASN1Malformed
		}
		object.definite = true
		for index := 0; index < count; index++ {
			if object.length > (^uint64(0) >> 8) {
				return asn1Object{}, 0, errASN1Malformed
			}
			object.length = object.length<<8 | uint64(data[offset+index])
		}
		offset += count
	}

	if object.definite {
		if object.length > uint64(len(data)-offset) {
			return asn1Object{}, 0, errASN1Malformed
		}
		object.content = data[offset : offset+int(object.length)]
		offset += int(object.length)
		if object.constructed {
			children, consumed, err := parseASN1Children(object.content, depth-1, false)
			if err != nil || consumed != len(object.content) {
				return asn1Object{}, 0, errASN1Malformed
			}
			object.children = children
		}
		return object, offset, nil
	}

	children, consumed, err := parseASN1Children(data[offset:], depth-1, true)
	if err != nil {
		return asn1Object{}, 0, err
	}
	object.content = data[offset : offset+consumed-2]
	object.children = children
	return object, offset + consumed, nil
}

// parseASN1Children parses nested BER objects. For indefinite collections it
// stops after the end-of-contents marker and returns the number of consumed
// bytes including that marker.
func parseASN1Children(data []byte, depth int, indefinite bool) ([]asn1Object, int, error) {
	var children []asn1Object
	offset := 0
	for {
		if indefinite && offset+2 <= len(data) && data[offset] == 0 && data[offset+1] == 0 {
			return children, offset + 2, nil
		}
		if !indefinite && offset == len(data) {
			return children, offset, nil
		}
		if offset >= len(data) {
			return nil, 0, errASN1Malformed
		}
		child, consumed, err := parseASN1Object(data[offset:], depth)
		if err != nil || consumed <= 0 {
			return nil, 0, errASN1Malformed
		}
		children = append(children, child)
		offset += consumed
		if len(children) > asn1DefaultMaxFrames {
			return nil, 0, errASN1Malformed
		}
	}
}

func asn1ObjectMatches(object asn1Object, config *rule.ASN1Rule, depth int) bool {
	if depth == 0 {
		return true
	}
	// Suricata's check_object requires a definite length before applying any
	// check, including the bitstring and real-type overflow tests.
	if object.definite {
		if config.OversizeLength != nil &&
			(object.length > uint64(*config.OversizeLength) || uint64(len(object.content)) > uint64(*config.OversizeLength)) {
			return true
		}
		if config.BitstringOverflow && object.class == 0 && object.tag == 3 && !object.constructed &&
			object.length > 0 && len(object.content) > 0 &&
			uint64(object.content[0]) > object.length*8 {
			return true
		}
		if config.DoubleOverflow && object.class == 0 && object.tag == 9 && !object.constructed &&
			object.length > 0 && len(object.content) > 0 && object.content[0]&0xc0 == 0 &&
			(object.length > 256 || uint64(len(object.content)) > 256) {
			return true
		}
	}
	for _, child := range object.children {
		if asn1ObjectMatches(child, config, depth-1) {
			return true
		}
	}
	return false
}
