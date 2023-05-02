package yserx

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var MAGIC_BANNER = []byte{0xac, 0xed}

type JavaSerializationParser struct {
	MagicBanner       []byte
	JavaSerialVersion []byte
	ClassDetails      map[uint64]JavaSerializable
	ClassDescriptions []*JavaClassDesc
	_Handler          uint64

	output io.Writer
	indent int
}

func (j *JavaSerializationParser) increaseIndent() {
	j.indent++
}

func (j *JavaSerializationParser) decreaseIndent() {
	j.indent--
}

func (j *JavaSerializationParser) debug(tmp string, item ...interface{}) {
	if j.output != nil {
		_, _ = fmt.Fprintln(j.output, strings.Repeat(INDENT, j.indent)+fmt.Sprintf(tmp, item...))
	}
}

func MarshalJavaObjects(res ...JavaSerializable) []byte {
	raw := MAGIC_BANNER
	raw = append(raw, 0x00, 0x05)
	for _, i := range res {
		raw = append(raw, i.Marshal()...)
	}
	return raw
}
func ParseFromBytes(raw []byte) (*JavaObject, error) {
	objs, err := ParseJavaSerialized(raw)
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, utils.Error("No JavaSerializable struct found")
	}
	obj, ok := objs[0].(*JavaObject)
	if !ok {
		return nil, utils.Error("Object is not a JavaObject")
	}

	return obj, nil
}
func ParseJavaSerialized(raw []byte) ([]JavaSerializable, error) {
	r := bufio.NewReader(bytes.NewBuffer(raw))
	return ParseJavaSerializedEx(r, ioutil.Discard)
}

func JavaSerializedDumper(raw []byte) string {
	var buf bytes.Buffer
	_, err := ParseJavaSerializedEx(bufio.NewReader(bytes.NewBuffer(raw)), &buf)
	if err != nil {
		buf.Write([]byte("\n\nERROR!" + err.Error()))
	}
	return buf.String()
}

func ParseJavaSerializedFromReader(r io.Reader, callback ...func(serializable JavaSerializable)) ([]JavaSerializable, error) {
	return ParseJavaSerializedEx(bufio.NewReader(r), ioutil.Discard, callback...)
}

func ParseJavaSerializedEx(r *bufio.Reader, writer io.Writer, callback ...func(j JavaSerializable)) ([]JavaSerializable, error) {
	magicBanner, err := ReadBytesLengthInt(r, 2)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(magicBanner, MAGIC_BANNER) {
		return nil, utils.Error("not a valid java ser")
	}

	ver, err := ReadBytesLengthInt(r, 2)
	if err != nil {
		return nil, err
	}

	p := JavaSerializationParser{
		_Handler: 0x7e0000, ClassDetails: make(map[uint64]JavaSerializable),
		ClassDescriptions: []*JavaClassDesc{}, output: writer,
	}
	p.MagicBanner = magicBanner
	p.JavaSerialVersion = ver

	var data []JavaSerializable
	handleResult := func(result JavaSerializable) {
		if result != nil {
			initTCType(result)
			for _, cb := range callback {
				cb(result)
			}
			data = append(data, result)
		}
	}
	for {
		result, err := p.readContentElement(r)
		if err == io.EOF {
			handleResult(result)
			break
		}

		if err != nil {
			log.Errorf("read content element failed: %s", err)
			break
		}
		handleResult(result)
	}

	return data, nil
}

func ParseHexJavaSerialized(raw string) ([]JavaSerializable, error) {
	bytes, err := codec.DecodeHex(raw)
	if err != nil {
		return nil, err
	}
	return ParseJavaSerialized(bytes)
}
