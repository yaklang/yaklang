package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// dicomPDVRecord describes one validated P-DATA presentation data value.
// Offset and FragmentOffset are byte offsets relative to the supplied body.
// Length is the on-wire PDV Length: context + control + fragment, excluding
// the four-byte length field. Control preserves all eight bits; its high six
// reserved bits are not rejected. No fragment or input bytes are copied.
type dicomPDVRecord struct {
	Offset         int
	Length         uint32
	ContextID      uint8
	Control        uint8
	FragmentOffset int
	FragmentLength int
}

// decodeDICOMPDVBody validates a complete P-DATA-TF body before allocating
// result records. It preserves the DICOM rule's acceptance of empty fragments
// and reserved control bits, and its 4096-PDV limit. The caller remains
// responsible for the enclosing PDU length/16 MiB limit and transfer syntax.
// Descriptors are local values, not Nodes or a cache. A failure always returns
// nil records, even if earlier PDVs were valid. Input must not be concurrently
// modified while decoding or while the caller uses the returned byte ranges.
func decodeDICOMPDVBody(body []byte) ([]dicomPDVRecord, error) {
	if len(body) < 6 {
		return nil, fmt.Errorf("dicom: missing presentation data value")
	}
	count := 0
	var previous uint8
	for offset := 0; offset < len(body); {
		count++
		if count > 4096 {
			return nil, fmt.Errorf("dicom: PDV count exceeds 4096 limit")
		}
		if len(body)-offset < 4 {
			return nil, fmt.Errorf("dicom: invalid PDV length")
		}
		length := binary.BigEndian.Uint32(body[offset:])
		// Compare before converting to int, including on 32-bit targets.
		if length < 2 || uint64(length) > uint64(len(body)-offset-4) {
			return nil, fmt.Errorf("dicom: invalid PDV length")
		}
		context := body[offset+4]
		if context == 0 || context&1 == 0 {
			return nil, fmt.Errorf("dicom: PDV context ID must be odd")
		}
		if length&1 != 0 {
			return nil, fmt.Errorf("dicom: message fragment must have even byte length")
		}
		if previous != 0 && previous != context {
			return nil, fmt.Errorf("dicom: PDVs in one PDU must use the same context")
		}
		previous = context
		offset += 4 + int(length)
	}

	// All ranges and the exact record count are now validated. Allocate once,
	// without copying the body or creating a partial result on any error path.
	records := make([]dicomPDVRecord, count)
	for index, offset := 0, 0; index < count; index++ {
		length := binary.BigEndian.Uint32(body[offset:])
		records[index] = dicomPDVRecord{
			Offset: offset, Length: length, ContextID: body[offset+4], Control: body[offset+5],
			FragmentOffset: offset + 6, FragmentLength: int(length) - 2,
		}
		offset += 4 + int(length)
	}
	return records, nil
}
