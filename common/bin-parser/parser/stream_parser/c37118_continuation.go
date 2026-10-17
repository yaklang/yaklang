package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// DecodeC37118ConfigurationImage accepts only an already collected CFG-3 image
// in the fixed GPA/GSF 22a9aa1 BinaryImageFrames layout. Its first physical frame
// is 65535 bytes and CONT_IDX=1. Subsequent 16-byte headers must have been stripped
// by the caller after association/order validation. The unchanged first header
// and all payload bytes participate in the one cumulative CRC. This function
// makes no assertion about how fragments were received. It never aliases input.
// maxBytes bounds the logical image before collection allocation; maxElements
// bounds decoded PMUs and channels. DecodeC37118Message still requires a physical
// frame boundary, and does not accept an oversized image as a physical frame.
func DecodeC37118ConfigurationImage(image []byte, maxElements, maxBytes int) (*C37118Message, error) {
	if maxBytes < 1 || len(image) > maxBytes {
		return nil, ErrC37118CollectionLimit
	}
	if len(image) <= 65535 || image[0] != 0xaa || image[1] != 0x52 ||
		binary.BigEndian.Uint16(image[2:]) != 65535 || binary.BigEndian.Uint16(image[14:]) != 1 {
		return nil, fmt.Errorf("c37118: invalid collected CFG-3 envelope")
	}
	crc := binary.BigEndian.Uint16(image[len(image)-2:])
	if c37118CRC(image[:len(image)-2]) != crc {
		return nil, fmt.Errorf("c37118: cumulative CFG-3 CRC mismatch")
	}
	c, err := c37118ReadConfiguration3Body(image[14:len(image)-2], maxElements, true)
	if err != nil {
		return nil, err
	}
	return &C37118Message{FrameType: 5, Version: 2, FrameSize: 65535,
		IDCode: binary.BigEndian.Uint16(image[4:]), SecondOfCentury: binary.BigEndian.Uint32(image[6:]),
		TimeQuality: image[10], FractionOfSecond: binary.BigEndian.Uint32(image[10:]) & 0xffffff,
		Checksum: crc, BodyDecoded: true, Configuration: c}, nil
}
