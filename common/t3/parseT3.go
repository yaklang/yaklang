package t3

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"yaklang.io/yaklang/common/yserx"
)

type T3Header struct {
	cmd          byte
	qos          byte
	flags        byte
	hasJVMIDs    bool
	hasTX        bool
	hasTrace     bool
	resopnseId   int
	invokableId  int
	abbrevOffset int
}
type T3Request struct {
	Header T3Header
}
type reader struct {
	buf      []byte
	pos      int
	chunkPos int
}

type InboundMsgAbbrev struct {
	obj [][]yserx.JavaSerializable
	n   int
}

func (i *InboundMsgAbbrev) push(obj []yserx.JavaSerializable) {
	i.obj = append(i.obj, obj)
	i.n += 1
}
func (i *InboundMsgAbbrev) get() []yserx.JavaSerializable {
	i.n -= 1
	return i.obj[i.n+1]
}
func NewReader(b []byte) *reader {

	return &reader{chunkPos: 0, buf: b, pos: 0}
}
func (r *reader) ReadByte() byte {
	b := r.buf[r.chunkPos]
	r.chunkPos += 1
	return b
}
func (r *reader) ReadByteN(n int) []byte {
	pre := r.chunkPos
	r.chunkPos += n
	return r.buf[pre:r.chunkPos]
}
func (r *reader) ReadLength() int {
	var b byte
	b = r.ReadByte()
	len := int(b & 255)
	if len < 254 {

	} else if len == 254 {
		chs := r.ReadByteN(2)
		ch1 := int(chs[0] & 255)
		ch2 := int(chs[1] & 255)
		len = (ch1 << 8) + ch2
	} else {
		bs := r.ReadByteN(4)
		ch1 := int(bs[0] & 255)
		ch2 := int(bs[1] & 255)
		ch3 := int(bs[2] & 255)
		ch4 := int(bs[3] & 255)
		len = (ch1 << 24) + (ch2 << 16) + (ch3 << 8) + (ch4 << 0)
	}
	return len
}
func (r *reader) ReadInt() int {
	chs := r.ReadByteN(4)
	ch1 := int(chs[0]&255) << 24
	ch2 := int(chs[1]&255) << 16
	ch3 := int(chs[2]&255) << 8
	ch4 := int(chs[3]&255) << 0
	ch := ch1 + ch2 + ch3 + ch4
	if ch == 255 {
		ch = -1
	}
	return ch
}

func (r *reader) Skip(n int) error {
	r.chunkPos += n
	return nil
}
func readObject(buf *reader) []yserx.JavaSerializable {
	buf.ReadByte()

	bfs := buf.buf[buf.chunkPos:]
	l := len(bfs)
	checkn := 5
	for i := l - checkn; i < l; i++ {
		if bfs[i] == 254 {
			bfs = bfs[:i]
			break
		}
	}
	//println(codec.EncodeToHex(bfs))
	br := bytes.NewReader(bfs)
	bfr := bufio.NewReader(br)
	obj, _ := yserx.ParseJavaSerializedEx(bfr, ioutil.Discard)
	if obj == nil {
		println("parse object error")
		os.Exit(0)
	}
	elem := reflect.ValueOf(bfr).Elem()
	r := elem.FieldByName("r")
	bufAdd := int(r.Int())
	if bufAdd == 0 {
		bufAdd = len(bfs) + 1
	}

	buf.chunkPos += int(bufAdd)
	return obj
}
func ParseT3(data []byte) *T3Request {
	headerLen := 19
	var extentbyte []byte
	t3 := &T3Request{Header: T3Header{}}
	buf := NewReader(data)
	buf.Skip(4)
	t3.Header.cmd = buf.ReadByte()
	t3.Header.qos = buf.ReadByte()
	t3.Header.flags = buf.ReadByte()
	t3.Header.flags = t3.Header.flags & 255
	t3.Header.hasJVMIDs = t3.Header.flags&1 != 0
	t3.Header.hasTX = t3.Header.flags&2 != 0
	t3.Header.hasTrace = t3.Header.flags&4 != 0
	t3.Header.resopnseId = buf.ReadInt()
	t3.Header.invokableId = buf.ReadInt()
	t3.Header.abbrevOffset = buf.ReadInt()
	buf.Skip(headerLen - 19)
	s := t3.Header.abbrevOffset - buf.chunkPos
	extentbyte = buf.buf[headerLen:s]
	_ = extentbyte
	buf.Skip(s)
	len := buf.ReadLength()

	abbrevs := InboundMsgAbbrev{n: 0}
	res := ""
	for i := 0; i < len; i++ {
		abbrev := buf.ReadLength()
		if abbrev > 255 {
			obj := readObject(buf)
			json, err := yserx.ToJson(obj)
			if err != nil {
				println(err.Error())
				return nil
			}
			res += fmt.Sprintf("contextObj%d=`%s`\n\n", i, string(json))
			abbrevs.push(obj)
		} else {
			res += fmt.Sprintf("contextObj%d=nil\n\n", i)
			abbrevs.push(nil)
		}
	}
	//ioutil.WriteFile(fmt.Sprintf("/Users/z3/Downloads/contextObjs.json"), []byte(res), 0666)
	return nil
}
