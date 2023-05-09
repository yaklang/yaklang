package godzilla

import (
	"bytes"
	"encoding/binary"
	"strings"
)

type Parameter struct {
	HashMap map[string]interface{}
	Size    int
}

func NewParameter() *Parameter {
	return &Parameter{
		HashMap: make(map[string]interface{}, 2),
		Size:    0,
	}
}

func (p *Parameter) AddString(key, value string) {
	p.addParameterString(key, value)
}

func (p *Parameter) AddBytes(key string, value []byte) {
	p.addParameterByteArray(key, value)
}

func (p *Parameter) addParameterString(key, value string) {
	p.addParameterByteArray(key, []byte(value))
}

func (p *Parameter) addParameterByteArray(key string, value []byte) {
	p.HashMap[key] = value
	p.Size += len(value)
}

func (p *Parameter) Serialize() []byte {
	var outputStream bytes.Buffer
	for key, value := range p.HashMap {
		outputStream.Write([]byte(key))
		outputStream.WriteByte(2)
		// 根据这个判断 value 的长度
		outputStream.Write(intToBytes(len(value.([]byte))))
		outputStream.Write(value.([]byte))
	}
	return outputStream.Bytes()
}

func intToBytes(n int) []byte {
	x := int32(n)
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.LittleEndian, x)
	return bytesBuffer.Bytes()
}

//func (p *Parameter) UnSerialize(parameterByte []byte) *Parameter {
//	par := NewParameter()
//	for _, b := range parameterByte {
//
//	}
//	return par
//}

func SplitArgs(input string, maxParts int, removeAllEscapeSequences bool) []string {
	r := []rune(strings.Trim(input, " "))
	var i, parts, nextFragmentStart int
	var inBounds bool
	var fragments []string
	for i < len(r) {
		c := r[i]
		if c == '\\' {
			if removeAllEscapeSequences || (i+1 < len(r) && isEscapeable(r[i+1])) {
				r = deleteIndex(r, i)
			}
		} else if c == '"' && checkBounds(i, nextFragmentStart, r, inBounds) {
			inBounds = !inBounds
			r = deleteIndex(r, i)
			i--
		} else if !inBounds && isSpace(c) {
			fragments = addFragment(fragments, r, nextFragmentStart, i)
			nextFragmentStart = i + 1
			parts++
			if parts+1 >= maxParts {
				break
			}
		}
		i++
	}
	if nextFragmentStart < len(r) {
		fragments = addFragment(fragments, r, nextFragmentStart, -1)
	}
	return fragments
}

// 删除指定下标的切片
func deleteIndex(r []rune, i int) []rune {
	return append(r[:i], r[i+1:]...)
}

func addFragment(fragments []string, r []rune, start, end int) []string {
	if end <= start && end >= 0 {
		return []string{}
	}
	if end < 0 {
		end = len(r)
	}
	fragment := string(r[start:end])
	return append(fragments, fragment)
}

func checkBounds(i, nextFragmentStart int, r []rune, inBounds bool) bool {
	if inBounds {
		return i+1 == len(r) || isSpace(r[i+1])
	} else {
		return i == nextFragmentStart
	}
}

func isSpace(c rune) bool {
	return c == ' ' || c == '\t'
}

func isEscapeable(c rune) bool {
	switch c {
	case ' ':
		fallthrough
	case '"':
		fallthrough
	default:
		return false
	}
}
