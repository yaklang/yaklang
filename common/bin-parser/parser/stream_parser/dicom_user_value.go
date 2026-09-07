package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// dicomUserRecord describes one validated User Information sub-item. Offsets
// are relative to the supplied body; Offset points to the four-byte item header,
// while ValueOffset and ValueLength identify its value without copying it.
// The reserved header byte remains in the caller's input and is not restricted.
type dicomUserRecord struct {
	Offset      int
	Type        uint8
	Length      uint16
	ValueOffset int
	ValueLength int
}

// decodeDICOMUserInformationBody preserves the DICOMUserInformation,
// DICOMUserItem and DICOMUID rule's field checks, unordered sub-items and shared
// 4096-item budget. initialCount is the number of association/syntax/user items
// already counted by the caller; this function does not change that state.
// The caller owns the enclosing association/PDU size limits. All validation
// precedes descriptor allocation, and any error returns no partial records.
// Input must not be concurrently modified while decoding or using its ranges.
func decodeDICOMUserInformationBody(body []byte, initialCount uint64) ([]dicomUserRecord, error) {
	if initialCount > 4096 {
		return nil, fmt.Errorf("dicom: item count exceeds 4096 limit")
	}
	count := uint64(0)
	maximum, implementation, version := 0, 0, 0
	for offset := 0; offset < len(body); {
		// Subtract the bounded initial value instead of adding to it, including
		// when a caller supplies a count near the uint64 limit.
		if count >= 4096-initialCount {
			return nil, fmt.Errorf("dicom: item count exceeds 4096 limit")
		}
		if len(body)-offset < 4 {
			return nil, fmt.Errorf("dicom: user sub-item length exceeds parent")
		}
		kind := body[offset]
		length := int(binary.BigEndian.Uint16(body[offset+2:]))
		if length > len(body)-offset-4 {
			return nil, fmt.Errorf("dicom: user sub-item length exceeds parent")
		}
		value := body[offset+4 : offset+4+length]
		switch kind {
		case 81:
			if length != 4 {
				return nil, fmt.Errorf("dicom: maximum length sub-item must contain four bytes")
			}
			maximum++
		case 82:
			if err := validateDICOMUserUID(value); err != nil {
				return nil, err
			}
			implementation++
		case 85:
			if length == 0 || length > 16 {
				return nil, fmt.Errorf("dicom: implementation version length must be 1 to 16")
			}
			for _, octet := range value {
				if octet < 32 || octet > 126 {
					return nil, fmt.Errorf("dicom: invalid implementation version character")
				}
			}
			version++
		}
		count++
		offset += 4 + length
	}
	if maximum != 1 || implementation != 1 || version > 1 {
		return nil, fmt.Errorf("dicom: required user information missing or duplicated")
	}

	records := make([]dicomUserRecord, int(count))
	for index, offset := 0, 0; index < len(records); index++ {
		length := binary.BigEndian.Uint16(body[offset+2:])
		records[index] = dicomUserRecord{Offset: offset, Type: body[offset], Length: length, ValueOffset: offset + 4, ValueLength: int(length)}
		offset += 4 + int(length)
	}
	return records, nil
}

func validateDICOMUserUID(value []byte) error {
	if len(value) == 0 || len(value) > 64 {
		return fmt.Errorf("dicom: UID length must be 1 to 64")
	}
	component := 0
	var first byte
	for _, octet := range value {
		if octet == '.' {
			if component == 0 {
				return fmt.Errorf("dicom: empty UID component")
			}
			component = 0
		} else {
			if octet < '0' || octet > '9' {
				return fmt.Errorf("dicom: invalid UID character")
			}
			if component == 0 {
				first = octet
			} else if first == '0' {
				return fmt.Errorf("dicom: leading zero in UID component")
			}
			component++
		}
	}
	if component == 0 {
		return fmt.Errorf("dicom: empty UID component")
	}
	return nil
}
