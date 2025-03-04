package yserx

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"io/ioutil"
	"strings"
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
func MarshalJavaObjectWithConfig(serIns JavaSerializable, cfg *MarshalContext) []byte {
	return MarshalJavaObjectsWithConfig(cfg, serIns)
}
func MarshalJavaObjectsWithConfig(cfg *MarshalContext, res ...JavaSerializable) []byte {
	if cfg == nil {
		cfg = NewMarshalContext()
	}
	raw := MAGIC_BANNER
	raw = append(raw, 0x00, 0x05)
	if cfg.DirtyDataLength != 0 {
		buf := bytes.Buffer{}
		serIns, err := GetJavaObjectArrayIns()
		if err != nil {
			log.Errorf("generate java object array instance error: %v", err)
		}
		arrayObjDescSer := serIns.Marshal(cfg)
		buf.Write(arrayObjDescSer[:len(arrayObjDescSer)-4])
		buf.Write(IntTo4Bytes(2))
		buf.Write([]byte{0x7C})
		buf.Write(Uint64To8Bytes(uint64(cfg.DirtyDataLength)))
		buf.Write([]byte(utils.RandStringBytes(cfg.DirtyDataLength)))
		buf.Write([]byte{0x7b})
		raw = append(raw, buf.Bytes()...)
	}
	for _, i := range res {
		raw = append(raw, i.Marshal(cfg)...)
	}
	return raw
}
func MarshalJavaObjects(res ...JavaSerializable) []byte {
	return MarshalJavaObjectsWithConfig(nil, res...)
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
func ParseJavaObject(r io.Reader) (*JavaObject, error) {
	ser, err := ParseSingleJavaSerializedFromReader(r)
	if err != nil {
		return nil, err
	}
	if objIns, ok := ser.(*JavaObject); ok {
		return objIns, nil
	}
	return nil, errors.New("invalid serialize data")
}
func ParseJavaSerializedFromReader(r io.Reader, callback ...func(serializable JavaSerializable)) ([]JavaSerializable, error) {
	return ParseJavaSerializedEx(r, ioutil.Discard, callback...)
}
func ParseSingleJavaSerializedFromReader(r io.Reader, callback ...func(serializable JavaSerializable)) (JavaSerializable, error) {
	serInses, err := ParseMultiJavaSerializedEx(r, ioutil.Discard, 1, callback...)
	if err != nil {
		return nil, err
	}
	if len(serInses) != 1 {
		return nil, utils.Error("invalid serialize data")
	}
	return serInses[0], err
}
func ParseMultiJavaSerializedEx(r io.Reader, writer io.Writer, n int, callback ...func(j JavaSerializable)) ([]JavaSerializable, error) {
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
	var parseErr error
	for i := 0; i != n; i++ {
		result, err := p.readContentElement(r)
		if err == io.EOF {
			handleResult(result)
			break
		}

		if err != nil {
			parseErr = err
			break
		}
		handleResult(result)
	}
	return data, parseErr
}
func ParseJavaSerializedEx(r io.Reader, writer io.Writer, callback ...func(j JavaSerializable)) ([]JavaSerializable, error) {
	return ParseMultiJavaSerializedEx(r, writer, -1, callback...)
}

func ParseHexJavaSerialized(raw string) ([]JavaSerializable, error) {
	bytes, err := codec.DecodeHex(raw)
	if err != nil {
		return nil, err
	}
	return ParseJavaSerialized(bytes)
}
