package yserx

import (
	"bufio"
	"bytes"
)

type JavaReference struct {
	Type        byte   `json:"type"`
	TypeVerbose string `json:"type_verbose"`
	Value       []byte `json:"value"`
	Handle      uint64 `json:"handle"`
}

func (j *JavaReference) GetHandle() uint64 {
	r, err := Read4ByteToUint64(bufio.NewReader(bytes.NewBuffer(j.Value)))
	if err != nil {
		return 0
	}
	return r
}

func (j *JavaReference) Marshal() []byte {
	return append([]byte{TC_REFERENCE}, j.Value...)
}

func NewJavaReference(handle uint64) *JavaReference {
	r := &JavaReference{
		Type:        TC_REFERENCE,
		TypeVerbose: tcToVerbose(TC_REFERENCE),
		Value:       Uint64To4Bytes(handle),
		Handle:      handle,
	}
	initTCType(r)
	return r
}
